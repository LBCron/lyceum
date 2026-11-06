package testhooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const fixedNow = "2026-01-01T00:00:00Z"

// TestReleaseIgnoresTestVars_AC116 builds testdata/nowrand with and without the
// lyceumtest tag and runs each twice with LYCEUM_NOW and LYCEUM_SEED set: only the
// test build may honour them.
func TestReleaseIgnoresTestVars_AC116(t *testing.T) {
	env := append(os.Environ(), "LYCEUM_NOW="+fixedNow, "LYCEUM_SEED=1")

	t.Run("test build honours them", func(t *testing.T) {
		bin := buildProbe(t, "lyceumtest")
		now1, rand1 := runProbe(t, bin, env)
		now2, rand2 := runProbe(t, bin, env)
		if now1 != fixedNow || now2 != fixedNow {
			t.Errorf("Now = %q, %q; want %q", now1, now2, fixedNow)
		}
		if rand1 != rand2 {
			t.Errorf("seeded Rand differs across runs: %s vs %s", rand1, rand2)
		}
	})

	t.Run("release build ignores them", func(t *testing.T) {
		bin := buildProbe(t, "")
		before := time.Now().Add(-time.Second)
		now1, rand1 := runProbe(t, bin, env)
		now2, rand2 := runProbe(t, bin, env)
		after := time.Now().Add(time.Second)
		for _, s := range []string{now1, now2} {
			got, err := time.Parse(time.RFC3339Nano, s)
			if err != nil {
				t.Fatalf("Now = %q: %v", s, err)
			}
			if got.Before(before) || got.After(after) {
				t.Errorf("Now = %s; want the real clock, between %s and %s", got, before, after)
			}
		}
		if rand1 == rand2 {
			t.Errorf("Rand repeated across runs: %s", rand1)
		}
	})
}

// buildProbe builds testdata/nowrand with the given build tags into a temp dir.
func buildProbe(t *testing.T, tags string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "nowrand")
	args := []string{"build", "-o", bin}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, "./testdata/nowrand")
	// The test binary runs where it was built, so GOROOT is the go running this test.
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), args...) //nolint:staticcheck // SA1019, see above
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %v: %v\n%s", args, err, out)
	}
	return bin
}

// runProbe runs the probe and returns its two output lines: the time and the random hex.
func runProbe(t *testing.T, bin string, env []string) (now, random string) {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: %v", bin, err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		t.Fatalf("%s printed %q; want two lines", bin, out)
	}
	return lines[0], lines[1]
}
