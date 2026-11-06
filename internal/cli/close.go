package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/check"
	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/evidence"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/runner"
	"github.com/oritromax/lyceum/internal/testspec"
	"github.com/oritromax/lyceum/internal/unit"
	"github.com/spf13/cobra"
)

// runClose is `lyceum close [unit]` (units.md, "Close"). It
// closes the branch's open unit; a unit argument must name it. It refuses on
// a dirty tree and when the plan's CLI-owned fields disagree with the event
// log, so a hand-edited gate: closes nothing. A trivial chore needs every
// `check --ci` check to pass on its branch; any other unit needs
// its last gate before close cleared (AC125). A feat needs every area in
// areas: among the files its spec approval hashed (AC31), a refactor may
// change the area files only in cites: frontmatter (AC32), and a TESTING.md
// changed on the branch needs the owner's approval as it is now (AC33). A
// test unit whose new test fails is converted to a bug instead (convertTest,
// AC124). It
// then sets status closed and gate close, writes a close event and commits
// the plan with it, and opens a pull request or prints how (openPR). It
// warns, never refuses, when an open unit on another branch names one of its
// areas (areaWarnings).
func runClose(cmd *cobra.Command, args []string) error {
	what := "close the unit"
	if len(args) > 1 {
		return out.Refuse(what, "close takes at most one unit", `run "lyceum close" on the unit's branch`)
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
	branch, err := gitx.Branch(root)
	if err != nil {
		return err
	}
	p, err := openUnit(root, branch)
	if err != nil {
		return err
	}
	if p == nil {
		return out.Refuse(what, "this branch has no open unit, so there is nothing to close",
			`check out the branch of the unit to close, then run "lyceum close"`)
	}
	if len(args) == 1 && args[0] != p.ID {
		return out.Refuse(what, fmt.Sprintf("unit %s is not the open unit on this branch, which is %s", args[0], p.ID),
			fmt.Sprintf("check out the branch of unit %s, then run lyceum close %s", args[0], args[0]),
			fmt.Sprintf("or close this branch's unit: lyceum close %s", p.ID))
	}
	what = "close " + p.ID
	changed, err := events.TreeChanges(root)
	if err != nil {
		return err
	}
	if len(changed) > 0 {
		return dirtyRefusal(root, what, "the working tree has uncommitted changes:\n  "+strings.Join(changed, "\n  "), changed,
			`commit them: git add -A && git commit -m "<message>", then run "lyceum close" again`,
			`or stash them: git stash -u, then run "lyceum close" again`)
	}
	evs, err := events.Read(root)
	if err != nil {
		return err
	}
	ctx, base, err := checkContext(root, branch, cfg, p, evs, what, false)
	if err != nil {
		return err
	}
	typ, current, class, err := unitGate(p)
	if err != nil {
		return out.Refuse(what, fmt.Sprintf("the plan of unit %s has type %s and gate %s, which do not go together", p.ID, p.Type, p.Gate),
			`run "lyceum check" to see what changed in the plan`)
	}

	if typ == gate.Test && current != gate.Close {
		rec, found, err := failingNewTest(root, cfg.BaseBranch, p)
		if err != nil {
			return err
		}
		if found {
			return convertTest(cmd, root, cfg, branch, p, rec)
		}
	}

	trivial := trivialChore(p)
	var findings []check.Finding
	if trivial {
		findings, err = check.Run(ctx)
	} else {
		findings, err = check.CLIFields(ctx)
	}
	if err != nil {
		return err
	}
	if r := failingRefusal(what, findings); r != nil {
		return r
	}
	if !trivial && current != gate.Close {
		gates, err := gate.Gates(typ)
		if err != nil {
			return err
		}
		next := gateNext(class, current)
		if target, targetClass, ok := gate.Pending(typ, current); class == gate.Step && ok {
			next = fmt.Sprintf("finish the %s work described in the plan, then %s", current, gateNext(targetClass, target))
		}
		return out.Refuse(what, fmt.Sprintf("unit %s is at gate %s, and close needs its %s gate cleared first", p.ID, current, gates[len(gates)-2]), next)
	}

	planPath := path.Join(planDir, p.ID+".md")
	switch typ {
	case gate.Feat:
		if area, file := unhashedArea(evs, p); area != "" {
			return out.Refuse(what, fmt.Sprintf("the plan names area %s, but the spec approval did not hash %s, so its spec text was never approved", area, file),
				fmt.Sprintf(`write the spec of area %s in %s, reopen the spec: lyceum amend "<reason>", then run "lyceum approve spec" yourself in a terminal`, area, file))
		}
	case gate.Refactor:
		files, err := specDiffs(root, base, ctx.Changed)
		if err != nil {
			return err
		}
		if len(files) > 0 {
			return out.Refuse(what, "a refactor changes the spec only in cites: frontmatter, and the branch changes more in: "+strings.Join(files, ", "),
				"take those spec changes off this branch, keeping only cites:, then run \"lyceum close\" again",
				`or, if behaviour changed, split it out: lyceum add feat "<title>"`)
		}
	}
	stale, err := check.StaleEvidence(ctx)
	if err != nil {
		return err
	}
	if r := failingRefusal(what, stale); r != nil {
		return r
	}
	if slices.Contains(ctx.Changed, approval.TestingPath) {
		pending, err := testingPending(root)
		if err != nil {
			return err
		}
		if pending {
			return out.Refuse(what, approval.TestingPath+" changed on this branch, and the owner has not approved it as it is now",
				`run "lyceum approve testing" yourself in a terminal, then run "lyceum close" again`)
		}
	}

	for _, w := range areaWarnings(root, branch, p.ID, p.Areas) {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), w)
	}
	if err := closeUnit(root, p, map[string]any{}); err != nil {
		return err
	}
	if err := gitx.Commit(root, "lyceum: close "+p.ID, planPath, events.Path); err != nil {
		return err
	}
	baseName, err := baseBranchName(root, cfg)
	if err != nil {
		return err
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	w := cmd.OutOrStdout()
	if asJSON {
		// stdout holds only the one JSON object.
		w = cmd.ErrOrStderr()
	} else {
		_, _ = fmt.Fprintf(w, "Closed %s\n", p.ID)
	}
	next, err := openPR(w, root, branch, baseName, planPath, p)
	if err != nil {
		return err
	}
	out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(out.Result{
		Unit: p.ID,
		Gate: string(gate.Close),
		Next: next,
		Data: struct {
			Plan string `json:"plan"`
		}{planPath},
	})
	return nil
}

// failingNewTest returns the last failing run record of test unit p's new
// test: p's run is pending, its branch diff against base has test
// files and no code (newTestFails), and the run is gate evidence now
// (evidence.Check: the full suite, at HEAD, on a clean tree, under the
// approved TESTING.md) and exited non-zero without timing out. found is false
// when there is none.
func failingNewTest(root, base string, p *unit.Plan) (rec runner.Record, found bool, err error) {
	spec, err := testspec.Read(root)
	if err != nil {
		// With no TESTING.md that reads, no run is gate evidence.
		return runner.Record{}, false, nil
	}
	if ok, err := newTestFails(root, base, spec.TestFiles, p); err != nil || !ok {
		return runner.Record{}, false, err
	}
	recs, err := runner.Records(root)
	if err != nil {
		return runner.Record{}, false, err
	}
	for _, r := range slices.Backward(recs) {
		if r.Exit == 0 || r.TimedOut {
			continue
		}
		if ok, _, err := evidence.Check(root, r); err != nil {
			return runner.Record{}, false, err
		} else if ok {
			return r, true, nil
		}
	}
	return runner.Record{}, false, nil
}

// convertTest closes test unit p, on branch, whose new test fails on the
// current code in rec, by converting it to a bug (units.md, "Close"): a close event for p with converted_to, and a new bug unit on
// the same branch, with p's title, a plan and an add event, committed
// together. The bug's repro then clears on rec (clearGate), with every test
// file of the branch diff fingerprinted as its repro, so the bug is at
// diagnose, in-progress, as the event log replays it.
func convertTest(cmd *cobra.Command, root string, cfg *config.Config, branch string, p *unit.Plan, rec runner.Record) error {
	what := "close " + p.ID
	spec, err := testspec.Read(root)
	if err != nil {
		return err
	}
	tests, _, _, err := branchTests(root, cfg.BaseBranch, spec.TestFiles, what)
	if err != nil {
		return err
	}
	fp, err := fingerprint(root, tests)
	if err != nil {
		return err
	}
	baseName, err := baseBranchName(root, cfg)
	if err != nil {
		return err
	}
	taken, err := planHashes(root, baseName)
	if err != nil {
		return err
	}
	slug, err := unit.Slug(p.Title)
	if err != nil {
		return err
	}
	hash := unit.NewHash()
	for taken[hash] {
		hash = unit.NewHash()
	}
	id := unit.ID(hash, slug)
	bug, err := unit.NewPlan(id, string(gate.Bug), p.Title, branch, unit.StatusPlanned, string(gate.Repro))
	if err != nil {
		return err
	}

	testPath, bugPath := path.Join(planDir, p.ID+".md"), path.Join(planDir, id+".md")
	if err := closeUnit(root, p, map[string]any{"converted_to": id}); err != nil {
		return err
	}
	if err := unit.WritePlan(filepath.Join(root, filepath.FromSlash(bugPath)), bug); err != nil {
		return err
	}
	if _, err := events.Append(root, events.Event{Type: "add", Unit: id, Fields: map[string]any{
		"unit_type": string(gate.Bug), "branch": branch, "gate": string(gate.Repro), "status": unit.StatusPlanned,
	}}); err != nil {
		return err
	}
	if err := gitx.Commit(root, fmt.Sprintf("lyceum: close %s, converted to bug %s", p.ID, id), testPath, bugPath, events.Path); err != nil {
		return err
	}
	res := out.Result{Unit: id}
	cleared, err := clearGate(root, bug, gate.Bug, gate.Repro, map[string]any{"run": rec, "fingerprint": fp}, &res)
	if err != nil {
		return err
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	if !asJSON {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Closed %s: its new test fails on the current code (%s.out), so it is now bug %s with that test as its repro\nCreated %s\n%s",
			p.ID, runPath(rec), id, bugPath, cleared)
	}
	res.Data = struct {
		Plan          string   `json:"plan"`
		ConvertedFrom string   `json:"converted_from"`
		Repro         []string `json:"repro"`
	}{bugPath, p.ID, slices.Sorted(maps.Keys(fp))}
	out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(res)
	return nil
}

// closeUnit sets p's status to closed and its gate to close, writes a close
// event for it with fields and the status, and writes the plan. The caller
// commits them.
func closeUnit(root string, p *unit.Plan, fields map[string]any) error {
	// The plan is changed in memory before the event is written, so a plan the
	// CLI cannot rewrite leaves no event behind.
	if p.Gate != string(gate.Close) {
		if err := p.SetCLIField("gate", string(gate.Close)); err != nil {
			return err
		}
	}
	if err := p.SetCLIField("status", unit.StatusClosed); err != nil {
		return err
	}
	fields["status"] = unit.StatusClosed
	if _, err := events.Append(root, events.Event{Type: "close", Unit: p.ID, Fields: fields}); err != nil {
		return err
	}
	return unit.WritePlan(filepath.Join(root, filepath.FromSlash(planDir), p.ID+".md"), p)
}

// baseBranchName is the branch units start from: the config's base_branch,
// else the one origin's HEAD names.
func baseBranchName(root string, cfg *config.Config) (string, error) {
	if cfg.BaseBranch != "" {
		return cfg.BaseBranch, nil
	}
	return gitx.DefaultBranch(root)
}

// unhashedArea returns the first area in feat p's areas: whose file is not
// among the files p's latest counting spec approval in evs hashed, and
// that file, or "" when there is none.
func unhashedArea(evs []events.Event, p *unit.Plan) (area, file string) {
	var hashed map[string]any
	for _, e := range evs {
		if g, _ := e.Fields["gate"].(string); e.Type == "approve" && e.Unit == p.ID && g == string(gate.Spec) && e.Fields["counts"] != false {
			hashed, _ = e.Fields["hashes"].(map[string]any)
		}
	}
	for _, area := range p.Areas {
		file := path.Join("lyceum/spec", area+".md")
		if _, ok := hashed[file]; !ok {
			return area, file
		}
	}
	return "", ""
}

// specDiffs returns the area files, lyceum/spec/*.md, among changed, the
// paths the branch changes against base, that differ from the merge base in
// anything but their cites: frontmatter, in order. An added or
// deleted area file differs.
func specDiffs(root, base string, changed []string) ([]string, error) {
	mb, err := gitx.MergeBase(root, base)
	if err != nil {
		return nil, err
	}
	before, err := gitx.TreeFiles(root, mb, "lyceum/spec")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range changed {
		if dir, name := path.Split(f); dir != "lyceum/spec/" || !strings.HasSuffix(name, ".md") {
			continue
		}
		now, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if errors.Is(err, fs.ErrNotExist) || !slices.Contains(before, f) {
			files = append(files, f)
			continue
		}
		if err != nil {
			return nil, err
		}
		was, err := gitx.Show(root, mb, f)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(withoutCites(was), withoutCites(now)) {
			files = append(files, f)
		}
	}
	return files, nil
}

// withoutCites is an area file's data without its cites: frontmatter key; a
// frontmatter that leaves empty goes too, so adding cites: to a file with no
// frontmatter changes nothing else. Data with no frontmatter is kept as is.
func withoutCites(data []byte) []byte {
	stripped, err := unit.WithoutKeys(data, "cites")
	if err != nil {
		return data
	}
	if rest, ok := bytes.CutPrefix(stripped, []byte("---\n---\n")); ok {
		return rest
	}
	return stripped
}
