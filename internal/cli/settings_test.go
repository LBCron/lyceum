package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

const testCommand = "lyceum hook pre-tool --harness claude"

func merge(t *testing.T, data string) string {
	t.Helper()
	got, err := mergeHook([]byte(data), "Write|Bash", testCommand)
	if err != nil {
		t.Fatalf("mergeHook(%s): %v", data, err)
	}
	if !json.Valid(got) {
		t.Fatalf("mergeHook(%s) is not valid JSON:\n%s", data, got)
	}
	again, err := mergeHook(got, "Write|Bash", testCommand)
	if err != nil || string(again) != string(got) {
		t.Errorf("a second merge changed the bytes (err %v):\n%s\nthen\n%s", err, got, again)
	}
	return string(got)
}

// No settings file: the file holds Lyceum's entry only.
func TestMergeHook_New(t *testing.T) {
	want := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Write|Bash",
        "hooks": [
          {
            "type": "command",
            "command": "lyceum hook pre-tool --harness claude"
          }
        ]
      }
    ]
  }
}
`
	for _, data := range []string{"", "  \n", "{}"} {
		if got := merge(t, data); got != want {
			t.Errorf("mergeHook(%q) =\n%s\nwant\n%s", data, got, want)
		}
	}
}

// A user's keys, values and hook entries are kept, in their order; Lyceum's
// entry goes after the user's.
func TestMergeHook_KeepsUserSettings(t *testing.T) {
	data := `{"permissions": {"allow": ["Bash(make && make test)"]}, "hooks": {"PostToolUse": [{"matcher": "Write", "hooks": [{"type": "command", "command": "fmt.sh <file>"}]}], "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "audit.sh", "timeout": 1.50}]}]}, "model": "x"}`
	want := `{
  "permissions": {
    "allow": [
      "Bash(make && make test)"
    ]
  },
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write",
        "hooks": [
          {
            "type": "command",
            "command": "fmt.sh <file>"
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "audit.sh",
            "timeout": 1.50
          }
        ]
      },
      {
        "matcher": "Write|Bash",
        "hooks": [
          {
            "type": "command",
            "command": "lyceum hook pre-tool --harness claude"
          }
        ]
      }
    ]
  },
  "model": "x"
}
`
	if got := merge(t, data); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// Lyceum's entry, found by its command, is replaced where it is; a second one
// is dropped; a user's hook sharing an entry with it is kept.
func TestMergeHook_ReplacesLyceumEntry(t *testing.T) {
	data := `{"hooks": {"PreToolUse": [
		{"matcher": "Bash", "hooks": [{"type": "command", "command": "audit.sh"}]},
		{"matcher": "Write", "hooks": [{"type": "command", "command": "lyceum hook pre-tool --harness claude"}]},
		{"matcher": "Edit", "hooks": [{"type": "command", "command": "audit.sh"}]},
		{"matcher": "Read", "hooks": [{"type": "command", "command": "/usr/local/bin/lyceum hook pre-tool --harness claude"}, {"type": "command", "command": "mine.sh"}]}
	]}}`
	want := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "audit.sh"
          }
        ]
      },
      {
        "matcher": "Write|Bash",
        "hooks": [
          {
            "type": "command",
            "command": "lyceum hook pre-tool --harness claude"
          }
        ]
      },
      {
        "matcher": "Edit",
        "hooks": [
          {
            "type": "command",
            "command": "audit.sh"
          }
        ]
      },
      {
        "matcher": "Read",
        "hooks": [
          {
            "type": "command",
            "command": "mine.sh"
          }
        ]
      }
    ]
  }
}
`
	if got := merge(t, data); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// A file that isn't a settings object is an error, never overwritten.
func TestMergeHook_Invalid(t *testing.T) {
	for _, data := range []string{
		`{"hooks": `,
		`[]`,
		`{} {}`,
		`{"hooks": []}`,
		`{"hooks": {"PreToolUse": {}}}`,
	} {
		if got, err := mergeHook([]byte(data), "Write", testCommand); err == nil {
			t.Errorf("mergeHook(%s) = %s; want an error", data, got)
		}
	}
}

// AC111: eject takes Lyceum's hook out of a settings file, giving back the
// user's file as init found it, or nothing when init created it.
func TestUnmergeHook_AC111(t *testing.T) {
	user := `{
  "permissions": {
    "allow": [
      "Bash(make && make test)"
    ]
  },
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "audit.sh"
          }
        ]
      }
    ]
  }
}
`
	noHooks := "{\n  \"model\": \"x\"\n}\n"
	for _, tc := range []struct{ in, want string }{{"", ""}, {"{}", ""}, {user, user}, {noHooks, noHooks}} {
		got, err := unmergeHook([]byte(merge(t, tc.in)))
		if err != nil || string(got) != tc.want || (tc.want == "") != (got == nil) {
			t.Errorf("unmergeHook(merge(%q)) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	// Lyceum's hook sharing an entry with the user's: only Lyceum's goes.
	shared := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"audit.sh"},{"type":"command","command":"` + testCommand + `"}]}]}}`
	got, err := unmergeHook([]byte(shared))
	if err != nil || strings.Contains(string(got), testCommand) || !strings.Contains(string(got), "audit.sh") {
		t.Errorf("unmergeHook(%s) = %s, %v", shared, got, err)
	}
}
