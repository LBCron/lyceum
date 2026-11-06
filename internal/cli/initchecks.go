package cli

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/templates"
)

// workflowPath is the CI workflow init offers.
const workflowPath = ".github/workflows/lyceum.yml"

// writePreCommit installs Lyceum's block in the pre-commit hook. With no
// hook it offers a new one, default yes. Lyceum's own block is replaced
// without asking. A hook without it is never overwritten: init shows the block
// and asks before appending it, as for AGENTS.md. It returns a line saying
// what was skipped, or "".
func writePreCommit(prompt io.Writer, in *bufio.Reader, root string, r *initReport) (string, error) {
	rel, err := gitx.GitPath(root, "hooks/pre-commit")
	if err != nil {
		return "", err
	}
	path := rel
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	rel = filepath.ToSlash(rel)
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	skipped := `Skipped the pre-commit hook; run "lyceum init" again to add it`
	var content string
	switch {
	case err != nil:
		if !askDefault(prompt, in, "Install a pre-commit hook that runs lyceum check? [Y/n] ", true) {
			return skipped, nil
		}
		content = "#!/bin/sh\n" + templates.PreCommit
	case hasBlock(string(data)):
		content = replaceBlock(string(data))
	default:
		_, _ = io.WriteString(prompt, templates.PreCommit)
		if !ask(prompt, in, "Append this Lyceum check to your "+rel+"? [y/N] ") {
			return skipped, nil
		}
		content = string(data) + "\n" + templates.PreCommit
		if len(data) > 0 && data[len(data)-1] != '\n' {
			content = string(data) + "\n\n" + templates.PreCommit
		}
	}
	c, err := putFile(root, rel, []byte(content), 0o755)
	if err != nil {
		return "", err
	}
	r.add(rel, c)
	return "", nil
}

// hasBlock reports whether hook holds Lyceum's block, both markers in order.
func hasBlock(hook string) bool {
	i := strings.Index(hook, templates.HookStart)
	return i >= 0 && strings.Contains(hook[i:], templates.HookEnd)
}

// replaceBlock is hook with its Lyceum block replaced by the current one.
func replaceBlock(hook string) string {
	i := strings.Index(hook, templates.HookStart)
	j := i + strings.Index(hook[i:], templates.HookEnd) + len(templates.HookEnd)
	rest := strings.TrimPrefix(hook[j:], "\n")
	return hook[:i] + templates.PreCommit + rest
}

// removeBlock is hook without its Lyceum block, the inverse of
// writePreCommit: the block, the line end after it and the blank line put
// before an appended block go.
func removeBlock(hook string) string {
	i := strings.Index(hook, templates.HookStart)
	j := i + strings.Index(hook[i:], templates.HookEnd) + len(templates.HookEnd)
	head, rest := hook[:i], strings.TrimPrefix(hook[j:], "\n")
	if rest == "" && strings.HasSuffix(head, "\n\n") {
		head = head[:len(head)-1]
	}
	return head + rest
}

// writeWorkflow offers the CI workflow, default yes, once the build's CI
// Action is published. An existing one is left as it is. It returns a line saying what was skipped, or "".
func writeWorkflow(prompt io.Writer, in *bufio.Reader, root string, r *initReport) (string, error) {
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(workflowPath))); err == nil {
		r.add(workflowPath, unchanged)
		return "", nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if !ciPublished() {
		return "No CI workflow: CI arrives with the published Action", nil
	}
	if !askDefault(prompt, in, "Install a CI workflow that runs lyceum check --ci on pull requests? [Y/n] ", true) {
		return `Skipped the CI workflow; run "lyceum init" again to add it`, nil
	}
	c, err := putFile(root, workflowPath, []byte(templates.Workflow), 0o644)
	if err != nil {
		return "", err
	}
	r.add(workflowPath, c)
	return "", nil
}
