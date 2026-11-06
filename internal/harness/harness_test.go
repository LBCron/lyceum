package harness

import (
	"os"
	"runtime"
	"slices"
	"testing"
)

// clearKnownEnv unsets every known variable for the test; t.Setenv restores them.
func clearKnownEnv(t *testing.T) {
	t.Helper()
	for _, v := range KnownEnvVars {
		t.Setenv(v, "")
		if err := os.Unsetenv(v); err != nil {
			t.Fatal(err)
		}
	}
}

func TestKnownEnvVars_AC40(t *testing.T) {
	want := []string{"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CODEX_THREAD_ID", "CODEX_CI", "OPENCODE_TERMINAL",
		"PI_CODING_AGENT", "COMMANDCODE_SCRATCHPAD", "AI_AGENT"}
	if !slices.Equal(KnownEnvVars, want) {
		t.Fatalf("KnownEnvVars = %q; want %q", KnownEnvVars, want)
	}
}

func TestSetEnvVars_AC40(t *testing.T) {
	clearKnownEnv(t)
	if got := SetEnvVars(); len(got) != 0 {
		t.Fatalf("SetEnvVars() with none set = %q; want none", got)
	}

	t.Setenv("OPENCODE_TERMINAL", "1")
	t.Setenv("CLAUDECODE", "")
	got := SetEnvVars()
	if want := []string{"CLAUDECODE", "OPENCODE_TERMINAL"}; !slices.Equal(got, want) {
		t.Fatalf("SetEnvVars() = %q; want %q (an empty value counts as set)", got, want)
	}
}

func TestParentProcess(t *testing.T) {
	got := ParentProcess()
	if got == "" {
		t.Fatal("ParentProcess() is empty")
	}
	if (runtime.GOOS == "linux" || runtime.GOOS == "darwin") && got == "unknown" {
		t.Fatalf("ParentProcess() = unknown on %s", runtime.GOOS)
	}
}
