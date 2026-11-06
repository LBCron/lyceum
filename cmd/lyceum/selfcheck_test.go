package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestOwnMainPassesCheckCI_AC183 runs `lyceum check --ci`, built from the
// source under test (the lyceum-release binary TestMain builds), on Lyceum's
// own repo with its committed HEAD checked out as main. CI runs it on every
// push to main and on every pull request.
//
// It checks a throwaway clone of HEAD, so uncommitted changes in the source
// tree are deliberately not included. The clone works from a shallow source
// (actions/checkout defaults to depth 1) and needs no network, no origin and
// no particular branch in the source.
func TestOwnMainPassesCheckCI_AC183(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	env := cleanEnv()
	top, err := gitOut(env, modRoot, "rev-parse", "--show-toplevel")
	root, _ := filepath.EvalSymlinks(modRoot)
	if real, _ := filepath.EvalSymlinks(top); err != nil || real != root {
		t.Skipf("%s is not the root of a git checkout (e.g. a module-cache build): %v %s", modRoot, err, top)
	}
	head, err := gitOut(env, modRoot, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}

	clone := filepath.Join(t.TempDir(), "lyceum")
	// From a shallow source git ignores the local optimisations with a
	// warning and copies the objects it has, which include HEAD.
	if out, err := gitOut(env, "", "clone", "--quiet", "--no-hardlinks", "--no-checkout", modRoot, clone); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	if out, err := gitOut(env, clone, "checkout", "--quiet", "-B", "main", head); err != nil {
		t.Fatalf("checkout -B main %s: %v\n%s", head, err, out)
	}

	cmd := exec.Command(filepath.Join(binDir, "lyceum-release"), "check", "--ci")
	cmd.Dir = clone
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "No findings") {
		t.Fatalf("lyceum check --ci on main at %s: err=%v, want exit 0 and \"No findings\"; output:\n%s", head, err, out)
	}
}

// cleanEnv is the environment without GITHUB_* (so the CI event does not
// matter) and GIT_* (so a hook's or the host's git settings do not leak in).
func cleanEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GITHUB_") && !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return env
}

// gitOut runs git with args in dir under env and returns its trimmed combined output.
func gitOut(env []string, dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
