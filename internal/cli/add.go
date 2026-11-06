package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/unit"
	"github.com/spf13/cobra"
)

// planDir holds the plan files, relative to the repo root, in git's path form.
const planDir = "lyceum/plan"

// addWhat is what a refused add could not do.
const addWhat = "add a unit"

// runAddBare is `lyceum add` with no type, or an unknown one. Until the skills
// bring the choice prompt (AC15), it refuses and names the types.
func runAddBare(cmd *cobra.Command, args []string) error {
	reason := "no unit type given"
	if asJSON, _ := cmd.Flags().GetBool("json"); len(args) == 0 && !asJSON && isTerminal(cmd.InOrStdin()) && isTerminal(cmd.OutOrStdout()) {
		in := bufio.NewReader(cmd.InOrStdin())
		_, _ = io.WriteString(cmd.ErrOrStderr(), "Add which type? feat, bug, doc or test: ")
		typ, _ := in.ReadString('\n')
		typ = strings.TrimSpace(typ)
		for _, sub := range cmd.Commands() {
			if sub.Name() == typ {
				_, _ = io.WriteString(cmd.ErrOrStderr(), "Title: ")
				title, _ := in.ReadString('\n')
				if title = strings.TrimSpace(title); title == "" {
					return runAdd(sub, nil)
				}
				return runAdd(sub, []string{title})
			}
		}
		reason = fmt.Sprintf("unknown unit type %q", typ)
	}
	if len(args) > 0 {
		reason = fmt.Sprintf("unknown unit type %q", args[0])
	}
	return out.Refuse(addWhat, reason+"; the types are feat, bug, doc and test",
		`run lyceum add feat "<title>" (or bug, doc, test)`)
}

// runAdd is `lyceum add <type> "<title>"`, the CLI half of starting a unit
// (architecture.md, "IDs and branches"; units.md, "The plan file"). It refuses
// outside an initialised repo, on a dirty tree and when this branch
// already has an open unit. Otherwise it cuts the unit branch from the
// base branch, writes the plan, appends an add event and commits both. It
// warns when an open unit on another branch names an area the plan names
// (areaWarnings).
// `lyceum refactor` and `lyceum chore` start their units the same way.
func runAdd(cmd *cobra.Command, args []string) error {
	typ := cmd.Name()
	// usage is the command that starts this type; starter names it in "run ... again".
	usage, starter := cmd.CommandPath(), cmd.CommandPath()
	if cmd.Parent().Name() == "add" {
		starter = cmd.Parent().CommandPath()
	}
	if len(args) == 0 {
		return out.Refuse(addWhat, "no title given", fmt.Sprintf(`run %s "<title>"`, usage))
	}
	if len(args) > 1 {
		return out.Refuse(addWhat, "the title must be one argument",
			fmt.Sprintf(`quote the title: %s "%s"`, usage, strings.Join(args, " ")))
	}
	title := args[0]
	slug, err := unit.Slug(title)
	if err != nil {
		return out.Refuse(addWhat, err.Error(), fmt.Sprintf(`run %s "<title>" with a title that has letters or digits`, usage))
	}

	if err := gitx.CheckVersion(); err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	if !gitx.IsRepo(dir) {
		return out.Refuse(addWhat, "this is not a git repository, so Lyceum is not set up here", `run "lyceum init"`)
	}
	root, err := gitx.Root(dir)
	if err != nil {
		return err
	}
	cfg, err := config.Read(root)
	if errors.Is(err, fs.ErrNotExist) {
		return out.Refuse(addWhat, "Lyceum is not set up in this repo: "+config.Path+" is missing", `run "lyceum init"`)
	}
	if err != nil {
		return err
	}

	changed, err := events.TreeChanges(root)
	if err != nil {
		return err
	}
	if len(changed) > 0 {
		return dirtyRefusal(root, addWhat, "the working tree has uncommitted changes:\n  "+strings.Join(changed, "\n  "), changed,
			`commit them: git add -A && git commit -m "<message>", then run `+starter+` again`,
			"or stash them: git stash -u, then run "+starter+" again")
	}

	current, err := gitx.Branch(root)
	if err != nil {
		return err
	}
	if p, err := openUnit(root, current); err != nil {
		return err
	} else if p != nil {
		return out.Refuse(addWhat,
			fmt.Sprintf("unit %s is open on this branch (status %s), and a checkout holds one unit at a time", p.ID, p.Status),
			fmt.Sprintf("finish %s first, or check out another branch and run %s there", p.ID, starter))
	}

	base := cfg.BaseBranch
	if base == "" {
		if base, err = gitx.DefaultBranch(root); err != nil {
			return err
		}
	}
	if base == "" {
		return out.Refuse(addWhat, "HEAD is detached and "+config.Path+" sets no base_branch",
			"check out the branch units start from, or set base_branch in "+config.Path)
	}
	if ok, err := gitx.BranchExists(root, base); err != nil {
		return err
	} else if !ok {
		return out.Refuse(addWhat, fmt.Sprintf("the base branch %q does not exist here", base),
			fmt.Sprintf("create branch %s, or set base_branch in %s to the branch units start from", base, config.Path))
	}

	taken, err := planHashes(root, base)
	if err != nil {
		return err
	}
	prefix, err := unit.BranchPrefix(typ)
	if err != nil {
		return err
	}
	hash := unit.NewHash()
	for taken[hash] {
		hash = unit.NewHash()
	}
	id := unit.ID(hash, slug)
	branch := prefix + id
	g, err := gate.Initial(gate.Type(typ))
	if err != nil {
		return err
	}
	status := unit.InitialStatus(gate.Type(typ))
	plan, err := unit.NewPlan(id, typ, title, branch, status, string(g))
	if err != nil {
		return err
	}

	if err := gitx.SwitchNew(root, branch, base); err != nil {
		return err
	}
	planPath := path.Join(planDir, id+".md")
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(planDir)), 0o755); err != nil {
		return err
	}
	if err := unit.WritePlan(filepath.Join(root, filepath.FromSlash(planPath)), plan); err != nil {
		return err
	}
	if _, err := events.Append(root, events.Event{Type: "add", Unit: id, Fields: map[string]any{
		"unit_type": typ, "branch": branch, "gate": string(g), "status": status,
	}}); err != nil {
		return err
	}
	if err := gitx.Commit(root, "lyceum: add "+typ+" "+id, planPath, events.Path); err != nil {
		return err
	}

	// A new plan names no area yet, so this warns only once a plan can.
	for _, w := range areaWarnings(root, branch, id, plan.Areas) {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), w)
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	if !asJSON {
		_, _ = io.WriteString(cmd.OutOrStdout(),
			"Created "+planPath+"\nCreated branch "+branch+" from "+base+"\n")
	}
	out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(out.Result{
		Unit: id,
		Gate: string(g),
		Next: "draft the plan in " + planPath,
		Data: struct {
			Plan   string `json:"plan"`
			Branch string `json:"branch"`
			Base   string `json:"base"`
		}{planPath, branch, base},
	})
	return nil
}

// openUnit returns the plan in the checkout whose branch is branch and whose
// status is not closed, or nil when there is none.
func openUnit(root, branch string) (*unit.Plan, error) {
	names, err := planFiles(root)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		p, err := unit.ReadPlan(filepath.Join(root, filepath.FromSlash(planDir), name))
		if err != nil {
			return nil, err
		}
		if p.Branch == branch && p.Status != unit.StatusClosed {
			return p, nil
		}
	}
	return nil, nil
}

// planHashes returns the hashes already in use (AC7): the part before the first
// "-" of each lyceum/plan/<hash>-* file in the checkout, on base and on each
// remote-tracking branch, and of the last path element of each local and
// remote-tracking branch, since a unit's plan may live only on its own branch,
// and that branch only on a remote.
func planHashes(root, base string) (map[string]bool, error) {
	names, err := planFiles(root)
	if err != nil {
		return nil, err
	}
	onBase, err := gitx.TreeFiles(root, base, planDir)
	if err != nil {
		return nil, err
	}
	branches, err := gitx.Branches(root)
	if err != nil {
		return nil, err
	}
	refs, err := gitx.Refs(root)
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		if !strings.HasPrefix(ref, "refs/remotes/") {
			continue
		}
		onRemote, err := gitx.TreeFiles(root, ref, planDir)
		if err != nil {
			return nil, err
		}
		branches = append(append(branches, ref), onRemote...)
	}
	for _, p := range slices.Concat(onBase, branches) {
		names = append(names, path.Base(p))
	}
	taken := make(map[string]bool, len(names))
	for _, name := range names {
		if hash, _, ok := strings.Cut(name, "-"); ok {
			taken[hash] = true
		}
	}
	return taken, nil
}

// planFiles returns the names of the .md files in the checkout's plan directory.
func planFiles(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(planDir)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
