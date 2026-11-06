package harness

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGrantRoundTrip_AC122(t *testing.T) {
	root := t.TempDir()
	if g, err := ReadGrant(root); err != nil || g != nil {
		t.Fatalf("ReadGrant with none = %+v, %v; want nil", g, err)
	}
	g := NewGrant("6ae678-x", "spec", "claude", "lyceum approve spec", "default")
	if got := g.Expires.Sub(g.At); got != 2*time.Minute {
		t.Errorf("grant lasts %s; want 2m", got)
	}
	if g.Expired() {
		t.Error("a new grant is expired")
	}
	if err := WriteGrant(root, g); err != nil {
		t.Fatal(err)
	}
	got, err := ReadGrant(root)
	if err != nil || got == nil || *got != g {
		t.Fatalf("ReadGrant = %+v, %v; want %+v", got, err, g)
	}
	if err := RemoveGrant(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, GrantPath)); !os.IsNotExist(err) {
		t.Errorf("grant still there after RemoveGrant: %v", err)
	}
	if err := RemoveGrant(root); err != nil {
		t.Errorf("RemoveGrant with none: %v", err)
	}
	old := g
	old.Expires = time.Now().Add(-time.Second)
	if !old.Expired() {
		t.Error("a grant past its expiry is not expired")
	}
}

func TestInHarness(t *testing.T) {
	for _, v := range KnownEnvVars {
		t.Setenv(v, "")
		_ = os.Unsetenv(v)
	}
	for _, name := range Names {
		if InHarness(name) {
			t.Errorf("InHarness(%s) with no marker set", name)
		}
	}
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", "")
	if !InHarness("claude") || InHarness("pi") {
		t.Error("CLAUDE_CODE_ENTRYPOINT set: want claude only")
	}
	t.Setenv("PI_CODING_AGENT", "1")
	t.Setenv("OPENCODE_TERMINAL", "1")
	if !InHarness("pi") || !InHarness("opencode") || InHarness("codex") {
		t.Error("pi and opencode markers set: want both, and never codex")
	}
}
