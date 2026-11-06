// Package harness recognises when Lyceum runs inside an agent harness: by the
// environment variables harnesses set, and by the name of the parent process
// (enforcement.md, "lyceum approve [gate]"). Its adapters read each
// harness's pre-tool hook payload and write its answer (enforcement.md,
// "Harness hooks").
package harness

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// KnownEnvVars are the environment variables that mark an agent harness: the
// list in enforcement.md step 3, extended as harnesses are found.
var KnownEnvVars = []string{
	"CLAUDECODE",
	"CLAUDE_CODE_ENTRYPOINT",
	"CODEX_THREAD_ID",
	"CODEX_CI",
	"OPENCODE_TERMINAL",
	"PI_CODING_AGENT",
	"COMMANDCODE_SCRATCHPAD",
	"AI_AGENT",
}

// envMarkers are the KnownEnvVars that mark each harness whose hook can ask
// the human: a harness-prompt grant counts only inside its harness.
var envMarkers = map[string][]string{
	"claude":   {"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT"},
	"opencode": {"OPENCODE_TERMINAL"},
	"pi":       {"PI_CODING_AGENT"},
}

// InHarness reports whether the environment carries a marker of harness
// name, one of its envMarkers set with any value.
func InHarness(name string) bool {
	for _, v := range envMarkers[name] {
		if _, ok := os.LookupEnv(v); ok {
			return true
		}
	}
	return false
}

// SetEnvVars returns the KnownEnvVars present in the environment, in list
// order. A variable counts as set with any value, including empty.
func SetEnvVars() []string {
	var set []string
	for _, v := range KnownEnvVars {
		if _, ok := os.LookupEnv(v); ok {
			set = append(set, v)
		}
	}
	return set
}

// psTimeout bounds the ps call on macOS.
const psTimeout = 5 * time.Second

// ParentProcess returns the name of this process's parent, or "unknown" when
// it cannot be read.
func ParentProcess() string {
	ppid := strconv.Itoa(os.Getppid())
	var name string
	switch runtime.GOOS {
	case "linux":
		if b, err := os.ReadFile("/proc/" + ppid + "/comm"); err == nil {
			name = string(b)
		}
	case "darwin":
		ctx, cancel := context.WithTimeout(context.Background(), psTimeout)
		defer cancel()
		if b, err := exec.CommandContext(ctx, "ps", "-o", "comm=", "-p", ppid).Output(); err == nil {
			name = string(b)
		}
	}
	if name = strings.TrimSpace(name); name == "" {
		return "unknown"
	}
	return name
}
