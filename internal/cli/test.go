package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/oritromax/lyceum/internal/acmap"
	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/check"
	"github.com/oritromax/lyceum/internal/codepath"
	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/evidence"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/guard"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/runner"
	"github.com/oritromax/lyceum/internal/testspec"
	"github.com/oritromax/lyceum/internal/unit"
	"github.com/spf13/cobra"
)

// tailLines is how much of a failing run's output is printed.
const tailLines = 50

// goCached matches a line of go test output for a package it did not run.
var goCached = regexp.MustCompile(`(?m)^ok\s+\S+\s+\(cached\)`)

// runTest is `lyceum test [subset]` (test-runner.md, "lyceum test [subset]"). It
// runs the TESTING.md suite, or the named subset, and records the run. It runs
// nothing without TESTING.md (AC55), for an unknown subset (AC58) or when no
// file matches test_files (AC63), where a doc unit's verify may clear with no
// run (noTestsClear). Every run is recorded; when the run is not
// gate evidence (an unapproved or changed TESTING.md, a subset, a dirty
// tree, an old HEAD), the output says why it can't clear a gate. A failing
// run prints the end of its output and is a refusal; the CLI never touches a
// test file. Unknown TESTING.md keys are warned about. A passing
// gate-evidence run clears the unit's mechanical gate when its condition
// holds (autoClear). Before the run, a bug at cause with a code commit after
// its verify clear goes back to fix (reopenBug).
func runTest(cmd *cobra.Command, args []string) error {
	what := "run tests"
	if len(args) > 1 {
		return out.Refuse(what, "test takes at most one subset", `run "lyceum test" for the full suite, or "lyceum test <subset>"`)
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
	cfg, err := config.Read(root)
	if errors.Is(err, fs.ErrNotExist) {
		return out.Refuse(what, "Lyceum is not set up in this repo: "+config.Path+" is missing", `run "lyceum init"`)
	} else if err != nil {
		return err
	}

	data, err := os.ReadFile(filepath.Join(root, testspec.Path))
	if errors.Is(err, fs.ErrNotExist) {
		if asJSON, _ := cmd.Flags().GetBool("json"); !asJSON {
			_, _ = io.WriteString(cmd.ErrOrStderr(), testspec.Schema)
		}
		return out.Refuse(what, "this is the first run: "+testspec.Path+" does not exist, and it must be drafted and approved before tests run",
			"the agent drafts "+testspec.Path+" from the project's test frameworks",
			`then run "lyceum approve testing" yourself in a terminal`).WithJSON(map[string]any{"schema": testspec.Schema})
	} else if err != nil {
		return err
	}
	spec, err := testspec.Parse(data)
	if err != nil {
		return out.Refuse(what, err.Error(),
			`fix the frontmatter of `+testspec.Path+`, then run "lyceum approve testing" yourself in a terminal`)
	}
	for _, key := range spec.Unknown {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s frontmatter key %q is not one this version reads; it is ignored\n", testspec.Path, key)
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	stdout, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	branch, err := gitx.Branch(root)
	if err != nil {
		return err
	}
	p, err := openUnit(root, branch)
	if err != nil {
		return err
	}
	// Before the run, so it runs for the gate the bug is really at.
	reopened, err := reopenBug(root, p)
	if err != nil {
		return err
	}
	if reopened != "" && !asJSON {
		_, _ = io.WriteString(stdout, reopenedLine(p.ID, reopened))
	}
	if cmd.Flags().Changed("repro") {
		name, _ := cmd.Flags().GetString("repro")
		return runRepro(cmd, root, cfg.BaseBranch, spec, name, args)
	}

	c := runner.Command{Command: spec.Suite, Timeout: spec.Timeout}
	label := "the suite"
	if len(args) == 1 {
		c.Subset = args[0]
		var ok bool
		if c.Command, ok = spec.Subsets[c.Subset]; !ok {
			reason := fmt.Sprintf("%s defines no subset %q", testspec.Path, c.Subset)
			next := []string{`run "lyceum test" for the full suite`}
			if names := spec.SubsetNames(); len(names) > 0 {
				reason += "; the defined subsets are: " + strings.Join(names, ", ")
				next = append([]string{`run "lyceum test <subset>" with one of: ` + strings.Join(names, ", ")}, next...)
			} else {
				reason += "; it defines no subsets"
			}
			return out.Refuse(what, reason, next...)
		}
		label = "subset " + c.Subset
	}

	found, err := spec.HasTests(root)
	if err != nil {
		return err
	}
	if !found {
		reason := "0 tests found: " + testspec.Path + " sets no test_files"
		if len(spec.TestFiles) > 0 {
			reason = "0 tests found: no file matches test_files in " + testspec.Path + " (" + strings.Join(spec.TestFiles, ", ") + ")"
		}
		if c.Subset == "" {
			res := out.Result{Data: map[string]any{"no_tests": true}}
			line, err := noTestsClear(root, cfg.BaseBranch, p, reason, &res)
			if err != nil {
				return err
			}
			if line != "" {
				if !asJSON {
					_, _ = io.WriteString(stdout, line)
				}
				out.Writer{Stdout: stdout, Stderr: stderr, JSON: asJSON}.Result(res)
				return nil
			}
		}
		return out.Refuse(what, reason,
			`write tests for the project (on existing code, characterization tests that pin its current behaviour), then run "lyceum test" again`)
	}

	live := stdout
	if asJSON {
		// stdout holds only the one JSON object.
		live = stderr
	}
	rec, err := runner.Run(root, c, live, stderr)
	if err != nil {
		return err
	}
	if out, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(runner.Dir), rec.Output)); err == nil && goCached.Match(out) {
		_, _ = fmt.Fprintln(stderr, `Warning: this run reused cached Go test results, so those packages did not run: add -count=1 to the go test command in `+testspec.Path)
	}
	_, reasons, err := evidence.Check(root, rec)
	if err != nil {
		return err
	}
	// A TESTING.md reason names approve testing (AC60); a subset run's own
	// next step is the unit's, not a full run.
	notice, noticeNext := "", ""
	for _, r := range reasons {
		notice += "This run can't clear a gate: " + r.Why + "\n"
		if r.Testing || noticeNext == "" && rec.Subset == nil {
			noticeNext = r.Next
		}
	}

	summary, err := runSummary(root, spec.Timeout, rec)
	if err != nil {
		return err
	}
	runs := runPath(rec)
	failed := rec.Exit != 0
	if !asJSON {
		_, _ = io.WriteString(stdout, summary+notice)
	}

	if failed {
		reason := fmt.Sprintf("%s exited %d", c.Command, rec.Exit)
		if rec.TimedOut {
			reason = fmt.Sprintf("%s ran over the %s timeout and was killed", c.Command, spec.Timeout)
		}
		reason += "; the full output is in " + runs + ".out"
		next := []string{`fix the code, never the failing test, then run "lyceum test" again`,
			`or, for a new failure, start a bug: lyceum add bug "<title>"`}
		newTest, err := newTestFails(root, cfg.BaseBranch, spec.TestFiles, p)
		if err != nil {
			return err
		}
		if newTest {
			// close converts the unit to a bug with this test as its repro.
			reason += "; the branch changes only test files, so the new test fails on the current code"
			next = []string{`to keep it as a bug: run "lyceum close", which closes this unit and starts a bug on this branch with this test as its repro`}
		}
		return out.Refuse("pass "+label, reason, next...).WithJSON(map[string]any{"data": rec})
	}

	res := out.Result{Next: `start a unit: lyceum add feat "<title>"`, Data: rec}
	if p != nil {
		res.Unit, res.Gate = p.ID, p.Gate
		if _, current, class, err := unitGate(p); err == nil {
			res.Next = gateNext(class, current)
		}
		if len(reasons) == 0 {
			bench := func(command string) (runner.Record, string, error) {
				brec, err := runner.Run(root, runner.Command{Command: command, Benchmark: true, Timeout: spec.Timeout}, live, stderr)
				if err != nil {
					return runner.Record{}, "", err
				}
				summary, err := runSummary(root, spec.Timeout, brec)
				return brec, summary, err
			}
			line, err := autoClear(root, cfg.BaseBranch, spec.TestFiles, p, rec, bench, &res)
			if err != nil {
				return err
			}
			if !asJSON {
				_, _ = io.WriteString(stdout, line)
			}
		}
	}
	if noticeNext != "" {
		res.Next = noticeNext
	}
	out.Writer{Stdout: stdout, Stderr: stderr, JSON: asJSON}.Result(res)
	return nil
}

// autoClear clears the mechanical gate that rec, a passing gate-evidence run,
// proves for p: p's current gate when it is mechanical, or the gate
// after the work steps p is at, which end with it (gate.Pending). base is the
// configured base branch and testFiles are the TESTING.md test_files globs.
// When that gate's own condition holds, it clears it (clearGate) with rec
// inlined in the gate-clear event (AC65). A bug's repro clears only on
// `lyceum test --repro` (runRepro), and its verify only while the repro test
// files are as fingerprinted or approved (frozenChanged), unless repro was
// skipped with --no-repro (reproSkipped). A doc's verify clears only when
// the branch diff has no code (branchCode). A refactor's baseline and verify
// clear as baselineClear and refactorVerify say; bench runs a refactor's
// benchmark. A test unit's run clears only when the branch diff has a test
// file and no code. It sets res to where p is now and returns the line saying what
// happened, or "" when p has no mechanical gate to clear.
func autoClear(root, base string, testFiles []string, p *unit.Plan, rec runner.Record, bench benchRunner, res *out.Result) (string, error) {
	typ, current, _, err := unitGate(p)
	if err != nil {
		// A hand-edited type: or gate: clears nothing; check reports it.
		return "", nil
	}
	target, class, ok := gate.Pending(typ, current)
	// Code committed after verify cleared: this run, at HEAD, clears it again.
	evs, err := events.Read(root)
	if err != nil {
		return "", err
	}
	stale, err := check.StaleEvidence(check.Context{Root: root, Plan: p, Events: evs})
	if err != nil {
		return "", err
	}
	if len(stale) > 0 {
		target, class, ok = gate.Verify, gate.Mechanical, true
	}
	if !ok || class != gate.Mechanical {
		return "", nil
	}
	if typ == gate.Bug && target == gate.Repro {
		res.Next = reproNext
		return fmt.Sprintf("Not cleared: %s: it clears only on a failing repro test run by lyceum test --repro <test>, not on the full suite\n", target), nil
	}
	if typ == gate.Doc && target == gate.Verify {
		code, err := branchCode(root, base, "clear "+string(gate.Verify))
		if err != nil {
			return "", err
		}
		if len(code) > 0 {
			res.Next = `take those changes off this branch, keeping only docs and other non-code files, then run "lyceum test"`
			return fmt.Sprintf("Not cleared: %s: the branch changes code, and a doc unit changes only non-code files: %s\n", target, strings.Join(code, ", ")), nil
		}
	}
	if typ == gate.Refactor && target == gate.Baseline {
		return baselineClear(root, base, testFiles, p, rec, res)
	}
	if typ == gate.Refactor && target == gate.Verify {
		return refactorVerify(root, testFiles, p, rec, bench, res)
	}
	if typ == gate.Test && target == gate.Run {
		tests, code, _, err := branchTests(root, base, testFiles, "clear "+string(gate.Run))
		if err != nil {
			return "", err
		}
		switch {
		case len(code) > 0:
			res.Next = "take those changes off this branch, keeping only test files (test_files in " + testspec.Path + `), then run "lyceum test"`
			return fmt.Sprintf("Not cleared: %s: the branch changes files that are not tests, and a test unit changes only tests: %s\n", target, strings.Join(code, ", ")), nil
		case len(tests) == 0:
			res.Next = "write the new test in a file matching test_files in " + testspec.Path + `, commit it, then run "lyceum test"`
			return fmt.Sprintf("Not cleared: %s: the branch diff has no test file, so the new test is not in the run\n", target), nil
		}
	}
	if typ == gate.Bug && target == gate.Verify {
		changed, _, commit, found, err := frozenChanged(root, p.ID, gate.Repro)
		if err != nil {
			return "", err
		}
		skipped, err := reproSkipped(root, p.ID)
		if err != nil {
			return "", err
		}
		switch {
		case skipped:
			// No repro test to hold: the run is enough (AC107).
		case !found:
			res.Next = `run "lyceum check" to see how the plan got past repro`
			return fmt.Sprintf("Not cleared: %s: the event log records no repro clear for %s\n", target, p.ID), nil
		case guard.ReprofreezeEnabled && len(changed) > 0:
			res.Next = fmt.Sprintf(`restore them as they were when repro cleared: git checkout %s -- %s, commit, then run "lyceum test"; or the owner approves the change: run "lyceum approve tests" yourself in a terminal, then run "lyceum test"`, commit, strings.Join(changed, " "))
			return fmt.Sprintf("Not cleared: %s: these repro test files changed since repro cleared, and a change to a repro test needs the owner's approval: %s\n", target, strings.Join(changed, ", ")), nil
		}
	}
	if typ == gate.Feat && target == gate.Verify {
		unmapped, err := unmappedACs(root, testFiles, p.ACs)
		if err != nil {
			return "", err
		}
		if len(unmapped) > 0 {
			res.Next = `name each AC in a test name or a test comment, in a file matching test_files in ` + testspec.Path + `, then run "lyceum test"`
			return fmt.Sprintf("Not cleared: %s: these ACs in the plan's acs: are not mapped to a test: %s\n", target, strings.Join(unmapped, ", ")), nil
		}
	}

	return clearGate(root, p, typ, target, map[string]any{"run": rec}, res)
}

// noTestsClear clears doc unit p's verify with no run when the project has no
// tests (AC132): verify is p's pending gate (gate.Pending), the tree is
// clean, TESTING.md is approved and unchanged since, and the branch diff
// against base has no code (branchCode). The gate-clear event records
// no_tests: true. reason is why no tests ran. A branch diff with code is a
// refusal naming it. It returns "" when p is not such a unit, and the caller
// keeps the AC63 refusal; otherwise it sets res to where p is now and returns
// the lines saying what happened.
func noTestsClear(root, base string, p *unit.Plan, reason string, res *out.Result) (string, error) {
	if p == nil {
		return "", nil
	}
	typ, current, _, err := unitGate(p)
	if err != nil || typ != gate.Doc {
		return "", nil
	}
	if target, _, ok := gate.Pending(typ, current); !ok || target != gate.Verify {
		return "", nil
	}
	dirty, err := events.TreeChanges(root)
	if err != nil {
		return "", err
	}
	approved, changed, err := testspec.Approved(root)
	if err != nil {
		return "", err
	}
	if len(dirty) > 0 || !approved || changed {
		return "", nil
	}
	code, err := branchCode(root, base, "clear "+string(gate.Verify))
	if err != nil {
		return "", err
	}
	if len(code) > 0 {
		return "", out.Refuse("run tests", reason+"; a doc unit clears verify with no tests only when the branch changes no code, and it changes: "+strings.Join(code, ", "),
			`take those changes off this branch, keeping only docs and other non-code files, then run "lyceum test"`)
	}
	res.Unit = p.ID
	line, err := clearGate(root, p, typ, gate.Verify, map[string]any{"no_tests": true}, res)
	if err != nil {
		return "", err
	}
	return reason + "\n" + strings.TrimSuffix(line, "\n") + " with no tests: the branch changes no code\n", nil
}

// baselineClear clears refactor p's baseline on rec, a passing gate-evidence
// run at HEAD, when the branch diff against base touches no
// code but test files (testFiles), the same rule as repro, and a counting
// `approve tests` covers each test file in it, the characterization tests, at
// its current hash. A test file the branch deleted has no hash
// to approve, so it holds the gate too. The gate-clear event inlines rec and
// fingerprints every file matching testFiles at HEAD (presentTests).
func baselineClear(root, base string, testFiles []string, p *unit.Plan, rec runner.Record, res *out.Result) (string, error) {
	tests, code, mb, err := branchTests(root, base, testFiles, "clear "+string(gate.Baseline))
	if err != nil {
		return "", err
	}
	if len(code) > 0 {
		res.Next = "take those changes off this branch, keeping only test files (test_files in " + testspec.Path + `); the refactor's code changes come after plan is approved, then run "lyceum test"`
		return fmt.Sprintf("Not cleared: %s: the branch changes files that are not tests, and before plan only characterization tests may change: %s\n", gate.Baseline, strings.Join(code, ", ")), nil
	}
	unapproved, gone, err := unapprovedTests(root, p.ID, tests)
	if err != nil {
		return "", err
	}
	if len(gone) > 0 {
		res.Next = fmt.Sprintf(`restore them: git checkout %s -- %s, commit, then run "lyceum test"`, mb, strings.Join(gone, " "))
		return fmt.Sprintf("Not cleared: %s: the branch deletes these test files, and the owner can't approve a deletion in this version: %s\n", gate.Baseline, strings.Join(gone, ", ")), nil
	}
	if len(unapproved) > 0 {
		res.Next = `the owner approves them: run "lyceum approve tests" yourself in a terminal, then run "lyceum test"`
		return fmt.Sprintf("Not cleared: %s: these test files in the branch diff are characterization tests, and they need the owner's approval at their current hash: %s\n", gate.Baseline, strings.Join(unapproved, ", ")), nil
	}
	all, err := presentTests(root, testFiles)
	if err != nil {
		return "", err
	}
	fp, err := fingerprint(root, all)
	if err != nil {
		return "", err
	}
	return clearGate(root, p, gate.Refactor, gate.Baseline, map[string]any{"run": rec, "fingerprint": fp}, res)
}

// newTestFails reports whether p, the branch's unit if any, is a test unit
// whose run is pending and whose branch diff against base has test files
// (testFiles) and no code: a failing run then means the new test fails on the
// current code.
func newTestFails(root, base string, testFiles []string, p *unit.Plan) (bool, error) {
	if p == nil {
		return false, nil
	}
	typ, current, _, err := unitGate(p)
	if err != nil || typ != gate.Test {
		return false, nil
	}
	if target, _, ok := gate.Pending(typ, current); !ok || target != gate.Run {
		return false, nil
	}
	tests, code, _, err := branchTests(root, base, testFiles, "pass the suite")
	if err != nil {
		return false, err
	}
	return len(tests) > 0 && len(code) == 0, nil
}

// benchRunner runs command as a refactor's benchmark and records it,
// returning the record and the summary runSummary prints about it.
type benchRunner func(command string) (runner.Record, string, error)

// refactorVerify clears refactor p's verify on rec, a passing gate-evidence
// run, when each file of the baseline fingerprint
// matches its fingerprint or its latest counting `approve tests` hash
// (frozenChanged), each test file present now that the fingerprint lacks is
// covered by such a hash (unapprovedTests), public_surface: is as the latest
// counting plan approval recorded it (approvedSurface), and, when
// the plan sets benchmark:, that command, run by bench after the suite passed, exits 0. A
// fingerprinted file that is gone holds the gate: the owner can't approve a
// deletion in this version.
func refactorVerify(root string, testFiles []string, p *unit.Plan, rec runner.Record, bench benchRunner, res *out.Result) (string, error) {
	changed, fp, commit, found, err := frozenChanged(root, p.ID, gate.Baseline)
	if err != nil {
		return "", err
	}
	if !found {
		res.Next = `run "lyceum check" to see how the plan got past baseline`
		return fmt.Sprintf("Not cleared: %s: the event log records no baseline clear for %s\n", gate.Verify, p.ID), nil
	}
	present, err := presentTests(root, testFiles)
	if err != nil {
		return "", err
	}
	if !guard.BaselinefreezeEnabled {
		changed, present = nil, nil
	}
	var edited, gone []string
	for _, f := range changed {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); errors.Is(err, fs.ErrNotExist) {
			gone = append(gone, f)
		} else if err != nil {
			return "", err
		} else {
			edited = append(edited, f)
		}
	}
	if len(gone) > 0 {
		res.Next = fmt.Sprintf(`restore them: git checkout %s -- %s, commit, then run "lyceum test"`, commit, strings.Join(gone, " "))
		return fmt.Sprintf("Not cleared: %s: these test files of the baseline fingerprint are deleted, and the owner can't approve a deletion in this version: %s\n", gate.Verify, strings.Join(gone, ", ")), nil
	}
	var added []string
	for _, f := range present {
		if _, ok := fp[f]; !ok {
			added = append(added, f)
		}
	}
	added, _, err = unapprovedTests(root, p.ID, added)
	if err != nil {
		return "", err
	}
	if diff := slices.Sorted(slices.Values(append(edited, added...))); len(diff) > 0 {
		res.Next = fmt.Sprintf(`undo the test changes so the test files are as at the baseline (commit %s), commit, then run "lyceum test"; or the owner approves them: run "lyceum approve tests" yourself in a terminal, then run "lyceum test"`, commit)
		shown, err := testsSummary(root, commit, false, diff)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Not cleared: %s: these test files were changed or added since the baseline, and a test change in a refactor needs the owner's approval: %s\n", gate.Verify, strings.Join(diff, ", ")) + shown, nil
	}
	approved, at, found, err := approvedSurface(root, p)
	if err != nil {
		return "", err
	}
	if !found {
		res.Next = `run "lyceum check" to see how the plan got past plan`
		return fmt.Sprintf("Not cleared: %s: the event log records no counting plan approval for %s\n", gate.Verify, p.ID), nil
	}
	if !slices.Equal(approved, p.PublicSurface) {
		planPath := path.Join(planDir, p.ID+".md")
		res.Next = fmt.Sprintf(`restore public_surface: in %s as approved, commit, then run "lyceum test"; or the owner re-approves the plan with the new list: reopen it with lyceum amend "<reason>", then run "lyceum approve plan" yourself in a terminal`, planPath)
		return fmt.Sprintf("Not cleared: %s: public_surface: changed since the owner approved the plan (commit %s), and a public-surface change needs the owner's re-approval of the plan: approved [%s], now [%s]\n",
			gate.Verify, at, strings.Join(approved, ", "), strings.Join(p.PublicSurface, ", ")), nil
	}
	line := ""
	if p.Benchmark != "" {
		brec, summary, err := bench(p.Benchmark)
		if err != nil {
			return "", err
		}
		line = "Benchmark: " + summary
		if brec.Exit != 0 {
			res.Next = `make the refactor meet the benchmark, commit, then run "lyceum test"`
			return line + fmt.Sprintf("Not cleared: %s: the benchmark %s exited %d; the full output is in %s.out\n", gate.Verify, p.Benchmark, brec.Exit, runPath(brec)), nil
		}
	}
	cleared, err := clearGate(root, p, gate.Refactor, gate.Verify, map[string]any{"run": rec}, res)
	return line + cleared, err
}

// approvedSurface returns public_surface: of refactor p's plan as the latest
// counting plan approval recorded it: the plan in the commit that approval
// made, the first after the approve event's commit. found is
// false when the event log has no counting plan approval of p, and at is
// that commit.
func approvedSurface(root string, p *unit.Plan) (surface []string, at string, found bool, err error) {
	evs, err := events.Read(root)
	if err != nil {
		return nil, "", false, err
	}
	commit := ""
	for _, e := range evs {
		if g, _ := e.Fields["gate"].(string); e.Type == "approve" && e.Unit == p.ID && g == string(gate.Plan) && e.Fields["counts"] != false {
			commit = e.Commit
		}
	}
	if commit == "" {
		return nil, "", false, nil
	}
	if at, err = gitx.FirstAfter(root, commit); err != nil {
		return nil, "", false, err
	}
	if at == "" {
		return nil, "", false, fmt.Errorf("plan approval of %s at commit %s: no approval commit follows it", p.ID, commit)
	}
	data, err := gitx.Show(root, at, path.Join(planDir, p.ID+".md"))
	if err != nil {
		return nil, "", false, err
	}
	approved, err := unit.ParsePlan(data)
	if err != nil {
		return nil, "", false, fmt.Errorf("plan %s at commit %s: %w", p.ID, at, err)
	}
	return approved.PublicSurface, at[:7], true, nil
}

// branchTests splits the paths the branch changes against base, resolved as
// check resolves it, into the test files (testFiles globs) and the
// code that is not a test (codepath.IsCode), in order, with the merge base.
// what is what a missing base refuses.
func branchTests(root, base string, testFiles []string, what string) (tests, code []string, mb string, err error) {
	ref, err := baseRef(root, base, what, false)
	if err != nil {
		return nil, nil, "", err
	}
	changed, err := gitx.BranchChanges(root, ref)
	if err != nil {
		return nil, nil, "", err
	}
	if mb, err = gitx.MergeBase(root, ref); err != nil {
		return nil, nil, "", err
	}
	for _, f := range changed {
		switch {
		case acmap.MatchAny(testFiles, f):
			tests = append(tests, f)
		case codepath.IsCode(f):
			code = append(code, f)
		}
	}
	return tests, code, mb, nil
}

// branchCode returns the paths the branch changes against base, resolved as
// check resolves it, that are code by the code-path rule
// (codepath.IsCode), test files included, in order. what is what a missing
// base refuses.
func branchCode(root, base, what string) ([]string, error) {
	ref, err := baseRef(root, base, what, false)
	if err != nil {
		return nil, err
	}
	changed, err := gitx.BranchChanges(root, ref)
	if err != nil {
		return nil, err
	}
	var code []string
	for _, f := range changed {
		if codepath.IsCode(f) {
			code = append(code, f)
		}
	}
	return code, nil
}

// presentTests returns the test files (testFiles globs) there are now, in
// order: those in HEAD's tree and those with uncommitted changes, less the
// gone ones. On a clean tree, as a gate-evidence run has, these are HEAD's.
func presentTests(root string, testFiles []string) ([]string, error) {
	files, err := gitx.TreeFiles(root, "HEAD", ".")
	if err != nil {
		return nil, err
	}
	dirty, err := gitx.Status(root)
	if err != nil {
		return nil, err
	}
	var tests []string
	for _, f := range append(files, dirty...) {
		if !acmap.MatchAny(testFiles, f) {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); err == nil {
			tests = append(tests, f)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	slices.Sort(tests)
	return slices.Compact(tests), nil
}

// unapprovedTests returns those of files, test files under root, whose
// sha256 is not the one id's latest counting `approve tests` recorded for
// them (testsApproved), and apart those that are gone, in the order of files.
func unapprovedTests(root, id string, files []string) (unapproved, gone []string, err error) {
	evs, err := events.Read(root)
	if err != nil {
		return nil, nil, err
	}
	approved := testsApproved(evs, id)
	for _, f := range files {
		h, err := approval.Hash(filepath.Join(root, filepath.FromSlash(f)))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			gone = append(gone, f)
		case err != nil:
			return nil, nil, err
		case h != approved[f]:
			unapproved = append(unapproved, f)
		}
	}
	return unapproved, gone, nil
}

// testsApproved returns, per file, the sha256 unit id's latest counting
// `approve tests` in evs recorded for it. One an amend invalidated
// does not count (check.Counting).
func testsApproved(evs []events.Event, id string) map[string]string {
	approved := map[string]string{}
	for _, e := range check.Counting(evs) {
		if g, _ := e.Fields["gate"].(string); e.Type == "approve" && e.Unit == id && g == string(approval.Tests) && e.Fields["counts"] != false {
			hashes, _ := e.Fields["hashes"].(map[string]any)
			for f, h := range hashes {
				approved[f], _ = h.(string)
			}
		}
	}
	return approved
}

// clearGate clears gate target of p, of type typ: it writes one
// gate-clear event with fields, the gate and the status the clear moves to,
// if any, moves the plan's gate past target and commits the plan and
// the event log. It sets res to where p is now and returns the line saying so.
func clearGate(root string, p *unit.Plan, typ gate.Type, target gate.Gate, fields map[string]any, res *out.Result) (string, error) {
	after, err := gate.Next(typ, target)
	if err != nil {
		return "", err
	}
	afterClass, err := gate.Kind(typ, after)
	if err != nil {
		return "", err
	}
	// The plan is changed in memory before the event is written, so a plan the
	// CLI cannot rewrite leaves no event behind.
	fields["gate"] = string(target)
	if err := p.SetCLIField("gate", string(after)); err != nil {
		return "", err
	}
	if status := unit.StatusAfter(typ, target, p.Status); status != p.Status {
		if err := p.SetCLIField("status", status); err != nil {
			return "", err
		}
		fields["status"] = status
	}
	if _, err := events.Append(root, events.Event{Type: "gate-clear", Unit: p.ID, Fields: fields}); err != nil {
		return "", err
	}
	planPath := path.Join(planDir, p.ID+".md")
	if err := unit.WritePlan(filepath.Join(root, filepath.FromSlash(planPath)), p); err != nil {
		return "", err
	}
	if err := gitx.Commit(root, fmt.Sprintf("lyceum: clear %s %s", target, p.ID), planPath, events.Path); err != nil {
		return "", err
	}
	res.Gate, res.Next = string(after), gateNext(afterClass, after)
	return fmt.Sprintf("Cleared %s for %s\n", target, p.ID), nil
}

// frozenChanged returns the test files the fingerprint of id's latest fpGate
// gate-clear lists whose sha256 under root is neither the
// fingerprinted one nor the one id's latest counting `approve tests` recorded
// for that file, or that are gone, in order, with the fingerprint and
// the commit fpGate cleared at. found is false when the log has no fpGate
// gate-clear of id with a fingerprint.
func frozenChanged(root, id string, fpGate gate.Gate) (changed []string, fp map[string]string, commit string, found bool, err error) {
	evs, err := events.Read(root)
	if err != nil {
		return nil, nil, "", false, err
	}
	approved := testsApproved(evs, id)
	for _, e := range evs {
		if g, _ := e.Fields["gate"].(string); e.Type == "gate-clear" && e.Unit == id && g == string(fpGate) {
			m, ok := e.Fields["fingerprint"].(map[string]any)
			if !ok {
				continue
			}
			fp, commit, found = map[string]string{}, e.Commit, true
			for f, h := range m {
				fp[f], _ = h.(string)
			}
		}
	}
	for _, f := range slices.Sorted(maps.Keys(fp)) {
		h, err := approval.Hash(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, "", false, err
		}
		if h == "" || h != fp[f] && h != approved[f] {
			if fpGate == gate.Repro && h != "" {
				still, err := reproLinesKept(root, commit, f)
				if err != nil {
					return nil, nil, "", false, err
				}
				if still {
					continue
				}
			}
			changed = append(changed, f)
		}
	}
	return changed, fp, commit, found, nil
}

// reproLinesKept reports whether every line test file f had at commit is still
// there, in order, so new tests may be added to it. A file git
// can't show at commit, as after a rebase, does not match.
func reproLinesKept(root, commit, f string) (bool, error) {
	then, err := gitx.Show(root, commit, f)
	if err != nil {
		return false, nil //nolint:nilerr // no repro-time content to compare is a change.
	}
	now, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
	if err != nil {
		return false, err
	}
	rest := strings.Split(string(now), "\n")
	for line := range strings.Lines(string(then)) {
		i := slices.Index(rest, strings.TrimSuffix(line, "\n"))
		if i < 0 {
			return false, nil
		}
		rest = rest[i+1:]
	}
	return true, nil
}

// reproSkipped reports whether the latest repro event of bug id is a
// counting `approve cause --no-repro`, not a repro clear.
func reproSkipped(root, id string) (bool, error) {
	evs, err := events.Read(root)
	if err != nil {
		return false, err
	}
	skipped := false
	for _, e := range evs {
		if g, _ := e.Fields["gate"].(string); e.Unit != id || g != string(gate.Repro) {
			continue
		}
		switch {
		case e.Type == "gate-clear":
			skipped = false
		case e.Type == "approve" && e.Fields["counts"] != false && e.Fields["no_repro"] == true:
			skipped = true
		}
	}
	return skipped, nil
}

// runPath is where rec's record and output are, relative to the repo root
// in git's path form, without the extension.
func runPath(rec runner.Record) string {
	return path.Join(runner.Dir, strings.TrimSuffix(rec.Output, ".out"))
}

// runSummary is what is printed about rec, a run under timeout, once it
// ended: how it ended and, when it failed, the end of its output and where
// the output is, then where its record is.
func runSummary(root string, timeout time.Duration, rec runner.Record) (string, error) {
	runs := runPath(rec)
	var b strings.Builder
	duration := (time.Duration(rec.DurationMS) * time.Millisecond).String()
	failed := rec.Exit != 0
	switch {
	case rec.TimedOut:
		fmt.Fprintf(&b, "Failed: %s ran over the %s timeout and was killed (exit %d)\n", rec.Command, timeout, rec.Exit)
	case failed:
		fmt.Fprintf(&b, "Failed: %s exited %d after %s\n", rec.Command, rec.Exit, duration)
	default:
		fmt.Fprintf(&b, "Passed: %s exited 0 after %s\n", rec.Command, duration)
	}
	if failed {
		output, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(runs)+".out"))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "Last %d lines of the output:\n%s", tailLines, tail(output, tailLines))
		fmt.Fprintf(&b, "Output: %s.out\n", runs)
	}
	fmt.Fprintf(&b, "Recorded: %s.json\n", runs)
	return b.String(), nil
}

// unmappedACs returns the ACs in acs that no file under root matching
// testFiles maps (acmap), in order. None is unmapped when acs is empty.
func unmappedACs(root string, testFiles, acs []string) ([]string, error) {
	if len(acs) == 0 {
		return nil, nil
	}
	mapped, err := acmap.Mapped(root, testFiles)
	if err != nil {
		return nil, err
	}
	var unmapped []string
	for _, id := range acs {
		if len(mapped[id]) == 0 {
			unmapped = append(unmapped, id)
		}
	}
	return unmapped, nil
}

// tail returns the last n lines of output, ending with a newline when output has any.
func tail(output []byte, n int) string {
	lines := bytes.SplitAfter(output, []byte("\n"))
	if len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	s := string(bytes.Join(lines, nil))
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}
