package cli

import (
	"testing"

	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/unit"
)

func TestUnitProgress(t *testing.T) {
	no := false
	const bugNext = `write the root cause, commit the fix, then run "lyceum test": verify clears on a passing run`
	for _, tc := range []struct {
		name, typ, gate, status string
		approvals               []unit.Approval
		last, next              string
		class                   gate.Class
	}{
		{"first gate", "feat", "spec", unit.StatusPlanned, nil, "", `run "lyceum approve spec" yourself in a terminal`, gate.Judgment},
		{"later gate", "feat", "verify", unit.StatusInProgress, nil, "build", `run "lyceum test"`, gate.Mechanical},
		{"bug at diagnose", "bug", "diagnose", unit.StatusInProgress, nil, "repro", bugNext, gate.Step},
		{"bug at fix", "bug", "fix", unit.StatusInProgress, nil, "diagnose", bugNext, gate.Step},
		{"at close", "feat", "close", unit.StatusInProgress, nil, "verify", `run "lyceum close"`, gate.Step},
		{"closed", "feat", "close", unit.StatusClosed, nil, "close", "nothing: the unit is closed", gate.Step},
		{"trivial chore", "chore", "build", unit.StatusInProgress, nil, "", "finish the build work described in the plan", gate.Step},
		{"planned chore at plan", "chore", "plan", unit.StatusPlanned, nil, "", `run "lyceum approve plan" yourself in a terminal`, gate.Judgment},
		{"planned chore past plan", "chore", "build", unit.StatusInProgress, []unit.Approval{{Gate: "plan"}}, "plan", "finish the build work described in the plan", gate.Step},
		{"chore with a plan approval that does not count", "chore", "build", unit.StatusInProgress, []unit.Approval{{Gate: "plan", Counts: &no}}, "", "finish the build work described in the plan", gate.Step},
		{"unknown gate", "feat", "nope", unit.StatusPlanned, nil, "", `run "lyceum check" to see what is wrong with the plan`, gate.Step},
	} {
		p := &unit.Plan{ID: "6ae678-x", Type: tc.typ, Gate: tc.gate, Status: tc.status, Approvals: tc.approvals}
		last, next, class := unitProgress(p)
		got := ""
		if last != nil {
			got = *last
		}
		if got != tc.last || next != tc.next || class != tc.class {
			t.Errorf("%s: unitProgress = %q, %q, %v; want %q, %q, %v", tc.name, got, next, class, tc.last, tc.next, tc.class)
		}
	}
}

func TestOwnBranch(t *testing.T) {
	for _, tc := range []struct {
		ref, branch string
		want        bool
	}{
		{"refs/heads/feat/6ae678-x", "feat/6ae678-x", true},
		{"refs/remotes/origin/feat/6ae678-x", "feat/6ae678-x", true},
		{"refs/heads/main", "feat/6ae678-x", false},
		{"refs/heads/xfeat/6ae678-x", "feat/6ae678-x", false},
		{"refs/remotes/origin/main", "feat/6ae678-x", false},
	} {
		if got := ownBranch(tc.ref, tc.branch); got != tc.want {
			t.Errorf("ownBranch(%q, %q) = %v; want %v", tc.ref, tc.branch, got, tc.want)
		}
	}
}
