package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/check"
	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/harness"
	"github.com/oritromax/lyceum/internal/library"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/records"
	"github.com/oritromax/lyceum/internal/unit"
	"github.com/spf13/cobra"
)

// projectInterviewNext is the next step while the greenfield project interview
// has not written the areas.
const projectInterviewNext = `finish the project interview with your agent's lyceum-init skill, then run "lyceum approve project" yourself in a terminal`

// statusUnit is one unit as status shows it.
type statusUnit struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Branch      string  `json:"branch"`
	Status      string  `json:"status"`
	Gate        string  `json:"gate"`
	LastCleared *string `json:"last_cleared"`
	Next        string  `json:"next"`
	// Current marks the unit of the checked-out branch.
	Current bool `json:"current"`
	// Amended is the reason of the amend the current unit is back at its
	// gate for, until the owner approves the gate again.
	Amended string `json:"amended,omitempty"`
}

// repoGate is a repo-level gate as status shows it. A gate that does
// not apply, project on a brownfield repo or testing with no TESTING.md, has
// none.
type repoGate struct {
	Gate   string `json:"gate"`
	State  string `json:"state"`            // approved, pending or changed
	Commit string `json:"commit,omitempty"` // of the last counting approval
}

// The states of a repoGate.
const (
	repoApproved = "approved"
	repoPending  = "pending"
	repoChanged  = "changed"
)

// pendingApproval is a gate waiting for the owner's approval.
type pendingApproval struct {
	Gate string `json:"gate"`
	Unit string `json:"unit"` // "" for the repo-wide testing and project gates
	Next string `json:"next"`
}

// foundPlan is a plan status read, and the ref it was read from: "" for the checkout.
type foundPlan struct {
	plan *unit.Plan
	ref  string
}

// runStatus is `lyceum status` (commands.md, "lyceum status"). It lists every
// unit with a plan in the checkout or on another local or remote-tracking
// branch, closed ones only with --all, and the approvals pending: the current
// unit's judgment gate, project on a greenfield repo and TESTING.md,
// and the reason of an amend the current unit is back at its gate
// for, and every record and area file whose cites are not fresh.
// It writes nothing, except that
// the current unit, a bug at cause with a code commit after its verify
// clear, first goes back to fix (reopenBug). While a harness's hook
// could not read its last payload, status warns about it.
func runStatus(cmd *cobra.Command, args []string) error {
	what := "show status"
	if len(args) > 0 {
		return out.Refuse(what, "status takes no arguments", `run "lyceum status"`)
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
	reopened, err := reopenBug(root, p)
	if err != nil {
		return err
	}
	plans, err := allPlans(root, branch)
	if err != nil {
		return err
	}
	project, err := projectPending(root)
	if err != nil {
		return err
	}
	testing, err := testingPending(root)
	if err != nil {
		return err
	}
	gates, err := repoGates(root)
	if err != nil {
		return err
	}
	// Greenfield with no project approval and no area registered: the project
	// interview, which writes the areas, has not finished.
	interview := !project && slices.ContainsFunc(gates, func(g repoGate) bool {
		return g.Gate == string(approval.Project) && g.State == repoPending
	})
	warning, err := harness.ReadWarning(root)
	if err != nil {
		return err
	}
	entries, err := records.All(root)
	if err != nil {
		return err
	}
	stale, fresh := []records.Entry{}, 0
	for _, e := range entries {
		if e.State == records.Fresh {
			fresh++
		} else {
			stale = append(stale, e)
		}
	}

	all, _ := cmd.Flags().GetBool("all")
	units, pending := []statusUnit{}, []pendingApproval{}
	var current *statusUnit
	for _, f := range plans {
		p := f.plan
		if p.Status == unit.StatusClosed && !all {
			continue
		}
		u := statusUnit{ID: p.ID, Type: p.Type, Branch: p.Branch, Status: p.Status, Gate: p.Gate}
		// The checkout's first open plan on this branch is its unit, as openUnit finds it.
		u.Current = current == nil && f.ref == "" && branch != "" && p.Branch == branch && p.Status != unit.StatusClosed
		var class gate.Class
		u.LastCleared, u.Next, class = unitProgress(p)
		units = append(units, u)
		if u.Current {
			current = &units[len(units)-1]
			if current.Amended, err = amendReason(root, p.ID); err != nil {
				return err
			}
			if class == gate.Judgment {
				pending = append(pending, pendingApproval{Gate: p.Gate, Unit: p.ID, Next: u.Next})
			}
			if testing && reliesOnTesting(p) {
				current.Next = gateNext(gate.Judgment, approval.Testing)
			}
		}
	}
	if project {
		pending = append(pending, pendingApproval{Gate: string(approval.Project), Next: gateNext(gate.Judgment, approval.Project)})
	}
	if testing {
		pending = append(pending, pendingApproval{Gate: string(approval.Testing), Next: gateNext(gate.Judgment, approval.Testing)})
	}

	evs, err := events.Read(root)
	if err != nil {
		return err
	}
	drift, err := check.ApprovalIntegrity(newContext(root, branch, cfg, p, evs))
	if err != nil {
		return err
	}
	for _, f := range drift {
		if !slices.ContainsFunc(pending, func(a pendingApproval) bool { return a.Gate == f.Gate && a.Unit == f.Unit }) {
			pending = append(pending, pendingApproval{Gate: f.Gate, Unit: f.Unit, Next: f.Next})
		}
	}

	res := out.Result{Next: `start a unit: lyceum add feat "<title>"`}
	switch {
	case current != nil:
		res = out.Result{Unit: current.ID, Gate: current.Gate, Next: current.Next}
	case interview:
		res.Next = projectInterviewNext
	case project:
		res.Next = gateNext(gate.Judgment, approval.Project)
	case testing:
		res.Next = gateNext(gate.Judgment, approval.Testing)
	}
	// Nothing in flight and nothing pending: the roadmap's top entry.
	inFlight := slices.ContainsFunc(units, func(u statusUnit) bool { return u.Status != unit.StatusClosed })
	if current == nil && !inFlight && !interview && len(pending) == 0 {
		entries, err := library.Read(root)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			res.Next = entries[0].Command()
		}
	}
	res.Data = struct {
		Units     []statusUnit      `json:"units"`
		Pending   []pendingApproval `json:"pending"`
		RepoGates []repoGate        `json:"repo_gates"`
		// Records are the records and area files that are not fresh.
		Records      []records.Entry  `json:"records"`
		RecordsFresh int              `json:"records_fresh"`
		HookWarning  *harness.Warning `json:"hook_warning,omitempty"`
	}{units, pending, gates, stale, fresh, warning}

	asJSON, _ := cmd.Flags().GetBool("json")
	if !asJSON {
		if reopened != "" {
			_, _ = io.WriteString(cmd.OutOrStdout(), reopenedLine(p.ID, reopened))
		}
		_, _ = io.WriteString(cmd.OutOrStdout(), statusText(units, pending, all))
		_, _ = io.WriteString(cmd.OutOrStdout(), repoGatesText(gates, interview))
		_, _ = io.WriteString(cmd.OutOrStdout(), recordsText(stale, fresh))
		if warning != nil {
			_, _ = io.WriteString(cmd.OutOrStdout(), hookWarningText(warning))
		}
	}
	out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(res)
	return nil
}

// repoGates returns the state of the repo-level gates.
func repoGates(root string) ([]repoGate, error) {
	gates := []repoGate{}
	evs, err := events.Read(root)
	if err != nil {
		return nil, err
	}
	for _, g := range []gate.Gate{approval.Project, approval.Testing} {
		last := lastRepoApproval(evs, g)
		switch {
		case last != nil:
			changed, err := approvalChanged(root, evs, last)
			if err != nil {
				return nil, err
			}
			state := repoApproved
			if changed {
				state = repoChanged
			}
			gates = append(gates, repoGate{Gate: string(g), State: state, Commit: last.Commit})
		case g == approval.Project:
			found, err := brownfield(root)
			if err != nil {
				return nil, err
			}
			if found == nil {
				gates = append(gates, repoGate{Gate: string(g), State: repoPending})
			}
		default:
			_, err := os.Stat(filepath.Join(root, approval.TestingPath))
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			gates = append(gates, repoGate{Gate: string(g), State: repoPending})
		}
	}
	return gates, nil
}

// lastRepoApproval returns the last counting approval of repo-level gate g in
// evs, or nil.
func lastRepoApproval(evs []events.Event, g gate.Gate) *events.Event {
	for i := len(evs) - 1; i >= 0; i-- {
		e := &evs[i]
		if e.Type == "approve" && e.Unit == "" && e.Fields["gate"] == string(g) && e.Fields["counts"] != false {
			return e
		}
	}
	return nil
}

// approvalChanged reports whether a file e hashed differs now from the
// approval relied on for it (check.Covering), or is gone.
func approvalChanged(root string, evs []events.Event, e *events.Event) (bool, error) {
	counting := check.Counting(evs)
	hashes, _ := e.Fields["hashes"].(map[string]any)
	for f := range hashes {
		covering, ok := check.Covering(counting, f)
		if !ok {
			continue
		}
		relied, _ := covering.Fields["hashes"].(map[string]any)
		want := relied[f]
		got, err := approval.Hash(filepath.Join(root, filepath.FromSlash(f)))
		if errors.Is(err, fs.ErrNotExist) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if got != want {
			return true, nil
		}
	}
	return false, nil
}

// repoGatesText is the human form of the repo-level gates, or nothing when
// none applies.
func repoGatesText(gates []repoGate, interview bool) string {
	if len(gates) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Repo gates:\n")
	for _, g := range gates {
		switch {
		case g.State == repoApproved:
			fmt.Fprintf(&b, "  %s: approved at %s\n", g.Gate, g.Commit)
		case g.State == repoChanged:
			fmt.Fprintf(&b, "  %s: changed since approval at %s\n", g.Gate, g.Commit)
		case interview && g.Gate == string(approval.Project):
			fmt.Fprintf(&b, "  %s: pending, the project interview has not finished\n", g.Gate)
		default:
			fmt.Fprintf(&b, "  %s: pending\n", g.Gate)
		}
	}
	return b.String()
}

// reliesOnTesting reports whether p's next mechanical gate relies on
// TESTING.md: the next gate that does not clear by steps alone is
// mechanical.
func reliesOnTesting(p *unit.Plan) bool {
	typ, current, _, err := unitGate(p)
	if err != nil {
		return false
	}
	_, class, ok := gate.Pending(typ, current)
	return ok && class == gate.Mechanical
}

// recordsText is the human form of the records' freshness: a count by state,
// then each record or area file that is not fresh with what changed,
// or nothing when there are none.
func recordsText(stale []records.Entry, fresh int) string {
	if len(stale) == 0 && fresh == 0 {
		return ""
	}
	count := map[records.State]int{}
	for _, e := range stale {
		count[e.State]++
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Records: %d fresh, %d drifted, %d broken", fresh, count[records.Drifted], count[records.Broken])
	if n := count[records.Invalid]; n > 0 {
		fmt.Fprintf(&b, ", %d invalid", n)
	}
	b.WriteString("\n")
	for _, e := range stale {
		fmt.Fprintf(&b, "  %s %s: %s\n", e.State, e.Path, e.Detail())
	}
	return b.String()
}

// hookWarningText is the human form of a hook warning.
func hookWarningText(w *harness.Warning) string {
	return fmt.Sprintf("Warning: the %s hook could not read a tool call at %s and let it through: %s. "+
		"Until it reads one again, it may not be blocking anything; check that the harness and lyceum are up to date\n",
		w.Harness, w.At.UTC().Format(time.RFC3339), w.Error)
}

// amendReason is the reason of unit id's amend while the unit is still back
// at the amended gate (check.Amended), or "".
func amendReason(root, id string) (string, error) {
	evs, err := events.Read(root)
	if err != nil {
		return "", err
	}
	a, ok := check.Amended(evs, id)
	if !ok {
		return "", nil
	}
	reason, _ := a.Fields["reason"].(string)
	return reason, nil
}

// unitProgress returns the gate p last cleared (nil when none), its next action
// and the class of its current gate. Gates clear in list order, so the last
// cleared is the one before the current gate, or the current gate once the
// unit is closed.
func unitProgress(p *unit.Plan) (*string, string, gate.Class) {
	if p.Status == unit.StatusClosed {
		g := p.Gate
		return &g, "nothing: the unit is closed", gate.Step
	}
	typ, current, class, err := unitGate(p)
	if err != nil {
		return nil, `run "lyceum check" to see what is wrong with the plan`, gate.Step
	}
	gates, err := gate.Gates(typ)
	if err != nil {
		return nil, `run "lyceum check" to see what is wrong with the plan`, gate.Step
	}
	var last *string
	if i := slices.Index(gates, current); i > 0 {
		g := string(gates[i-1])
		last = &g
	}
	return last, gateNext(class, current), class
}

// allPlans returns the plans in the checkout and, read from git, those on
// every local and remote-tracking branch but the checked-out one, sorted by
// unit ID. A unit on several takes the checkout's plan, else its own branch's,
// local before remote-tracking, else the first found. It runs one ls-tree per
// branch and one cat-file for every plan.
func allPlans(root, branch string) ([]foundPlan, error) {
	byID := map[string]foundPlan{}
	names, err := planFiles(root)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		p, err := unit.ReadPlan(filepath.Join(root, filepath.FromSlash(planDir), name))
		if err != nil {
			return nil, err
		}
		if _, ok := byID[p.ID]; !ok {
			byID[p.ID] = foundPlan{plan: p}
		}
	}

	refs, err := gitx.Refs(root)
	if err != nil {
		return nil, err
	}
	type planAt struct{ ref, path, blob string }
	var onRefs []planAt
	var blobs []string
	for _, ref := range refs {
		if branch != "" && ref == "refs/heads/"+branch {
			continue // the checkout is this branch
		}
		entries, err := gitx.TreeBlobs(root, ref, planDir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Path, ".md") {
				onRefs = append(onRefs, planAt{ref, e.Path, e.Blob})
				blobs = append(blobs, e.Blob)
			}
		}
	}
	slices.Sort(blobs)
	contents, err := gitx.Blobs(root, slices.Compact(blobs))
	if err != nil {
		return nil, err
	}
	for _, at := range onRefs {
		p, err := unit.ParsePlan(contents[at.blob])
		if err != nil {
			return nil, fmt.Errorf("plan %s:%s: %w", strings.TrimPrefix(at.ref, "refs/"), at.path, err)
		}
		if old, ok := byID[p.ID]; ok && (old.ref == "" || ownBranch(old.ref, old.plan.Branch) || !ownBranch(at.ref, p.Branch)) {
			continue
		}
		byID[p.ID] = foundPlan{plan: p, ref: at.ref}
	}

	plans := make([]foundPlan, 0, len(byID))
	for _, f := range byID {
		plans = append(plans, f)
	}
	slices.SortFunc(plans, func(a, b foundPlan) int { return strings.Compare(a.plan.ID, b.plan.ID) })
	return plans, nil
}

// areaWarnings returns one warning per open unit, other than id, on another
// branch than branch, read by allPlans, that names one of areas. The
// warnings never block: plans that can't be read are a warning too. With no
// areas it reads nothing.
func areaWarnings(root, branch, id string, areas []string) []string {
	if len(areas) == 0 {
		return nil
	}
	plans, err := allPlans(root, branch)
	if err != nil {
		return []string{fmt.Sprintf("Warning: could not read the plans on other branches to compare areas: %v", err)}
	}
	var warnings []string
	for _, f := range plans {
		q := f.plan
		if q.ID == id || q.Status == unit.StatusClosed || q.Branch == branch {
			continue
		}
		var shared []string
		for _, a := range q.Areas {
			if slices.Contains(areas, a) {
				shared = append(shared, a)
			}
		}
		if len(shared) > 0 {
			warnings = append(warnings, fmt.Sprintf("Warning: unit %s on branch %s is open and names area %s too; changes to one area on two branches may conflict",
				q.ID, q.Branch, strings.Join(shared, ", ")))
		}
	}
	return warnings
}

// ownBranch reports whether ref is branch itself or a remote-tracking copy of it.
func ownBranch(ref, branch string) bool {
	return ref == "refs/heads/"+branch || strings.HasPrefix(ref, "refs/remotes/") && strings.HasSuffix(ref, "/"+branch)
}

// statusText is the human form of status's units and pending approvals.
func statusText(units []statusUnit, pending []pendingApproval, all bool) string {
	var b strings.Builder
	switch {
	case len(units) == 0 && all:
		b.WriteString("No units\n")
	case len(units) == 0:
		b.WriteString("No open units\n")
	default:
		tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, " \tUNIT\tTYPE\tBRANCH\tSTATUS\tGATE\tLAST CLEARED\tNEXT")
		for _, u := range units {
			mark, last := " ", "-"
			if u.Current {
				mark = "*"
			}
			if u.LastCleared != nil {
				last = *u.LastCleared
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", mark, u.ID, u.Type, u.Branch, u.Status, u.Gate, last, u.Next)
		}
		_ = tw.Flush()
		for _, u := range units {
			if u.Amended != "" {
				fmt.Fprintf(&b, "Amended %s, back at %s: %s\n", u.ID, u.Gate, u.Amended)
			}
		}
	}
	if len(pending) == 0 {
		b.WriteString("Pending approvals: none\n")
		return b.String()
	}
	b.WriteString("Pending approvals:\n")
	for _, a := range pending {
		on := approval.TestingPath
		switch {
		case a.Unit != "":
			on = a.Unit
		case a.Gate == string(approval.Project):
			on = approval.SpecPath
		}
		fmt.Fprintf(&b, "  %s for %s: %s\n", a.Gate, on, a.Next)
	}
	return b.String()
}
