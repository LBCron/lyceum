package evidence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gittest"
	"github.com/oritromax/lyceum/internal/runner"
)

const testingMD = "---\nlyceum: 1\nsuite: sh ./t.sh\n---\n"

// repo returns a repo with TESTING.md committed and, when approve is set, a
// counting testing approval of it in the event log.
func repo(t *testing.T, approve bool) string {
	t.Helper()
	root := gittest.InitWithCommit(t)
	if err := os.MkdirAll(filepath.Join(root, "lyceum"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, root, "TESTING.md", testingMD)
	if approve {
		h, err := approval.Hash(filepath.Join(root, "TESTING.md"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := events.Append(root, events.Event{Type: "approve", Fields: map[string]any{
			"gate": "testing", "hashes": map[string]string{"TESTING.md": h}, "counts": true,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	gittest.Git(t, root, "add", "-A")
	gittest.Git(t, root, "commit", "-q", "-m", "setup")
	return root
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// record is a passing, clean, full-suite record at HEAD with TESTING.md's hash now.
func record(t *testing.T, root string) runner.Record {
	t.Helper()
	h, err := approval.Hash(filepath.Join(root, "TESTING.md"))
	if err != nil {
		t.Fatal(err)
	}
	return runner.Record{Command: "sh ./t.sh", Commit: gittest.Git(t, root, "rev-parse", "--short", "HEAD"), TestingSHA256: h}
}

// check runs Check and returns the reasons' Why, joined by "; ".
func check(t *testing.T, root string, rec runner.Record) (bool, string) {
	t.Helper()
	ok, reasons, err := Check(root, rec)
	if err != nil {
		t.Fatal(err)
	}
	var whys []string
	for _, r := range reasons {
		whys = append(whys, r.Why)
	}
	if ok != (len(reasons) == 0) {
		t.Fatalf("ok = %v with reasons %q", ok, whys)
	}
	return ok, strings.Join(whys, "; ")
}

func TestCheckEvidence(t *testing.T) {
	root := repo(t, true)
	if ok, why := check(t, root, record(t, root)); !ok {
		t.Errorf("a clean full-suite run at HEAD with TESTING.md approved is not evidence: %s", why)
	}
}

func TestCheckSubset_AC57(t *testing.T) {
	root := repo(t, true)
	rec := record(t, root)
	subset := "unit"
	rec.Subset = &subset
	if ok, why := check(t, root, rec); ok || why != "it ran subset unit, not the full suite" {
		t.Errorf("subset run: ok %v, %q", ok, why)
	}
}

// A repro run is never full-suite gate evidence, and only a repro run clears repro.
// A refactor's benchmark run is recorded like a run, but never gate evidence.
func TestCheckBenchmark(t *testing.T) {
	root := repo(t, true)
	rec := record(t, root)
	rec.Benchmark = true
	if ok, why := check(t, root, rec); ok || why != "it ran the benchmark, not the full suite" {
		t.Errorf("benchmark run: ok %v, %q", ok, why)
	}
}

func TestCheckRepro(t *testing.T) {
	root := repo(t, true)
	rec := record(t, root)
	name := "TestLoginPlus"
	rec.Repro = &name
	if ok, why := check(t, root, rec); ok || why != "it ran the repro test TestLoginPlus, not the full suite" {
		t.Errorf("repro run as suite evidence: ok %v, %q", ok, why)
	}
	if ok, reasons, err := Repro(root, rec); err != nil || !ok {
		t.Errorf("repro run for repro: ok %v, %+v, %v", ok, reasons, err)
	}
	rec.Dirty = true
	if ok, reasons, err := Repro(root, rec); err != nil || ok || len(reasons) != 1 || reasons[0].Why != "the working tree had uncommitted changes" {
		t.Errorf("dirty repro run: ok %v, %+v, %v", ok, reasons, err)
	}
	if ok, reasons, err := Repro(root, record(t, root)); err != nil || ok || len(reasons) != 1 || reasons[0].Why != "it did not run a repro test" {
		t.Errorf("suite run for repro: ok %v, %+v, %v", ok, reasons, err)
	}
}

func TestCheckDirty_AC59(t *testing.T) {
	root := repo(t, true)
	rec := record(t, root)
	rec.Dirty = true
	if ok, why := check(t, root, rec); ok || why != "the working tree had uncommitted changes" {
		t.Errorf("dirty run: ok %v, %q", ok, why)
	}
}

func TestCheckTimedOut_AC112(t *testing.T) {
	root := repo(t, true)
	rec := record(t, root)
	rec.TimedOut, rec.Exit = true, 137
	if ok, why := check(t, root, rec); ok || why != "it ran over the timeout and was killed" {
		t.Errorf("timed-out run: ok %v, %q", ok, why)
	}
}

func TestCheckTesting_AC60(t *testing.T) {
	root := repo(t, false)
	if ok, why := check(t, root, record(t, root)); ok || why != "TESTING.md is not approved" {
		t.Errorf("unapproved TESTING.md: ok %v, %q", ok, why)
	}

	// Edited and committed after its approval: the run read the edited file.
	root = repo(t, true)
	write(t, root, "TESTING.md", testingMD+"\nedited\n")
	gittest.Git(t, root, "commit", "-q", "-am", "edit")
	if ok, why := check(t, root, record(t, root)); ok || why != "TESTING.md changed since its approval" {
		t.Errorf("edited TESTING.md: ok %v, %q", ok, why)
	}

	// The run read an edited file, since restored: it still ran under the wrong file.
	root = repo(t, true)
	rec := record(t, root)
	rec.TestingSHA256 = "sha256:other"
	ok, reasons, err := Check(root, rec)
	if err != nil {
		t.Fatal(err)
	}
	if ok || len(reasons) != 1 || !reasons[0].Testing || !strings.Contains(reasons[0].Next, "lyceum approve testing") {
		t.Errorf("run under another TESTING.md: ok %v, reasons %+v", ok, reasons)
	}
}

func TestCheckHeadMoved_AC113(t *testing.T) {
	root := repo(t, true)
	rec := record(t, root)
	gittest.Git(t, root, "commit", "-q", "--allow-empty", "-m", "B")
	head := gittest.Git(t, root, "rev-parse", "--short", "HEAD")
	want := "it ran at commit " + rec.Commit + ", but HEAD is now " + head
	if ok, why := check(t, root, rec); ok || why != want {
		t.Errorf("run at an old commit: ok %v, %q; want %q", ok, why, want)
	}
	if ok, why := check(t, root, record(t, root)); !ok {
		t.Errorf("a fresh record at the new HEAD is not evidence: %s", why)
	}
}

func TestCheckAllReasons(t *testing.T) {
	root := repo(t, false)
	rec := record(t, root)
	subset := "unit"
	rec.Subset, rec.Dirty, rec.TimedOut, rec.Commit = &subset, true, true, "abc1234"
	if _, reasons, err := Check(root, rec); err != nil || len(reasons) != 5 {
		t.Errorf("Check = %+v, %v; want 5 reasons", reasons, err)
	}
}
