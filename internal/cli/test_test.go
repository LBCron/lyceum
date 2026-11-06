package cli

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/gittest"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/runner"
	"github.com/oritromax/lyceum/internal/unit"
)

func TestTail(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"a\n", "a\n"},
		{"a\nb\nc\n", "b\nc\n"},
		{"a\nb\nc", "b\nc\n"},
		{"\n\n\n", "\n\n"},
	} {
		if got := tail([]byte(tc.in), 2); got != tc.want {
			t.Errorf("tail(%q, 2) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// planAt commits a plan for unit id of type typ at gate g with status st in a new repo, and returns the repo and the plan.
func planAt(t *testing.T, id, typ, g, st string) (string, *unit.Plan) {
	t.Helper()
	root := gittest.InitWithCommit(t)
	p, err := unit.NewPlan(id, typ, "x", typ+"/"+id, st, g)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "lyceum/events.jsonl", "")
	if err := os.MkdirAll(filepath.Join(root, "lyceum", "plan"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unit.WritePlan(filepath.Join(root, "lyceum", "plan", id+".md"), p); err != nil {
		t.Fatal(err)
	}
	gittest.Git(t, root, "add", "-A")
	gittest.Git(t, root, "commit", "-q", "-m", "setup")
	return root, p
}

// TestAutoClear_AC65: a planned chore at verify, which needs only the run,
// clears: one gate-clear event inlining the run's full
// record, the status it moves to, the plan at close, and one commit of the plan
// and the log.
func TestAutoClear_AC65(t *testing.T) {
	id := "aaaaaa-x"
	root, _ := planAt(t, id, "chore", "verify", unit.StatusInProgress)
	planPath := filepath.Join(root, "lyceum", "plan", id+".md")
	p, err := unit.ReadPlan(planPath)
	if err != nil {
		t.Fatal(err)
	}
	rec := runner.Record{TS: "2026-01-01T00:00:00Z", Command: "sh ./t.sh", Commit: "abc1234",
		Output: "r.out", OutputSHA256: "sha256:o", TestingSHA256: "sha256:t", DurationMS: 5}
	var res out.Result
	line, err := autoClear(root, "", nil, p, rec, nil, &res)
	if err != nil {
		t.Fatal(err)
	}
	if line != "Cleared verify for "+id+"\n" || res.Gate != "close" {
		t.Errorf("autoClear = %q, gate %q", line, res.Gate)
	}
	evs, err := events.Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Type != "gate-clear" || evs[0].Unit != id || evs[0].Fields["gate"] != "verify" || evs[0].Fields["status"] != unit.StatusImplemented {
		t.Fatalf("events = %+v", evs)
	}
	run, _ := json.Marshal(evs[0].Fields["run"])
	want, _ := json.Marshal(rec)
	var got, wantMap map[string]any
	_ = json.Unmarshal(run, &got)
	_ = json.Unmarshal(want, &wantMap)
	if !reflect.DeepEqual(got, wantMap) {
		t.Errorf("run = %s; want the full record %s", run, want)
	}
	p, err = unit.ReadPlan(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if p.Gate != "close" || p.Status != unit.StatusImplemented {
		t.Errorf("plan at %s, %s; want close, implemented", p.Gate, p.Status)
	}
	if msg := gittest.Git(t, root, "log", "-1", "--format=%s"); msg != "lyceum: clear verify "+id {
		t.Errorf("commit %q", msg)
	}
	if files := gittest.Git(t, root, "show", "--name-only", "--format=", "HEAD"); files != "lyceum/events.jsonl\nlyceum/plan/"+id+".md" {
		t.Errorf("committed %q", files)
	}
}

// TestAutoClearNothing: no clear at a judgment gate, at a work step with no
// mechanical gate next, or at a bug's or refactor's verify with no repro or baseline clear in the log;
// the last ones say why.
func TestAutoClearNothing(t *testing.T) {
	for _, tc := range []struct{ typ, g, st, line string }{
		{"feat", "spec", unit.StatusPlanned, ""},
		{"chore", "build", unit.StatusInProgress, ""},
		{"bug", "diagnose", unit.StatusInProgress, "Not cleared: verify: the event log records no repro clear for aaaaaa-x\n"},
		{"refactor", "build", unit.StatusInProgress, "Not cleared: verify: the event log records no baseline clear for aaaaaa-x\n"},
		{"feat", "close", unit.StatusImplemented, ""},
		{"test", "write", unit.StatusInProgress, "Not cleared: run: the branch diff has no test file, so the new test is not in the run\n"},
	} {
		root, p := planAt(t, "aaaaaa-x", tc.typ, tc.g, tc.st)
		var res out.Result
		line, err := autoClear(root, "", nil, p, runner.Record{}, nil, &res)
		if err != nil {
			t.Fatal(err)
		}
		if line != tc.line {
			t.Errorf("%s at %s: %q; want %q", tc.typ, tc.g, line, tc.line)
		}
		if evs, _ := events.Read(root); len(evs) != 0 {
			t.Errorf("%s at %s: wrote %+v", tc.typ, tc.g, evs)
		}
	}
}

// TestUnmappedACs_AC62: TestExport_AC14 maps AC14; AC140 alone does not, and
// neither does a file outside test_files.
func TestUnmappedACs_AC62(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a_test.go", "func TestExport_AC140(t *testing.T) {}\n")
	writeFile(t, root, "notes.md", "AC14\n")
	globs := []string{"**/*_test.go"}
	got, err := unmappedACs(root, globs, []string{"AC14"})
	if err != nil || !reflect.DeepEqual(got, []string{"AC14"}) {
		t.Errorf("with AC140 only: %q, %v; want AC14 unmapped", got, err)
	}
	writeFile(t, root, "b_test.go", "func TestExport_AC14(t *testing.T) {}\n")
	if got, err := unmappedACs(root, globs, []string{"AC14", "AC140"}); err != nil || got != nil {
		t.Errorf("with TestExport_AC14: %q, %v; want none unmapped", got, err)
	}
	if got, err := unmappedACs(root, globs, nil); err != nil || got != nil {
		t.Errorf("no ACs: %q, %v; want none unmapped", got, err)
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"TestLoginPlus": `'TestLoginPlus'`,
		"a b; rm -rf x": `'a b; rm -rf x'`,
		"it's":          `'it'\''s'`,
		"":              `''`,
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s; want %s", in, got, want)
		}
	}
}

// TestFrozenChanged_AC105: a fingerprinted file edited or gone is changed; one
// as fingerprinted, or as the latest counting approve tests hashed it,
// is not. Without a repro clear in the log nothing is found.
func TestFrozenChanged_AC105(t *testing.T) {
	root, _ := planAt(t, "aaaaaa-x", "bug", "diagnose", unit.StatusInProgress)
	if _, _, _, found, err := frozenChanged(root, "aaaaaa-x", gate.Repro); err != nil || found {
		t.Fatalf("no repro clear: found %v, %v", found, err)
	}
	writeFile(t, root, "a_test.go", "a\n")
	writeFile(t, root, "b_test.go", "b\n")
	writeFile(t, root, "c_test.go", "c\n")
	fp, err := fingerprint(root, []string{"a_test.go", "b_test.go", "c_test.go"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := events.Append(root, events.Event{Type: "gate-clear", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "repro", "fingerprint": fp}}); err != nil {
		t.Fatal(err)
	}
	if changed, _, _, found, err := frozenChanged(root, "aaaaaa-x", gate.Repro); err != nil || !found || changed != nil {
		t.Fatalf("as fingerprinted: %q, %v, %v; want none changed", changed, found, err)
	}
	writeFile(t, root, "b_test.go", "b edited\n")
	if err := os.Remove(filepath.Join(root, "c_test.go")); err != nil {
		t.Fatal(err)
	}
	if changed, _, _, _, err := frozenChanged(root, "aaaaaa-x", gate.Repro); err != nil || !reflect.DeepEqual(changed, []string{"b_test.go", "c_test.go"}) {
		t.Fatalf("edited and gone: %q, %v; want b_test.go, c_test.go", changed, err)
	}

	// A non-owner's approval does not count; the owner's does, until the file changes again.
	h, err := approval.Hash(filepath.Join(root, "b_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, counts := range []bool{false, true} {
		if _, err := events.Append(root, events.Event{Type: "approve", Unit: "aaaaaa-x", Fields: map[string]any{"gate": "tests", "hashes": map[string]string{"b_test.go": h}, "counts": counts}}); err != nil {
			t.Fatal(err)
		}
		want := []string{"c_test.go"}
		if !counts {
			want = []string{"b_test.go", "c_test.go"}
		}
		if changed, _, _, _, err := frozenChanged(root, "aaaaaa-x", gate.Repro); err != nil || !reflect.DeepEqual(changed, want) {
			t.Fatalf("approved, counts %v: %q, %v; want %q", counts, changed, err, want)
		}
	}
	writeFile(t, root, "b_test.go", "b edited again\n")
	if changed, _, _, _, err := frozenChanged(root, "aaaaaa-x", gate.Repro); err != nil || !reflect.DeepEqual(changed, []string{"b_test.go", "c_test.go"}) {
		t.Fatalf("edited after approval: %q, %v; want b_test.go, c_test.go", changed, err)
	}
	// A refactor's baseline fingerprint is keyed the same way; this log has none.
	if _, _, _, found, err := frozenChanged(root, "aaaaaa-x", gate.Baseline); err != nil || found {
		t.Fatalf("no baseline clear: found %v, %v", found, err)
	}
}

// TestBaselineClear: a refactor's baseline does not clear while the branch
// diff has code, deletes a test file, or adds a test file no counting approve
// tests covers at its current hash; once approved it clears, fingerprinting
// every test file at HEAD.
func TestBaselineClear(t *testing.T) {
	id := "aaaaaa-x"
	root, _ := planAt(t, id, "refactor", "baseline", unit.StatusPlanned)
	writeFile(t, root, "a_test.go", "a\n")
	writeFile(t, root, "gone_test.go", "g\n")
	gittest.Git(t, root, "add", "-A")
	gittest.Git(t, root, "commit", "-q", "-m", "base")
	gittest.Git(t, root, "switch", "-q", "-c", "refactor/"+id)
	globs := []string{"**/*_test.go"}
	clear := func() string {
		t.Helper()
		p, err := unit.ReadPlan(filepath.Join(root, "lyceum", "plan", id+".md"))
		if err != nil {
			t.Fatal(err)
		}
		var res out.Result
		line, err := autoClear(root, "main", globs, p, runner.Record{}, nil, &res)
		if err != nil {
			t.Fatal(err)
		}
		return line
	}
	commit := func(msg string) {
		t.Helper()
		gittest.Git(t, root, "add", "-A")
		gittest.Git(t, root, "commit", "-q", "-m", msg)
	}

	writeFile(t, root, "b.go", "package b\n")
	commit("code")
	if line := clear(); line != "Not cleared: baseline: the branch changes files that are not tests, and before plan only characterization tests may change: b.go\n" {
		t.Errorf("code in the diff: %q", line)
	}
	gittest.Git(t, root, "rm", "-q", "b.go", "gone_test.go")
	gittest.Git(t, root, "commit", "-q", "-m", "drop code, delete a test")
	if line := clear(); line != "Not cleared: baseline: the branch deletes these test files, and the owner can't approve a deletion in this version: gone_test.go\n" {
		t.Errorf("deleted test: %q", line)
	}
	writeFile(t, root, "gone_test.go", "g\n")
	writeFile(t, root, "c_test.go", "c\n")
	commit("restore, add a characterization test")
	if line := clear(); line != "Not cleared: baseline: these test files in the branch diff are characterization tests, and they need the owner's approval at their current hash: c_test.go\n" {
		t.Errorf("unapproved: %q", line)
	}
	h, err := approval.Hash(filepath.Join(root, "c_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := events.Append(root, events.Event{Type: "approve", Unit: id, Fields: map[string]any{"gate": "tests", "hashes": map[string]string{"c_test.go": h}, "counts": true}}); err != nil {
		t.Fatal(err)
	}
	commit("approve")
	if line := clear(); line != "Cleared baseline for "+id+"\n" {
		t.Fatalf("approved: %q", line)
	}
	evs, err := events.Read(root)
	if err != nil {
		t.Fatal(err)
	}
	fp, _ := evs[len(evs)-1].Fields["fingerprint"].(map[string]any)
	if got := slices.Sorted(maps.Keys(fp)); !reflect.DeepEqual(got, []string{"a_test.go", "c_test.go", "gone_test.go"}) {
		t.Errorf("fingerprint of %q; want every test file at HEAD", got)
	}
}
