package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/oritromax/lyceum/internal/check"
	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/testhooks"
	"github.com/oritromax/lyceum/internal/unit"
	"github.com/spf13/cobra"
)

// amendForm is how amend is run.
const amendForm = `lyceum amend "<reason>"`

// runAmend is `lyceum amend [unit] "<reason>"` (units.md, "Amend"). It reopens the approved plan of the branch's open unit; a unit
// argument must name it. It refuses without a reason, on a dirty tree, when
// the plan's CLI-owned fields disagree with the event log, on a bug (a new
// cause is a new unit), on a doc or test unit (no judgment gate), on a
// trivial chore (nothing approved) and when the code-opening gate has no
// counting approval yet. Otherwise it invalidates that approval and every
// counting approval of the unit after it (check.Invalidates), moves the
// gate back to the code-opening gate with status planned, writes an amend
// event, appends the reason to the plan's ## Log and commits the plan with
// the event. check then fails code changed after the amend until the owner
// approves the gate again (AC35). With --intent, a refactor goes back to
// intent instead of plan, invalidating intent and every later approval;
// --intent on any other type refuses (AC131).
func runAmend(cmd *cobra.Command, args []string) error {
	what := "amend the plan"
	if len(args) > 2 {
		return out.Refuse(what, "amend takes at most a unit and one reason", fmt.Sprintf(`quote the reason: lyceum amend "%s"`, strings.Join(args, " ")))
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
	evs, err := events.Read(root)
	if err != nil {
		return err
	}
	p, err := openUnit(root, branch)
	if err != nil {
		return err
	}
	if p == nil {
		if closed, err := closedUnit(root, branch, evs); err != nil {
			return err
		} else if closed != nil {
			return out.Refuse(what, fmt.Sprintf("unit %s is closed, so its plan can no longer change", closed.ID),
				`start a new unit for the change: lyceum add feat "<title>"`)
		}
		return out.Refuse(what, "this branch has no open unit, so there is no plan to amend",
			"check out the branch of the unit to amend, then run "+amendForm)
	}
	reason := ""
	switch {
	case len(args) == 2 && args[0] != p.ID:
		return out.Refuse(what, fmt.Sprintf("unit %s is not the open unit on this branch, which is %s", args[0], p.ID),
			fmt.Sprintf(`check out the branch of unit %s, then run lyceum amend %s "<reason>"`, args[0], args[0]),
			fmt.Sprintf(`or amend this branch's unit: lyceum amend %s "<reason>"`, p.ID))
	case len(args) == 2:
		reason = args[1]
	case len(args) == 1 && args[0] != p.ID:
		reason = args[0]
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return out.Refuse(what, "no reason given; amend records why the approved plan changes",
			"run "+amendForm)
	}
	what = "amend " + p.ID
	changed, err := events.TreeChanges(root)
	if err != nil {
		return err
	}
	if len(changed) > 0 {
		return dirtyRefusal(root, what, "the working tree has uncommitted changes:\n  "+strings.Join(changed, "\n  "), changed,
			`commit them: git add -A && git commit -m "<message>", then run lyceum amend again`,
			"or stash them: git stash -u, then run lyceum amend again")
	}
	ctx, _, err := checkContext(root, branch, cfg, p, evs, what, false)
	if err != nil {
		return err
	}
	findings, err := check.CLIFields(ctx)
	if err != nil {
		return err
	}
	if r := failingRefusal(what, findings); r != nil {
		return r
	}

	typ, _, class, err := unitGate(p)
	if err != nil {
		return out.Refuse(what, fmt.Sprintf("the plan of unit %s has type %s and gate %s, which do not go together", p.ID, p.Type, p.Gate),
			`run "lyceum check" to see what changed in the plan`)
	}
	opening, ok := gate.CodeOpening(typ)
	toIntent, _ := cmd.Flags().GetBool("intent")
	switch {
	case toIntent && typ != gate.Refactor:
		return out.Refuse(what, fmt.Sprintf("--intent is only for a refactor, whose intent is approved apart from its plan; %s is a %s", p.ID, p.Type),
			"run "+amendForm)
	case typ == gate.Bug:
		return out.Refuse(what, "a bug has no plan to amend: a different cause is a different bug",
			`start a unit for the new cause: lyceum add bug "<title>"`)
	case trivialChore(p):
		return out.Refuse(what, fmt.Sprintf("chore %s is trivial, so it has no approved plan to amend", p.ID),
			`run "lyceum plan" to move it to its planned tier, where the owner approves a plan`)
	case !ok:
		return out.Refuse(what, fmt.Sprintf("a %s unit has no judgment gate, so it has no approved plan to amend", p.Type),
			gateNext(class, gate.Gate(p.Gate)))
	}
	// A refactor goes back to plan, keeping intent, or with --intent to intent.
	back := opening
	if toIntent {
		back = gate.Intent
	}
	counting := check.Counting(evs)
	idx := check.Invalidates(counting, p.ID, back)
	if len(idx) == 0 {
		return out.Refuse(what, fmt.Sprintf("unit %s has no approval of its %s gate yet, so there is nothing to amend", p.ID, back),
			fmt.Sprintf("change the plan in %s; the owner approves it at %s", path.Join(planDir, p.ID+".md"), back),
			`or run "lyceum status" to see where the unit is`)
	}
	var invalidated []string
	for _, i := range idx {
		if g, _ := counting[i].Fields["gate"].(string); !slices.Contains(invalidated, g) {
			invalidated = append(invalidated, g)
		}
	}

	// The plan is changed in memory before the event is written, so a plan the
	// CLI cannot rewrite leaves no event behind.
	planPath := path.Join(planDir, p.ID+".md")
	if err := p.SetCLIField("gate", string(back)); err != nil {
		return err
	}
	if err := p.SetCLIField("status", unit.StatusPlanned); err != nil {
		return err
	}
	p.AppendLog(fmt.Sprintf("- %s: amended: %s", testhooks.Now().Format("2006-01-02"), reason))
	if _, err := events.Append(root, events.Event{Type: "amend", Unit: p.ID, Fields: map[string]any{
		"gate": string(back), "reason": reason, "invalidated": invalidated, "status": unit.StatusPlanned,
	}}); err != nil {
		return err
	}
	if err := unit.WritePlan(filepath.Join(root, filepath.FromSlash(planPath)), p); err != nil {
		return err
	}
	if err := gitx.Commit(root, "lyceum: amend "+p.ID, planPath, events.Path); err != nil {
		return err
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	if !asJSON {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Amended %s: %s\nInvalidated: %s\nBack at %s, status %s; code changed from now on fails check until the owner approves %s again\n",
			p.ID, reason, strings.Join(invalidated, ", "), back, unit.StatusPlanned, opening)
	}
	out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(out.Result{
		Unit: p.ID,
		Gate: string(back),
		Next: fmt.Sprintf("change the plan for the amend in %s, then %s", planPath, gateNext(gate.Judgment, back)),
		Data: struct {
			Plan        string   `json:"plan"`
			Reason      string   `json:"reason"`
			Invalidated []string `json:"invalidated"`
		}{planPath, reason, invalidated},
	})
	return nil
}
