package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/unit"
)

const planPath = "lyceum/plan/aaaaaa-x.md"

const plan = `---
id: aaaaaa-x
type: feat
title: X
branch: feat/aaaaaa-x
status: planned
gate: spec
approvals: []
areas: [orders]
---

## What and why
`

// repo writes files under a temp root and returns it.
func repo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// approve returns an approve event of gate on unitID with the current hashes
// of files under root.
func approve(t *testing.T, root, unitID, g string, counts bool, files ...string) events.Event {
	t.Helper()
	pp := ""
	if unitID != "" {
		pp = planPath
	}
	got, err := approval.Hashes(root, pp, gate.Gate(g), files)
	if err != nil {
		t.Fatal(err)
	}
	hashes := map[string]any{}
	for f, h := range got {
		hashes[f] = h
	}
	return events.Event{Type: "approve", Unit: unitID, Fields: map[string]any{"gate": g, "hashes": hashes, "counts": counts}}
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func context(t *testing.T, root string, evs ...events.Event) Context {
	t.Helper()
	p, err := unit.ReadPlan(filepath.Join(root, filepath.FromSlash(planPath)))
	if err != nil {
		t.Fatal(err)
	}
	return Context{Root: root, Plan: p, PlanPath: planPath, Events: evs}
}

func messages(fs []Finding) []string {
	var m []string
	for _, f := range fs {
		m = append(m, f.Message)
	}
	return m
}

// AC43: editing the plan body after its approval is a fail finding naming the
// gate and the file; editing the CLI-owned gate, status or approvals is not.
func TestApprovalIntegrityPlanEdit_AC43(t *testing.T) {
	root := repo(t, map[string]string{planPath: plan, "lyceum/spec/orders.md": "# Orders\n"})
	ev := approve(t, root, "aaaaaa-x", "spec", true, planPath, "lyceum/spec/orders.md")

	edited := strings.Replace(plan, "status: planned\ngate: spec\napprovals: []", "status: in-progress\ngate: build\napprovals: [{gate: spec}]", 1)
	write(t, root, planPath, edited)
	got, err := ApprovalIntegrity(context(t, root, ev))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("CLI-owned edit: findings = %v, want none", messages(got))
	}

	write(t, root, planPath, edited+"More.\n")
	got, err = ApprovalIntegrity(context(t, root, ev))
	if err != nil {
		t.Fatal(err)
	}
	want := Finding{
		Check: IntegrityCheck, Severity: Fail, Unit: "aaaaaa-x", Gate: "spec", Path: planPath,
		Message: "the spec approval no longer matches lyceum/plan/aaaaaa-x.md: it changed after it was approved",
		Next:    "restore lyceum/plan/aaaaaa-x.md to what the spec approval covered (git log -p -- lyceum/plan/aaaaaa-x.md shows the edits)",
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("findings = %+v, want [%+v]", got, want)
	}
}

// An area file that changed or is gone is a finding too.
func TestApprovalIntegrityAreaFile(t *testing.T) {
	root := repo(t, map[string]string{planPath: plan, "lyceum/spec/orders.md": "# Orders\n", "lyceum/spec/users.md": "# Users\n"})
	ev := approve(t, root, "aaaaaa-x", "spec", true, planPath, "lyceum/spec/orders.md", "lyceum/spec/users.md")
	write(t, root, "lyceum/spec/orders.md", "# Orders v2\n")
	if err := os.Remove(filepath.Join(root, "lyceum", "spec", "users.md")); err != nil {
		t.Fatal(err)
	}
	got, err := ApprovalIntegrity(context(t, root, ev))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"the spec approval no longer matches lyceum/spec/orders.md: it changed after it was approved",
		"the spec approval no longer matches lyceum/spec/users.md: the file no longer exists",
	}
	if strings.Join(messages(got), "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings = %q, want %q", messages(got), want)
	}
}

// Only the latest counting approval of each gate is relied on: a non-counting
// approval, a superseded one and another unit's are not checked.
func TestApprovalIntegrityReliedOn(t *testing.T) {
	root := repo(t, map[string]string{planPath: plan, "lyceum/spec/orders.md": "# Orders\n"})
	old := approve(t, root, "aaaaaa-x", "spec", true, "lyceum/spec/orders.md")
	other := approve(t, root, "bbbbbb-y", "spec", true, "lyceum/spec/orders.md")
	write(t, root, "lyceum/spec/orders.md", "# Orders v2\n")
	notCounting := approve(t, root, "aaaaaa-x", "plan", false, "lyceum/spec/orders.md")
	write(t, root, "lyceum/spec/orders.md", "# Orders v3\n")
	latest := approve(t, root, "aaaaaa-x", "spec", true, "lyceum/spec/orders.md")

	got, err := ApprovalIntegrity(context(t, root, old, other, notCounting, latest))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("findings = %q, want none", messages(got))
	}
}

// The latest counting testing approval is checked with or without a unit.
func TestApprovalIntegrityTesting(t *testing.T) {
	root := repo(t, map[string]string{approval.TestingPath: "v1\n"})
	ev := approve(t, root, "", "testing", true, approval.TestingPath)
	write(t, root, approval.TestingPath, "v2\n")
	got, err := ApprovalIntegrity(Context{Root: root, Events: []events.Event{ev}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Unit != "" || got[0].Message != "the testing approval no longer matches TESTING.md: it changed after it was approved" {
		t.Fatalf("findings = %+v", got)
	}
}

// AC38: the latest counting project approval is checked like testing:
// lyceum/SPEC.md and each area file it hashed, with no unit.
func TestApprovalIntegrityProject_AC38(t *testing.T) {
	root := repo(t, map[string]string{approval.SpecPath: "v1\n", "lyceum/spec/orders.md": "# Orders\n", "lyceum/spec/users.md": "# Users\n"})
	ev := approve(t, root, "", "project", true, approval.SpecPath, "lyceum/spec/orders.md", "lyceum/spec/users.md")
	got, err := ApprovalIntegrity(Context{Root: root, Events: []events.Event{ev}})
	if err != nil || len(got) != 0 {
		t.Fatalf("findings = %+v, %v; want none", got, err)
	}
	write(t, root, approval.SpecPath, "v2\n")
	if err := os.Remove(filepath.Join(root, "lyceum", "spec", "users.md")); err != nil {
		t.Fatal(err)
	}
	got, err = ApprovalIntegrity(Context{Root: root, Events: []events.Event{ev}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"the project approval no longer matches lyceum/SPEC.md: it changed after it was approved",
		"the project approval no longer matches lyceum/spec/users.md: the file no longer exists",
	}
	if strings.Join(messages(got), "\n") != strings.Join(want, "\n") || got[0].Unit != "" ||
		got[0].Next != `the owner runs "lyceum approve project" in a terminal, or restore lyceum/SPEC.md (git log -p -- lyceum/SPEC.md shows the edits)` {
		t.Fatalf("findings = %+v, want %q", got, want)
	}
}

// addEvent is the add event lyceum add writes for plan.
var addEvent = events.Event{Type: "add", Unit: "aaaaaa-x", Fields: map[string]any{
	"unit_type": "feat", "branch": "feat/aaaaaa-x", "gate": "spec", "status": "planned",
}}

func fields(fs []Finding) []string {
	var f []string
	for _, x := range fs {
		f = append(f, x.Field+": "+x.Message)
	}
	return f
}

// AC17: replaying the add event and every approve event rebuilds the plan's
// CLI-owned fields: a non-counting approval is listed but does not move the
// gate, and a counting one moves the gate and carries the new status.
func TestCLIFieldsReplay_AC17(t *testing.T) {
	root := repo(t, map[string]string{planPath: plan})
	other := events.Event{Type: "approve", Unit: "aaaaaa-x", Commit: "1111111", Fields: map[string]any{
		"gate": "spec", "hashes": map[string]any{planPath: "sha256:aa"}, "by": "other@example.com",
		"at": "2026-01-01T00:00:00Z", "counts": false,
	}}
	owner := events.Event{Type: "approve", Unit: "aaaaaa-x", Commit: "2222222", Fields: map[string]any{
		"gate": "spec", "hashes": map[string]any{planPath: "sha256:bb"}, "by": "owner@example.com",
		"at": "2026-01-01T00:01:00Z", "counts": true, "status": "in-progress",
	}}
	bypass := events.Event{Type: "bypass", Unit: "aaaaaa-x", Fields: map[string]any{"check": "x", "message": "y"}}
	otherUnit := events.Event{Type: "approve", Unit: "bbbbbb-y", Fields: map[string]any{"gate": "spec", "counts": true, "status": "closed"}}

	ctx := context(t, root, addEvent, bypass, otherUnit)
	ctx.Branch = "feat/aaaaaa-x"
	got, err := CLIFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("after add: findings = %q, want none", fields(got))
	}

	approved := strings.Replace(plan, "status: planned\ngate: spec\napprovals: []\n", `status: in-progress
gate: build
approvals:
  - gate: spec
    hashes: {lyceum/plan/aaaaaa-x.md: "sha256:aa"}
    commit: 1111111
    by: other@example.com
    at: 2026-01-01T00:00:00Z
    counts: false
  - gate: spec
    hashes: {lyceum/plan/aaaaaa-x.md: "sha256:bb"}
    commit: 2222222
    by: owner@example.com
    at: 2026-01-01T00:01:00Z
`, 1)
	write(t, root, planPath, approved)
	ctx = context(t, root, addEvent, other, bypass, owner)
	ctx.Branch = "feat/aaaaaa-x"
	if got, err = CLIFields(ctx); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("after approvals: findings = %q, want none", fields(got))
	}

	// Without the counting approval, the plan is ahead of the log on gate,
	// status and approvals.
	ctx = context(t, root, addEvent, other)
	ctx.Branch = "feat/aaaaaa-x"
	if got, err = CLIFields(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"status: plan field status is in-progress, but the event log implies planned",
		"gate: plan field gate is build, but the event log implies spec",
		"approvals: plan field approvals has 2 entries, but the event log implies 1",
	}
	if strings.Join(fields(got), "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings = %q, want %q", fields(got), want)
	}

	// An entry edited in place is named by its position.
	write(t, root, planPath, strings.Replace(approved, "by: other@example.com", "by: owner@example.com", 1))
	ctx = context(t, root, addEvent, other, owner)
	ctx.Branch = "feat/aaaaaa-x"
	if got, err = CLIFields(ctx); err != nil {
		t.Fatal(err)
	}
	want = []string{"approvals: plan field approvals entry 1 (gate spec) does not match its approve event (gate spec)"}
	if strings.Join(fields(got), "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings = %q, want %q", fields(got), want)
	}
}

// AC17: a plan no add event records is a finding on id; a branch an
// add event claims with no plan on it is a finding on branch.
func TestCLIFieldsUnitLookup_AC17(t *testing.T) {
	root := repo(t, map[string]string{planPath: plan})
	ctx := context(t, root)
	ctx.Branch = "feat/aaaaaa-x"
	got, err := CLIFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"id: plan field id is aaaaaa-x, but no add event records this unit"}
	if strings.Join(fields(got), "\n") != strings.Join(want, "\n") {
		t.Fatalf("no add event: findings = %q, want %q", fields(got), want)
	}

	got, err = CLIFields(Context{Root: root, Branch: "feat/aaaaaa-x", Events: []events.Event{addEvent}})
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"branch: the add event of unit aaaaaa-x puts it on branch feat/aaaaaa-x, but no plan on this branch is unit aaaaaa-x"}
	if strings.Join(fields(got), "\n") != strings.Join(want, "\n") || got[0].Unit != "aaaaaa-x" || got[0].Path != planPath {
		t.Fatalf("no plan: findings = %+v, want %q", got, want)
	}

	// A unit the log shows closed no longer claims its branch.
	closed := events.Event{Type: "close", Unit: "aaaaaa-x", Fields: map[string]any{"status": "closed"}}
	if id, err := BranchUnit([]events.Event{addEvent, closed}, "feat/aaaaaa-x"); err != nil || id != "" {
		t.Fatalf("closed unit: BranchUnit = %q, %v, want none", id, err)
	}
	if id, err := BranchUnit([]events.Event{addEvent}, "feat/other"); err != nil || id != "" {
		t.Fatalf("other branch: BranchUnit = %q, %v, want none", id, err)
	}
}

// A chore approved at plan is on its planned tier: the gate moves to build.
func TestCLIFieldsPlannedChore(t *testing.T) {
	add := events.Event{Type: "add", Unit: "aaaaaa-x", Fields: map[string]any{
		"unit_type": "chore", "branch": "chore/aaaaaa-x", "gate": "plan", "status": "planned",
	}}
	ok := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "plan", "counts": true}}
	exp, found, err := replay([]events.Event{add, ok}, "aaaaaa-x")
	if err != nil || !found || exp.gate != "build" {
		t.Fatalf("replay = %+v, %v, %v, want gate build", exp, found, err)
	}
}

// AC17: a gate-clear event moves the replayed gate past the mechanical gate it
// cleared, from that gate or the work step before it, and carries the status.
// One the log does not lead to moves nothing. An approve tests moves nothing;
// a --no-repro skip and approve cause move a bug like any approval. A
// refactor clears baseline and verify.
func TestReplayGateClear_AC17(t *testing.T) {
	spec := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "spec", "counts": true, "status": "in-progress"}}
	verify := events.Event{Type: "gate-clear", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "verify", "run": map[string]any{}, "status": "implemented"}}
	exp, _, err := replay([]events.Event{addEvent, spec, verify}, "aaaaaa-x")
	if err != nil || exp.gate != "close" || exp.status != "implemented" {
		t.Fatalf("cleared from build: replay = %+v, %v; want close, implemented", exp, err)
	}
	// At spec, a judgment gate, verify is not next: the clear is skipped.
	exp, _, err = replay([]events.Event{addEvent, verify}, "aaaaaa-x")
	if err != nil || exp.gate != "spec" || exp.status != "planned" {
		t.Fatalf("forged clear at spec: replay = %+v, %v; want spec, planned", exp, err)
	}
	// A cleared gate the type lacks, or one that is not mechanical, is skipped.
	for _, g := range []string{"repro", "spec", "build"} {
		bad := events.Event{Type: "gate-clear", Unit: "aaaaaa-x", Fields: map[string]any{"gate": g}}
		if exp, _, err = replay([]events.Event{addEvent, spec, bad}, "aaaaaa-x"); err != nil || exp.gate != "build" {
			t.Fatalf("clear of %s: replay = %+v, %v; want build", g, exp, err)
		}
	}
	// A bug at diagnose: verify ends both work steps, diagnose and fix.
	bugAdd := events.Event{Type: "add", Unit: "aaaaaa-x", Fields: map[string]any{"unit_type": "bug", "branch": "fix/aaaaaa-x", "gate": "repro", "status": "planned"}}
	repro := events.Event{Type: "gate-clear", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "repro", "run": map[string]any{}, "status": "in-progress"}}
	if exp, _, err = replay([]events.Event{bugAdd, repro, verify}, "aaaaaa-x"); err != nil || exp.gate != "cause" || exp.status != "implemented" {
		t.Fatalf("bug at diagnose: replay = %+v, %v; want cause, implemented", exp, err)
	}
	// A bug's approve tests is listed but moves no gate.
	tests := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "tests", "hashes": map[string]any{"a_test.go": "sha256:aa"}, "counts": true}}
	if exp, _, err = replay([]events.Event{bugAdd, repro, tests}, "aaaaaa-x"); err != nil || exp.gate != "diagnose" || len(exp.approvals) != 1 || exp.approvals[0].Gate != "tests" {
		t.Fatalf("approve tests: replay = %+v, %v; want diagnose and one tests approval", exp, err)
	}
	if exp, _, err = replay([]events.Event{bugAdd, repro, tests, verify}, "aaaaaa-x"); err != nil || exp.gate != "cause" {
		t.Fatalf("approve tests, then verify: replay = %+v, %v; want cause", exp, err)
	}
	// A bug's --no-repro skip moves it from repro to diagnose and in-progress;
	// verify then clears, and approve cause moves it to close (AC107).
	skip := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "repro", "hashes": map[string]any{}, "no_repro": true, "reason": "visual only", "counts": true, "status": "in-progress"}}
	cause := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "cause", "hashes": map[string]any{}, "counts": true}}
	if exp, _, err = replay([]events.Event{bugAdd, skip}, "aaaaaa-x"); err != nil || exp.gate != "diagnose" || exp.status != "in-progress" {
		t.Fatalf("no-repro: replay = %+v, %v; want diagnose, in-progress", exp, err)
	}
	if exp, _, err = replay([]events.Event{bugAdd, skip, verify, cause}, "aaaaaa-x"); err != nil || exp.gate != "close" || exp.status != "implemented" || len(exp.approvals) != 2 {
		t.Fatalf("no-repro, verify, cause: replay = %+v, %v; want close, implemented, two approvals", exp, err)
	}
	// A refactor: intent, then baseline (an approve tests there moves nothing),
	// plan, and verify from build, implemented.
	refAdd := events.Event{Type: "add", Unit: "aaaaaa-x", Fields: map[string]any{"unit_type": "refactor", "branch": "refactor/aaaaaa-x", "gate": "intent", "status": "planned"}}
	intent := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "intent", "counts": true}}
	baseline := events.Event{Type: "gate-clear", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "baseline", "run": map[string]any{}, "fingerprint": map[string]any{"a_test.go": "sha256:aa"}}}
	refPlan := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "plan", "counts": true, "status": "in-progress"}}
	if exp, _, err = replay([]events.Event{refAdd, intent, tests, baseline}, "aaaaaa-x"); err != nil || exp.gate != "plan" || exp.status != "planned" {
		t.Fatalf("refactor baseline: replay = %+v, %v; want plan, planned", exp, err)
	}
	if exp, _, err = replay([]events.Event{refAdd, intent, verify}, "aaaaaa-x"); err != nil || exp.gate != "baseline" {
		t.Fatalf("refactor verify at baseline: replay = %+v, %v; want baseline", exp, err)
	}
	if exp, _, err = replay([]events.Event{refAdd, intent, baseline, refPlan, tests, verify}, "aaaaaa-x"); err != nil || exp.gate != "close" || exp.status != "implemented" {
		t.Fatalf("refactor verify: replay = %+v, %v; want close, implemented", exp, err)
	}
	// A planned chore at build clears verify.
	add := events.Event{Type: "add", Unit: "aaaaaa-x", Fields: map[string]any{"unit_type": "chore", "branch": "chore/aaaaaa-x", "gate": "build", "status": "in-progress"}}
	plan := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "plan", "counts": true}}
	if exp, _, err = replay([]events.Event{add, plan, verify}, "aaaaaa-x"); err != nil || exp.gate != "close" {
		t.Fatalf("planned chore: replay = %+v, %v; want close", exp, err)
	}
}

// AC132: a gate-clear recording no_tests clears a doc unit's verify; on a feat
// it moves nothing and is a finding: the clear has no run as evidence.
func TestCLIFieldsNoTests_AC132(t *testing.T) {
	noTests := events.Event{Type: "gate-clear", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "verify", "no_tests": true, "status": "implemented"}}
	cleared := strings.Replace(plan, "status: planned\ngate: spec\n", "status: implemented\ngate: close\n", 1)

	docAdd := events.Event{Type: "add", Unit: "aaaaaa-x", Fields: map[string]any{"unit_type": "doc", "branch": "docs/aaaaaa-x", "gate": "draft", "status": "in-progress"}}
	root := repo(t, map[string]string{planPath: strings.NewReplacer("type: feat", "type: doc", "branch: feat/", "branch: docs/").Replace(cleared)})
	ctx := context(t, root, docAdd, noTests)
	ctx.Branch = "docs/aaaaaa-x"
	got, err := CLIFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("doc verify with no tests: findings = %q, want none", fields(got))
	}

	featAdd := events.Event{Type: "add", Unit: "aaaaaa-x", Fields: map[string]any{"unit_type": "feat", "branch": "feat/aaaaaa-x", "gate": "build", "status": "in-progress"}}
	root = repo(t, map[string]string{planPath: cleared})
	ctx = context(t, root, featAdd, noTests)
	ctx.Branch = "feat/aaaaaa-x"
	if got, err = CLIFields(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"status: plan field status is implemented, but the event log implies in-progress",
		"gate: plan field gate is close, but the event log implies build",
		"gate: the gate-clear event of verify records no_tests, but only a doc unit's verify clears with no tests: the clear has no run as evidence",
	}
	if strings.Join(fields(got), "\n") != strings.Join(want, "\n") {
		t.Fatalf("forged feat verify: findings = %q, want %q", fields(got), want)
	}
}

// AC17: a gate-reopen event moves a bug at cause back to fix, where verify
// clears again, and sets its status; one at any other gate,
// or to another gate, moves nothing.
func TestReplayGateReopen_AC17(t *testing.T) {
	bugAdd := events.Event{Type: "add", Unit: "aaaaaa-x", Fields: map[string]any{"unit_type": "bug", "branch": "fix/aaaaaa-x", "gate": "repro", "status": "planned"}}
	repro := events.Event{Type: "gate-clear", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "repro", "run": map[string]any{}, "status": "in-progress"}}
	verify := events.Event{Type: "gate-clear", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "verify", "run": map[string]any{}, "status": "implemented"}}
	reopen := events.Event{Type: "gate-reopen", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "fix", "status": "in-progress", "reason": "a code commit"}}
	exp, _, err := replay([]events.Event{bugAdd, repro, verify, reopen}, "aaaaaa-x")
	if err != nil || exp.gate != "fix" || exp.status != "in-progress" {
		t.Fatalf("reopen at cause: replay = %+v, %v; want fix, in-progress", exp, err)
	}
	if exp, _, err = replay([]events.Event{bugAdd, repro, verify, reopen, verify}, "aaaaaa-x"); err != nil || exp.gate != "cause" || exp.status != "implemented" {
		t.Fatalf("reopen, then verify: replay = %+v, %v; want cause, implemented", exp, err)
	}
	if exp, _, err = replay([]events.Event{bugAdd, repro, reopen}, "aaaaaa-x"); err != nil || exp.gate != "diagnose" || exp.status != "in-progress" {
		t.Fatalf("reopen at diagnose: replay = %+v, %v; want diagnose", exp, err)
	}
	toRepro := events.Event{Type: "gate-reopen", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "repro", "status": "in-progress"}}
	if exp, _, err = replay([]events.Event{bugAdd, repro, verify, toRepro}, "aaaaaa-x"); err != nil || exp.gate != "cause" || exp.status != "implemented" {
		t.Fatalf("reopen to repro: replay = %+v, %v; want cause, implemented", exp, err)
	}
}

func TestTestingDrift(t *testing.T) {
	const testing1 = "---\nlyceum: 1\nsuite: x\n---\n"
	root := repo(t, map[string]string{planPath: plan, "TESTING.md": testing1})
	testingOK := approve(t, root, "", "testing", true, "TESTING.md")
	spec := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "spec", "counts": true}}
	write(t, root, "TESTING.md", testing1+"edited\n")

	// At spec, a judgment gate followed by a work step: nothing relies on TESTING.md.
	got, err := TestingDrift(context(t, root, addEvent, testingOK))
	if err != nil || len(got) != 0 {
		t.Fatalf("at spec: %q, %v; want none", messages(got), err)
	}
	// At build, verify follows: a fail finding naming TESTING.md.
	got, err = TestingDrift(context(t, root, addEvent, testingOK, spec))
	if err != nil || len(got) != 1 || got[0].Severity != Fail || got[0].Path != "TESTING.md" ||
		got[0].Message != "TESTING.md changed since its last approval, and the verify gate relies on it" {
		t.Fatalf("at build: %+v, %v", got, err)
	}
	// The integrity check leaves the testing approval to the drift finding.
	if got, err = ApprovalIntegrity(context(t, root, addEvent, testingOK, spec)); err != nil || len(got) != 0 {
		t.Fatalf("at build, approval integrity: %q, %v; want none", messages(got), err)
	}
	// Re-approved: no finding.
	if got, err = TestingDrift(context(t, root, addEvent, testingOK, spec, approve(t, root, "", "testing", true, "TESTING.md"))); err != nil || len(got) != 0 {
		t.Fatalf("re-approved: %q, %v; want none", messages(got), err)
	}
	// A non-counting re-approval does not count.
	if got, err = TestingDrift(context(t, root, addEvent, testingOK, spec, approve(t, root, "", "testing", false, "TESTING.md"))); err != nil || len(got) != 1 {
		t.Fatalf("non-counting re-approval: %q, %v; want one finding", messages(got), err)
	}
	// Never approved: nothing it changed from.
	if got, err = TestingDrift(context(t, root, addEvent, spec)); err != nil || len(got) != 0 {
		t.Fatalf("never approved: %q, %v; want none", messages(got), err)
	}
	// Gone.
	if err := os.Remove(filepath.Join(root, "TESTING.md")); err != nil {
		t.Fatal(err)
	}
	got, err = TestingDrift(context(t, root, addEvent, testingOK, spec))
	if err != nil || len(got) != 1 || got[0].Message != "TESTING.md no longer exists, and the verify gate relies on it" {
		t.Fatalf("gone: %q, %v", messages(got), err)
	}
}

// The replay and the CLI follow one tier rule. A chore clears verify only
// once its plan is approved; `lyceum plan`'s move holds it at plan through a
// non-owner's approval, and an amend's invalidated approval leaves it planned.
func TestReplayChoreTier_AC168(t *testing.T) {
	add := events.Event{Type: "add", Unit: "aaaaaa-x", Fields: map[string]any{
		"unit_type": "chore", "branch": "chore/aaaaaa-x", "gate": "build", "status": "in-progress",
	}}
	move := events.Event{Type: "gate-reopen", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "plan", "status": "planned"}}
	approve := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "plan", "counts": true, "status": "in-progress"}}
	nonOwner := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "plan", "counts": false}}
	verify := events.Event{Type: "gate-clear", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "verify", "run": map[string]any{}, "status": "implemented"}}
	amend := events.Event{Type: "amend", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "plan", "invalidated": []any{"plan"}, "status": "planned"}}
	for _, tc := range []struct {
		name string
		evs  []events.Event
		want string
	}{
		{"a clear does not lead a trivial chore to close", []events.Event{add, verify}, "build"},
		{"moved", []events.Event{add, move}, "plan"},
		{"a non-owner's approval moves nothing", []events.Event{add, move, nonOwner}, "plan"},
		{"approved", []events.Event{add, move, approve}, "build"},
		{"verify cleared", []events.Event{add, move, approve, verify}, "close"},
		{"amended", []events.Event{add, move, approve, amend}, "plan"},
		{"amended, a second move does nothing", []events.Event{add, move, approve, amend, move}, "plan"},
		{"amended, no clear before the re-approval", []events.Event{add, move, approve, amend, verify}, "plan"},
		{"amended and approved again", []events.Event{add, move, approve, amend, approve, verify}, "close"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exp, found, err := replay(tc.evs, "aaaaaa-x")
			if err != nil || !found || exp.gate != tc.want {
				t.Fatalf("replay = %+v, %v, %v; want gate %s", exp, found, err, tc.want)
			}
		})
	}
}
