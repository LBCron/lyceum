//go:build unix

package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestExecStreamsAndCaptures(t *testing.T) {
	var stdout, stderr, capture bytes.Buffer
	o, err := Exec(t.TempDir(), "echo out1; echo err1 >&2; echo out2; pwd >/dev/null; exit 3", time.Minute, &stdout, &stderr, &capture)
	if err != nil {
		t.Fatal(err)
	}
	if o.Exit != 3 || o.TimedOut {
		t.Errorf("Outcome = %+v; want exit 3, not timed out", o)
	}
	if stdout.String() != "out1\nout2\n" || stderr.String() != "err1\n" {
		t.Errorf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
	lines := strings.Split(strings.TrimSuffix(capture.String(), "\n"), "\n")
	slices.Sort(lines)
	if !slices.Equal(lines, []string{"err1", "out1", "out2"}) {
		t.Errorf("capture = %q; want both streams", capture.String())
	}
}

func TestExecRunsInDir(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	if _, err := Exec(dir, "echo x > here.txt", time.Minute, &stdout, &stdout, &stdout); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "here.txt")); err != nil {
		t.Errorf("the command did not run in dir: %v", err)
	}
}

func TestExecPasses(t *testing.T) {
	var b bytes.Buffer
	o, err := Exec(t.TempDir(), "true", time.Minute, &b, &b, &b)
	if err != nil || o.Exit != 0 || o.TimedOut {
		t.Errorf("Exec = %+v, %v; want exit 0", o, err)
	}
}

// TestExecTimeoutKillsGroup_AC112: a command over the timeout is killed with
// every process it started, and ends non-zero and timed out.
func TestExecTimeoutKillsGroup_AC112(t *testing.T) {
	dir := t.TempDir()
	var b bytes.Buffer
	start := time.Now()
	o, err := Exec(dir, "(sleep 2; echo alive > child.txt) & sleep 5", time.Second, &b, &b, &b)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Errorf("Exec took %s; want it killed after 1s", elapsed)
	}
	if !o.TimedOut || o.Exit == 0 {
		t.Errorf("Outcome = %+v; want timed out with a non-zero exit", o)
	}
	// The background child would write its file 2s in, had it survived.
	time.Sleep(time.Until(start.Add(3 * time.Second)))
	if _, err := os.Stat(filepath.Join(dir, "child.txt")); err == nil {
		t.Error("the command's child survived the timeout")
	}
}

func TestCreate(t *testing.T) {
	root := t.TempDir()
	names := map[string]bool{}
	for range 3 {
		f, name, err := Create(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if !regexp.MustCompile(`^\d{8}T\d{6}Z-[0-9a-f]{8}$`).MatchString(name) {
			t.Errorf("name = %q; want <ts>-<rand>", name)
		}
		if _, err := os.Stat(filepath.Join(root, ".lyceum", "runs", name+".out")); err != nil {
			t.Error(err)
		}
		names[name] = true
	}
	if len(names) != 3 {
		t.Errorf("names = %v; want 3 distinct", names)
	}
}
