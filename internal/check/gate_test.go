package check

import (
	"slices"
	"strings"
	"testing"

	"github.com/oritromax/lyceum/internal/events"
)

const testingMD = "---\ntest_files: [\"**/*_test.go\"]\n---\n"

// planAt is plan with its type and gate replaced, and files: set when files is not "".
func planAt(typ, gate, files string) string {
	p := strings.Replace(plan, "type: feat\n", "type: "+typ+"\n", 1)
	p = strings.Replace(p, "gate: spec\n", "gate: "+gate+"\n", 1)
	if files != "" {
		p = strings.Replace(p, "areas: [orders]\n", "areas: [orders]\nfiles: "+files+"\n", 1)
	}
	return p
}

func TestGateBeforeCode_AC47(t *testing.T) {
	counting := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "spec", "counts": true}}
	notCounting := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "spec", "counts": false}}
	otherUnit := events.Event{Type: "approve", Unit: "bbbbbb-y", Fields: map[string]any{"gate": "spec", "counts": true}}
	planApproved := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "plan", "counts": true}}
	bugAdd := events.Event{Type: "add", Unit: "aaaaaa-x", Fields: map[string]any{"unit_type": "bug", "branch": "feat/aaaaaa-x", "gate": "repro", "status": "planned"}}
	reproCleared := events.Event{Type: "gate-clear", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "repro", "run": map[string]any{}, "status": "in-progress"}}
	for _, tc := range []struct {
		name       string
		typ, gate  string
		testing    bool // write a TESTING.md
		changed    []string
		evs        []events.Event
		wantPaths  string // "" for no finding
		wantOpenAt string
	}{
		{name: "feat code", typ: "feat", gate: "spec", changed: []string{"src/a.go"}, wantPaths: "src/a.go", wantOpenAt: "spec"},
		{name: "feat ci config", typ: "feat", gate: "spec", changed: []string{".github/workflows/ci.yml"}, wantPaths: ".github/workflows/ci.yml", wantOpenAt: "spec"},
		{name: "feat no code", typ: "feat", gate: "spec", changed: []string{planPath, "lyceum/spec/orders.md", "docs/x.png", "README.md"}},
		{name: "feat test file is code", typ: "feat", gate: "spec", testing: true, changed: []string{"src/a_test.go"}, wantPaths: "src/a_test.go", wantOpenAt: "spec"},
		{name: "feat spec approved", typ: "feat", gate: "build", changed: []string{"src/a.go"}, evs: []events.Event{counting}},
		{name: "feat approval not counting", typ: "feat", gate: "spec", changed: []string{"src/a.go"}, evs: []events.Event{notCounting}, wantPaths: "src/a.go", wantOpenAt: "spec"},
		{name: "feat approval of another unit", typ: "feat", gate: "spec", changed: []string{"src/a.go"}, evs: []events.Event{otherUnit}, wantPaths: "src/a.go", wantOpenAt: "spec"},
		{name: "bug test file at repro", typ: "bug", gate: "repro", testing: true, changed: []string{"src/a_test.go"}},
		{name: "bug code at repro", typ: "bug", gate: "repro", testing: true, changed: []string{"src/a.go", "src/a_test.go"}, wantPaths: "src/a.go", wantOpenAt: "repro"},
		{name: "bug test file without TESTING.md", typ: "bug", gate: "repro", changed: []string{"src/a_test.go"}, wantPaths: "src/a_test.go", wantOpenAt: "repro"},
		{name: "bug repro cleared", typ: "bug", gate: "diagnose", changed: []string{"src/a.go"}, evs: []events.Event{bugAdd, reproCleared}},
		{name: "bug past repro without a clear", typ: "bug", gate: "diagnose", changed: []string{"src/a.go"}, evs: []events.Event{bugAdd}, wantPaths: "src/a.go", wantOpenAt: "repro"},
		{name: "refactor test file at intent", typ: "refactor", gate: "intent", testing: true, changed: []string{"src/a_test.go"}, wantPaths: "src/a_test.go", wantOpenAt: "plan"},
		{name: "refactor test file at baseline", typ: "refactor", gate: "baseline", testing: true, changed: []string{"src/a_test.go"}},
		{name: "refactor code at baseline", typ: "refactor", gate: "baseline", testing: true, changed: []string{"src/a.go"}, wantPaths: "src/a.go", wantOpenAt: "plan"},
		{name: "refactor test file at plan", typ: "refactor", gate: "plan", testing: true, changed: []string{"src/a_test.go"}},
		{name: "refactor code at plan", typ: "refactor", gate: "plan", testing: true, changed: []string{"src/a.go", "src/a_test.go"}, wantPaths: "src/a.go", wantOpenAt: "plan"},
		{name: "refactor plan approved", typ: "refactor", gate: "build", changed: []string{"src/a.go"}, evs: []events.Event{planApproved}},
		{name: "trivial chore", typ: "chore", gate: "build", changed: []string{"src/a.go"}},
		{name: "planned chore at plan", typ: "chore", gate: "plan", changed: []string{"src/a.go"}, wantPaths: "src/a.go", wantOpenAt: "plan"},
		{name: "doc", typ: "doc", gate: "draft", changed: []string{"src/a.go"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{planPath: planAt(tc.typ, tc.gate, "")}
			if tc.testing {
				files["TESTING.md"] = testingMD
			}
			ctx := context(t, repo(t, files), tc.evs...)
			ctx.Changed = tc.changed
			got, err := GateBeforeCode(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantPaths == "" {
				if len(got) != 0 {
					t.Fatalf("findings = %q; want none", messages(got))
				}
				return
			}
			want := "the branch changes code before the " + tc.wantOpenAt + " gate is cleared: " + tc.wantPaths
			if len(got) != 1 || got[0].Message != want || got[0].Check != GateCheck || got[0].Severity != Fail || got[0].Unit != "aaaaaa-x" {
				t.Fatalf("findings = %+v; want one failing %q", got, want)
			}
		})
	}
}

// The gate the event log implies wins over a hand-edited plan gate: a feat whose
// plan says build but whose spec approval is not in the log still fails.
func TestGateBeforeCodeUsesEventLog_AC47(t *testing.T) {
	add := events.Event{Type: "add", Unit: "aaaaaa-x", Fields: map[string]any{"unit_type": "feat", "branch": "feat/aaaaaa-x", "gate": "spec", "status": "planned"}}
	ctx := context(t, repo(t, map[string]string{planPath: planAt("feat", "build", "")}), add)
	ctx.Changed = []string{"src/a.go"}
	got, err := GateBeforeCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("findings = %q; want one", messages(got))
	}
}

func TestGateBeforeCodeNoUnit(t *testing.T) {
	got, err := GateBeforeCode(Context{Root: t.TempDir(), Changed: []string{"src/a.go"}})
	if err != nil || len(got) != 0 {
		t.Fatalf("GateBeforeCode with no unit = %q, %v; want none", messages(got), err)
	}
}

func TestScope(t *testing.T) {
	for _, tc := range []struct {
		name    string
		files   string
		changed []string
		want    string // "" for no finding
	}{
		{name: "no files list", changed: []string{"src/a.go", "README.md"}, want: "src/a.go"},
		{name: "directory entry", files: "[src/]", changed: []string{"src/a.go", "src/deep/b.go"}},
		{name: "directory entry is a prefix only on a slash", files: "[src/]", changed: []string{"srcx/a.go"}, want: "srcx/a.go"},
		{name: "file entry", files: "[src/a.go]", changed: []string{"src/a.go", "src/b.go", "src/a.go.bak"}, want: "src/b.go, src/a.go.bak"},
		{name: "not code", files: "[]", changed: []string{planPath, "docs/x.png", "notes.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context(t, repo(t, map[string]string{planPath: planAt("feat", "build", tc.files)}))
			ctx.Changed = tc.changed
			got, err := Scope(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("findings = %q; want none", messages(got))
				}
				return
			}
			want := "the branch changes code outside the plan's files: list: " + tc.want
			if len(got) != 1 || got[0].Message != want || got[0].Severity != Warn || got[0].Field != "files" {
				t.Fatalf("findings = %+v; want one warning %q", got, want)
			}
		})
	}
	if got, err := Scope(Context{Changed: []string{"src/a.go"}}); err != nil || len(got) != 0 {
		t.Fatalf("Scope with no unit = %q, %v; want none", messages(got), err)
	}
}

// Test files (TESTING.md test_files) are not code for the scope check; without
// TESTING.md there are none, so they count.
func TestScopeIgnoresTestFiles(t *testing.T) {
	testingMD := "---\nlyceum: 1\nsuite: go test ./...\ntest_files: [\"**/*_test.go\"]\n---\n"
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{name: "with TESTING.md", files: map[string]string{"TESTING.md": testingMD}, want: "src/b.go"},
		{name: "without TESTING.md", files: map[string]string{}, want: "src/a_test.go, src/b.go"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.files[planPath] = planAt("bug", "diagnose", "[src/a.go]")
			ctx := context(t, repo(t, tc.files))
			ctx.Changed = []string{"src/a.go", "src/a_test.go", "src/b.go"}
			got, err := Scope(ctx)
			if err != nil {
				t.Fatal(err)
			}
			want := "the branch changes code outside the plan's files: list: " + tc.want
			if len(got) != 1 || got[0].Message != want {
				t.Fatalf("findings = %q; want %q", messages(got), want)
			}
		})
	}
}

// A trivial chore has no scope check; the Chore check covers it. Once its
// plan clears, the scope check runs.
func TestScopeTrivialChore_AC29(t *testing.T) {
	add := events.Event{Type: "add", Unit: "aaaaaa-x", Fields: map[string]any{"unit_type": "chore", "branch": "chore/aaaaaa-x", "gate": "build", "status": "in-progress"}}
	planApproved := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "plan", "counts": true}}
	for _, tc := range []struct {
		name string
		evs  []events.Event
		want int
	}{
		{name: "trivial", evs: []events.Event{add}},
		{name: "plan approved", evs: []events.Event{add, planApproved}, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context(t, repo(t, map[string]string{planPath: planAt("chore", "build", "")}), tc.evs...)
			ctx.Changed = []string{"package.json"}
			got, err := Scope(ctx)
			if err != nil || len(got) != tc.want {
				t.Fatalf("Scope = %q, %v; want %d findings", messages(got), err, tc.want)
			}
		})
	}
}

func TestAllRunsGateAndScope(t *testing.T) {
	add := events.Event{Type: "add", Unit: "aaaaaa-x", Fields: map[string]any{"unit_type": "feat", "branch": "feat/aaaaaa-x", "gate": "spec", "status": "planned"}}
	ctx := context(t, repo(t, map[string]string{planPath: plan}), add)
	ctx.Branch = "feat/aaaaaa-x"
	ctx.Changed = []string{"src/a.go"}
	got, err := Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var checks []string
	for _, f := range got {
		checks = append(checks, f.Check)
	}
	if !slices.Equal(checks, []string{GateCheck, ScopeCheck}) {
		t.Fatalf("checks = %q; want %q", checks, []string{GateCheck, ScopeCheck})
	}
}

func TestChore_AC28_AC29(t *testing.T) {
	addChore := events.Event{Type: "add", Unit: "aaaaaa-x", Fields: map[string]any{"unit_type": "chore", "branch": "chore/aaaaaa-x", "gate": "build", "status": "in-progress"}}
	planApproved := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "plan", "counts": true}}
	planNotCounting := events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "plan", "counts": false}}
	for _, tc := range []struct {
		name      string
		typ, gate string
		changed   []string
		evs       []events.Event
		want      string // "" for no finding
	}{
		{name: "AC28 md only", typ: "chore", gate: "build", changed: []string{"README.md", "CHANGELOG.md", "sub/notes.md"}},
		{name: "docs and text", typ: "chore", gate: "build", changed: []string{planPath, "docs/x.png", "notes.txt"}},
		{name: "AC29 manifest", typ: "chore", gate: "build", changed: []string{"README.md", "package.json"}, want: "package.json"},
		{name: "lockfile", typ: "chore", gate: "build", changed: []string{"package-lock.json", "go.sum"}, want: "package-lock.json, go.sum"},
		{name: "ci config", typ: "chore", gate: "build", changed: []string{".github/workflows/ci.yml"}, want: ".github/workflows/ci.yml"},
		{name: "code", typ: "chore", gate: "build", changed: []string{"src/a.go"}, evs: []events.Event{addChore}, want: "src/a.go"},
		{name: "plan approved", typ: "chore", gate: "build", changed: []string{"package.json"}, evs: []events.Event{addChore, planApproved}},
		{name: "plan approval not counting", typ: "chore", gate: "build", changed: []string{"package.json"}, evs: []events.Event{addChore, planNotCounting}, want: "package.json"},
		// On its planned tier before the plan clears, GateBeforeCode reports it.
		{name: "planned chore at plan", typ: "chore", gate: "plan", changed: []string{"package.json"}},
		{name: "not a chore", typ: "doc", gate: "draft", changed: []string{"package.json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context(t, repo(t, map[string]string{planPath: planAt(tc.typ, tc.gate, "")}), tc.evs...)
			ctx.Changed = tc.changed
			got, err := Chore(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("findings = %q; want none", messages(got))
				}
				return
			}
			want := "the chore changes code, a manifest, a lockfile or CI config without an approved plan: " + tc.want
			if len(got) != 1 || got[0].Message != want || got[0].Check != ChoreCheck || got[0].Severity != Fail || got[0].Unit != "aaaaaa-x" {
				t.Fatalf("findings = %+v; want one failing %q", got, want)
			}
		})
	}
	if got, err := Chore(Context{Changed: []string{"package.json"}}); err != nil || len(got) != 0 {
		t.Fatalf("Chore with no unit = %q, %v; want none", messages(got), err)
	}
}

func TestNoUnit_AC48_AC49(t *testing.T) {
	for _, tc := range []struct {
		name    string
		changed []string
		want    string // "" for no finding
	}{
		{name: "AC48 code", changed: []string{"README.md", "src/main.go"}, want: "src/main.go"},
		{name: "AC49 docs only", changed: []string{"README.md", "docs/x.md"}},
		{name: "manifest, lockfile and ci config", changed: []string{".github/workflows/ci.yml", "go.mod", "go.sum"}, want: ".github/workflows/ci.yml, go.mod, go.sum"},
		{name: "no diff", changed: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NoUnit(Context{Root: t.TempDir(), Branch: "topic", Changed: tc.changed})
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("findings = %q; want none", messages(got))
				}
				return
			}
			want := "the branch has no unit and changes code, a manifest, a lockfile or CI config: " + tc.want
			if len(got) != 1 || got[0].Message != want || got[0].Check != NoUnitCheck || got[0].Severity != Fail || got[0].Unit != "" {
				t.Fatalf("findings = %+v; want one failing %q", got, want)
			}
		})
	}
	ctx := context(t, repo(t, map[string]string{planPath: plan}))
	ctx.Changed = []string{"src/main.go"}
	if got, err := NoUnit(ctx); err != nil || len(got) != 0 {
		t.Fatalf("NoUnit with a unit = %q, %v; want none", messages(got), err)
	}
}

// TestCountingAmend_AC35: an amend invalidates, of the gates it lists, the
// unit's counting approvals from its latest counting approval of its gate on;
// earlier ones, other units' and non-counting ones are left as they are. A
// second amend reaches back only to the approval given after the first.
// Amended holds until the gate is approved again.
func TestCountingAmend_AC35(t *testing.T) {
	ev := func(typ, unit string, f map[string]any) events.Event {
		return events.Event{Type: typ, Unit: unit, Fields: f}
	}
	evs := []events.Event{
		ev("approve", "aaaaaa-x", map[string]any{"gate": "intent", "counts": true}),
		ev("approve", "aaaaaa-x", map[string]any{"gate": "tests", "counts": true}),
		ev("approve", "aaaaaa-x", map[string]any{"gate": "plan", "counts": true}),
		ev("approve", "aaaaaa-x", map[string]any{"gate": "tests", "counts": false}),
		ev("approve", "bbbbbb-y", map[string]any{"gate": "plan", "counts": true}),
		ev("approve", "aaaaaa-x", map[string]any{"gate": "tests", "counts": true}),
		ev("amend", "aaaaaa-x", map[string]any{"gate": "plan", "invalidated": []any{"plan", "tests"}, "reason": "r1"}),
	}
	countsOf := func(evs []events.Event) []bool {
		var c []bool
		for _, e := range evs {
			if e.Type == "approve" {
				c = append(c, e.Fields["counts"] != false)
			}
		}
		return c
	}
	if got, want := countsOf(Counting(evs)), []bool{true, true, false, false, true, false}; !slices.Equal(got, want) {
		t.Errorf("after one amend, counts = %v, want %v", got, want)
	}
	if countsOf(evs)[2] != true {
		t.Error("Counting changed the events it was given")
	}
	if got := Invalidates(Counting(evs), "aaaaaa-x", "plan"); got != nil {
		t.Errorf("Invalidates after the amend = %v, want none: the plan approval no longer counts", got)
	}
	if a, ok := Amended(evs, "aaaaaa-x"); !ok || a.Fields["reason"] != "r1" {
		t.Errorf("Amended = %v, %v; want the r1 amend", a.Fields, ok)
	}

	evs = append(evs,
		ev("approve", "aaaaaa-x", map[string]any{"gate": "plan", "counts": true}),
	)
	if _, ok := Amended(evs, "aaaaaa-x"); ok {
		t.Error("Amended holds after the plan is approved again")
	}
	evs = append(evs,
		ev("amend", "aaaaaa-x", map[string]any{"gate": "plan", "invalidated": []any{"plan"}, "reason": "r2"}),
	)
	if got, want := countsOf(Counting(evs)), []bool{true, true, false, false, true, false, false}; !slices.Equal(got, want) {
		t.Errorf("after two amends, counts = %v, want %v", got, want)
	}
	if a, ok := Amended(evs, "aaaaaa-x"); !ok || a.Fields["reason"] != "r2" {
		t.Errorf("Amended = %v, %v; want the r2 amend", a.Fields, ok)
	}
}
