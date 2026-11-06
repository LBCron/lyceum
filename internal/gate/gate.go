// Package gate holds the fixed gate list of each unit type and the transitions
// between gates (units.md, "Gate lists").
//
// A transition here is a unit moving forward to its next gate. Re-opening an
// earlier gate (amend; plan --edit) is not a transition of this table.
package gate

import "fmt"

// Type is a unit type, as written in the plan's type field, or PlannedChore.
type Type string

// Unit types (units.md, "Unit types").
const (
	Feat     Type = "feat"
	Bug      Type = "bug"
	Refactor Type = "refactor"
	Chore    Type = "chore"
	Doc      Type = "doc"
	Test     Type = "test"
)

// PlannedChore is a chore on its planned tier. It is not a plan type:
// a chore starts on the trivial tier (Chore) at build, and `lyceum plan`
// moves it to plan, after which its gates follow PlannedChore.
const PlannedChore Type = "planned chore"

// Gate is a gate name, as written in the plan's gate field.
type Gate string

// Gate names, in the order they first appear in the gate lists.
const (
	Spec     Gate = "spec"
	Build    Gate = "build"
	Verify   Gate = "verify"
	Close    Gate = "close"
	Repro    Gate = "repro"
	Diagnose Gate = "diagnose"
	Fix      Gate = "fix"
	Cause    Gate = "cause"
	Intent   Gate = "intent"
	Baseline Gate = "baseline"
	Plan     Gate = "plan"
	Draft    Gate = "draft"
	Write    Gate = "write"
	Run      Gate = "run"
)

// Class is how a gate clears.
type Class int

// Gate classes (units.md, "Gate lists").
const (
	// Step is a work step: neither mechanical nor judgment.
	Step Class = iota
	// Mechanical clears on a CLI-recorded run.
	Mechanical
	// Judgment clears by owner approval.
	Judgment
)

func (c Class) String() string {
	switch c {
	case Mechanical:
		return "M"
	case Judgment:
		return "J"
	default:
		return "step"
	}
}

type entry struct {
	gate  Gate
	class Class
}

// lists are the gate lists in order (units.md, "Gate lists"). Bug's
// `approve cause --no-repro` skips repro and moves the bug to diagnose: the
// same transition as clearing repro. Refactor's characterization-test and
// test-diff approvals are conditions on baseline and verify, not gates.
var lists = map[Type][]entry{
	Feat:         {{Spec, Judgment}, {Build, Step}, {Verify, Mechanical}, {Close, Step}},
	Bug:          {{Repro, Mechanical}, {Diagnose, Step}, {Fix, Step}, {Verify, Mechanical}, {Cause, Judgment}, {Close, Step}},
	Refactor:     {{Intent, Judgment}, {Baseline, Mechanical}, {Plan, Judgment}, {Build, Step}, {Verify, Mechanical}, {Close, Step}},
	Chore:        {{Build, Step}, {Close, Step}},
	PlannedChore: {{Plan, Judgment}, {Build, Step}, {Verify, Mechanical}, {Close, Step}},
	Doc:          {{Draft, Step}, {Verify, Mechanical}, {Close, Step}},
	Test:         {{Write, Step}, {Run, Mechanical}, {Close, Step}},
}

// extra are the legal transitions that are not a move to the next gate in a
// list: `lyceum plan` moves a trivial chore from build to plan, the first gate
// of PlannedChore.
var extra = map[Type][][2]Gate{
	Chore: {{Build, Plan}},
}

// codeOpening is the gate that must clear before application code may be
// written (units.md, "Code-opening gate"). For bug, the --no-repro skip
// counts as clearing repro. Types not listed have none.
var codeOpening = map[Type]Gate{
	Feat:         Spec,
	Bug:          Repro,
	Refactor:     Plan,
	PlannedChore: Plan,
}

func list(t Type) ([]entry, error) {
	l, ok := lists[t]
	if !ok {
		return nil, fmt.Errorf("unknown unit type %q", t)
	}
	return l, nil
}

func index(t Type, g Gate) ([]entry, int, error) {
	l, err := list(t)
	if err != nil {
		return nil, 0, err
	}
	for i, e := range l {
		if e.gate == g {
			return l, i, nil
		}
	}
	return nil, 0, fmt.Errorf("%s has no gate %q", t, g)
}

// Gates returns t's gates in order.
func Gates(t Type) ([]Gate, error) {
	l, err := list(t)
	if err != nil {
		return nil, err
	}
	gates := make([]Gate, len(l))
	for i, e := range l {
		gates[i] = e.gate
	}
	return gates, nil
}

// Initial returns the gate a new unit of type t starts at. PlannedChore has
// none: a chore starts at Chore's build.
func Initial(t Type) (Gate, error) {
	if t == PlannedChore {
		return "", fmt.Errorf("a planned chore starts as a chore at %s; `lyceum plan` moves it to %s", Build, Plan)
	}
	l, err := list(t)
	if err != nil {
		return "", err
	}
	return l[0].gate, nil
}

// Next returns the gate after current in t's list. For Chore at build that is
// close; `lyceum plan` instead moves it to plan (see Allowed).
func Next(t Type, current Gate) (Gate, error) {
	l, i, err := index(t, current)
	if err != nil {
		return "", err
	}
	if i == len(l)-1 {
		return "", fmt.Errorf("%s is the last gate of %s", current, t)
	}
	return l[i+1].gate, nil
}

// Pending returns the gate a unit of type t at gate g waits on to clear, and
// its class: g itself unless it is a work step, else the first gate after the
// work steps from g on, which ends them. ok is false when only work
// steps follow g, or t has no gate g.
func Pending(t Type, g Gate) (Gate, Class, bool) {
	l, i, err := index(t, g)
	if err != nil {
		return "", Step, false
	}
	for ; i < len(l); i++ {
		if l[i].class != Step {
			return l[i].gate, l[i].class, true
		}
	}
	return "", Step, false
}

// Kind returns how gate g of type t clears.
func Kind(t Type, g Gate) (Class, error) {
	l, i, err := index(t, g)
	if err != nil {
		return Step, err
	}
	return l[i].class, nil
}

// Allowed reports whether a unit of type t may move from gate from to gate to.
// Unknown types and gates are never allowed.
func Allowed(t Type, from, to Gate) bool {
	if next, err := Next(t, from); err == nil && next == to {
		return true
	}
	for _, e := range extra[t] {
		if e == [2]Gate{from, to} {
			return true
		}
	}
	return false
}

// CodeOpening returns t's code-opening gate: application code may be written
// only once it has cleared. ok is false for types that never open code
// (Chore on its trivial tier, Doc, Test).
func CodeOpening(t Type) (g Gate, ok bool) {
	g, ok = codeOpening[t]
	return g, ok
}
