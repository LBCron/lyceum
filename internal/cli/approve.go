package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/oritromax/lyceum/internal/acmap"
	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/guard"
	"github.com/oritromax/lyceum/internal/harness"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/runner"
	"github.com/oritromax/lyceum/internal/testhooks"
	"github.com/oritromax/lyceum/internal/testspec"
	"github.com/oritromax/lyceum/internal/unit"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// runApprove is `lyceum approve [gate]` (enforcement.md, "lyceum approve [gate]").
// Before anything else, it refuses unless no known agent harness variable is
// set and stdin and stdout are terminals, so an agent always gets
// the owner-only refusal. With a gate, it then refuses unless gate is
// testing or project, tests where the unit allows it (testsApprovable),
// cause with --no-repro for a bug at repro, or the current unit's
// gate and a judgment gate. A bug at cause with a code commit after its
// verify clear first goes back to fix (reopenBug), and approve cause is
// then refused. Bare, it walks
// every pending gate in one session (AC46). It approves each gate in turn
// (approveGate).
func runApprove(cmd *cobra.Command, args []string) error {
	bare := len(args) == 0
	var g gate.Gate
	what, run := "approve", "lyceum approve"
	if !bare {
		g = gate.Gate(args[0])
		what, run = what+" "+string(g), run+" "+string(g)
	}

	// A matching harness-prompt grant replaces the env and TTY refusals and
	// the typed confirmation, and nothing else.
	grant := promptGrant(g)

	if grant == nil {
		if err := ownerOnly(cmd, what, run, "approval"); err != nil {
			return err
		}
	}

	if len(args) > 1 {
		return out.Refuse("approve", "approve takes one gate", `run "lyceum approve <gate>" yourself in a terminal`)
	}
	skip := cmd.Flags().Changed("no-repro")
	if skip && g != gate.Cause {
		return out.Refuse(what, "--no-repro goes with approve cause only", noReproRun)
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
	branch, err := gitx.Branch(root)
	if err != nil {
		return err
	}
	p, err := openUnit(root, branch)
	if err != nil {
		return err
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	reopened, err := reopenBug(root, p)
	if err != nil {
		return err
	}
	if reopened != "" && g == gate.Cause && !skip {
		return out.Refuse(what, fmt.Sprintf("unit %s went back to %s: %s", p.ID, gate.Fix, reopened),
			`run "lyceum test" so verify clears again, then run "lyceum approve cause" yourself in a terminal`)
	}
	if reopened != "" && !asJSON {
		_, _ = io.WriteString(cmd.OutOrStdout(), reopenedLine(p.ID, reopened))
	}
	// testing and project are repo-wide: they need no unit, and
	// neither does a bare walk, which may find only testing pending.
	if p == nil && !bare && g != approval.Testing && g != approval.Project {
		return out.Refuse(what, "this branch has no open unit, so there is nothing to approve",
			"check out the branch of the unit to approve",
			`or start one: lyceum add feat "<title>"`)
	}
	cfg, err := config.Read(root)
	if errors.Is(err, fs.ErrNotExist) {
		return out.Refuse(what, "Lyceum is not set up in this repo: "+config.Path+" is missing", `run "lyceum init"`)
	}
	if err != nil {
		return err
	}

	var typ gate.Type
	var current gate.Gate
	var class gate.Class
	if p != nil {
		if typ, current, class, err = unitGate(p); err != nil {
			return err
		}
	}
	w := out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}
	var gates []gate.Gate
	if bare {
		testing, err := testingPending(root)
		if err != nil {
			return err
		}
		if testing {
			gates = append(gates, approval.Testing)
		}
		if p != nil && class == gate.Judgment {
			gates = append(gates, current)
		}
	} else {
		if skip {
			if typ != gate.Bug || current != gate.Repro {
				return out.Refuse(what+" --no-repro", fmt.Sprintf("unit %s is a %s at gate %s; --no-repro skips only a bug's repro gate", p.ID, p.Type, current),
					gateNext(class, current))
			}
			if reason, _ := cmd.Flags().GetString("no-repro"); strings.TrimSpace(reason) == "" {
				return out.Refuse(what+" --no-repro", "--no-repro needs a reason: why this bug can't have a repro test", noReproRun)
			}
			// The skip approves repro.
			g = gate.Repro
		} else if g == approval.Tests {
			if err := testsApprovable(p, typ, current, class); err != nil {
				return err
			}
			files, _, err := testsToApprove(root, cfg.BaseBranch, p, typ)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				msg := "Nothing to approve: every fingerprinted test file matches its fingerprint or its latest approved hash\n"
				switch {
				case current == gate.Baseline:
					msg = "Nothing to approve: every test file in the branch diff matches its latest approved hash\n"
				case typ == gate.Refactor:
					msg = "Nothing to approve: every test file matches its baseline fingerprint or its latest approved hash\n"
				}
				if !asJSON {
					_, _ = io.WriteString(cmd.OutOrStdout(), msg)
				}
				w.Result(walkResult(p, current, class, []string{}))
				return nil
			}
		} else if g != approval.Testing && g != approval.Project && (g != current || class != gate.Judgment) {
			reason := fmt.Sprintf("unit %s is at gate %s", p.ID, current)
			switch class {
			case gate.Judgment:
				reason += "; " + string(current) + " clears by owner approval"
			case gate.Mechanical:
				reason += "; " + string(current) + " clears on a recorded test run, not an approval"
			default:
				reason += "; " + string(current) + " is a work step and clears when its work is done, not by an approval"
			}
			return out.Refuse(what, reason, gateNext(class, current))
		}
		gates = []gate.Gate{g}
	}

	if len(gates) == 0 {
		if !asJSON {
			_, _ = io.WriteString(cmd.OutOrStdout(), "Nothing pending: no gate is waiting for approval\n")
		}
		w.Result(walkResult(p, current, class, []string{}))
		return nil
	}

	// One reader for the session, so no typed answer is lost between gates.
	in := bufio.NewReader(cmd.InOrStdin())
	if !bare {
		reason, _ := cmd.Flags().GetString("no-repro")
		hashes, err := approveGate(cmd, in, root, cfg, p, typ, g, strings.TrimSpace(reason), grant)
		if err != nil {
			return err
		}
		res := out.Result{Next: `run "lyceum test"`}
		if g == approval.Project && p == nil {
			res.Next = `start the first slice of work: lyceum add feat "<title>"`
		}
		if g != approval.Testing && g != approval.Tests && g != approval.Project {
			next, err := gate.Next(typ, g)
			if err != nil {
				return err
			}
			nextClass, err := gate.Kind(typ, next)
			if err != nil {
				return err
			}
			res = out.Result{Unit: p.ID, Gate: string(next), Next: gateNext(nextClass, next)}
		} else if p != nil {
			// testing, tests and project move no gate.
			res = out.Result{Unit: p.ID, Gate: string(current), Next: gateNext(class, current)}
		}
		res.Data = struct {
			Approved string            `json:"approved"`
			Hashes   map[string]string `json:"hashes"`
		}{string(g), hashes}
		w.Result(res)
		return nil
	}

	var names []string
	for _, g := range gates {
		names = append(names, string(g))
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Pending gates: %s\n", strings.Join(names, ", "))

	// A gate that is refused is skipped and the walk goes on (AC46), except
	// that a mismatched confirmation stops it there with one refusal (AC143).
	approved, skipped := []string{}, []string{}
	for i, g := range gates {
		_, err := approveGate(cmd, in, root, cfg, p, typ, g, "", nil)
		var mismatch notConfirmed
		if errors.As(err, &mismatch) {
			reason := fmt.Sprintf("the confirmation of %s did not match, so the walk stopped; not approved: %s",
				g, strings.Join(append(skipped, names[i:]...), ", "))
			if len(approved) > 0 {
				reason = "approved " + strings.Join(approved, ", ") + "; " + reason
			}
			return out.Refuse(what, reason, `run "lyceum approve" again for the gates still pending`)
		}
		var r *out.Refusal
		if errors.As(err, &r) {
			if !asJSON {
				out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr()}.Fail(r)
			}
			skipped = append(skipped, string(g))
			continue
		}
		if err != nil {
			return err
		}
		approved = append(approved, string(g))
		if g != approval.Testing {
			if current, err = gate.Next(typ, g); err != nil {
				return err
			}
			if class, err = gate.Kind(typ, current); err != nil {
				return err
			}
		}
	}
	if len(skipped) > 0 {
		reason := "not approved: " + strings.Join(skipped, ", ")
		if len(approved) > 0 {
			reason = "approved " + strings.Join(approved, ", ") + "; " + reason
		}
		return out.Refuse(what, reason, `run "lyceum approve" again for the gates still pending`)
	}
	w.Result(walkResult(p, current, class, approved))
	return nil
}

// promptGrant returns the harness-prompt grant `approve g` runs with, consumed, or nil when none matches: a grant matches when its unit
// and gate are the ones approve resolves, it is unexpired, and the
// environment carries its harness's marker. Bare approve has none. A grant
// it can't read, or can't consume, is none, so approve refuses as without one.
func promptGrant(g gate.Gate) *harness.Grant {
	if g == "" {
		return nil
	}
	dir, err := os.Getwd()
	if err != nil || !gitx.IsRepo(dir) {
		return nil
	}
	root, err := gitx.Root(dir)
	if err != nil {
		return nil
	}
	grant, err := harness.ReadGrant(root)
	if err != nil || grant == nil {
		return nil
	}
	branch, err := gitx.Branch(root)
	if err != nil {
		return nil
	}
	p, err := openUnit(root, branch)
	if err != nil {
		return nil
	}
	unitID := ""
	if g != approval.Testing && g != approval.Project && p != nil {
		unitID = p.ID
	}
	if guard.HarnesspromptEnabled && (grant.Unit != unitID || grant.Gate != string(g) || grant.Expired() || !harness.InHarness(grant.Harness)) {
		return nil
	}
	if harness.RemoveGrant(root) != nil {
		return nil
	}
	return grant
}

// ownerOnly refuses what, run as run, inside a known agent harness and
// unless stdin and stdout are terminals, checked before anything else,
// so an agent always gets the plain refusal that act is for the owner.
// approve and eject are the owner's alike.
func ownerOnly(cmd *cobra.Command, what, run, act string) error {
	// Before the TTY check: a harness that gives the command a pty is still refused.
	if set := harness.SetEnvVars(); guard.ApproveenvEnabled && len(set) > 0 {
		verb := "is"
		if len(set) > 1 {
			verb = "are"
		}
		return out.Refuse(what, fmt.Sprintf("this is running inside an agent harness (%s %s set); %s is for the owner, typed in their own terminal",
			strings.Join(set, ", "), verb, act),
			fmt.Sprintf(`run "%s" yourself in your own terminal, outside the agent`, run), separateTerminal)
	}
	if guard.ApprovettyEnabled && (!isTerminal(cmd.InOrStdin()) || !isTerminal(cmd.OutOrStdout())) {
		return out.Refuse(what, fmt.Sprintf("stdin and stdout must both be a terminal; %s is for the owner, typed in their own terminal", act),
			fmt.Sprintf(`run "%s" yourself in a terminal`, run), separateTerminal)
	}
	return nil
}

// notConfirmed is the refusal of a typed confirmation that did not match.
type notConfirmed struct{ *out.Refusal }

// Unwrap returns the refusal, so it prints as any other.
func (n notConfirmed) Unwrap() error { return n.Refusal }

// typedConfirmation writes prompt and reports whether the line read back from
// in is exactly word.
func typedConfirmation(w io.Writer, in *bufio.Reader, prompt, word string) bool {
	_, _ = io.WriteString(w, prompt)
	line, err := in.ReadString('\n')
	if err != nil {
		// The answer, if any, was not ended by the user's Enter.
		_, _ = io.WriteString(w, "\n")
	}
	return !guard.ApproveconfirmEnabled || strings.TrimRight(line, "\r\n") == word
}

// separateTerminal ends every harness and TTY refusal of approve and eject
// (step 10): the in-agent shell is still the agent's.
const separateTerminal = "use a separate terminal that the agent doesn't control; " +
	`the in-agent shell, such as Claude Code's "!" prefix, runs inside the agent and doesn't work`

// noReproRun is the next step for a malformed --no-repro.
const noReproRun = `run "lyceum approve cause --no-repro '<reason>'" yourself in a terminal, while the bug is at repro`

// unitGate returns p's unit type and its current gate with the gate's class.
func unitGate(p *unit.Plan) (gate.Type, gate.Gate, gate.Class, error) {
	current := gate.Gate(p.Gate)
	typ := unit.Tier(p.Type, p.Gate, p.Approvals)
	class, err := gate.Kind(typ, current)
	return typ, current, class, err
}

// walkResult is how a bare approve ends: p, if any, at gate current of class,
// with the gates approved in this session.
func walkResult(p *unit.Plan, current gate.Gate, class gate.Class, approved []string) out.Result {
	res := out.Result{Next: `start a unit: lyceum add feat "<title>"`}
	if p != nil {
		res = out.Result{Unit: p.ID, Gate: string(current), Next: gateNext(class, current)}
	}
	res.Data = struct {
		Approved []string `json:"approved"`
	}{approved}
	return res
}

// testsApprovable refuses `approve tests` unless p, of type typ at gate
// current of class, is a bug after repro clears and before cause, or a
// refactor at baseline or at verify, build included since verify ends it.
func testsApprovable(p *unit.Plan, typ gate.Type, current gate.Gate, class gate.Class) error {
	bug := typ == gate.Bug && (current == gate.Diagnose || current == gate.Fix || current == gate.Verify)
	if bug || typ == gate.Refactor && (current == gate.Baseline || current == gate.Build || current == gate.Verify) {
		return nil
	}
	return out.Refuse("approve tests", fmt.Sprintf("unit %s is a %s at gate %s; approve tests is for a bug after repro clears and before cause, or a refactor at baseline, build or verify", p.ID, p.Type, current),
		gateNext(class, current))
}

// testsToApprove returns the files an `approve tests` of p, of type typ,
// hashes, and the commit the summary diffs them against. For a
// refactor at baseline, those are the test files in the branch diff against
// base, the characterization tests, that no counting approve tests covers at
// their current hash, against the merge base. Otherwise they are each
// test file of the fingerprint (a bug's repro, a refactor's baseline) that
// matches neither its fingerprint nor its latest approved hash
// (frozenChanged), and for a refactor each test file present now that the
// fingerprint lacks and no approved hash covers, against the commit
// the fingerprint was taken at. It refuses when there is no fingerprint, and
// when a file is gone, since a missing file has no hash to approve.
func testsToApprove(root, base string, p *unit.Plan, typ gate.Type) ([]string, string, error) {
	what := "approve tests"
	if typ == gate.Refactor && gate.Gate(p.Gate) == gate.Baseline {
		globs, err := acmap.TestingGlobs(filepath.Join(root, approval.TestingPath))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, "", err
		}
		tests, _, mb, err := branchTests(root, base, globs, what)
		if err != nil {
			return nil, "", err
		}
		files, gone, err := unapprovedTests(root, p.ID, tests)
		if err != nil {
			return nil, "", err
		}
		if len(gone) > 0 {
			return nil, "", out.Refuse(what, "the branch deletes these test files, and approve tests approves a file by its hash: "+strings.Join(gone, ", "),
				fmt.Sprintf(`restore them: git checkout %s -- %s, commit, then run "lyceum approve tests" again`, mb, strings.Join(gone, " ")))
		}
		return files, mb, nil
	}
	fpGate := gate.Repro
	if typ == gate.Refactor {
		fpGate = gate.Baseline
	}
	changed, fp, commit, found, err := frozenChanged(root, p.ID, fpGate)
	if err != nil {
		return nil, "", err
	}
	if !found {
		return nil, "", out.Refuse(what, fmt.Sprintf("no %s fingerprint: the event log records no %s clear with a fingerprint for %s", fpGate, fpGate, p.ID),
			fmt.Sprintf(`run "lyceum check" to see how the plan got past %s`, fpGate))
	}
	var gone []string
	for _, f := range changed {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); errors.Is(err, fs.ErrNotExist) {
			gone = append(gone, f)
		} else if err != nil {
			return nil, "", err
		}
	}
	if len(gone) > 0 {
		return nil, "", out.Refuse(what, "these fingerprinted test files are gone, and approve tests approves a file by its hash: "+strings.Join(gone, ", "),
			fmt.Sprintf(`restore them: git checkout %s -- %s, commit, then run "lyceum approve tests" again`, commit, strings.Join(gone, " ")))
	}
	if typ != gate.Refactor {
		return changed, commit, nil
	}
	// A test file added since the baseline is a change too.
	globs, err := acmap.TestingGlobs(filepath.Join(root, approval.TestingPath))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, "", err
	}
	present, err := presentTests(root, globs)
	if err != nil {
		return nil, "", err
	}
	var added []string
	for _, f := range present {
		if _, ok := fp[f]; !ok {
			added = append(added, f)
		}
	}
	if added, _, err = unapprovedTests(root, p.ID, added); err != nil {
		return nil, "", err
	}
	return slices.Sorted(slices.Values(append(changed, added...))), commit, nil
}

// causeApproval returns the files an `approve cause` of bug p hashes: the plan, planPath, then every file the branch changes against base
// that is not a test file (TESTING.md test_files) and still exists, leaving
// out the event log, which the approval itself appends to. It also returns
// a summary: the repro test and the end of its recorded
// failing output (reproSummary), then the fix diff since the merge base.
func causeApproval(root, base, planPath string, p *unit.Plan) ([]string, string, error) {
	ref, err := baseRef(root, base, "approve cause", false)
	if err != nil {
		return nil, "", err
	}
	changed, err := gitx.BranchChanges(root, ref)
	if err != nil {
		return nil, "", err
	}
	globs, err := acmap.TestingGlobs(filepath.Join(root, approval.TestingPath))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, "", err
	}
	files, fix := []string{planPath}, []string{}
	for _, f := range changed {
		if acmap.MatchAny(globs, f) || f == events.Path || f == planPath {
			continue
		}
		fix = append(fix, f)
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); err == nil {
			files = append(files, f)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, "", err
		}
	}
	// The spec delta is folded in at the gate, as for a feat.
	for _, id := range p.ACs {
		file, err := approval.ACFile(root, id)
		if err != nil {
			return nil, "", err
		}
		if file == "" {
			return nil, "", out.Refuse("approve cause", fmt.Sprintf("the plan's acs: names %s, but no area file in lyceum/spec/ defines it", id),
				fmt.Sprintf("write the %s row into its area file and the registry range, commit, then run \"lyceum test\" so verify clears again", id))
		}
		if !slices.Contains(files, file) {
			files = append(files, file)
		}
	}
	summary, err := reproSummary(root, p.ID)
	if err != nil {
		return nil, "", err
	}
	mb, err := gitx.MergeBase(root, ref)
	if err != nil {
		return nil, "", err
	}
	diff := ""
	if len(fix) > 0 {
		if diff, err = gitx.Diff(root, mb, fix...); err != nil {
			return nil, "", err
		}
	}
	return files, summary + fmt.Sprintf("Fix diff (files that are not tests, since commit %s):\n%s", mb, diff), nil
}

// reproSummary is what an `approve cause` summary says about bug id's repro:
// the repro test its latest repro clear ran, or for a bug converted
// from a test unit the test files it fingerprinted, and the last lines of
// that run's output, read from the output file the inlined run record names
// and checked against its sha256; or the reason repro was skipped with
// --no-repro.
func reproSummary(root, id string) (string, error) {
	evs, err := events.Read(root)
	if err != nil {
		return "", err
	}
	var rec, fp map[string]any
	reason := ""
	for _, e := range evs {
		if g, _ := e.Fields["gate"].(string); e.Unit != id || g != string(gate.Repro) {
			continue
		}
		switch {
		case e.Type == "gate-clear":
			rec, _ = e.Fields["run"].(map[string]any)
			fp, _ = e.Fields["fingerprint"].(map[string]any)
			reason = ""
		case e.Type == "approve" && e.Fields["counts"] != false && e.Fields["no_repro"] == true:
			rec = nil
			reason, _ = e.Fields["reason"].(string)
		}
	}
	if rec == nil {
		if reason != "" {
			return "No repro test: repro was skipped with --no-repro. Reason: " + reason + "\n", nil
		}
		return "No repro test: the event log records no repro clear for " + id + "\n", nil
	}
	name, _ := rec["repro"].(string)
	if name == "" {
		// A bug converted from a test unit cleared repro on a suite run: its
		// repro is the fingerprinted test files.
		name = strings.Join(slices.Sorted(maps.Keys(fp)), ", ")
	}
	output, _ := rec["output"].(string)
	want, _ := rec["output_sha256"].(string)
	outPath := path.Join(runner.Dir, output)
	full := filepath.Join(root, filepath.FromSlash(outPath))
	s := "Repro test: " + name + "\n"
	h, err := approval.Hash(full)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return s + fmt.Sprintf("Its recorded output, %s, is not on this machine.\n", outPath), nil
	case err != nil:
		return "", err
	case h != want:
		return s + fmt.Sprintf("Its recorded output, %s, changed since the run: it no longer matches the sha256 the run recorded.\n", outPath), nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return s + fmt.Sprintf("Its failing output, last %d lines (%s):\n%s", tailLines, outPath, tail(data, tailLines)), nil
}

// testingPending reports whether TESTING.md exists and is unapproved or changed
// since its last counting testing approval. It is repo-wide: the unit an
// approval was given under does not matter.
func testingPending(root string) (bool, error) {
	approved, changed, err := testspec.Approved(root)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return !approved || changed, err
}

// projectPending reports whether the project gate waits for the owner:
// the repo is greenfield (brownfield finds no code), its area registry lists
// at least one area, and no counting project approval exists. Re-approving
// is allowed, but once approved the gate is never pending again.
func projectPending(root string) (bool, error) {
	areas, err := approval.RegisteredAreas(root)
	if err != nil || len(areas) == 0 {
		return false, err
	}
	if approved, err := projectApproved(root); err != nil || approved {
		return false, err
	}
	found, err := brownfield(root)
	return found == nil, err
}

// projectApproved reports whether the event log under root has a counting
// project approval.
func projectApproved(root string) (bool, error) {
	evs, err := events.Read(root)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(evs, func(e events.Event) bool {
		return e.Type == "approve" && e.Unit == "" && e.Fields["gate"] == string(approval.Project) && e.Fields["counts"] != false
	}), nil
}

// approveGate approves gate g, of p (nil for testing and project) of type typ, the checks
// common to every gate already passed, under cfg. At the code-opening gate it
// refuses unless the tree is clean outside lyceum/. It then shows what
// will be approved and requires the gate name typed back exactly on in,
// and records the approval. An approval by anyone but the owner is recorded
// but does not count, and is a refusal. With a reason, g is repro skipped
// by `approve cause --no-repro`, confirmed by typing cause: it hashes no file
// and the event records no_repro and the reason. Without
// one, cause refuses a dirty tree. With grant, the human confirmed in
// the harness's prompt: nothing is typed, and the event records via
// harness-prompt, the harness and its permission mode; otherwise via terminal.
// It returns the approved hashes.
func approveGate(cmd *cobra.Command, in *bufio.Reader, root string, cfg *config.Config, p *unit.Plan, typ gate.Type, g gate.Gate, reason string, grant *harness.Grant) (map[string]string, error) {
	owner := cfg.Owner
	what, run := "approve "+string(g), "lyceum approve "+string(g)
	if reason != "" {
		what, run = "approve cause --no-repro", "lyceum approve cause --no-repro '<reason>'"
	}
	unitID, planPath := "", ""
	switch g {
	case approval.Testing:
		if _, err := os.Stat(filepath.Join(root, approval.TestingPath)); errors.Is(err, fs.ErrNotExist) {
			return nil, out.Refuse(what, approval.TestingPath+" does not exist, so there is nothing to approve",
				"the agent drafts "+approval.TestingPath+`, then run "lyceum approve testing" yourself in a terminal`)
		} else if err != nil {
			return nil, err
		}
		// What lyceum test could not run is not approved.
		if _, err := testspec.Read(root); err != nil {
			return nil, out.Refuse(what, err.Error(), "fix "+approval.TestingPath+`, then run "lyceum approve testing" yourself in a terminal`)
		}
	case approval.Project:
	default:
		unitID, planPath = p.ID, path.Join(planDir, p.ID+".md")
	}

	// Before the summary, so the owner is not asked to type the gate name for
	// nothing (step 6).
	if opening, ok := gate.CodeOpening(typ); guard.ApprovedirtyEnabled && ok && opening == g {
		changed, err := gitx.Status(root)
		if err != nil {
			return nil, err
		}
		var outside []string
		for _, c := range changed {
			if !strings.HasPrefix(c, "lyceum/") {
				outside = append(outside, c)
			}
		}
		if len(outside) > 0 {
			return nil, dirtyRefusal(root, what, fmt.Sprintf("these files outside lyceum/ have changes made before %s was approved:\n  %s", g, strings.Join(outside, "\n  ")), outside,
				fmt.Sprintf("review them, then stash them: git stash -u, then run %s again", run),
				fmt.Sprintf(`or, if they belong in the branch, commit them on purpose: git add <paths> && git commit -m "<message>", then run %s again`, run))
		}
	}

	// Every approved fix file is committed, so it was in the suite run.
	if g == gate.Cause && reason == "" {
		changed, err := events.TreeChanges(root)
		if err != nil {
			return nil, err
		}
		if len(changed) > 0 {
			return nil, dirtyRefusal(root, what, "the working tree has uncommitted changes, and an approved fix must be committed and in the suite run:\n  "+strings.Join(changed, "\n  "), changed,
				fmt.Sprintf(`commit first: git add <paths> && git commit -m "<message>", then run "lyceum test" so verify clears again, then run %s again`, run))
		}
	}

	var files []string
	var since, cause string
	var err error
	switch {
	case reason != "":
		since = "Skipping repro with no repro test (--no-repro). Reason: " + reason + "\n"
	case g == gate.Cause:
		if files, cause, err = causeApproval(root, cfg.BaseBranch, planPath, p); err != nil {
			return nil, err
		}
	case g == approval.Tests:
		// The summary is each file's diff since the fingerprint, or at
		// a refactor's baseline since the base; a file that commit
		// lacks, untracked or added since, is shown in full.
		var commit string
		if files, commit, err = testsToApprove(root, cfg.BaseBranch, p, typ); err != nil {
			return nil, err
		}
		if since, err = testsSummary(root, commit, gate.Gate(p.Gate) == gate.Baseline, files); err != nil {
			return nil, err
		}
	default:
		if files, err = approval.FilesFor(root, typ, g, planPath, p); err != nil {
			return nil, err
		}
	}
	// The hashes are taken now, on the files as the owner sees them. Recording
	// then rewrites the plan's approvals and gate, which PlanHashBasis leaves
	// out, so the plan's hash stays the hash of the plan as approved.
	hashes, err := approval.Hashes(root, planPath, g, files)
	if err != nil {
		return nil, err
	}
	if g != approval.Tests && reason == "" {
		if since, err = sinceLastApproval(root, unitID, g, files); err != nil {
			return nil, err
		}
	}
	since += cause
	var summary strings.Builder
	if unitID != "" {
		fmt.Fprintf(&summary, "Unit: %s\n", unitID)
	}
	fmt.Fprintf(&summary, "Gate: %s\nFiles to approve:\n", g)
	for _, f := range files {
		fmt.Fprintf(&summary, "  %s %s\n", f, hashes[f])
	}
	if g == gate.Spec || g == approval.Project || g == gate.Cause {
		marked, err := provenanceLines(root, files)
		if err != nil {
			return nil, err
		}
		summary.WriteString(marked)
	}
	summary.WriteString(since)
	// --no-repro approves repro but is confirmed by typing cause.
	word := string(g)
	if reason != "" {
		word = string(gate.Cause)
	}
	if grant != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%sConfirmed in the %s prompt\n", summary.String(), grant.Harness)
	} else {
		if !typedConfirmation(cmd.ErrOrStderr(), in, fmt.Sprintf("%sType the gate name to approve (%s): ", summary.String(), word), word) {
			return nil, notConfirmed{out.Refuse(what, "confirmation did not match; nothing was approved",
				fmt.Sprintf(`run "%s" again and type %s exactly`, run, word))}
		}
	}

	var next gate.Gate
	if unitID != "" && g != approval.Tests {
		if next, err = gate.Next(typ, g); err != nil {
			return nil, err
		}
	}
	extra := map[string]any{"via": "terminal"}
	if grant != nil {
		extra = map[string]any{"via": "harness-prompt", "harness": grant.Harness}
		if grant.PermissionMode != "" {
			extra["permission_mode"] = grant.PermissionMode
		}
	}
	if reason != "" {
		extra["no_repro"] = true
		extra["reason"] = reason
	}
	by, counts, err := recordApproval(root, planPath, unitID, owner, cfg.Approvals.Signed, typ, g, next, files, hashes, extra)
	if err != nil {
		return nil, err
	}
	if !counts {
		return nil, out.Refuse(what, fmt.Sprintf("your approval was recorded, but it does not count: you are %s, and the configured owner is %s; gate %s is still open", by, owner, g),
			fmt.Sprintf(`the owner, %s, runs "%s" in their own terminal`, owner, run))
	}
	if asJSON, _ := cmd.Flags().GetBool("json"); !asJSON {
		if unitID != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Approved %s for %s\n", g, unitID)
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Approved %s\n", g)
		}
	}
	return hashes, nil
}

// provenanceLines is the part of an approval summary listing every line in
// files, relative to root, that carries a provenance marker, so the owner sees
// which text is the agent's. It is "" when there are none.
func provenanceLines(root string, files []string) (string, error) {
	var b strings.Builder
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			return "", err
		}
		n := 0
		for line := range strings.Lines(string(data)) {
			n++
			if strings.Contains(line, "[AGENT-PROPOSED") || strings.Contains(line, "[DERIVED") {
				fmt.Fprintf(&b, "  %s:%d: %s\n", f, n, strings.TrimSpace(line))
			}
		}
	}
	if b.Len() == 0 {
		return "", nil
	}
	return "Text the agent wrote and you did not state:\n" + b.String(), nil
}

// testsSummary is the last part of an `approve tests` summary of files: the
// diff since commit, the fingerprint's or at a refactor's baseline the
// base's, of the files commit has, then the full content of each it lacks.
func testsSummary(root, commit string, atBaseline bool, files []string) (string, error) {
	inCommit, err := gitx.TreeFiles(root, commit, ".")
	if err != nil {
		return "", err
	}
	var tracked, added []string
	for _, f := range files {
		if slices.Contains(inCommit, f) {
			tracked = append(tracked, f)
		} else {
			added = append(added, f)
		}
	}
	var b strings.Builder
	if len(tracked) > 0 {
		diff, err := gitx.Diff(root, commit, tracked...)
		if err != nil {
			return "", err
		}
		from := "the fingerprint"
		if atBaseline {
			from = "the base"
		}
		fmt.Fprintf(&b, "Changes since %s (commit %s):\n%s", from, commit, diff)
	}
	for _, f := range added {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "New test file %s, not in commit %s, in full:\n%s", f, commit, data)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}

// sinceLastApproval is the summary's last part: the diff of files since this
// gate's last counting approval of the unit, or a note that this is the first.
func sinceLastApproval(root, unitID string, g gate.Gate, files []string) (string, error) {
	evs, err := events.Read(root)
	if err != nil {
		return "", err
	}
	last := ""
	for _, e := range evs {
		if e.Type == "approve" && e.Unit == unitID && e.Fields["gate"] == string(g) && e.Fields["counts"] != false {
			last = e.Commit
		}
	}
	if last == "" {
		return "First approval of " + string(g) + ": no earlier approval to diff against.\n", nil
	}
	diff, err := gitx.Diff(root, last, files...)
	if err != nil {
		return "", err
	}
	if diff == "" {
		return fmt.Sprintf("No changes since the last %s approval (commit %s).\n", g, last), nil
	}
	return fmt.Sprintf("Changes since the last %s approval (commit %s):\n%s", g, last, diff), nil
}

// recordApproval writes the approval (enforcement.md, "lyceum approve [gate]",
// steps 7 and 8): one approve event, mirrored into the plan's approvals, the
// plan's gate advanced to next (left as it is when next is "", for tests),
// committed with the approved files. When
// clearing g moves the plan's status, the plan gets the new status and
// the event carries it. An approval by anyone but owner (git emails compared
// trimmed, case-insensitive) is written with counts false and leaves the gate
// and status where they are. With no unitID, for the repo-wide testing
// gate, there is no plan: only the event is written and committed with the
// approved files. extra are further event fields. It returns who approved and
// whether it counts. With signed, the commit is signed by the user's key, and
// a key git can't sign with leaves no event behind.
func recordApproval(root, planPath, unitID, owner string, signed bool, typ gate.Type, g, next gate.Gate, files []string, hashes map[string]string, extra map[string]any) (string, bool, error) {
	by, err := gitx.UserEmail(root)
	if err != nil {
		return "", false, err
	}
	commitWith := gitx.Commit
	if signed {
		commitWith = gitx.CommitSigned
		if err := gitx.CanSign(root); err != nil {
			return "", false, out.Refuse("approve "+string(g), "approvals.signed is on, and git can't sign a commit: "+err.Error(),
				`set up commit signing (user.signingkey and gpg.format), then run "lyceum approve `+string(g)+`" again`)
		}
	}
	counts := !guard.ApproveownerEnabled || strings.EqualFold(strings.TrimSpace(by), strings.TrimSpace(owner))
	commit, err := gitx.HeadSHA(root)
	if err != nil {
		return "", false, err
	}
	at := testhooks.Now().Truncate(time.Second)

	fields := map[string]any{
		"gate":   string(g),
		"hashes": hashes,
		"by":     by,
		"at":     at.Format(time.RFC3339),
		"parent": harness.ParentProcess(),
		"counts": counts,
	}
	maps.Copy(fields, extra)
	commitFiles := append(slices.Clone(files), events.Path)
	message := fmt.Sprintf("lyceum: approve %s", g)
	var p *unit.Plan
	full := filepath.Join(root, filepath.FromSlash(planPath))
	if unitID != "" {
		commitFiles = append(commitFiles, planPath)
		message += " " + unitID
		// Re-read the plan: it is written back below, and must keep any edit made
		// while the prompt waited. Such an edit is not covered by the hash.
		if p, err = unit.ReadPlan(full); err != nil {
			return "", false, err
		}
		// The plan is changed in memory before the event is written, so a plan the
		// CLI cannot rewrite leaves no event behind.
		a := unit.Approval{Gate: string(g), Hashes: hashes, Commit: commit, By: by, At: at}
		if !counts {
			a.Counts = &counts
		}
		if err := p.SetCLIField("approvals", append(slices.Clone(p.Approvals), a)); err != nil {
			return "", false, err
		}
		if counts && next != "" {
			if err := p.SetCLIField("gate", string(next)); err != nil {
				return "", false, err
			}
			if status := unit.StatusAfter(typ, g, p.Status); status != p.Status {
				if err := p.SetCLIField("status", status); err != nil {
					return "", false, err
				}
				fields["status"] = status
			}
		}
	}
	if _, err := events.Append(root, events.Event{Type: "approve", Unit: unitID, Fields: fields}); err != nil {
		return "", false, err
	}
	if p != nil {
		if err := unit.WritePlan(full, p); err != nil {
			return "", false, err
		}
	}
	return by, counts, commitWith(root, message, commitFiles...)
}

// gateNext is the next action for a unit that is at gate g of class c.
func gateNext(c gate.Class, g gate.Gate) string {
	switch c {
	case gate.Judgment:
		return fmt.Sprintf(`run "lyceum approve %s" yourself in a terminal`, g)
	case gate.Mechanical:
		if g == gate.Repro {
			return reproNext
		}
		return `run "lyceum test"`
	default:
		switch g {
		case gate.Close:
			return `run "lyceum close"`
		case gate.Diagnose, gate.Fix:
			return `write the root cause, commit the fix, then run "lyceum test": verify clears on a passing run`
		}
		return "finish the " + string(g) + " work described in the plan"
	}
}

// isTerminal reports whether v is a file open on a terminal.
func isTerminal(v any) bool {
	f, ok := v.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(f.Fd()))
}
