// Package gittest makes real, isolated git repos for tests: no global or system
// git config, a fixed identity and fixed commit dates.
package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Email is the user.email of every repo Init makes.
const Email = "owner@example.com"

const fixedDate = "2026-01-01T00:00:00Z"

// Isolate points HOME at a temp dir and turns off the global and system git config
// for the rest of the test.
func Isolate(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_DATE", fixedDate)
	t.Setenv("GIT_COMMITTER_DATE", fixedDate)
}

// Init isolates git (see Isolate) and returns a new repo on branch main with
// user.email set to Email and no commits.
func Init(t *testing.T) string {
	t.Helper()
	Isolate(t)
	dir := t.TempDir()
	// Resolve symlinks so paths compare equal to git's --show-toplevel.
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	Git(t, dir, "init", "-q", "-b", "main")
	Git(t, dir, "config", "user.name", "Test Owner")
	Git(t, dir, "config", "user.email", Email)
	return dir
}

// InitWithCommit is Init plus one empty commit.
func InitWithCommit(t *testing.T) string {
	t.Helper()
	dir := Init(t)
	Git(t, dir, "commit", "-q", "--allow-empty", "-m", "initial")
	return dir
}

// Git runs git with args in dir, fails the test on error, and returns trimmed stdout.
func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		t.Fatalf("git %v: %v\n%s", args, err, stderr)
	}
	return strings.TrimSpace(string(out))
}
