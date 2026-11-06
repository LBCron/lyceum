package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/unit"
)

// prTimeout bounds a gh or glab call.
const prTimeout = 2 * time.Minute

// plainChars need no shell quoting in a printed command.
const plainChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./:@="

// openPR is close's last step (units.md, "Close" step 5): it opens a
// pull request for closed unit p, whose plan is planPath, from branch into
// base, or prints how. With an origin on GitHub and gh on PATH it runs
// `gh pr create`, and with one on GitLab and glab it runs `glab mr create`,
// once the branch has an upstream; Lyceum never pushes, so without one it
// prints the push first and runs neither. Otherwise it prints the exact push
// and merge commands. The tool's output goes to w. The merge is the owner's,
// and no merge event is written. It returns the next action.
func openPR(w io.Writer, root, branch, base, planPath string, p *unit.Plan) (string, error) {
	push := "git push -u origin " + branch
	merge := fmt.Sprintf("git switch %s && git merge --no-ff %s", base, branch)
	url, err := gitx.RemoteURL(root, "origin")
	if err != nil {
		return "", err
	}
	if url == "" {
		_, _ = fmt.Fprintf(w, "No origin remote, so no pull request. To merge, the owner runs:\n  %s\n", merge)
		return "the owner merges the branch: " + merge, nil
	}

	acs := "none"
	if len(p.ACs) > 0 {
		acs = strings.Join(p.ACs, ", ")
	}
	body := fmt.Sprintf("Plan: %s; ACs: %s", planPath, acs)
	var tool string
	var args []string
	switch {
	case strings.Contains(url, "github.com"):
		tool, args = "gh", []string{"pr", "create", "--base", base, "--head", branch, "--title", p.Title, "--body", body}
	case strings.Contains(url, "gitlab"):
		tool, args = "glab", []string{"mr", "create", "--source-branch", branch, "--target-branch", base, "--title", p.Title, "--description", body, "--yes"}
	}
	if tool != "" {
		if _, err := exec.LookPath(tool); err != nil {
			tool = ""
		}
	}
	if tool == "" {
		merge += " && git push origin " + base
		_, _ = fmt.Fprintf(w, "To push the branch:\n  %s\nThen open a pull request from %s into %s; or, to merge without one, the owner runs:\n  %s\n", push, branch, base, merge)
		return fmt.Sprintf("%s, then open a pull request from %s into %s; the owner merges it", push, branch, base), nil
	}

	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = a
		if a == "" || strings.IndexFunc(a, func(r rune) bool { return !strings.ContainsRune(plainChars, r) }) >= 0 {
			quoted[i] = shellQuote(a)
		}
	}
	create := tool + " " + strings.Join(quoted, " ")
	upstream, err := gitx.Upstream(root, branch)
	if err != nil {
		return "", err
	}
	if upstream == "" {
		_, _ = fmt.Fprintf(w, "Branch %s has no upstream, and Lyceum does not push. Push it first:\n  %s\nthen open the pull request:\n  %s\n", branch, push, create)
		return fmt.Sprintf("%s, then run: %s", push, create), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), prTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, tool, args...)
	c.Dir, c.Stdout, c.Stderr = root, w, w
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) && ctx.Err() == nil {
			return "", err
		}
		_, _ = fmt.Fprintf(w, "%s did not open the pull request (%v); the unit is closed. Open it by hand:\n  %s\n", tool, err, create)
		return fmt.Sprintf("open the pull request by hand: %s", create), nil
	}
	_, _ = fmt.Fprintf(w, "Opened a pull request from %s into %s with %s\n", branch, base, tool)
	return "push the close commit so the pull request has it: git push; the owner merges the pull request", nil
}
