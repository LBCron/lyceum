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

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/check"
	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/unit"
	"github.com/spf13/cobra"
)

// choreUpgraded is the reason of the gate-reopen event `lyceum plan` writes
// when it moves a trivial chore to plan.
const choreUpgraded = "chore upgraded to planned"

// runPlan is `lyceum plan [--new-area <name>]` (units.md, "Plan command"). With
// --new-area it registers an area (newArea). Otherwise it shows the current
// unit's plan, first moving a trivial chore to plan (upgradeChore).
func runPlan(cmd *cobra.Command, args []string) error {
	what := "show the plan"
	if cmd.Flags().Changed("new-area") {
		what = "add an area"
	}
	if len(args) > 0 {
		return out.Refuse(what, "plan takes no arguments", `run "lyceum plan"`, "or register an area: lyceum plan --new-area <name>")
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
	if _, err := config.Read(root); errors.Is(err, fs.ErrNotExist) {
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
	asJSON, _ := cmd.Flags().GetBool("json")
	w := out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}
	if cmd.Flags().Changed("new-area") {
		name, _ := cmd.Flags().GetString("new-area")
		return newArea(cmd, w, root, name, p)
	}

	if p == nil {
		return out.Refuse(what, "this branch has no open unit, so there is no plan to show",
			"check out the branch of the unit", `or start one: lyceum add feat "<title>"`)
	}
	planPath := path.Join(planDir, p.ID+".md")
	if trivialChore(p) {
		if err := upgradeChore(root, planPath, p); err != nil {
			return err
		}
		if !asJSON {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Moved %s to %s: %s\n", p.ID, gate.Plan, choreUpgraded)
		}
	}
	fields, err := p.Fields()
	if err != nil {
		return err
	}
	if !asJSON {
		var b strings.Builder
		fmt.Fprintf(&b, "Plan: %s\nType: %s\nStatus: %s\n", planPath, p.Type, p.Status)
		fmt.Fprintf(&b, "Areas: %s\nFiles: %s\nACs: %s\n", listOrNone(p.Areas), listOrNone(p.Files), listOrNone(p.ACs))
		b.Write(p.Body)
		if !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n")
		}
		_, _ = io.WriteString(cmd.OutOrStdout(), b.String())
	}
	_, next, _ := unitProgress(p)
	w.Result(out.Result{Unit: p.ID, Gate: p.Gate, Next: next, Data: struct {
		Plan        string         `json:"plan"`
		Frontmatter map[string]any `json:"frontmatter"`
	}{planPath, fields}})
	return nil
}

// trivialChore reports whether p is a chore on its trivial tier, at build.
func trivialChore(p *unit.Plan) bool {
	return unit.Tier(p.Type, p.Gate, p.Approvals) == gate.Chore && p.Gate == string(gate.Build)
}

// upgradeChore moves trivial chore p, whose plan is planPath, to plan with
// status planned, writing a gate-reopen event, and commits the plan
// and the event log. From then on it is a planned chore: check requires the
// plan approval before code (AC128).
func upgradeChore(root, planPath string, p *unit.Plan) error {
	// The plan is changed in memory before the event is written, so a plan the
	// CLI cannot rewrite leaves no event behind.
	if err := p.SetCLIField("gate", string(gate.Plan)); err != nil {
		return err
	}
	if err := p.SetCLIField("status", unit.StatusPlanned); err != nil {
		return err
	}
	if _, err := events.Append(root, events.Event{Type: "gate-reopen", Unit: p.ID,
		Fields: map[string]any{"gate": string(gate.Plan), "status": unit.StatusPlanned, "reason": choreUpgraded}}); err != nil {
		return err
	}
	if err := unit.WritePlan(filepath.Join(root, filepath.FromSlash(planPath)), p); err != nil {
		return err
	}
	return gitx.Commit(root, fmt.Sprintf("lyceum: plan %s %s", gate.Plan, p.ID), planPath, events.Path)
}

// newArea is `lyceum plan --new-area <name>`: it prints the registered
// areas, then adds a row for name to the area registry, creates
// lyceum/spec/<name>.md with a heading and commits both. A file that exists
// with no registry row is adopted, unchanged. A name already
// registered is refused. The registry is part of the
// project approval, so when one counts, it says the owner re-approves.
// p is the open unit, if any, for the ending.
func newArea(cmd *cobra.Command, w out.Writer, root, name string, p *unit.Plan) error {
	what := "add an area"
	if got, err := unit.Slug(name); err != nil || got != name {
		return out.Refuse(what, fmt.Sprintf("area name %q must be lowercase letters, digits and single dashes, at most %d characters", name, unit.MaxSlug),
			"run lyceum plan --new-area <name> with a name such as orders or user-auth")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(approval.SpecPath))); errors.Is(err, fs.ErrNotExist) {
		return out.Refuse(what, approval.SpecPath+" is missing, so there is no area registry", `run "lyceum init", which writes it`)
	} else if err != nil {
		return err
	}
	files, err := approval.RegisteredAreas(root)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, strings.TrimSuffix(path.Base(f), ".md"))
	}
	if !w.JSON {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Registered areas: %s\n", listOrNone(names))
	}
	file := path.Join("lyceum/spec", name+".md")
	if slices.Contains(files, file) {
		return out.Refuse(what, fmt.Sprintf("area %s is already registered in %s", name, approval.SpecPath),
			fmt.Sprintf("name %s in areas: in the plan", name), "or register another name: lyceum plan --new-area <name>")
	}
	full := filepath.Join(root, filepath.FromSlash(file))
	adopted := true
	if _, err := os.Stat(full); errors.Is(err, fs.ErrNotExist) {
		adopted = false
	} else if err != nil {
		return err
	}

	if err := approval.RegisterArea(root, name); err != nil {
		return err
	}
	if !adopted {
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte("# "+name+"\n"), 0o644); err != nil {
			return err
		}
	}
	if err := gitx.Commit(root, "lyceum: new area "+name, approval.SpecPath, file); err != nil {
		return err
	}
	reapprove, err := projectApproved(root)
	if err != nil {
		return err
	}
	if !w.JSON {
		verb := "Created"
		if adopted {
			verb = "Adopted"
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Registered area %s in %s\n%s %s\n", name, approval.SpecPath, verb, file)
		if reapprove {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s changed since the project approval: check reports it until %s\n", approval.SpecPath, check.ReapproveStep(p, []string{approval.SpecPath}, approval.SpecPath))
		}
	}
	res := out.Result{Next: "write the spec of area " + name + " in " + file}
	if adopted {
		res.Next = "name " + name + " in areas: in the plan"
	}
	if p != nil {
		res.Unit, res.Gate = p.ID, p.Gate
	}
	res.Data = struct {
		Area             string   `json:"area"`
		File             string   `json:"file"`
		Registered       []string `json:"registered"`
		ReapproveProject bool     `json:"reapprove_project"`
	}{name, file, names, reapprove}
	w.Result(res)
	return nil
}

// listOrNone joins items with ", ", or is "none" when there are none.
func listOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}
