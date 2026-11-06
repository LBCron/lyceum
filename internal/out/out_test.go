package out

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func writers(json bool) (Writer, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return Writer{Stdout: &stdout, Stderr: &stderr, JSON: json}, &stdout, &stderr
}

func mustPanic(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: no panic", name)
		}
	}()
	f()
}

// AC108: a refusal can't be built without a concrete next step.
func TestRefuseNeedsNextStep_AC108(t *testing.T) {
	mustPanic(t, "no step", func() { _ = Refuse("approve", "no reason") })
	mustPanic(t, "blank step", func() { _ = Refuse("approve", "no reason", "lyceum status", "  ") })
}

// AC108: a refusal prints its reason and every next step on stderr, exit 2.
func TestRefusalHuman_AC108(t *testing.T) {
	w, stdout, stderr := writers(false)
	err := fmt.Errorf("wrapped: %w", Refuse("approve yet", "src/a.go has changes", "Review them", "git stash"))
	if code := w.Fail(err); code != ExitRefused {
		t.Errorf("exit code = %d, want %d", code, ExitRefused)
	}
	want := "Can't approve yet: src/a.go has changes\nNext: Review them\nNext: git stash\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

// AC5 (shape): a refusal in JSON mode is one object on stdout, nothing on stderr, exit 2.
func TestRefusalJSON_AC5(t *testing.T) {
	w, stdout, stderr := writers(true)
	if code := w.Fail(Refuse("close", "x < y & z", "lyceum close <unit>")); code != ExitRefused {
		t.Errorf("exit code = %d, want %d", code, ExitRefused)
	}
	want := `{"ok":false,"refusal":{"reason":"x < y & z","next":["lyceum close <unit>"]}}` + "\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

// AC5: a result ends with the unit, the gate and the next action.
func TestResultHuman_AC5(t *testing.T) {
	cases := []struct {
		name string
		r    Result
		want string
	}{
		{"unit and gate", Result{Unit: "a07bb8-m1-core", Gate: "build", Next: "lyceum test", Data: 1}, "Unit: a07bb8-m1-core\nGate: build\nNext: lyceum test\n"},
		{"next only", Result{Next: "lyceum add feat"}, "Next: lyceum add feat\n"},
	}
	for _, c := range cases {
		w, stdout, stderr := writers(false)
		if code := w.Result(c.r); code != ExitOK {
			t.Errorf("%s: exit code = %d, want %d", c.name, code, ExitOK)
		}
		if stdout.String() != c.want {
			t.Errorf("%s: stdout = %q, want %q", c.name, stdout.String(), c.want)
		}
		if stderr.Len() != 0 {
			t.Errorf("%s: stderr = %q, want empty", c.name, stderr.String())
		}
	}
}

// AC5 (shape): a result in JSON mode is one object with ok, unit, gate, next and data.
func TestResultJSON_AC5(t *testing.T) {
	cases := []struct {
		name string
		r    Result
		want string
	}{
		{"all set", Result{Unit: "a07bb8-m1-core", Gate: "build", Next: "lyceum test", Data: map[string]int{"units": 1}},
			`{"ok":true,"unit":"a07bb8-m1-core","gate":"build","next":"lyceum test","data":{"units":1}}`},
		{"unset", Result{Next: "lyceum add feat"},
			`{"ok":true,"unit":null,"gate":null,"next":"lyceum add feat","data":null}`},
		{"extra keys", Result{Next: "lyceum check", Extra: map[string]any{"findings": []string{}, "b": "<&>"}},
			`{"ok":true,"unit":null,"gate":null,"next":"lyceum check","data":null,"b":"<&>","findings":[]}`},
	}
	for _, c := range cases {
		w, stdout, stderr := writers(true)
		if code := w.Result(c.r); code != ExitOK {
			t.Errorf("%s: exit code = %d, want %d", c.name, code, ExitOK)
		}
		if stdout.String() != c.want+"\n" {
			t.Errorf("%s: stdout = %q, want %q", c.name, stdout.String(), c.want+"\n")
		}
		if stderr.Len() != 0 {
			t.Errorf("%s: stderr = %q, want empty", c.name, stderr.String())
		}
	}
}

// An unexpected error goes to stderr in both modes, exit 1, nothing on stdout.
func TestUnexpectedError(t *testing.T) {
	for _, json := range []bool{false, true} {
		w, stdout, stderr := writers(json)
		if code := w.Fail(errors.New("disk full")); code != ExitError {
			t.Errorf("json=%v: exit code = %d, want %d", json, code, ExitError)
		}
		if stderr.String() != "disk full\n" {
			t.Errorf("json=%v: stderr = %q, want %q", json, stderr.String(), "disk full\n")
		}
		if stdout.Len() != 0 {
			t.Errorf("json=%v: stdout = %q, want empty", json, stdout.String())
		}
	}
}

// A Refusal built without Refuse has no next step, so it is never printed as a refusal.
func TestZeroRefusalIsAnError(t *testing.T) {
	w, stdout, _ := writers(false)
	if code := w.Fail(&Refusal{}); code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

// Data that can't be encoded is an unexpected error, with nothing on stdout.
func TestResultJSONEncodeError(t *testing.T) {
	w, stdout, stderr := writers(true)
	if code := w.Result(Result{Next: "x", Data: func() {}}); code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() == 0 {
		t.Error("stderr empty, want the encoding error")
	}
}
