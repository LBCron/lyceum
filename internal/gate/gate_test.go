package gate

import (
	"slices"
	"testing"
)

// want is the gate table of units.md ("Gate lists"), written out by hand
// so the tests check the code against the spec, not against itself.
var want = map[Type][]struct {
	gate  Gate
	class Class
}{
	Feat:         {{Spec, Judgment}, {Build, Step}, {Verify, Mechanical}, {Close, Step}},
	Bug:          {{Repro, Mechanical}, {Diagnose, Step}, {Fix, Step}, {Verify, Mechanical}, {Cause, Judgment}, {Close, Step}},
	Refactor:     {{Intent, Judgment}, {Baseline, Mechanical}, {Plan, Judgment}, {Build, Step}, {Verify, Mechanical}, {Close, Step}},
	Chore:        {{Build, Step}, {Close, Step}},
	PlannedChore: {{Plan, Judgment}, {Build, Step}, {Verify, Mechanical}, {Close, Step}},
	Doc:          {{Draft, Step}, {Verify, Mechanical}, {Close, Step}},
	Test:         {{Write, Step}, {Run, Mechanical}, {Close, Step}},
}

// legal is every legal transition per type, by hand from the spec.
var legal = map[Type][][2]Gate{
	Feat: {{Spec, Build}, {Build, Verify}, {Verify, Close}},
	// repro → diagnose is also the --no-repro skip.
	Bug:      {{Repro, Diagnose}, {Diagnose, Fix}, {Fix, Verify}, {Verify, Cause}, {Cause, Close}},
	Refactor: {{Intent, Baseline}, {Baseline, Plan}, {Plan, Build}, {Build, Verify}, {Verify, Close}},
	// build → plan is `lyceum plan` moving a trivial chore to the planned tier.
	Chore:        {{Build, Close}, {Build, Plan}},
	PlannedChore: {{Plan, Build}, {Build, Verify}, {Verify, Close}},
	Doc:          {{Draft, Verify}, {Verify, Close}},
	Test:         {{Write, Run}, {Run, Close}},
}

var allTypes = []Type{Feat, Bug, Refactor, Chore, PlannedChore, Doc, Test}

// allGates is every gate name plus ones no list has, so pairs with unknown
// gates are refused too.
var allGates = []Gate{Spec, Build, Verify, Close, Repro, Diagnose, Fix, Cause, Intent, Baseline, Plan, Draft, Write, Run, "", "nope"}

func TestGateLists(t *testing.T) {
	for _, typ := range allTypes {
		gates, err := Gates(typ)
		if err != nil {
			t.Fatalf("Gates(%s): %v", typ, err)
		}
		var wantGates []Gate
		for _, w := range want[typ] {
			wantGates = append(wantGates, w.gate)
			c, err := Kind(typ, w.gate)
			if err != nil || c != w.class {
				t.Errorf("Kind(%s, %s) = %v, %v; want %v", typ, w.gate, c, err, w.class)
			}
		}
		if !slices.Equal(gates, wantGates) {
			t.Errorf("Gates(%s) = %v; want %v", typ, gates, wantGates)
		}
	}
	if len(lists) != len(want) {
		t.Errorf("lists has %d types; want %d", len(lists), len(want))
	}
}

func TestLegalTransitions(t *testing.T) {
	for _, typ := range allTypes {
		for _, tr := range legal[typ] {
			if !Allowed(typ, tr[0], tr[1]) {
				t.Errorf("Allowed(%s, %s, %s) = false; want true", typ, tr[0], tr[1])
			}
		}
	}
}

func TestIllegalTransitionsRefused(t *testing.T) {
	types := append(slices.Clone(allTypes), "", "epic")
	n := 0
	for _, typ := range types {
		for _, from := range allGates {
			for _, to := range allGates {
				if slices.Contains(legal[typ], [2]Gate{from, to}) {
					continue
				}
				n++
				if Allowed(typ, from, to) {
					t.Errorf("Allowed(%q, %q, %q) = true; want false", typ, from, to)
				}
			}
		}
	}
	if n == 0 {
		t.Fatal("no illegal transitions generated")
	}
}

func TestNext(t *testing.T) {
	for _, typ := range allTypes {
		gates := want[typ]
		for i, w := range gates {
			got, err := Next(typ, w.gate)
			if i == len(gates)-1 {
				if err == nil {
					t.Errorf("Next(%s, %s) = %s; want an error at the last gate", typ, w.gate, got)
				}
				continue
			}
			if err != nil || got != gates[i+1].gate {
				t.Errorf("Next(%s, %s) = %s, %v; want %s", typ, w.gate, got, err, gates[i+1].gate)
			}
			if !Allowed(typ, w.gate, got) {
				t.Errorf("Next(%s, %s) = %s, which Allowed refuses", typ, w.gate, got)
			}
		}
	}
	for _, c := range []struct {
		typ Type
		g   Gate
	}{{Feat, Repro}, {Bug, Spec}, {Chore, Verify}, {"epic", Build}, {Feat, "nope"}} {
		if got, err := Next(c.typ, c.g); err == nil {
			t.Errorf("Next(%q, %q) = %s; want an error", c.typ, c.g, got)
		}
		if _, err := Kind(c.typ, c.g); err == nil {
			t.Errorf("Kind(%q, %q): want an error", c.typ, c.g)
		}
	}
}

func TestPending(t *testing.T) {
	for _, tc := range []struct {
		typ   Type
		at    Gate
		want  Gate
		class Class
		ok    bool
	}{
		{Feat, Spec, Spec, Judgment, true},
		{Feat, Build, Verify, Mechanical, true},
		{Bug, Repro, Repro, Mechanical, true},
		{Bug, Diagnose, Verify, Mechanical, true},
		{Bug, Fix, Verify, Mechanical, true},
		{Bug, Verify, Verify, Mechanical, true},
		{Bug, Close, "", Step, false},
		{Chore, Build, "", Step, false},
		{Feat, Repro, "", Step, false},
		{"nope", Build, "", Step, false},
	} {
		g, class, ok := Pending(tc.typ, tc.at)
		if g != tc.want || class != tc.class || ok != tc.ok {
			t.Errorf("Pending(%s, %s) = %s, %s, %v; want %s, %s, %v", tc.typ, tc.at, g, class, ok, tc.want, tc.class, tc.ok)
		}
	}
}

func TestInitial(t *testing.T) {
	for typ, g := range map[Type]Gate{Feat: Spec, Bug: Repro, Refactor: Intent, Chore: Build, Doc: Draft, Test: Write} {
		if got, err := Initial(typ); err != nil || got != g {
			t.Errorf("Initial(%s) = %s, %v; want %s", typ, got, err, g)
		}
	}
	for _, typ := range []Type{PlannedChore, "epic"} {
		if got, err := Initial(typ); err == nil {
			t.Errorf("Initial(%q) = %s; want an error", typ, got)
		}
	}
}

func TestCodeOpening(t *testing.T) {
	cases := map[Type]Gate{Feat: Spec, Bug: Repro, Refactor: Plan, PlannedChore: Plan, Chore: "", Doc: "", Test: "", "epic": ""}
	for typ, g := range cases {
		got, ok := CodeOpening(typ)
		if got != g || ok != (g != "") {
			t.Errorf("CodeOpening(%q) = %q, %v; want %q, %v", typ, got, ok, g, g != "")
		}
	}
}

func TestClassString(t *testing.T) {
	for c, s := range map[Class]string{Mechanical: "M", Judgment: "J", Step: "step"} {
		if c.String() != s {
			t.Errorf("%d.String() = %q; want %q", c, c.String(), s)
		}
	}
}
