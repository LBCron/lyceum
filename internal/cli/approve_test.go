package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/gittest"
	"github.com/oritromax/lyceum/internal/unit"
)

// A counting approval of the code-opening gate moves a planned unit to
// in-progress, in the plan and in the event; one that does not count moves
// nothing.
func TestRecordApprovalStatus(t *testing.T) {
	root := gittest.InitWithCommit(t)
	planPath := "lyceum/plan/6ae678-csv-export.md"
	full := filepath.Join(root, filepath.FromSlash(planPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := unit.NewPlan("6ae678-csv-export", "feat", "CSV export", "feat/6ae678-csv-export", unit.StatusPlanned, "spec")
	if err != nil {
		t.Fatal(err)
	}
	if err := unit.WritePlan(full, p); err != nil {
		t.Fatal(err)
	}
	files := []string{planPath}
	hashes := map[string]string{planPath: "sha256:0"}

	for _, c := range []struct {
		owner      string
		wantStatus string
		wantField  any
	}{
		{"someone@example.com", unit.StatusPlanned, nil},
		{gittest.Email, unit.StatusInProgress, unit.StatusInProgress},
	} {
		_, counts, err := recordApproval(root, planPath, p.ID, c.owner, false, gate.Feat, gate.Spec, gate.Build, files, hashes, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := unit.ReadPlan(full)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != c.wantStatus {
			t.Errorf("owner %s (counts %v): plan status = %s; want %s", c.owner, counts, got.Status, c.wantStatus)
		}
		evs, err := events.Read(root)
		if err != nil {
			t.Fatal(err)
		}
		if f := evs[len(evs)-1].Fields["status"]; f != c.wantField {
			t.Errorf("owner %s (counts %v): event status = %v; want %v", c.owner, counts, f, c.wantField)
		}
	}
}

// approve tests is allowed for a bug after repro and before cause, and for a
// refactor at verify; anywhere else it is refused, naming the gate.
func TestTestsApprovable(t *testing.T) {
	for _, c := range []struct {
		typ  gate.Type
		g    gate.Gate
		want bool
	}{
		{gate.Bug, gate.Repro, false},
		{gate.Bug, gate.Diagnose, true},
		{gate.Bug, gate.Fix, true},
		{gate.Bug, gate.Verify, true},
		{gate.Bug, gate.Cause, false},
		{gate.Refactor, gate.Baseline, true},
		{gate.Refactor, gate.Build, true},
		{gate.Refactor, gate.Plan, false},
		{gate.Refactor, gate.Verify, true},
		{gate.Feat, gate.Verify, false},
	} {
		p := &unit.Plan{ID: "aaaaaa-x", Type: string(c.typ), Gate: string(c.g)}
		class, err := gate.Kind(c.typ, c.g)
		if err != nil {
			t.Fatal(err)
		}
		err = testsApprovable(p, c.typ, c.g, class)
		if got := err == nil; got != c.want {
			t.Errorf("%s at %s: allowed %v, want %v (%v)", c.typ, c.g, got, c.want, err)
		}
		if err != nil && !strings.Contains(err.Error(), "at gate "+string(c.g)) {
			t.Errorf("%s at %s: refusal %q does not name the gate", c.typ, c.g, err)
		}
	}
}

// A refactor's approve tests needs the baseline fingerprint; with one, the
// files are those that differ from it.
func TestTestsToApproveRefactor(t *testing.T) {
	root, p := planAt(t, "aaaaaa-x", "refactor", "verify", unit.StatusInProgress)
	if _, _, err := testsToApprove(root, "", p, gate.Refactor); err == nil || !strings.Contains(err.Error(), "no baseline fingerprint") {
		t.Fatalf("no baseline clear: err = %v, want no baseline fingerprint", err)
	}
	writeFile(t, root, "a_test.go", "a\n")
	writeFile(t, root, "b_test.go", "b\n")
	fp, err := fingerprint(root, []string{"a_test.go", "b_test.go"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := events.Append(root, events.Event{Type: "gate-clear", Unit: p.ID, Fields: map[string]any{"gate": "baseline", "fingerprint": fp}}); err != nil {
		t.Fatal(err)
	}
	if files, _, err := testsToApprove(root, "", p, gate.Refactor); err != nil || len(files) != 0 {
		t.Fatalf("as fingerprinted: %q, %v; want none", files, err)
	}
	writeFile(t, root, "b_test.go", "b edited\n")
	if files, _, err := testsToApprove(root, "", p, gate.Refactor); err != nil || !reflect.DeepEqual(files, []string{"b_test.go"}) {
		t.Fatalf("edited: %q, %v; want b_test.go", files, err)
	}
	if err := os.Remove(filepath.Join(root, "a_test.go")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := testsToApprove(root, "", p, gate.Refactor); err == nil || !strings.Contains(err.Error(), "gone") {
		t.Fatalf("gone: err = %v, want a refusal naming the gone file", err)
	}
}
