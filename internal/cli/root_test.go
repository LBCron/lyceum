package cli

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

// AC95: bare `lyceum` prints the Lifecycle, Change and Inspect groups and exits 0.
func TestBareMenu_AC95(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(nil, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", code, stderr.String())
	}
	out := stdout.String()

	last := -1
	for _, heading := range []string{"Lifecycle:", "Change:", "Inspect:"} {
		i := strings.Index(out, heading)
		if i < 0 {
			t.Fatalf("heading %q missing from menu:\n%s", heading, out)
		}
		if i < last {
			t.Errorf("heading %q out of order in menu:\n%s", heading, out)
		}
		last = i
	}

	listed := func(name string) bool {
		return regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(name) + `(\s|$)`).MatchString(out)
	}
	described := func(name string) bool {
		return regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(name) + ` +\S`).MatchString(out)
	}
	for _, name := range []string{
		"init", "status", "check", "approve", "plan", "amend", "close", "test", "log", "eject", "doctor",
		"add", "refactor", "chore",
		"understand", "trace", "audit", "export",
	} {
		if !listed(name) {
			t.Errorf("command %q missing from menu:\n%s", name, out)
		} else if !described(name) {
			t.Errorf("command %q has no description in menu:\n%s", name, out)
		}
	}
	for _, name := range []string{"gate", "hook", "changed-files", "help"} {
		if listed(name) {
			t.Errorf("command %q listed in menu:\n%s", name, out)
		}
	}
	if strings.Contains(out, "Additional Commands:") {
		t.Errorf("menu has an Additional Commands group:\n%s", out)
	}

	// --help prints the same menu.
	var help bytes.Buffer
	if code := Run([]string{"--help"}, strings.NewReader(""), &help, &stderr); code != 0 {
		t.Fatalf("--help exit code = %d, want 0; stderr:\n%s", code, stderr.String())
	}
	if help.String() != out {
		t.Errorf("--help output differs from the bare menu:\n%s", help.String())
	}
}

// AC108: a command that is not built yet refuses with a reason and a next step, exit 2.
func TestNotBuiltRefuses_AC108(t *testing.T) {
	cases := []struct {
		args           []string
		stdout, stderr string
	}{
		{[]string{"gate"}, "",
			"Can't run lyceum gate: not built yet in this version\nNext: run \"lyceum\" to see the available commands\n"},
		{[]string{"gate", "--json"},
			`{"ok":false,"refusal":{"reason":"not built yet in this version","next":["run \"lyceum\" to see the available commands"]}}` + "\n", ""},
	}
	for _, c := range cases {
		var stdout, stderr bytes.Buffer
		if code := Run(c.args, strings.NewReader(""), &stdout, &stderr); code != 2 {
			t.Errorf("%v: exit code = %d, want 2", c.args, code)
		}
		if stdout.String() != c.stdout {
			t.Errorf("%v: stdout = %q, want %q", c.args, stdout.String(), c.stdout)
		}
		if stderr.String() != c.stderr {
			t.Errorf("%v: stderr = %q, want %q", c.args, stderr.String(), c.stderr)
		}
	}
}

// AC117: an unknown command or a bad flag refuses with a next step, exit 2;
// --version --json prints only {"version":…}.
func TestUnknownCommandAndBadFlag_AC117(t *testing.T) {
	const next = `run \"lyceum\" to see the commands`
	cases := []struct {
		args           []string
		code           int
		stdout, stderr string
	}{
		{[]string{"nope"}, 2, "",
			"Can't run lyceum nope: unknown command \"nope\"\nNext: run \"lyceum\" to see the commands\n"},
		{[]string{"nope", "--json"}, 2,
			`{"ok":false,"refusal":{"reason":"unknown command \"nope\"","next":["` + next + `"]}}` + "\n", ""},
		{[]string{"help"}, 2, "",
			"Can't run lyceum help: unknown command \"help\"\nNext: run \"lyceum\" to see the commands\n"},
		{[]string{"--bogus"}, 2, "",
			"Can't run lyceum: unknown flag: --bogus\nNext: run \"lyceum\" to see the commands\n"},
		{[]string{"status", "--bogus"}, 2, "",
			"Can't run lyceum status: unknown flag: --bogus\nNext: run \"lyceum\" to see the commands\n"},
		// --json after the bad flag is never parsed, but still applies.
		{[]string{"status", "--bogus", "--json"}, 2,
			`{"ok":false,"refusal":{"reason":"unknown flag: --bogus","next":["` + next + `"]}}` + "\n", ""},
		{[]string{"--json", "--bogus"}, 2,
			`{"ok":false,"refusal":{"reason":"unknown flag: --bogus","next":["` + next + `"]}}` + "\n", ""},
		{[]string{"--version"}, 0, "lyceum dev\n", ""},
		{[]string{"--version", "--json"}, 0, `{"version":"dev"}` + "\n", ""},
		{[]string{"--json", "--version"}, 0, `{"version":"dev"}` + "\n", ""},
	}
	for _, c := range cases {
		var stdout, stderr bytes.Buffer
		if code := Run(c.args, strings.NewReader(""), &stdout, &stderr); code != c.code {
			t.Errorf("%v: exit code = %d, want %d", c.args, code, c.code)
		}
		if stdout.String() != c.stdout {
			t.Errorf("%v: stdout = %q, want %q", c.args, stdout.String(), c.stdout)
		}
		if stderr.String() != c.stderr {
			t.Errorf("%v: stderr = %q, want %q", c.args, stderr.String(), c.stderr)
		}
	}
}
