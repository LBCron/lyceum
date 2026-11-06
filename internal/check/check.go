// Package check holds the checks `lyceum check` runs (enforcement.md,
// "lyceum check"). Each check is independent: it reads the repo through a
// Context and reports what it finds.
package check

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oritromax/lyceum/internal/acmap"
	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/codepath"
	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/guard"
	"github.com/oritromax/lyceum/internal/library"
	"github.com/oritromax/lyceum/internal/records"
	"github.com/oritromax/lyceum/internal/testspec"
	"github.com/oritromax/lyceum/internal/unit"
)

// Severity says whether a finding fails the check in CI or only warns.
type Severity string

// Severities.
const (
	Fail Severity = "fail"
	Warn Severity = "warn"
)

// Finding is one thing a check found wrong.
type Finding struct {
	Check    string   `json:"check"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Unit     string   `json:"unit"` // "" for a repo-level finding
	Gate     string   `json:"-"`    // the approval an integrity finding names
	Path     string   `json:"path,omitempty"`
	Field    string   `json:"field,omitempty"`
	Next     string   `json:"next"` // how to fix it
}

// Context is what a check reads.
type Context struct {
	Root     string
	Branch   string     // the current git branch
	Plan     *unit.Plan // the branch's open unit; nil when there is none
	PlanPath string     // Plan's file, relative to Root in git's path form
	Events   []events.Event
	// Changed are the paths the branch changes against the base branch,
	// committed or not (gitx.BranchChanges), relative to Root in git's path form.
	// It is empty on the base branch itself, which has no branch diff.
	Changed []string
	// HarnessPrompt is the config's approvals.harness_prompt.
	HarnessPrompt string
	// Owner is the configured owner's git email; Signed is approvals.signed.
	Owner  string
	Signed bool
}

// Check is one independent check.
type Check func(Context) ([]Finding, error)

// All lists the checks `lyceum check` runs, in order.
var All = []Check{ApprovalIntegrity, CLIFields, GateBeforeCode, Scope, Chore, NoUnit, TestingDrift, Reopen, StaleEvidence, Registry, Unclaimed, CauseApproval, SignedApprovals, HarnessPrompt, Records}

// Run runs every check in All and returns their findings, in order.
func Run(ctx Context) ([]Finding, error) {
	findings := []Finding{}
	for _, c := range All {
		f, err := c(ctx)
		if err != nil {
			return nil, err
		}
		findings = append(findings, f...)
	}
	return findings, nil
}

// IntegrityCheck names the approval integrity check in findings.
const IntegrityCheck = "approval-integrity"

// ApprovalIntegrity re-hashes the files of every approval still relied on and
// reports each one that changed or is gone since. The files come from
// the latest counting approval of each of the unit's gates and the latest
// counting repo-level testing and project approvals. Per file, the
// approval relied on is the latest counting one whose hashes include it,
// whichever gate or unit wrote it (Covering), so a unit's approve spec
// takes over from project for the files it hashed. When TestingDrift reports
// TESTING.md, the testing approval is left to it, so the change is reported
// once.
func ApprovalIntegrity(ctx Context) ([]Finding, error) {
	if !guard.CheckintegrityEnabled {
		return nil, nil
	}
	drift, err := TestingDrift(ctx)
	if err != nil {
		return nil, err
	}
	counting := Counting(ctx.Events)
	type key struct{ unit, gate string }
	latest := map[key]events.Event{}
	var order []key
	for _, e := range counting {
		g, _ := e.Fields["gate"].(string)
		if e.Type != "approve" || e.Fields["counts"] == false {
			continue
		}
		ofUnit := ctx.Plan != nil && e.Unit == ctx.Plan.ID
		repo := e.Unit == "" && (g == string(approval.Testing) || g == string(approval.Project))
		if !ofUnit && !repo {
			continue
		}
		if g == string(approval.Testing) && len(drift) > 0 {
			continue
		}
		k := key{e.Unit, g}
		if _, ok := latest[k]; !ok {
			order = append(order, k)
		}
		latest[k] = e
	}

	var findings []Finding
	seen := map[string]bool{}
	for _, k := range order {
		hashes, _ := latest[k].Fields["hashes"].(map[string]any)
		files := make([]string, 0, len(hashes))
		for f := range hashes {
			files = append(files, f)
		}
		slices.Sort(files)
		for _, f := range files {
			if seen[f] {
				continue
			}
			seen[f] = true
			e, ok := Covering(counting, f)
			if !ok {
				continue
			}
			g, _ := e.Fields["gate"].(string)
			want, _ := e.Fields["hashes"].(map[string]any)
			planPath := ""
			if ctx.Plan != nil && e.Unit == ctx.Plan.ID {
				planPath = ctx.PlanPath
			}
			got, err := approval.Hashes(ctx.Root, planPath, gate.Gate(g), []string{f})
			var problem string
			switch {
			case errors.Is(err, fs.ErrNotExist):
				problem = "the file no longer exists"
			case err != nil:
				return nil, err
			case got[f] != want[f]:
				problem = "it changed after it was approved"
			default:
				continue
			}
			next := fmt.Sprintf("restore %s to what the %s approval covered (git log -p -- %s shows the edits)", f, g, f)
			if k.unit == "" && k.gate == string(approval.Project) && (ctx.Plan == nil || e.Unit != ctx.Plan.ID) {
				next = fmt.Sprintf(`%s, or restore %s (git log -p -- %s shows the edits)`, ReapproveStep(ctx.Plan, ctx.Changed, f), f, f)
			}
			findings = append(findings, Finding{
				Check:    IntegrityCheck,
				Severity: Fail,
				Message:  fmt.Sprintf("the %s approval no longer matches %s: %s", g, f, problem),
				Unit:     e.Unit,
				Gate:     g,
				Path:     f,
				Next:     next,
			})
		}
	}
	return findings, nil
}

// ReapproveStep is how the owner re-approves file, changed outside the unit's
// own approvals: through the gate of p, the open unit, that will hash it,
// or the project gate when none will. changed is the branch diff. A feat's spec
// hashes its plan's area files and the registry, and it reopens through amend
// once passed; a bug's cause hashes the branch diff, when the bug gets there.
func ReapproveStep(p *unit.Plan, changed []string, file string) string {
	step := `the owner runs "lyceum approve %s" in a terminal`
	if p == nil || p.Status == unit.StatusClosed {
		return fmt.Sprintf(step, approval.Project)
	}
	switch p.Type {
	case string(gate.Feat):
		if file != approval.SpecPath && !slices.ContainsFunc(p.Areas, func(a string) bool { return "lyceum/spec/"+a+".md" == file }) {
			break
		}
		if p.Gate != string(gate.Spec) {
			step = `reopen the spec: lyceum amend "<reason>", then ` + step
		}
		return fmt.Sprintf(step, gate.Spec)
	case string(gate.Bug):
		if !slices.Contains(changed, file) {
			break
		}
		if p.Gate != string(gate.Cause) {
			step = `once the bug reaches cause, ` + step
		}
		return fmt.Sprintf(step, gate.Cause)
	}
	return fmt.Sprintf(step, approval.Project)
}

// Covering returns the approval relied on for file: the latest counting
// approve event in evs, as Counting returns them, whose hashes include it.
func Covering(evs []events.Event, file string) (events.Event, bool) {
	for _, e := range slices.Backward(evs) {
		if e.Type != "approve" || e.Fields["counts"] == false {
			continue
		}
		if hashes, _ := e.Fields["hashes"].(map[string]any); hashes != nil {
			if _, ok := hashes[file]; ok {
				return e, true
			}
		}
	}
	return events.Event{}, false
}

// StaleCheck names the stale evidence check in findings.
const StaleCheck = "stale-evidence"

// StaleEvidence reports code committed after an open unit's verify cleared:
// the run no longer describes HEAD. A passing lyceum test at HEAD
// writes a new clear. Bugs keep the Reopen rule.
func StaleEvidence(ctx Context) ([]Finding, error) {
	p := ctx.Plan
	if p == nil || p.Type == string(gate.Bug) || p.Status == unit.StatusClosed || p.Gate != string(gate.Close) {
		return nil, nil
	}
	since := ""
	for _, e := range ctx.Events {
		if g, _ := e.Fields["gate"].(string); e.Type == "gate-clear" && e.Unit == p.ID && g == string(gate.Verify) {
			since = e.Commit
		}
	}
	if since == "" {
		return nil, nil
	}
	committed, err := gitx.CommittedSince(ctx.Root, since)
	if err != nil {
		return nil, err
	}
	code := codeIn(committed)
	if len(code) == 0 {
		return nil, nil
	}
	return []Finding{{
		Check:    StaleCheck,
		Severity: Fail,
		Message:  fmt.Sprintf("a commit after verify cleared at %s changes code, so that run no longer describes HEAD: %s", since, strings.Join(code, ", ")),
		Unit:     p.ID,
		Next:     `run "lyceum test" so a passing run at HEAD clears verify again`,
	}}, nil
}

// CauseCheck names the check that a verified bug has its cause approved.
const CauseCheck = "cause"

// CauseApproval fails an open bug whose verify cleared with no counting
// approve cause (AC102): the fix is proven, and its cause is not yet the owner's.
// A bug sent back to fix, or about to be (Reopen says so), has to clear verify first.
func CauseApproval(ctx Context) ([]Finding, error) {
	if ctx.Plan == nil || ctx.Plan.Type != string(gate.Bug) || ctx.Plan.Status == unit.StatusClosed ||
		ctx.Plan.Gate != string(gate.Cause) && ctx.Plan.Gate != string(gate.Close) {
		return nil, nil
	}
	if reason, err := Reopened(ctx.Root, ctx.Plan, ctx.Events); err != nil || reason != "" {
		return nil, err
	}
	verified, approved := false, false
	for _, e := range Counting(ctx.Events) {
		if e.Unit != ctx.Plan.ID || e.Fields["counts"] == false {
			continue
		}
		g, _ := e.Fields["gate"].(string)
		verified = verified || e.Type == "gate-clear" && g == string(gate.Verify)
		approved = approved || e.Type == "approve" && g == string(gate.Cause)
	}
	if !verified || approved {
		return nil, nil
	}
	return []Finding{{Check: CauseCheck, Severity: Fail,
		Message: fmt.Sprintf("bug %s has verify cleared and no approve cause event", ctx.Plan.ID),
		Next:    `run "lyceum approve cause" yourself in a terminal`}}, nil
}

// SignedCheck names the signed-approval check in findings.
const SignedCheck = "signed-approvals"

// SignedApprovals, with approvals.signed, fails each latest approval of the
// branch's unit and of the repo gates whose event line came in a commit without
// a good signature by the owner's key (AC51). The line is traced through
// git, so an event added by hand can't borrow a signed commit. A line not yet
// committed has no signature to check.
func SignedApprovals(ctx Context) ([]Finding, error) {
	if !ctx.Signed {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(ctx.Root, filepath.FromSlash(events.Path)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	type key struct{ unit, gate string }
	latest := map[key]string{}
	var order []key
	for line := range strings.Lines(string(data)) {
		var e events.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, err
		}
		g, _ := e.Fields["gate"].(string)
		ofUnit := ctx.Plan != nil && e.Unit == ctx.Plan.ID
		repo := e.Unit == "" && (g == string(approval.Testing) || g == string(approval.Project))
		if e.Type != "approve" || e.Fields["counts"] == false || !ofUnit && !repo {
			continue
		}
		k := key{e.Unit, g}
		if _, ok := latest[k]; !ok {
			order = append(order, k)
		}
		latest[k] = strings.TrimSuffix(line, "\n")
	}
	var findings []Finding
	for _, k := range order {
		commit, err := gitx.IntroducedBy(ctx.Root, events.Path, latest[k])
		if err != nil {
			return nil, err
		}
		if commit == "" {
			continue
		}
		status, signer, err := gitx.Signature(ctx.Root, commit)
		if err != nil {
			return nil, err
		}
		owner := strings.ToLower(strings.TrimSpace(ctx.Owner))
		signer = strings.ToLower(strings.TrimSpace(signer))
		if status == "G" && (signer == owner || strings.HasSuffix(signer, "<"+owner+">")) {
			continue
		}
		findings = append(findings, Finding{Check: SignedCheck, Severity: Fail, Path: events.Path,
			Message: fmt.Sprintf("the %s approval is in commit %s, which has no good signature by the owner's key (%s)", k.gate, commit, ctx.Owner),
			Next:    fmt.Sprintf(`check the owner's key is trusted here; otherwise the owner approves again with signing set up: run "lyceum approve %s" yourself in a terminal`, k.gate)})
	}
	return findings, nil
}

// UnclaimedCheck names the unclaimed AC check in findings.
const UnclaimedCheck = "unclaimed-acs"

// Unclaimed warns, never fails, about every AC in lyceum/spec/ that no closed
// plan's acs: lists and no lyceum/LIBRARY.md entry names. It catches
// forgotten work: acs_partial: is no claim, and neither is an open plan.
func Unclaimed(ctx Context) ([]Finding, error) {
	ids, err := acmap.OpenSpecACs(filepath.Join(ctx.Root, "lyceum", "spec"))
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	claimed := map[string]bool{}
	plans, err := filepath.Glob(filepath.Join(ctx.Root, "lyceum", "plan", "*.md"))
	if err != nil {
		return nil, err
	}
	for _, f := range plans {
		p, err := unit.ReadPlan(f)
		if err != nil {
			continue // a plan that does not read claims nothing; never a failure here
		}
		if p.Status == unit.StatusClosed {
			for _, id := range p.ACs {
				claimed[id] = true
			}
		}
	}
	entries, err := library.Read(ctx.Root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		for _, id := range e.ACs {
			claimed[id] = true
		}
	}
	ids = slices.DeleteFunc(ids, func(id string) bool { return claimed[id] })
	if len(ids) == 0 {
		return nil, nil
	}
	return []Finding{{Check: UnclaimedCheck, Severity: Warn,
		Message: "no closed plan lists these ACs and no " + library.Path + " entry names them: " + strings.Join(ids, ", "),
		Next:    "name them in an entry of " + library.Path + ", or claim them in a unit's acs:"}}, nil
}

// RegistryCheck names the area registry check in findings.
const RegistryCheck = "registry"

var acRange = regexp.MustCompile(`AC([0-9]+)(?:\s*[-–]\s*AC([0-9]+))?`)

// Registry reports every area file in lyceum/spec/ with no registry row, and
// every AC defined in an area file that its row's ACs cell omits.
func Registry(ctx Context) ([]Finding, error) {
	rows, err := approval.RegistryRows(ctx.Root)
	if err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(ctx.Root, "lyceum", "spec", "*.md"))
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	var findings []Finding
	for _, full := range files {
		f := "lyceum/spec/" + filepath.Base(full)
		i := slices.IndexFunc(rows, func(r approval.RegistryRow) bool { return r.File == f })
		if i < 0 {
			findings = append(findings, Finding{Check: RegistryCheck, Severity: Fail, Path: f,
				Message: fmt.Sprintf("%s has no row in the area registry of %s", f, approval.SpecPath),
				Next:    fmt.Sprintf("register it: lyceum plan --new-area %s", strings.TrimSuffix(filepath.Base(full), ".md"))})
			continue
		}
		if !rows[i].HasACs {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		var missing []string
		for _, def := range acmap.Definitions(data) {
			if !inRanges(rows[i].ACs, strings.TrimPrefix(def.ID, "AC")) {
				missing = append(missing, def.ID)
			}
		}
		if len(missing) > 0 {
			findings = append(findings, Finding{Check: RegistryCheck, Severity: Fail, Path: f,
				Message: fmt.Sprintf("the registry row of %s omits ACs it defines: %s", f, strings.Join(missing, ", ")),
				Next:    fmt.Sprintf("add them to the row of %s in %s", f, approval.SpecPath)})
		}
	}
	return findings, nil
}

// inRanges reports whether AC number id falls in cell, such as "AC1–AC4, AC9".
func inRanges(cell, id string) bool {
	n, err := strconv.Atoi(id)
	if err != nil {
		return false
	}
	for _, m := range acRange.FindAllStringSubmatch(cell, -1) {
		lo, _ := strconv.Atoi(m[1])
		hi := lo
		if m[2] != "" {
			hi, _ = strconv.Atoi(m[2])
		}
		if lo <= n && n <= hi {
			return true
		}
	}
	return false
}

// FieldsCheck names the CLI-owned field check in findings.
const FieldsCheck = "cli-fields"

// expected is what the event log implies for one unit's CLI-owned plan fields.
type expected struct {
	typ, branch, status, gate string
	approvals                 []unit.Approval
}

// replay rebuilds the CLI-owned plan fields of unit id from evs: the
// first add event sets type, branch, gate and status; each later approve event
// of the unit adds an approvals entry, and a counting one moves the gate past
// the gate it approved, except tests, which moves no gate; a gate-clear event moves the gate past the
// mechanical gate it cleared, when the unit was at that gate or at the work
// step before it (clearedTo), and is skipped otherwise; any later event
// carrying status sets it. A gate-reopen event moves a bug at cause
// back to fix, or a chore at build to plan, and sets its status, and is
// skipped otherwise. An amend event moves the unit
// back to the code-opening gate it names, or a refactor to intent,
// and sets its status;
// the approvals it invalidated stay in approvals, which mirror the approve
// events (Counting). A close event moves the gate to
// close, where a trivial chore at build and a converted test unit end too.
// The run a gate-clear event inlines is not
// checked against the run records. ok is false when no add event records the unit.
func replay(evs []events.Event, id string) (exp expected, ok bool, err error) {
	for _, e := range evs {
		if e.Unit != id {
			continue
		}
		if e.Type == "add" {
			if !ok {
				ok = true
				exp.typ, _ = e.Fields["unit_type"].(string)
				exp.branch, _ = e.Fields["branch"].(string)
				exp.gate, _ = e.Fields["gate"].(string)
				exp.status, _ = e.Fields["status"].(string)
			}
			continue
		}
		if !ok {
			continue
		}
		if e.Type == "gate-clear" {
			next, follows := clearedTo(exp, e)
			if !follows {
				// A clear the log does not lead to moves nothing, so a plan
				// advanced by it disagrees with the log.
				continue
			}
			exp.gate = next
		}
		if e.Type == "gate-reopen" {
			// The reopens there are: a bug back from cause to fix,
			// and `lyceum plan` moving a trivial chore from build to plan.
			g, _ := e.Fields["gate"].(string)
			bug := exp.typ == string(gate.Bug) && exp.gate == string(gate.Cause) && g == string(gate.Fix)
			chore := unit.Tier(exp.typ, exp.gate, exp.approvals) == gate.Chore && exp.gate == string(gate.Build) && g == string(gate.Plan)
			if bug || chore {
				exp.gate = g
				if s, has := e.Fields["status"].(string); has {
					exp.status = s
				}
			}
			continue
		}
		if e.Type == "amend" {
			g, _ := e.Fields["gate"].(string)
			typ := unit.Tier(exp.typ, exp.gate, exp.approvals)
			opening, ok := gate.CodeOpening(typ)
			if ok && typ != gate.Bug && (g == string(opening) || typ == gate.Refactor && g == string(gate.Intent)) {
				exp.gate = g
				if s, has := e.Fields["status"].(string); has {
					exp.status = s
				}
			}
			continue
		}
		if e.Type == "close" {
			exp.gate = string(gate.Close)
		}
		if s, has := e.Fields["status"].(string); has {
			exp.status = s
		}
		if e.Type != "approve" {
			continue
		}
		a, err := approvalOf(e)
		if err != nil {
			return expected{}, false, err
		}
		exp.approvals = append(exp.approvals, a)
		if a.Counts != nil || a.Gate == string(approval.Tests) {
			continue
		}
		typ := unit.Tier(exp.typ, a.Gate, exp.approvals)
		next, err := gate.Next(typ, gate.Gate(a.Gate))
		if err != nil {
			return expected{}, false, fmt.Errorf("%s: approve event of unit %s: %w", events.Path, id, err)
		}
		exp.gate = string(next)
	}
	return exp, ok, nil
}

// clearedTo returns the gate a unit at exp moves to when gate-clear event e
// clears its gate: the gate after it. follows is false unless that gate is
// mechanical and exp is at it, or at work steps it ends, and, when e
// records no_tests, unless it clears a doc's verify (noTestsForged).
func clearedTo(exp expected, e events.Event) (next string, follows bool) {
	if noTestsForged(exp.typ, e) {
		return "", false
	}
	g, _ := e.Fields["gate"].(string)
	typ, cleared, current := unit.Tier(exp.typ, exp.gate, exp.approvals), gate.Gate(g), gate.Gate(exp.gate)
	if class, err := gate.Kind(typ, cleared); err != nil || class != gate.Mechanical {
		return "", false
	}
	if pending, _, ok := gate.Pending(typ, current); !ok || pending != cleared {
		return "", false
	}
	n, err := gate.Next(typ, cleared)
	if err != nil {
		return "", false
	}
	return string(n), true
}

// noTestsForged reports whether gate-clear event e, of a unit of type typ,
// records no_tests: true for anything but a doc unit's verify, the one gate
// that clears with no run.
func noTestsForged(typ string, e events.Event) bool {
	g, _ := e.Fields["gate"].(string)
	return e.Fields["no_tests"] == true && (typ != string(gate.Doc) || g != string(gate.Verify))
}

// approvalOf is the plan approvals entry approve event e mirrors.
func approvalOf(e events.Event) (unit.Approval, error) {
	a := unit.Approval{Commit: e.Commit, Hashes: map[string]string{}}
	a.Gate, _ = e.Fields["gate"].(string)
	a.By, _ = e.Fields["by"].(string)
	switch h := e.Fields["hashes"].(type) {
	case map[string]any:
		for f, v := range h {
			a.Hashes[f], _ = v.(string)
		}
	case map[string]string:
		maps.Copy(a.Hashes, h)
	}
	if at, _ := e.Fields["at"].(string); at != "" {
		t, err := time.Parse(time.RFC3339, at)
		if err != nil {
			return unit.Approval{}, fmt.Errorf("%s: approve event of unit %s: %w", events.Path, e.Unit, err)
		}
		a.At = t
	}
	if e.Fields["counts"] == false {
		counts := false
		a.Counts = &counts
	}
	return a, nil
}

// BranchUnit returns the unit an add event places on branch and the event log
// does not show closed, or "" when there is none.
func BranchUnit(evs []events.Event, branch string) (string, error) {
	id := ""
	for _, e := range evs {
		if b, _ := e.Fields["branch"].(string); e.Type != "add" || b != branch {
			continue
		}
		exp, _, err := replay(evs, e.Unit)
		if err != nil {
			return "", err
		}
		if exp.branch == branch && exp.status != unit.StatusClosed {
			id = e.Unit
		}
	}
	return id, nil
}

// CLIFields compares the plan's CLI-owned fields with what the event log
// implies and reports each that differs, naming the field (AC17). A
// plan no add event records is a finding on id, and so is a branch an add
// event claims for a unit whose plan is not the branch's plan, and so is a
// gate-clear event recording no_tests anywhere but a doc's verify, which has
// no run as evidence (noTestsForged). title is set at
// add but the agent may edit it, so it is not checked.
func CLIFields(ctx Context) ([]Finding, error) {
	if !guard.CheckfieldsEnabled {
		return nil, nil
	}
	var findings []Finding
	claimed, err := BranchUnit(ctx.Events, ctx.Branch)
	if err != nil {
		return nil, err
	}
	if claimed != "" && (ctx.Plan == nil || ctx.Plan.ID != claimed) {
		planPath := "lyceum/plan/" + claimed + ".md"
		findings = append(findings, Finding{
			Check:    FieldsCheck,
			Severity: Fail,
			Message:  fmt.Sprintf("the add event of unit %s puts it on branch %s, but no plan on this branch is unit %s", claimed, ctx.Branch, claimed),
			Unit:     claimed,
			Path:     planPath,
			Field:    "branch",
			Next:     fmt.Sprintf("restore %s as the CLI wrote it (git log -p -- %s shows the edits)", planPath, planPath),
		})
	}
	if ctx.Plan == nil {
		return findings, nil
	}

	p := ctx.Plan
	exp, ok, err := replay(ctx.Events, p.ID)
	if err != nil {
		return nil, err
	}
	finding := func(field, message, next string) {
		findings = append(findings, Finding{
			Check: FieldsCheck, Severity: Fail, Message: message,
			Unit: p.ID, Path: ctx.PlanPath, Field: field, Next: next,
		})
	}
	if !ok {
		finding("id", fmt.Sprintf("plan field id is %s, but no add event records this unit", p.ID),
			fmt.Sprintf(`start units with "lyceum add", which records the add event; if id: in %s was edited by hand, restore it`, ctx.PlanPath))
		return findings, nil
	}
	for _, f := range []struct{ field, got, want string }{
		{"type", p.Type, exp.typ},
		{"branch", p.Branch, exp.branch},
		{"status", p.Status, exp.status},
		{"gate", p.Gate, exp.gate},
	} {
		if f.got != f.want {
			finding(f.field, fmt.Sprintf("plan field %s is %s, but the event log implies %s", f.field, f.got, f.want),
				fmt.Sprintf("set %s: back to %s in %s; only the CLI writes it", f.field, f.want, ctx.PlanPath))
		}
	}
	if msg := approvalsDiff(p.Approvals, exp.approvals); msg != "" {
		finding("approvals", "plan field approvals "+msg,
			fmt.Sprintf("restore approvals: in %s to mirror the approve events in %s (git log -p -- %s shows the edits)", ctx.PlanPath, events.Path, ctx.PlanPath))
	}
	for _, e := range ctx.Events {
		if g, _ := e.Fields["gate"].(string); e.Type == "gate-clear" && e.Unit == p.ID && noTestsForged(exp.typ, e) {
			finding("gate", fmt.Sprintf("the gate-clear event of %s records no_tests, but only a doc unit's verify clears with no tests: the clear has no run as evidence", g),
				fmt.Sprintf(`write tests and run "lyceum test", so a passing run clears %s`, g))
		}
	}
	return findings, nil
}

// GateCheck names the gate-before-code check in findings.
const GateCheck = "gate-before-code"

// GateBeforeCode reports a branch that changes code while the unit's
// code-opening gate is not cleared: spec for feat, repro for bug,
// plan for refactor and planned chore. A judgment gate clears by a counting
// approval. Bug's repro is mechanical: it is cleared once the event log
// moves the bug past it (replay), by a gate-clear event or by
// `approve cause --no-repro`. Test files (TESTING.md test_files; none
// without a TESTING.md) are allowed while a bug is at repro or a refactor at
// baseline, and at plan, which keeps the ones written during baseline.
// Code is codepath.IsCode. Once an amend moved the unit back to its
// code-opening gate (Amended), only the paths changed since the amend count:
// the code built before it was built under an approval (AC35).
func GateBeforeCode(ctx Context) ([]Finding, error) {
	if !guard.CheckgateEnabled {
		return nil, nil
	}
	paths, problem := ctx.Changed, "the branch changes code before the %s gate is cleared: %s"
	if ctx.Plan != nil {
		if a, ok := Amended(ctx.Events, ctx.Plan.ID); ok {
			since, err := gitx.BranchChanges(ctx.Root, a.Commit)
			if err != nil {
				return nil, err
			}
			paths, problem = since, "the branch changes code since the amend, before the %s gate is approved again: %s"
		}
	}
	opening, code, err := CodeBeforeGate(ctx, paths)
	if err != nil || len(code) == 0 {
		return nil, err
	}
	next := fmt.Sprintf(`the owner runs "lyceum approve %s" in a terminal to clear the gate, or take the code changes off this branch`, opening)
	if opening == gate.Repro {
		next = "take the code changes off this branch, keeping only test files, until the repro gate clears"
	}
	return []Finding{{
		Check:    GateCheck,
		Severity: Fail,
		Message:  fmt.Sprintf(problem, opening, strings.Join(code, ", ")),
		Unit:     ctx.Plan.ID,
		Next:     next,
	}}, nil
}

// CodeBeforeGate returns the paths among paths, relative to ctx.Root in git's
// path form, that GateBeforeCode's rule blocks for ctx.Plan, in order, and
// the code-opening gate that blocks them. It returns none when there is no
// unit or its code-opening gate is cleared. The pre-tool hook decides each
// write with it.
func CodeBeforeGate(ctx Context, paths []string) (gate.Gate, []string, error) {
	p := ctx.Plan
	if p == nil {
		return "", nil, nil
	}
	typ, current, err := unitGate(ctx)
	if err != nil {
		return "", nil, err
	}
	opening, ok := gate.CodeOpening(typ)
	if !ok || cleared(ctx.Events, p.ID, opening) || typ == gate.Bug && current != gate.Repro {
		return "", nil, nil
	}
	var globs []string
	if typ == gate.Bug && current == gate.Repro || typ == gate.Refactor && (current == gate.Baseline || current == gate.Plan) {
		globs, err = acmap.TestingGlobs(filepath.Join(ctx.Root, approval.TestingPath))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", nil, err
		}
	}
	var code []string
	for _, f := range paths {
		if codepath.IsCode(f) && !acmap.MatchAny(globs, f) {
			code = append(code, f)
		}
	}
	return opening, code, nil
}

// unitGate is the type and gate of ctx.Plan the event log implies, or the
// plan's own when no add event records it. A chore on its planned tier has type
// PlannedChore (unit.Tier).
func unitGate(ctx Context) (gate.Type, gate.Gate, error) {
	p := ctx.Plan
	typ, current := unit.Tier(p.Type, p.Gate, p.Approvals), gate.Gate(p.Gate)
	exp, ok, err := replay(ctx.Events, p.ID)
	if err != nil {
		return "", "", err
	}
	if ok {
		typ, current = unit.Tier(exp.typ, exp.gate, exp.approvals), gate.Gate(exp.gate)
	}
	return typ, current, nil
}

// cleared reports whether evs hold a counting approval of gate g of unit id
// that no amend invalidated (Counting).
func cleared(evs []events.Event, id string, g gate.Gate) bool {
	for _, e := range Counting(evs) {
		if gg, _ := e.Fields["gate"].(string); e.Type == "approve" && e.Unit == id && gg == string(g) && e.Fields["counts"] != false {
			return true
		}
	}
	return false
}

// ScopeCheck names the scope check in findings.
const ScopeCheck = "scope"

// Scope warns about changed code outside the plan's files: list.
// An entry ending in "/" covers every path under it; any other entry covers
// that path only. Test files (TESTING.md test_files; none without TESTING.md)
// are not code here. It is a warning, never a failure. It does not run
// for a trivial chore, which Chore covers.
func Scope(ctx Context) ([]Finding, error) {
	p := ctx.Plan
	if p == nil {
		return nil, nil
	}
	if trivial, err := trivialChore(ctx); err != nil || trivial {
		return nil, err
	}
	globs, err := acmap.TestingGlobs(filepath.Join(ctx.Root, approval.TestingPath))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var outside []string
	for _, f := range ctx.Changed {
		if codepath.IsCode(f) && !acmap.MatchAny(globs, f) && !slices.ContainsFunc(p.Files, func(e string) bool {
			return e == f || strings.HasSuffix(e, "/") && strings.HasPrefix(f, e)
		}) {
			outside = append(outside, f)
		}
	}
	if len(outside) == 0 {
		return nil, nil
	}
	return []Finding{{
		Check:    ScopeCheck,
		Severity: Warn,
		Message:  "the branch changes code outside the plan's files: list: " + strings.Join(outside, ", "),
		Unit:     p.ID,
		Path:     ctx.PlanPath,
		Field:    "files",
		Next:     fmt.Sprintf("add them to files: in %s; the owner approves it at the next stop", ctx.PlanPath),
	}}, nil
}

// ChoreCheck names the chore check in findings.
const ChoreCheck = "chore"

// Chore reports a chore that changes code with no approved plan: a
// chore on its trivial tier with no counting plan approval. Docs and text
// only chores pass. A chore with a counting plan approval follows the normal
// gate rules, and one on its planned tier before that is GateBeforeCode's.
// Code is codepath.IsCode, which counts manifests, lockfiles and CI config.
func Chore(ctx Context) ([]Finding, error) {
	if !guard.CheckchoreEnabled {
		return nil, nil
	}
	p := ctx.Plan
	if p == nil {
		return nil, nil
	}
	if trivial, err := trivialChore(ctx); err != nil || !trivial {
		return nil, err
	}
	code := codeIn(ctx.Changed)
	if len(code) == 0 {
		return nil, nil
	}
	return []Finding{{
		Check:    ChoreCheck,
		Severity: Fail,
		Message:  "the chore changes code, a manifest, a lockfile or CI config without an approved plan: " + strings.Join(code, ", "),
		Unit:     p.ID,
		Next:     `move the chore to its planned tier with "lyceum plan" and have the owner approve the plan, or take those changes off this branch`,
	}}, nil
}

// trivialChore reports whether ctx.Plan is a chore on its trivial tier.
func trivialChore(ctx Context) (bool, error) {
	typ, _, err := unitGate(ctx)
	if err != nil {
		return false, err
	}
	return typ == gate.Chore, nil
}

// NoUnitCheck names the no-unit check in findings.
const NoUnitCheck = "no-unit"

// NoUnit reports a branch with no unit whose diff changes code. Docs
// and text only branches pass, and so does the base branch, which has no
// branch diff. Code is codepath.IsCode.
func NoUnit(ctx Context) ([]Finding, error) {
	if !guard.ChecknounitEnabled {
		return nil, nil
	}
	if ctx.Plan != nil {
		return nil, nil
	}
	code := codeIn(ctx.Changed)
	if len(code) == 0 {
		return nil, nil
	}
	return []Finding{{
		Check:    NoUnitCheck,
		Severity: Fail,
		Message:  "the branch has no unit and changes code, a manifest, a lockfile or CI config: " + strings.Join(code, ", "),
		Next:     `start a unit with "lyceum add" and make the changes on its branch, or take them off this branch`,
	}}, nil
}

// TestingCheck names the TESTING.md check in findings.
const TestingCheck = "testing"

// TestingDrift reports TESTING.md changed, or gone, since its last counting
// approval while a mechanical gate of the unit relies on it: the unit's
// current gate is mechanical, or is a work step a mechanical gate ends. With
// no counting approval there is nothing it changed from.
func TestingDrift(ctx Context) ([]Finding, error) {
	p := ctx.Plan
	if p == nil {
		return nil, nil
	}
	approved := testspec.ApprovedHash(ctx.Events)
	if approved == "" {
		return nil, nil
	}
	typ, current, err := unitGate(ctx)
	if err != nil {
		return nil, err
	}
	// A gate the type lacks is CLIFields' finding.
	relying, class, ok := gate.Pending(typ, current)
	if !ok || class != gate.Mechanical {
		return nil, nil
	}
	problem := "changed since its last approval"
	h, err := approval.Hash(filepath.Join(ctx.Root, testspec.Path))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		problem = "no longer exists"
	case err != nil:
		return nil, err
	case h == approved:
		return nil, nil
	}
	return []Finding{{
		Check:    TestingCheck,
		Severity: Fail,
		Message:  fmt.Sprintf("%s %s, and the %s gate relies on it", testspec.Path, problem, relying),
		Unit:     p.ID,
		Path:     testspec.Path,
		Next:     fmt.Sprintf(`the owner runs "lyceum approve testing" in a terminal, or restore %s (git log -p -- %s shows the edits)`, testspec.Path, testspec.Path),
	}}, nil
}

// ReopenCheck names the reopen check in findings.
const ReopenCheck = "reopen"

// Reopen reports a bug at cause with a code commit after its verify clear
// (Reopened), which sends it back to fix. Locally the CLI
// writes the gate-reopen first, so only --ci finds it.
func Reopen(ctx Context) ([]Finding, error) {
	reason, err := Reopened(ctx.Root, ctx.Plan, ctx.Events)
	if err != nil || reason == "" {
		return nil, err
	}
	return []Finding{{
		Check:    ReopenCheck,
		Severity: Fail,
		Message:  reason + "; verify must clear again before approve cause",
		Unit:     ctx.Plan.ID,
		Next:     `run "lyceum test" locally so verify clears again`,
	}}, nil
}

// HarnessPromptCheck names the harness-prompt check in findings.
const HarnessPromptCheck = "harness-prompt"

// HarnessPrompt reports every counting approval of the branch's unit given
// through a harness prompt (via harness-prompt) when approvals.harness_prompt
// is deny. Approvals of other units stay as they were.
func HarnessPrompt(ctx Context) ([]Finding, error) {
	if ctx.HarnessPrompt != config.HarnessPromptDeny || ctx.Plan == nil {
		return nil, nil
	}
	var findings []Finding
	for _, e := range Counting(ctx.Events) {
		if e.Type != "approve" || e.Unit != ctx.Plan.ID || e.Fields["counts"] == false || e.Fields["via"] != "harness-prompt" {
			continue
		}
		h, _ := e.Fields["harness"].(string)
		findings = append(findings, Finding{
			Check:    HarnessPromptCheck,
			Severity: Fail,
			Message:  fmt.Sprintf("the %v approval of %s was given through the %s harness prompt, and approvals.harness_prompt is deny", e.Fields["gate"], e.Unit, h),
			Unit:     e.Unit,
			Next:     "approvals given through a harness prompt are turned off here; to accept them, set approvals.harness_prompt: allow in " + config.Path,
		})
	}
	return findings, nil
}

// Counting returns evs with each approve event an amend event invalidated
// marked counts: false, so that it no longer counts, as an approval by anyone
// but the owner does not. An amend event invalidates, of the
// gates its invalidated field lists, the approvals Invalidates finds before
// it. evs itself is left as it is.
func Counting(evs []events.Event) []events.Event {
	if !slices.ContainsFunc(evs, func(e events.Event) bool { return e.Type == "amend" }) {
		return evs
	}
	marked := slices.Clone(evs)
	for i, e := range marked {
		if e.Type != "amend" {
			continue
		}
		g, _ := e.Fields["gate"].(string)
		listed, _ := e.Fields["invalidated"].([]any)
		for _, j := range Invalidates(marked[:i], e.Unit, gate.Gate(g)) {
			if slices.Contains(listed, marked[j].Fields["gate"]) {
				fields := maps.Clone(marked[j].Fields)
				fields["counts"] = false
				marked[j].Fields = fields
			}
		}
	}
	return marked
}

// Invalidates returns the indexes in evs, as Counting returns them, of the
// approve events an amend of unit id back to gate g invalidates:
// every counting approval of the unit from its latest counting approval of g
// on, in order. There are none when the unit has no counting approval of g.
func Invalidates(evs []events.Event, id string, g gate.Gate) []int {
	from := -1
	for i, e := range evs {
		if e.Type == "approve" && e.Unit == id && e.Fields["gate"] == string(g) && e.Fields["counts"] != false {
			from = i
		}
	}
	if from < 0 {
		return nil
	}
	var idx []int
	for i := from; i < len(evs); i++ {
		if e := evs[i]; e.Type == "approve" && e.Unit == id && e.Fields["counts"] != false {
			idx = append(idx, i)
		}
	}
	return idx
}

// Amended returns unit id's latest amend event in evs while the unit is
// still back at its gate: no counting approval of that gate follows it. A
// refactor amended back to intent stays amended until plan, its
// code-opening gate, is approved again. ok is false otherwise.
func Amended(evs []events.Event, id string) (a events.Event, ok bool) {
	var until any
	for _, e := range evs {
		switch {
		case e.Unit != id:
		case e.Type == "amend":
			a, ok, until = e, true, e.Fields["gate"]
			if until == string(gate.Intent) {
				until = string(gate.Plan)
			}
		case ok && e.Type == "approve" && e.Fields["gate"] == until && e.Fields["counts"] != false:
			ok = false
		}
	}
	return a, ok
}

// Reopened says why bug p at cause goes back to fix: a commit after its
// latest verify clear in evs changes a file that is neither a test (TESTING.md
// test_files) nor under lyceum/. It returns "" when p is not a bug at
// cause or no such commit follows the clear.
func Reopened(root string, p *unit.Plan, evs []events.Event) (string, error) {
	if !guard.ReopenEnabled {
		return "", nil
	}
	if p == nil || p.Type != string(gate.Bug) || p.Gate != string(gate.Cause) {
		return "", nil
	}
	since := ""
	for _, e := range evs {
		if g, _ := e.Fields["gate"].(string); e.Type == "gate-clear" && e.Unit == p.ID && g == string(gate.Verify) {
			since = e.Commit
		}
	}
	if since == "" {
		return "", nil
	}
	globs, err := acmap.TestingGlobs(filepath.Join(root, approval.TestingPath))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	committed, err := gitx.CommittedSince(root, since)
	if err != nil {
		return "", err
	}
	var code []string
	for _, f := range committed {
		if !strings.HasPrefix(f, "lyceum/") && !acmap.MatchAny(globs, f) {
			code = append(code, f)
		}
	}
	if len(code) == 0 {
		return "", nil
	}
	return fmt.Sprintf("a commit after verify cleared at %s changes files that are not tests: %s", since, strings.Join(code, ", ")), nil
}

// codeIn returns the paths in paths that are code (codepath.IsCode), in order.
func codeIn(paths []string) []string {
	var code []string
	for _, f := range paths {
		if codepath.IsCode(f) {
			code = append(code, f)
		}
	}
	return code
}

// approvalsDiff says how the plan's approvals got differ from the ones the
// event log implies, want, or returns "" when they match.
func approvalsDiff(got, want []unit.Approval) string {
	if len(got) != len(want) {
		return fmt.Sprintf("has %d entr%s, but the event log implies %d", len(got), entries(len(got)), len(want))
	}
	for i := range got {
		g, w := got[i], want[i]
		same := g.Gate == w.Gate && g.Commit == w.Commit && g.By == w.By && g.At.Equal(w.At) &&
			maps.Equal(g.Hashes, w.Hashes) && counts(g) == counts(w)
		if !same {
			return fmt.Sprintf("entry %d (gate %s) does not match its approve event (gate %s)", i+1, g.Gate, w.Gate)
		}
	}
	return ""
}

// counts reports whether a counts: a nil Counts does.
func counts(a unit.Approval) bool {
	return a.Counts == nil || *a.Counts
}

func entries(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// RecordsCheck names the record freshness check in findings.
const RecordsCheck = "record-freshness"

// Records warns, never fails, locally or in CI, about every record and area
// file whose cites are drifted or broken, or that can't be checked.
func Records(ctx Context) ([]Finding, error) {
	entries, err := records.All(ctx.Root)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	for _, e := range entries {
		if e.State == records.Fresh {
			continue
		}
		next := "re-read the changed paths, refresh the record, then run \"lyceum record " + e.Path + "\""
		switch e.State {
		case records.Broken:
			next = "redo the record from the code, then run \"lyceum record " + e.Path + "\""
		case records.Invalid:
			next = "fix the record, then run \"lyceum record " + e.Path + "\""
		}
		if e.Kind == "area" && e.State != records.Invalid {
			next = "re-read the cited paths and update the as-built section and its cites: in " + e.Path
		}
		findings = append(findings, Finding{Check: RecordsCheck, Severity: Warn, Path: e.Path,
			Message: fmt.Sprintf("%s is %s: %s", e.Path, e.State, e.Detail()), Next: next})
	}
	return findings, nil
}
