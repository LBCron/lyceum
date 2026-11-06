package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

// fixtures is what each captured payload in testdata/payloads parses to.
var fixtures = map[string]Call{
	"claude/write.json":      {Kind: Write, Tool: "Write", Paths: []string{"/repo/note.txt"}, Cwd: "/repo", PermissionMode: "bypassPermissions"},
	"claude/shell.json":      {Kind: Shell, Tool: "Bash", Command: "echo lyceum approve spec", Cwd: "/repo", PermissionMode: "bypassPermissions"},
	"codex/write.json":       {Kind: Write, Tool: "apply_patch", Paths: []string{"/repo/src/a.txt"}, Cwd: "/repo", PermissionMode: "bypassPermissions"},
	"codex/edit.json":        {Kind: Write, Tool: "apply_patch", Paths: []string{"/repo/src/a.txt"}, Cwd: "/repo", PermissionMode: "bypassPermissions"},
	"codex/shell.json":       {Kind: Shell, Tool: "Bash", Command: "echo lyceum approve spec", Cwd: "/repo", PermissionMode: "bypassPermissions"},
	"commandcode/write.json": {Kind: Write, Tool: "write_file", Paths: []string{"/repo/note.txt"}, Cwd: "/repo", PermissionMode: "bypass"},
	"commandcode/shell.json": {Kind: Shell, Tool: "shell_command", Command: "echo lyceum approve spec", Cwd: "/repo", PermissionMode: "bypass"},
	"opencode/write.json":    {Kind: Write, Tool: "write", Paths: []string{"/repo/note.txt"}},
	"opencode/shell.json":    {Kind: Shell, Tool: "shell", Command: "echo lyceum approve spec"},
	"pi/write.json":          {Kind: Write, Tool: "write", Paths: []string{"/repo/note.txt"}},
	"pi/shell.json":          {Kind: Shell, Tool: "bash", Command: "echo lyceum approve spec"},
	// Not a hook payload: the OpenCode shim's evaluate hook sends it on as
	// {tool: "shell", input: {command: resources[0]}} (evaluatePayload).
	"opencode/permission.json": {Kind: Shell, Tool: "shell", Command: "echo lyceum approve spec"},
}

// evaluatePayload is the payload the OpenCode shim's permission evaluate hook
// sends for the shell event in data, as the shim builds it.
func evaluatePayload(t *testing.T, data []byte) []byte {
	t.Helper()
	var ev struct {
		Action    string   `json:"action"`
		Resources []string `json:"resources"`
	}
	if err := json.Unmarshal(data, &ev); err != nil || ev.Action != "shell" || len(ev.Resources) == 0 {
		t.Fatalf("permission event %s: %v; want action shell with a resource", data, err)
	}
	payload, err := json.Marshal(map[string]any{"tool": "shell", "input": map[string]string{"command": ev.Resources[0]}})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// TestParseFixtures parses every captured payload with its harness's adapter.
func TestParseFixtures(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "payloads", "*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	for _, f := range files {
		name := filepath.ToSlash(strings.TrimPrefix(f, filepath.Join("testdata", "payloads")+string(filepath.Separator)))
		seen = append(seen, name)
		want, ok := fixtures[name]
		if !ok {
			t.Errorf("fixture %s has no expected parse", name)
			continue
		}
		a, ok := Lookup(strings.Split(name, "/")[0])
		if !ok {
			t.Errorf("fixture %s: no adapter for its harness", name)
			continue
		}
		payload, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if name == "opencode/permission.json" {
			payload = evaluatePayload(t, payload)
		}
		got, err := a.Parse(payload)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s parsed to %+v; want %+v", name, got, want)
		}
	}
	for name := range fixtures {
		if !slices.Contains(seen, name) {
			t.Errorf("expected fixture %s is missing", name)
		}
	}
}

func TestNames(t *testing.T) {
	var keys []string
	for k := range adapters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !slices.Equal(keys, Names) {
		t.Fatalf("adapters %q; Names %q", keys, Names)
	}
	if _, ok := Lookup("gemini"); ok {
		t.Fatal("Lookup(gemini) found an adapter")
	}
}

// TestParseTools covers the write and shell tools of each harness that
// the fixtures don't: the same payload shape with another tool name.
func TestParseTools(t *testing.T) {
	cases := []struct {
		harness, payload string
		want             Call
	}{
		{"claude", `{"cwd":"/r","tool_name":"Edit","tool_input":{"file_path":"/r/a.go"}}`, Call{Kind: Write, Tool: "Edit", Paths: []string{"/r/a.go"}, Cwd: "/r"}},
		{"claude", `{"cwd":"/r","tool_name":"MultiEdit","tool_input":{"file_path":"a.go"}}`, Call{Kind: Write, Tool: "MultiEdit", Paths: []string{"a.go"}, Cwd: "/r"}},
		{"claude", `{"cwd":"/r","tool_name":"NotebookEdit","tool_input":{"file_path":"/r/n.ipynb"}}`, Call{Kind: Write, Tool: "NotebookEdit", Paths: []string{"/r/n.ipynb"}, Cwd: "/r"}},
		{"claude", `{"cwd":"/r","tool_name":"Read","tool_input":{"file_path":"/r/a.go"}}`, Call{Kind: Other, Tool: "Read", Cwd: "/r"}},
		{"commandcode", `{"cwd":"/r","tool_name":"edit_file","tool_input":{"file_path":"/r/a.go"}}`, Call{Kind: Write, Tool: "edit_file", Paths: []string{"/r/a.go"}, Cwd: "/r"}},
		{"commandcode", `{"cwd":"/r","tool_name":"Bash","tool_input":{"command":"ls"}}`, Call{Kind: Other, Tool: "Bash", Cwd: "/r"}},
		{"codex", `{"cwd":"/r","tool_name":"Write","tool_input":{"file_path":"/r/a.go"}}`, Call{Kind: Other, Tool: "Write", Cwd: "/r"}},
		{"pi", `{"toolName":"edit","input":{"path":"/r/a.go"}}`, Call{Kind: Write, Tool: "edit", Paths: []string{"/r/a.go"}}},
		{"pi", `{"toolName":"read","input":{"path":"/r/a.go"}}`, Call{Kind: Other, Tool: "read"}},
		{"pi", `{"toolName":"shell","input":{"command":"ls"}}`, Call{Kind: Other, Tool: "shell"}},
		{"opencode", `{"tool":"edit","input":{"path":"/r/a.go"}}`, Call{Kind: Write, Tool: "edit", Paths: []string{"/r/a.go"}}},
		{"opencode", `{"tool":"bash","input":{"command":"ls"}}`, Call{Kind: Other, Tool: "bash"}},
		{"opencode", `{"tool":"write","input":{"path":"a.go"},"cwd":"/r"}`, Call{Kind: Write, Tool: "write", Paths: []string{"a.go"}, Cwd: "/r"}},
	}
	for _, c := range cases {
		a, _ := Lookup(c.harness)
		got, err := a.Parse([]byte(c.payload))
		if err != nil {
			t.Errorf("%s %s: %v", c.harness, c.payload, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %s parsed to %+v; want %+v", c.harness, c.payload, got, c.want)
		}
	}
}

func TestParseRefuses(t *testing.T) {
	cases := []struct{ harness, payload string }{
		{"claude", `not json`},
		{"claude", `{"tool_input":{}}`},
		{"claude", `{"tool_name":"Write","tool_input":{}}`},
		{"claude", `{"tool_name":"Bash","tool_input":{"command":""}}`},
		{"codex", `{"tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch\n*** End Patch"}}`},
		{"commandcode", `[]`},
		{"pi", `{"input":{"path":"/r/a"}}`},
		{"pi", `{"toolName":"write","input":{}}`},
		{"pi", `{"toolName":"bash","input":"ls"}`},
		{"opencode", `{"tool":7}`},
		{"opencode", `{"tool":"shell","input":{}}`},
		{"opencode", `{"tool":"write","input":{"path":"a.go"},"cwd":7}`},
		{"opencode", ``},
	}
	for _, c := range cases {
		a, _ := Lookup(c.harness)
		if got, err := a.Parse([]byte(c.payload)); err == nil {
			t.Errorf("%s %q parsed to %+v; want an error", c.harness, c.payload, got)
		}
	}
}

func TestPatchPaths(t *testing.T) {
	patch := "*** Begin Patch\n" +
		"*** Add File: /r/new.go\n+package a\n" +
		"*** Update File: /r/old.go\n*** Move to: /r/moved.go\n@@\n-a\n+b\n" +
		"*** Delete File: /r/gone.go\r\n" +
		"*** End Patch"
	want := []string{"/r/new.go", "/r/old.go", "/r/moved.go", "/r/gone.go"}
	if got := PatchPaths(patch); !slices.Equal(got, want) {
		t.Fatalf("PatchPaths = %q; want %q", got, want)
	}
	a, _ := Lookup("codex")
	payload, err := json.Marshal(map[string]any{"cwd": "/r", "tool_name": "apply_patch", "tool_input": map[string]string{"command": patch}})
	if err != nil {
		t.Fatal(err)
	}
	c, err := a.Parse(payload)
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != Write || !slices.Equal(c.Paths, want) {
		t.Fatalf("codex apply_patch parsed to %+v; want a write of %q", c, want)
	}
}

func TestAnswers(t *testing.T) {
	reason := `blocked: run "lyceum approve spec" <owner> & stop`
	hookDeny := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"blocked: run \"lyceum approve spec\" <owner> & stop"}}` + "\n"
	shimDeny := `{"decision":"deny","reason":"blocked: run \"lyceum approve spec\" <owner> & stop"}` + "\n"
	for _, name := range Names {
		a, _ := Lookup(name)
		wantAllow, wantDeny := "", hookDeny
		if name == "opencode" || name == "pi" {
			wantAllow, wantDeny = `{"decision":"allow"}`+"\n", shimDeny
		}
		if got := a.Allow(); got != wantAllow {
			t.Errorf("%s Allow() = %q; want %q", name, got, wantAllow)
		}
		if got := a.Deny(reason); got != wantDeny {
			t.Errorf("%s Deny() = %q; want %q", name, got, wantDeny)
		}
	}
}

// TestAsk_AC122: the harness-prompt route's "ask" answer, in each prompting
// harness's format; Codex and Command Code have no prompt.
func TestAsk_AC122(t *testing.T) {
	reason := `approve "spec" <owner> & stop`
	want := map[string]string{
		"claude":   `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"approve \"spec\" <owner> & stop"}}` + "\n",
		"opencode": `{"decision":"ask","reason":"approve \"spec\" <owner> & stop"}` + "\n",
		"pi":       `{"decision":"ask","reason":"approve \"spec\" <owner> & stop"}` + "\n",
	}
	for _, name := range Names {
		a, _ := Lookup(name)
		w, prompts := want[name]
		if a.Prompts() != prompts {
			t.Errorf("%s Prompts() = %t; want %t", name, a.Prompts(), prompts)
		}
		if prompts {
			if got := a.Ask(reason); got != w {
				t.Errorf("%s Ask() = %q; want %q", name, got, w)
			}
		}
	}
}
