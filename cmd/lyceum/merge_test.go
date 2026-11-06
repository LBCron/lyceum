package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gittest"
)

// TestEventUnionMerge_AC10 runs the AC10 scenario 10 times, each in fresh temp
// repos (TESTING.md §5): two branches each append one event, then they are merged
// into main in both orders. Any conflict or lost line fails the test.
func TestEventUnionMerge_AC10(t *testing.T) {
	for run := 1; run <= 10; run++ {
		t.Run(fmt.Sprintf("run%02d", run), func(t *testing.T) {
			t.Setenv("LYCEUM_NOW", "2026-01-01T00:00:00Z")
			t.Setenv("LYCEUM_SEED", fmt.Sprint(run))
			repo := gittest.InitWithCommit(t)
			lyceum(t, repo, "init")
			gittest.Git(t, repo, "add", "-A")
			gittest.Git(t, repo, "commit", "-q", "-m", "lyceum init")

			lyceum(t, repo, "add", "feat", "Alpha")
			a := gittest.Git(t, repo, "branch", "--show-current")
			gittest.Git(t, repo, "switch", "-q", "main")
			lyceum(t, repo, "add", "feat", "Beta")
			b := gittest.Git(t, repo, "branch", "--show-current")
			gittest.Git(t, repo, "switch", "-q", "main")
			if a == b {
				t.Fatalf("both units are on branch %s", a)
			}
			want := slices.Concat(appended(t, repo, a), appended(t, repo, b))

			rev := filepath.Join(t.TempDir(), "rev")
			gittest.Git(t, repo, "clone", "-q", repo, rev)
			gittest.Git(t, rev, "config", "user.name", "Test Owner")
			gittest.Git(t, rev, "config", "user.email", gittest.Email)

			mergeAll(t, repo, a, b)
			checkLog(t, repo, want)
			mergeAll(t, rev, "origin/"+b, "origin/"+a)
			checkLog(t, rev, want)
		})
	}
}

// lyceum runs the test build of lyceum with args in dir and fails the test on error.
func lyceum(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(filepath.Join(binDir, "lyceum"), args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("lyceum %v: %v\n%s", args, err, out)
	}
}

// appended returns the event lines on branch that main's log does not have.
func appended(t *testing.T, repo, branch string) []string {
	t.Helper()
	before := lines(gittest.Git(t, repo, "show", "main:"+events.Path))
	var added []string
	for _, l := range lines(gittest.Git(t, repo, "show", branch+":"+events.Path)) {
		if !slices.Contains(before, l) {
			added = append(added, l)
		}
	}
	if len(added) != 1 {
		t.Fatalf("branch %s appended %d event lines; want 1: %q", branch, len(added), added)
	}
	return added
}

// mergeAll merges each branch into the checked-out branch with a merge commit,
// failing on any conflict.
func mergeAll(t *testing.T, repo string, branches ...string) {
	t.Helper()
	for _, br := range branches {
		cmd := exec.Command("git", "merge", "--no-ff", "-q", "-m", "merge "+br, br)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git merge %s in %s: %v\n%s", br, repo, err, out)
		}
	}
	if u := gittest.Git(t, repo, "diff", "--name-only", "--diff-filter=U"); u != "" {
		t.Fatalf("unmerged paths: %s", u)
	}
	if s := gittest.Git(t, repo, "status", "--porcelain"); s != "" {
		t.Fatalf("tree not clean after merge:\n%s", s)
	}
}

// checkLog fails unless the event log in repo is valid and holds exactly the
// lines in want, in any order.
func checkLog(t *testing.T, repo string, want []string) {
	t.Helper()
	evs, err := events.Read(repo)
	if err != nil {
		t.Fatalf("merged event log: %v", err)
	}
	if len(evs) != len(want) {
		t.Fatalf("merged event log has %d events; want %d", len(evs), len(want))
	}
	got := lines(gittest.Git(t, repo, "show", "HEAD:"+events.Path))
	for _, l := range want {
		if !slices.Contains(got, l) {
			t.Errorf("merged event log lost line %s\ngot:\n%s", l, strings.Join(got, "\n"))
		}
	}
}

// lines splits trimmed text into lines; empty text has none.
func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
