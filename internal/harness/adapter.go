package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Kind is what a pre-tool call does, as far as the hook is concerned.
type Kind string

// Kinds of tool call (enforcement.md, "Harness hooks").
const (
	Write Kind = "write"
	Shell Kind = "shell"
	Other Kind = "other"
)

// Call is one harness pre-tool payload, parsed.
type Call struct {
	Kind    Kind
	Tool    string   // the tool's name in the payload
	Paths   []string // Write: every path written, as the payload gives it
	Command string   // Shell: the command line
	Cwd     string   // the payload's cwd; "" when the harness sends none
	// PermissionMode is the harness's permission mode, when the payload has
	// one (Claude Code's permission_mode).
	PermissionMode string
}

// Adapter reads one harness's pre-tool payload and writes its answer. Its
// formats come from the captured payloads in testdata/payloads.
type Adapter struct {
	Name  string
	parse func([]byte) (Call, error)
	// shim is set for the harnesses whose JS/TS shim acts on Lyceum's own
	// one-line answer (Pi, OpenCode).
	shim bool
	// prompt is set for the harnesses whose hook can make the harness ask
	// the human, which the harness-prompt approval route needs.
	prompt bool
}

// Names are the harnesses with an adapter, the --harness values.
var Names = []string{"claude", "codex", "commandcode", "opencode", "pi"}

var adapters = map[string]Adapter{
	"claude":      {Name: "claude", parse: hookJSON([]string{"Write", "Edit", "MultiEdit", "NotebookEdit"}, nil, []string{"Bash"}), prompt: true},
	"codex":       {Name: "codex", parse: hookJSON(nil, []string{"apply_patch"}, []string{"Bash"})},
	"commandcode": {Name: "commandcode", parse: hookJSON([]string{"write_file", "edit_file"}, nil, []string{"shell_command"})},
	"opencode":    {Name: "opencode", parse: shimJSON("tool", "shell"), shim: true, prompt: true},
	"pi":          {Name: "pi", parse: shimJSON("toolName", "bash"), shim: true, prompt: true},
}

// Prompts reports whether the harness's hook can make it ask the human
// (Claude Code, OpenCode, Pi). Codex parses "ask" but runs the tool anyway,
// and Command Code has only allow and deny.
func (a Adapter) Prompts() bool { return a.prompt }

// Lookup returns the adapter of harness name.
func Lookup(name string) (Adapter, bool) {
	a, ok := adapters[name]
	return a, ok
}

// Parse reads payload. It fails when the payload is not the harness's
// pre-tool shape, or a write or shell call lacks its path or command.
func (a Adapter) Parse(payload []byte) (Call, error) {
	c, err := a.parse(payload)
	if err != nil {
		return Call{}, fmt.Errorf("%s pre-tool payload: %w", a.Name, err)
	}
	return c, nil
}

// Allow is the answer that lets the call run: empty for the JSON hook
// harnesses, {"decision":"allow"} for the shims.
func (a Adapter) Allow() string {
	if !a.shim {
		return ""
	}
	return encode(struct {
		Decision string `json:"decision"`
	}{"allow"})
}

// Deny is the answer that blocks the call, giving reason to the agent.
func (a Adapter) Deny(reason string) string { return a.decide("deny", reason) }

// Ask is the answer that makes the harness ask the human whether the call
// runs, showing reason. Only harnesses that Prompts get it.
func (a Adapter) Ask(reason string) string { return a.decide("ask", reason) }

// decide is the answer with decision deny or ask and reason: Lyceum's one
// line for the shims, the PreToolUse hook output for the JSON harnesses.
func (a Adapter) decide(decision, reason string) string {
	if a.shim {
		return encode(struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}{decision, reason})
	}
	type specific struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	}
	return encode(struct {
		HookSpecificOutput specific `json:"hookSpecificOutput"`
	}{specific{"PreToolUse", decision, reason}})
}

// encode is v as one line of JSON, newline-terminated, with <, > and & kept as they are.
func encode(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err) // only fixed structs of strings are encoded
	}
	return b.String()
}

// hookJSON parses the payload of a JSON command hook (Claude Code, Codex,
// Command Code): cwd, permission_mode, tool_name and tool_input. Tools in write take their path
// from tool_input.file_path, tools in patch are apply_patch calls whose paths
// are in the patch text in tool_input.command, and tools in shell take
// tool_input.command.
func hookJSON(write, patch, shell []string) func([]byte) (Call, error) {
	return func(payload []byte) (Call, error) {
		var p struct {
			Cwd            string `json:"cwd"`
			PermissionMode string `json:"permission_mode"`
			ToolName       string `json:"tool_name"`
			ToolInput      struct {
				FilePath string `json:"file_path"`
				Command  string `json:"command"`
			} `json:"tool_input"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return Call{}, err
		}
		if p.ToolName == "" {
			return Call{}, errors.New("no tool_name")
		}
		c := Call{Kind: Other, Tool: p.ToolName, Cwd: p.Cwd, PermissionMode: p.PermissionMode}
		switch {
		case slices.Contains(write, p.ToolName):
			if p.ToolInput.FilePath == "" {
				return Call{}, fmt.Errorf("%s call without tool_input.file_path", p.ToolName)
			}
			c.Kind, c.Paths = Write, []string{p.ToolInput.FilePath}
		case slices.Contains(patch, p.ToolName):
			paths := PatchPaths(p.ToolInput.Command)
			if len(paths) == 0 {
				return Call{}, fmt.Errorf("%s call with no file header in tool_input.command", p.ToolName)
			}
			c.Kind, c.Paths = Write, paths
		case slices.Contains(shell, p.ToolName):
			if p.ToolInput.Command == "" {
				return Call{}, fmt.Errorf("%s call without tool_input.command", p.ToolName)
			}
			c.Kind, c.Command = Shell, p.ToolInput.Command
		}
		return c, nil
	}
}

// shimJSON parses the event a Pi or OpenCode shim passes through: the tool's
// name in field toolField, input.path or input.command, and cwd when the shim
// adds it (OpenCode). Their write tools are write and edit; shell is the
// shell tool's name.
func shimJSON(toolField, shell string) func([]byte) (Call, error) {
	return func(payload []byte) (Call, error) {
		var p map[string]json.RawMessage
		if err := json.Unmarshal(payload, &p); err != nil {
			return Call{}, err
		}
		var tool string
		if err := json.Unmarshal(p[toolField], &tool); err != nil || tool == "" {
			return Call{}, errors.New("no " + toolField)
		}
		var in struct {
			Path    string `json:"path"`
			Command string `json:"command"`
		}
		if raw, ok := p["input"]; ok {
			if err := json.Unmarshal(raw, &in); err != nil {
				return Call{}, fmt.Errorf("input: %w", err)
			}
		}
		c := Call{Kind: Other, Tool: tool}
		if raw, ok := p["cwd"]; ok {
			if err := json.Unmarshal(raw, &c.Cwd); err != nil {
				return Call{}, fmt.Errorf("cwd: %w", err)
			}
		}
		switch tool {
		case "write", "edit":
			if in.Path == "" {
				return Call{}, fmt.Errorf("%s call without input.path", tool)
			}
			c.Kind, c.Paths = Write, []string{in.Path}
		case shell:
			if in.Command == "" {
				return Call{}, fmt.Errorf("%s call without input.command", tool)
			}
			c.Kind, c.Command = Shell, in.Command
		}
		return c, nil
	}
}

// patchHeaders are the apply_patch lines that name a path the patch writes.
var patchHeaders = []string{"*** Add File: ", "*** Update File: ", "*** Delete File: ", "*** Move to: "}

// PatchPaths returns every path an apply_patch text writes, in order: those
// of its Add, Update and Delete File lines and of its Move to lines.
func PatchPaths(patch string) []string {
	var paths []string
	for line := range strings.Lines(patch) {
		line = strings.TrimRight(line, "\r\n")
		for _, h := range patchHeaders {
			if p, ok := strings.CutPrefix(line, h); ok && strings.TrimSpace(p) != "" {
				paths = append(paths, strings.TrimSpace(p))
			}
		}
	}
	return paths
}
