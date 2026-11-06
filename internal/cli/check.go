package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/oritromax/lyceum/internal/check"
	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/unit"
	"github.com/spf13/cobra"
)

// runCheck is `lyceum check [--ci]` (enforcement.md, "lyceum check"). It runs
// every check on the branch's open unit, if any, else its closed one. Locally it warns: it prints
// the findings and exits 0, writing no bypass event; --pre-commit, which only the
// commit hook passes, also records each failing one as a bypass event.
// With --ci it writes nothing and refuses when any finding fails. Locally,
// a bug at cause with a code commit after its verify clear first goes back
// to fix (reopenBug); with --ci that is the reopen finding.
func runCheck(cmd *cobra.Command, args []string) error {
	ci, _ := cmd.Flags().GetBool("ci")
	preCommit, _ := cmd.Flags().GetBool("pre-commit")
	// A commit Lyceum makes itself goes past nothing, and its output is not shown.
	if preCommit && os.Getenv(gitx.OwnCommitEnv) != "" {
		return nil
	}
	what := "check"
	if len(args) > 0 {
		return out.Refuse(what, "check takes no arguments", `run "lyceum check", or "lyceum check --ci" in CI`)
	}
	if err := gitx.CheckVersion(); err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	if !gitx.IsRepo(dir) {
		return out.Refuse(what, "this is not a git repository, so Lyceum is not set up here", `run "lyceum init"`)
	}
	root, err := gitx.Root(dir)
	if err != nil {
		return err
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	cfg, err := config.Read(root)
	if errors.Is(err, fs.ErrNotExist) {
		reason, next := "Lyceum is not set up in this repo: "+config.Path+" is missing", `run "lyceum init"`
		if ci {
			return out.Refuse(what, reason, next)
		}
		// Locally it is a warning, like every other finding.
		findings := []check.Finding{{Check: "setup", Severity: check.Warn, Message: reason, Next: next}}
		if !asJSON {
			printFindings(cmd.OutOrStdout(), findings, 0)
		}
		out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(
			out.Result{Next: next, Extra: map[string]any{"findings": findings}})
		return nil
	} else if err != nil {
		return err
	}
	branch, err := gitx.Branch(root)
	if err != nil {
		return err
	}
	p, err := openUnit(root, branch)
	if err != nil {
		return err
	}
	// Locally, a bug at cause with a code commit after its verify clear goes
	// back to fix first, so the replay sees the gate-reopen event.
	reopened := ""
	if !ci {
		if reopened, err = reopenBug(root, p); err != nil {
			return err
		}
	}
	if reopened != "" && !asJSON {
		_, _ = io.WriteString(cmd.OutOrStdout(), reopenedLine(p.ID, reopened))
	}
	evs, err := events.Read(root)
	if err != nil {
		return err
	}
	if p == nil {
		// A plan whose branch: or status: was edited by hand is still the
		// branch's unit when an add event puts it here.
		if p, err = loggedUnit(root, branch, evs); err != nil {
			return err
		}
	}
	if p == nil {
		// After close, the branch's unit is its closed one.
		if p, err = closedUnit(root, branch, evs); err != nil {
			return err
		}
	}
	var ctx check.Context
	base := ""
	head, err := gitx.HeadSHA(root)
	if err != nil {
		return err
	}
	if head == "" {
		ctx, err = unbornContext(root, branch, cfg, p, evs)
	} else {
		ctx, base, err = checkContext(root, branch, cfg, p, evs, what, ci)
	}
	if err != nil {
		return err
	}
	findings, err := check.Run(ctx)
	if err != nil {
		return err
	}
	extra := map[string]any{"findings": findings}

	if r := failingRefusal("pass check --ci", findings); ci && r != nil {
		return r.WithJSON(extra)
	}

	written := 0
	if !ci && preCommit {
		bypasses, err := bypassFindings(root, ctx, base)
		if err != nil {
			return err
		}
		if written, err = recordBypasses(root, evs, bypasses); err != nil {
			return err
		}
	}

	if !asJSON {
		printFindings(cmd.OutOrStdout(), findings, written)
	}

	res := out.Result{Next: `start a unit: lyceum add feat "<title>"`}
	if p != nil {
		res = out.Result{Unit: p.ID, Gate: p.Gate}
		// A hand-edited type: or gate: is a finding, not an error.
		if _, current, class, err := unitGate(p); err == nil {
			res.Next = gateNext(class, current)
		} else if len(findings) == 0 {
			return err
		}
	}
	if len(findings) > 0 {
		res.Next = `fix the findings, then run "lyceum check" again`
	}
	res.Extra = extra
	out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(res)
	return nil
}

// checkContext is the context check runs on, on branch, for p, the branch's
// unit or nil, with evs, the event log, under cfg; and the base ref the branch
// diff is taken against (baseRef, which refuses as what).
func checkContext(root, branch string, cfg *config.Config, p *unit.Plan, evs []events.Event, what string, ci bool) (check.Context, string, error) {
	ctx := newContext(root, branch, cfg, p, evs)
	base, err := baseRef(root, cfg.BaseBranch, what, ci)
	if err != nil {
		return check.Context{}, "", err
	}
	// The base branch itself has no branch diff.
	if p != nil || "refs/heads/"+branch != base {
		if ctx.Changed, err = gitx.BranchChanges(root, base); err != nil {
			return check.Context{}, "", err
		}
	}
	return ctx, base, nil
}

// newContext is the check context with no branch diff yet.
func newContext(root, branch string, cfg *config.Config, p *unit.Plan, evs []events.Event) check.Context {
	ctx := check.Context{Root: root, Branch: branch, Plan: p, Events: evs, HarnessPrompt: cfg.Approvals.HarnessPrompt, Owner: cfg.Owner, Signed: cfg.Approvals.Signed}
	if p != nil {
		ctx.PlanPath = path.Join(planDir, p.ID+".md")
	}
	return ctx
}

// unbornContext is checkContext in a repo with no commits yet: the base is
// empty, never a refusal, so the branch diff is every path git status
// lists. The base branch itself still has no branch diff.
func unbornContext(root, branch string, cfg *config.Config, p *unit.Plan, evs []events.Event) (check.Context, error) {
	ctx := newContext(root, branch, cfg, p, evs)
	base := cfg.BaseBranch
	if base == "" {
		var err error
		if base, err = gitx.DefaultBranch(root); err != nil {
			return check.Context{}, err
		}
	}
	if p != nil || base != branch {
		var err error
		if ctx.Changed, err = gitx.Status(root); err != nil {
			return check.Context{}, err
		}
	}
	return ctx, nil
}

// failingRefusal is the refusal, as what, listing the failing ones of
// findings with how to fix each, or nil when none fails.
func failingRefusal(what string, findings []check.Finding) *out.Refusal {
	var failing []check.Finding
	for _, f := range findings {
		if f.Severity == check.Fail {
			failing = append(failing, f)
		}
	}
	if len(failing) == 0 {
		return nil
	}
	reason := fmt.Sprintf("%d failing finding%s:", len(failing), plural(len(failing)))
	var next []string
	for _, f := range failing {
		reason += "\n  " + findingLine(f)
		if !slices.Contains(next, f.Next) {
			next = append(next, f.Next)
		}
	}
	return out.Refuse(what, reason, next...)
}

// printFindings writes findings, each with how to fix it, and how many bypass
// events were written, to w.
func printFindings(w io.Writer, findings []check.Finding, written int) {
	var b strings.Builder
	if len(findings) == 0 {
		b.WriteString("No findings\n")
	}
	for _, f := range findings {
		b.WriteString(findingLine(f) + "\n  to fix: " + f.Next + "\n")
	}
	if written > 0 {
		fmt.Fprintf(&b, "Wrote %d bypass event%s to %s\n", written, plural(written), events.Path)
	}
	_, _ = io.WriteString(w, b.String())
}

// baseRef resolves the base branch check diffs against: base, or the
// add-time default when the config sets none, as a local branch, else as
// origin/<base>. It refuses, as what, when neither exists, never with git's own error.
func baseRef(root, base, what string, ci bool) (string, error) {
	if base == "" {
		var err error
		if base, err = gitx.DefaultBranch(root); err != nil {
			return "", err
		}
	}
	if base == "" {
		return "", out.Refuse(what, "HEAD is detached and "+config.Path+" sets no base_branch",
			"set base_branch in "+config.Path+" to the branch units start from")
	}
	ref, err := gitx.BaseRef(root, base)
	if err != nil || ref != "" {
		return ref, err
	}
	next := []string{"git fetch origin " + base}
	if ci {
		next = append(next, "fetch full history: actions/checkout with fetch-depth: 0")
	}
	return "", out.Refuse(what,
		fmt.Sprintf("the base branch %s is missing: neither %s nor origin/%s exists here", base, base, base), next...)
}

// loggedUnit returns the plan of the unit an add event in evs puts on branch,
// or nil when there is none or its plan file is gone.
func loggedUnit(root, branch string, evs []events.Event) (*unit.Plan, error) {
	id, err := check.BranchUnit(evs, branch)
	if err != nil || id == "" {
		return nil, err
	}
	p, err := unit.ReadPlan(filepath.Join(root, filepath.FromSlash(planDir), id+".md"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return p, err
}

// closedUnit returns the plan of the unit closed last, by the close events in
// evs, whose plan in the checkout is on branch and closed, or nil when there
// is none.
func closedUnit(root, branch string, evs []events.Event) (*unit.Plan, error) {
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type != "close" {
			continue
		}
		p, err := unit.ReadPlan(filepath.Join(root, filepath.FromSlash(planDir), evs[i].Unit+".md"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if p.Branch == branch && p.Status == unit.StatusClosed {
			return p, nil
		}
	}
	return nil, nil
}

// bypassFindings are the failing findings of ctx a bypass event records, at
// commit time: those on committed content or on what the
// commit in flight has staged. A finding caused only by changes the commit
// leaves out is left out, and so are approval drift and TESTING.md drift, which
// are pending re-approvals, and what warns locally and fails only in CI: stale
// evidence and the registry. With base "", in a repo with no commits, nothing
// is committed.
func bypassFindings(root string, ctx check.Context, base string) ([]check.Finding, error) {
	if base == "" {
		return nil, nil
	}
	staged, err := gitx.Staged(root)
	if err != nil {
		return nil, err
	}
	if len(ctx.Changed) > 0 {
		committed, err := gitx.BranchCommitted(root, base)
		if err != nil {
			return nil, err
		}
		ctx.Changed = slices.Compact(slices.Sorted(slices.Values(append(committed, staged...))))
	}
	findings, err := check.Run(ctx)
	if err != nil {
		return nil, err
	}
	dirty, err := gitx.Status(root)
	if err != nil {
		return nil, err
	}
	var kept []check.Finding
	for _, f := range findings {
		if f.Severity != check.Fail || f.Check == check.IntegrityCheck || f.Check == check.TestingCheck || f.Check == check.StaleCheck || f.Check == check.RegistryCheck || f.Check == check.CauseCheck || f.Check == check.SignedCheck ||
			slices.Contains(dirty, f.Path) && !slices.Contains(staged, f.Path) {
			continue
		}
		kept = append(kept, f)
	}
	return kept, nil
}

// recordBypasses appends one bypass event per finding, except where evs,
// or this call, already has one for the same check, unit and message. It
// returns how many it wrote.
func recordBypasses(root string, evs []events.Event, findings []check.Finding) (int, error) {
	type key struct{ check, unit, message string }
	seen := map[key]bool{}
	for _, e := range evs {
		if e.Type == "bypass" {
			c, _ := e.Fields["check"].(string)
			m, _ := e.Fields["message"].(string)
			seen[key{c, e.Unit, m}] = true
		}
	}
	written := 0
	for _, f := range findings {
		k := key{f.Check, f.Unit, f.Message}
		if seen[k] {
			continue
		}
		seen[k] = true
		if _, err := events.Append(root, events.Event{Type: "bypass", Unit: f.Unit,
			Fields: map[string]any{"check": f.Check, "message": f.Message}}); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// findingLine is f on one line: severity, check and message.
func findingLine(f check.Finding) string {
	return fmt.Sprintf("%s: %s: %s", f.Severity, f.Check, f.Message)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
