package unit

import (
	"testing"

	"github.com/oritromax/lyceum/internal/gate"
)

var statuses = []string{StatusPlanned, StatusInProgress, StatusImplemented, StatusClosed}

func TestInitialStatus(t *testing.T) {
	for typ, want := range map[gate.Type]string{
		gate.Feat: StatusPlanned, gate.Bug: StatusPlanned, gate.Refactor: StatusPlanned,
		gate.Chore: StatusInProgress, gate.Doc: StatusInProgress, gate.Test: StatusInProgress,
	} {
		if got := InitialStatus(typ); got != want {
			t.Errorf("InitialStatus(%s) = %s; want %s", typ, got, want)
		}
	}
}

// TestStatusAfter checks every gate of every type against every status. moves
// is the status table, by hand from the spec: a (gate, from) pair not listed
// leaves the status as it is.
func TestStatusAfter(t *testing.T) {
	type move struct {
		g    gate.Gate
		from string
	}
	opening := func(g gate.Gate) move { return move{g, StatusPlanned} }
	verify := move{gate.Verify, StatusInProgress}
	closing := move{gate.Close, StatusImplemented}
	moves := map[gate.Type]map[move]string{
		gate.Feat:     {opening(gate.Spec): StatusInProgress, verify: StatusImplemented, closing: StatusClosed},
		gate.Bug:      {opening(gate.Repro): StatusInProgress, verify: StatusImplemented, closing: StatusClosed},
		gate.Refactor: {opening(gate.Plan): StatusInProgress, verify: StatusImplemented, closing: StatusClosed},
		// A trivial chore has no verify, so the table never makes it implemented; only
		// the close rule applies.
		gate.Chore:        {closing: StatusClosed},
		gate.PlannedChore: {opening(gate.Plan): StatusInProgress, verify: StatusImplemented, closing: StatusClosed},
		gate.Doc:          {verify: StatusImplemented, closing: StatusClosed},
		gate.Test:         {{gate.Run, StatusInProgress}: StatusImplemented, closing: StatusClosed},
	}
	for typ, m := range moves {
		gates, err := gate.Gates(typ)
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range gates {
			for _, from := range statuses {
				want, ok := m[move{g, from}]
				if !ok {
					want = from
				}
				if got := StatusAfter(typ, g, from); got != want {
					t.Errorf("StatusAfter(%s, %s, %s) = %s; want %s", typ, g, from, got, want)
				}
			}
		}
	}

	// A gate the type does not have, or an unknown type, moves nothing.
	for _, c := range []struct {
		typ  gate.Type
		g    gate.Gate
		from string
	}{
		{gate.Test, gate.Verify, StatusInProgress},
		{gate.Doc, gate.Run, StatusInProgress},
		{gate.Chore, gate.Plan, StatusPlanned},
		{gate.Feat, gate.Repro, StatusPlanned},
		{"epic", gate.Verify, StatusInProgress},
		{"epic", gate.Close, StatusImplemented},
	} {
		if got := StatusAfter(c.typ, c.g, c.from); got != c.from {
			t.Errorf("StatusAfter(%q, %s, %s) = %s; want %s unchanged", c.typ, c.g, c.from, got, c.from)
		}
	}
}

// A chore's tier comes from one rule: the gate alone can't say, as
// build and close are on both tiers.
func TestTier_AC168(t *testing.T) {
	notCounting := false
	owner := []Approval{{Gate: "plan"}}
	nonOwner := []Approval{{Gate: "plan", Counts: &notCounting}}
	other := []Approval{{Gate: "tests"}}
	for _, tc := range []struct {
		name, typ, gate string
		approvals       []Approval
		want            gate.Type
	}{
		{"trivial at build", "chore", "build", nil, gate.Chore},
		{"trivial at close", "chore", "close", nil, gate.Chore},
		{"moved to plan", "chore", "plan", nil, gate.PlannedChore},
		{"at verify", "chore", "verify", nil, gate.PlannedChore},
		{"unknown gate", "chore", "nope", nil, gate.PlannedChore},
		{"plan approved, at build", "chore", "build", owner, gate.PlannedChore},
		{"plan approved, at close", "chore", "close", owner, gate.PlannedChore},
		{"non-owner approval at plan", "chore", "plan", nonOwner, gate.PlannedChore},
		{"non-owner approval says nothing", "chore", "build", nonOwner, gate.Chore},
		{"another gate's approval says nothing", "chore", "build", other, gate.Chore},
		{"not a chore", "feat", "build", owner, gate.Feat},
		{"refactor keeps its type", "refactor", "plan", nil, gate.Refactor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Tier(tc.typ, tc.gate, tc.approvals); got != tc.want {
				t.Fatalf("Tier(%s, %s) = %s; want %s", tc.typ, tc.gate, got, tc.want)
			}
		})
	}
}
