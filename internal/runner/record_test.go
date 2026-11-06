//go:build unix

package runner

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/gittest"
)

// repo returns a repo with TESTING.md and a .gitignore for .lyceum/ committed.
func repo(t *testing.T) string {
	t.Helper()
	root := gittest.InitWithCommit(t)
	for name, content := range map[string]string{
		"TESTING.md": "---\nlyceum: 1\nsuite: sh ./t.sh\n---\n",
		".gitignore": ".lyceum/\n",
		"t.sh":       "echo ok\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gittest.Git(t, root, "add", "-A")
	gittest.Git(t, root, "commit", "-q", "-m", "setup")
	return root
}

func readRecord(t *testing.T, root, output string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, Dir, strings.TrimSuffix(output, ".out")+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestRunRecord_AC56: the record holds the command as given, its exit code, the
// commit, the duration and the hash of the stored output file.
func TestRunRecord_AC56(t *testing.T) {
	root := repo(t)
	var stdout bytes.Buffer
	r, err := Run(root, Command{Command: "sh ./t.sh", Timeout: time.Minute}, &stdout, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "ok\n" {
		t.Errorf("streamed %q; want ok", stdout.String())
	}
	out, err := os.ReadFile(filepath.Join(root, Dir, r.Output))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "ok\n" {
		t.Errorf("output file = %q; want ok", out)
	}
	outHash, _ := approval.Hash(filepath.Join(root, Dir, r.Output))
	testingHash, _ := approval.Hash(filepath.Join(root, "TESTING.md"))
	head := gittest.Git(t, root, "rev-parse", "--short", "HEAD")

	m := readRecord(t, root, r.Output)
	for k, want := range map[string]any{
		"command": "sh ./t.sh", "subset": nil, "commit": head, "dirty": false, "exit": 0.0,
		"timed_out": false, "output": r.Output, "output_sha256": outHash, "testing_sha256": testingHash,
	} {
		if m[k] != want {
			t.Errorf("record %s = %v; want %v", k, m[k], want)
		}
	}
	if _, ok := m["duration_ms"].(float64); !ok {
		t.Errorf("record duration_ms = %v; want a number", m["duration_ms"])
	}
	if _, err := time.Parse(time.RFC3339, m["ts"].(string)); err != nil {
		t.Errorf("record ts: %v", err)
	}
	if len(m) != 11 {
		t.Errorf("record has %d keys; want 11: %v", len(m), m)
	}
}

func TestRunRecordSubsetAndFailure(t *testing.T) {
	root := repo(t)
	var b bytes.Buffer
	r, err := Run(root, Command{Command: "exit 4", Subset: "unit", Timeout: time.Minute}, &b, &b)
	if err != nil {
		t.Fatal(err)
	}
	m := readRecord(t, root, r.Output)
	if m["subset"] != "unit" || m["exit"] != 4.0 {
		t.Errorf("record subset, exit = %v, %v; want unit, 4", m["subset"], m["exit"])
	}
}

// TestRunDirty_AC59: a run on a tree with uncommitted changes is recorded dirty.
func TestRunDirty_AC59(t *testing.T) {
	root := repo(t)
	if err := os.WriteFile(filepath.Join(root, "t.sh"), []byte("echo changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	r, err := Run(root, Command{Command: "sh ./t.sh", Timeout: time.Minute}, &b, &b)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Dirty || readRecord(t, root, r.Output)["dirty"] != true {
		t.Errorf("dirty = %t; want true", r.Dirty)
	}
}

// TestRunsUntracked_AC64: 30 runs leave 30 records, none of them seen by git.
func TestRunsUntracked_AC64(t *testing.T) {
	root := repo(t)
	var b bytes.Buffer
	for range 30 {
		r, err := Run(root, Command{Command: "sh ./t.sh", Timeout: time.Minute}, &b, &b)
		if err != nil {
			t.Fatal(err)
		}
		if r.Dirty {
			t.Fatal("an earlier run's files made the tree dirty")
		}
	}
	records, err := filepath.Glob(filepath.Join(root, Dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 30 {
		t.Errorf("%d records; want 30", len(records))
	}
	if s := gittest.Git(t, root, "status", "--porcelain", "--untracked-files=all"); s != "" {
		t.Errorf("git status = %q; want clean", s)
	}
}
