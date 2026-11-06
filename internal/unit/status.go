package unit

import (
	"slices"

	"github.com/oritromax/lyceum/internal/gate"
)

// InitialStatus is the status a new unit of type t starts at (units.md, "Status
// transitions"): in-progress for types with no code-opening gate (doc,
// test and the trivial chore), planned for the rest.
func InitialStatus(t gate.Type) string {
	if _, ok := gate.CodeOpening(t); ok {
		return StatusPlanned
	}
	return StatusInProgress
}

// StatusAfter is the status of a unit of type t at status current once its gate
// cleared has cleared (units.md, "Status transitions"): planned becomes
// in-progress at the code-opening gate, in-progress becomes implemented at
// verify (test: run), and implemented becomes closed at close. Any other clear,
// or a gate t does not have, leaves current as it is.
func StatusAfter(t gate.Type, cleared gate.Gate, current string) string {
	if _, err := gate.Kind(t, cleared); err != nil {
		return current
	}
	if opening, ok := gate.CodeOpening(t); ok && cleared == opening && current == StatusPlanned {
		return StatusInProgress
	}
	switch {
	case (cleared == gate.Verify || cleared == gate.Run) && current == StatusInProgress:
		return StatusImplemented
	case cleared == gate.Close && current == StatusImplemented:
		return StatusClosed
	}
	return current
}

// Tier is the type whose gate list a unit of plan type typ at gate current
// follows, with approvals its plan approvals: typ itself, except that a chore
// is on its planned tier when its gate is one the trivial tier
// lacks, or when the owner approved its plan. An approval an amend invalidated
// still says so: the amend moved the unit back to plan, and it never returns
// to the trivial tier. A non-owner's approval (counts: false) cleared nothing
// and says nothing. `lyceum plan`'s gate-reopen event needs no rule of its
// own: it moves the chore to plan, and only the owner's approval moves it on.
func Tier(typ, current string, approvals []Approval) gate.Type {
	t := gate.Type(typ)
	if t != gate.Chore {
		return t
	}
	if _, err := gate.Kind(t, gate.Gate(current)); err != nil {
		return gate.PlannedChore
	}
	if slices.ContainsFunc(approvals, func(a Approval) bool { return a.Gate == string(gate.Plan) && a.Counts == nil }) {
		return gate.PlannedChore
	}
	return t
}
