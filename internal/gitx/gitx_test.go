package gitx

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/oritromax/lyceum/internal/gittest"
	"github.com/oritromax/lyceum/internal/out"
)

func TestCheckVersion(t *testing.T) {
	gittest.Isolate(t)
	if err := CheckVersion(); err != nil {
		t.Fatalf("CheckVersion with the installed git: %v", err)
	}
}

func TestCheckVersionOutput(t *testing.T) {
	for _, tc := range []struct {
		in      string
		refused bool
	}{
		{"git version 2.30.0", false},
		{"git version 2.43.0", false},
		{"git version 2.39.3 (Apple Git-146)", false},
		{"git version 2.45.1.windows.1", false},
		{"git version 3.0", false},
		{"git version 2.29.2", true},
		{"git version 1.9.5", true},
	} {
		err := checkVersion(tc.in)
		var r *out.Refusal
		if got := errors.As(err, &r); got != tc.refused {
			t.Errorf("checkVersion(%q) = %v; want refused %v", tc.in, err, tc.refused)
		}
		if !tc.refused && err != nil {
			t.Errorf("checkVersion(%q) = %v; want nil", tc.in, err)
		}
	}
	for _, in := range []string{"", "git version", "hub version 2.30", "git version x.y"} {
		if err := checkVersion(in); err == nil {
			t.Errorf("checkVersion(%q) = nil; want an error", in)
		}
	}
}

func TestIsRepoAndRoot(t *testing.T) {
	repo := gittest.Init(t)
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if !IsRepo(sub) {
		t.Errorf("IsRepo(%s) = false; want true", sub)
	}
	root, err := Root(sub)
	if err != nil || root != repo {
		t.Errorf("Root(%s) = %q, %v; want %q", sub, root, err, repo)
	}

	plain := t.TempDir()
	if IsRepo(plain) {
		t.Errorf("IsRepo(%s) = true for a plain dir", plain)
	}
	if _, err := Root(plain); err == nil {
		t.Errorf("Root(%s) succeeded for a plain dir", plain)
	}
}

func TestBranch(t *testing.T) {
	repo := gittest.Init(t)
	if b, err := Branch(repo); err != nil || b != "main" {
		t.Errorf("Branch with no commits = %q, %v; want main", b, err)
	}
	gittest.Git(t, repo, "commit", "-q", "--allow-empty", "-m", "one")
	gittest.Git(t, repo, "checkout", "-q", "-b", "feat/a3f9c2-csv-export")
	if b, err := Branch(repo); err != nil || b != "feat/a3f9c2-csv-export" {
		t.Errorf("Branch = %q, %v; want feat/a3f9c2-csv-export", b, err)
	}
	gittest.Git(t, repo, "checkout", "-q", "--detach")
	if b, err := Branch(repo); err != nil || b != "" {
		t.Errorf("Branch detached = %q, %v; want empty", b, err)
	}
}

func TestDefaultBranch(t *testing.T) {
	repo := gittest.InitWithCommit(t)
	gittest.Git(t, repo, "checkout", "-q", "-b", "dev")
	if b, err := DefaultBranch(repo); err != nil || b != "dev" {
		t.Errorf("DefaultBranch with no remote = %q, %v; want dev", b, err)
	}
	gittest.Git(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	gittest.Git(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	if b, err := DefaultBranch(repo); err != nil || b != "main" {
		t.Errorf("DefaultBranch with origin/HEAD = %q, %v; want main", b, err)
	}
}

func TestInit(t *testing.T) {
	gittest.Isolate(t)
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if !IsRepo(dir) {
		t.Errorf("IsRepo after Init = false")
	}
}

func TestGitPath(t *testing.T) {
	repo := gittest.Init(t)
	if got, err := GitPath(repo, "hooks/pre-commit"); err != nil || got != ".git/hooks/pre-commit" {
		t.Errorf("GitPath = %q, %v; want .git/hooks/pre-commit", got, err)
	}
	gittest.Git(t, repo, "config", "core.hooksPath", ".githooks")
	if got, err := GitPath(repo, "hooks/pre-commit"); err != nil || got != ".githooks/pre-commit" {
		t.Errorf("GitPath with core.hooksPath = %q, %v; want .githooks/pre-commit", got, err)
	}
}

func TestFiles(t *testing.T) {
	repo := gittest.Init(t)
	for name, content := range map[string]string{
		"tracked.go": "package a\n", "src/new file.js": "x\n", "build/out.js": "x\n", ".gitignore": "build/\n",
	} {
		p := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gittest.Git(t, repo, "add", "tracked.go")
	got, err := Files(filepath.Join(repo, "src"))
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if want := []string{".gitignore", "src/new file.js", "tracked.go"}; !slices.Equal(got, want) {
		t.Errorf("Files = %q; want %q", got, want)
	}
}

func TestFirstAfterAndShow(t *testing.T) {
	repo := gittest.InitWithCommit(t)
	start := gittest.Git(t, repo, "rev-parse", "--short", "HEAD")
	if c, err := FirstAfter(repo, start); err != nil || c != "" {
		t.Errorf("FirstAfter(HEAD) = %q, %v; want none", c, err)
	}
	for _, v := range []string{"one\n", "two\n"} {
		if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
		gittest.Git(t, repo, "add", "f.txt")
		gittest.Git(t, repo, "commit", "-q", "-m", v)
	}
	want := gittest.Git(t, repo, "rev-parse", "HEAD~1")
	c, err := FirstAfter(repo, start)
	if err != nil || c != want {
		t.Fatalf("FirstAfter = %q, %v; want %q", c, err, want)
	}
	if b, err := Show(repo, c, "f.txt"); err != nil || string(b) != "one\n" {
		t.Errorf("Show = %q, %v; want one", b, err)
	}
	if _, err := Show(repo, c, "nope.txt"); err == nil {
		t.Error("Show of a missing path succeeded")
	}
}

func TestHeadSHA(t *testing.T) {
	repo := gittest.Init(t)
	if sha, err := HeadSHA(repo); err != nil || sha != "" {
		t.Errorf("HeadSHA with no commits = %q, %v; want empty", sha, err)
	}
	gittest.Git(t, repo, "commit", "-q", "--allow-empty", "-m", "one")
	want := gittest.Git(t, repo, "rev-parse", "--short", "HEAD")
	if sha, err := HeadSHA(repo); err != nil || sha != want {
		t.Errorf("HeadSHA = %q, %v; want %q", sha, err, want)
	}
	if _, err := HeadSHA(t.TempDir()); err == nil {
		t.Error("HeadSHA outside a repo succeeded")
	}
}

func TestUserEmail(t *testing.T) {
	repo := gittest.Init(t)
	if e, err := UserEmail(repo); err != nil || e != gittest.Email {
		t.Errorf("UserEmail = %q, %v; want %q", e, err, gittest.Email)
	}
	gittest.Git(t, repo, "config", "--unset", "user.email")
	if e, err := UserEmail(repo); err != nil || e != "" {
		t.Errorf("UserEmail unset = %q, %v; want empty", e, err)
	}
}

func TestStatus(t *testing.T) {
	repo := gittest.InitWithCommit(t)
	if paths, err := Status(repo); err != nil || len(paths) != 0 {
		t.Fatalf("Status of a clean tree = %q, %v; want none", paths, err)
	}

	write := func(name, content string) {
		t.Helper()
		p := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("tracked.txt", "one\n")
	write("old name.txt", "rename me\n")
	gittest.Git(t, repo, "add", ".")
	gittest.Git(t, repo, "commit", "-q", "-m", "files")

	write("tracked.txt", "two\n") // modified, unstaged
	write("staged.txt", "new\n")  // added
	gittest.Git(t, repo, "add", "staged.txt")
	gittest.Git(t, repo, "mv", "old name.txt", "new name.txt") // renamed
	write("dir/untracked.txt", "x\n")                          // untracked, inside an untracked dir

	paths, err := Status(repo)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(paths)
	want := []string{"dir/untracked.txt", "new name.txt", "staged.txt", "tracked.txt"}
	if !slices.Equal(paths, want) {
		t.Errorf("Status = %q; want %q", paths, want)
	}
}

func TestBranchesAndBranchExists(t *testing.T) {
	repo := gittest.InitWithCommit(t)
	gittest.Git(t, repo, "branch", "feat/a3f9c2-csv-export")
	got, err := Branches(repo)
	if want := []string{"feat/a3f9c2-csv-export", "main"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("Branches = %q, %v; want %q", got, err, want)
	}
	for name, want := range map[string]bool{"main": true, "feat/a3f9c2-csv-export": true, "dev": false} {
		if ok, err := BranchExists(repo, name); err != nil || ok != want {
			t.Errorf("BranchExists(%s) = %v, %v; want %v", name, ok, err, want)
		}
	}
}

func TestBaseRef(t *testing.T) {
	repo := gittest.InitWithCommit(t)
	gittest.Git(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	gittest.Git(t, repo, "update-ref", "refs/remotes/origin/dev", "HEAD")
	for base, want := range map[string]string{
		"main": "refs/heads/main",         // local first
		"dev":  "refs/remotes/origin/dev", // then origin's
		"nope": "",                        // neither
	} {
		if got, err := BaseRef(repo, base); err != nil || got != want {
			t.Errorf("BaseRef(%s) = %q, %v; want %q", base, got, err, want)
		}
	}
}

func TestTreeFiles(t *testing.T) {
	repo := gittest.InitWithCommit(t)
	if got, err := TreeFiles(repo, "main", "lyceum/plan"); err != nil || len(got) != 0 {
		t.Errorf("TreeFiles with no such dir = %q, %v; want none", got, err)
	}
	for _, name := range []string{"lyceum/plan/a3f9c2-one.md", "lyceum/plan/b4e0d3-two.md", "lyceum/config.yaml"} {
		p := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gittest.Git(t, repo, "add", ".")
	gittest.Git(t, repo, "commit", "-q", "-m", "plans")
	got, err := TreeFiles(repo, "main", "lyceum/plan")
	if want := []string{"lyceum/plan/a3f9c2-one.md", "lyceum/plan/b4e0d3-two.md"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("TreeFiles = %q, %v; want %q", got, err, want)
	}
}

func TestRefsTreeBlobsAndBlobs(t *testing.T) {
	repo := gittest.InitWithCommit(t)
	for name, content := range map[string]string{"lyceum/plan/a3f9c2-one.md": "one\n", "lyceum/plan/b4e0d3-two.md": "", "lyceum/plan/sub/x.md": "x\n"} {
		p := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gittest.Git(t, repo, "add", ".")
	gittest.Git(t, repo, "commit", "-q", "-m", "plans")
	gittest.Git(t, repo, "branch", "feat/a3f9c2-one")
	gittest.Git(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	gittest.Git(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")

	refs, err := Refs(repo)
	if want := []string{"refs/heads/feat/a3f9c2-one", "refs/heads/main", "refs/remotes/origin/main"}; err != nil || !slices.Equal(refs, want) {
		t.Errorf("Refs = %q, %v; want %q (no symbolic refs)", refs, err, want)
	}

	if got, err := TreeBlobs(repo, "refs/heads/main", "lyceum/none"); err != nil || len(got) != 0 {
		t.Errorf("TreeBlobs with no such dir = %v, %v; want none", got, err)
	}
	entries, err := TreeBlobs(repo, "refs/heads/main", "lyceum/plan")
	if err != nil {
		t.Fatal(err)
	}
	var paths, ids []string
	for _, e := range entries {
		paths, ids = append(paths, e.Path), append(ids, e.Blob)
	}
	if want := []string{"lyceum/plan/a3f9c2-one.md", "lyceum/plan/b4e0d3-two.md"}; !slices.Equal(paths, want) {
		t.Errorf("TreeBlobs paths = %q; want %q (files only, not the sub directory)", paths, want)
	}

	blobs, err := Blobs(repo, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(blobs) != 2 || string(blobs[ids[0]]) != "one\n" || string(blobs[ids[1]]) != "" {
		t.Errorf("Blobs = %q; want one\\n and an empty blob", blobs)
	}
	if got, err := Blobs(repo, nil); err != nil || len(got) != 0 {
		t.Errorf("Blobs(nil) = %q, %v; want none", got, err)
	}
	if _, err := Blobs(repo, []string{strings.Repeat("0", len(ids[0]))}); err == nil {
		t.Error("Blobs with a missing object = nil error; want an error")
	}
}

func TestSwitchNewAndCommit(t *testing.T) {
	repo := gittest.InitWithCommit(t)
	base := gittest.Git(t, repo, "rev-parse", "HEAD")
	gittest.Git(t, repo, "checkout", "-q", "-b", "dev")
	gittest.Git(t, repo, "commit", "-q", "--allow-empty", "-m", "dev")

	if err := SwitchNew(repo, "feat/a3f9c2-csv-export", "main"); err != nil {
		t.Fatal(err)
	}
	if b, _ := Branch(repo); b != "feat/a3f9c2-csv-export" {
		t.Errorf("Branch after SwitchNew = %q", b)
	}
	if err := SwitchNew(repo, "feat/a3f9c2-csv-export", "main"); err == nil {
		t.Error("SwitchNew to an existing branch succeeded")
	}

	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gittest.Git(t, repo, "add", "b.txt") // staged before Commit, not one of its paths
	if err := Commit(repo, "add a", "a.txt"); err != nil {
		t.Fatal(err)
	}
	if got := gittest.Git(t, repo, "show", "--format=", "--name-only", "HEAD"); got != "a.txt" {
		t.Errorf("last commit has %q; want only a.txt", got)
	}
	if got := gittest.Git(t, repo, "log", "-1", "--format=%s %P"); got != "add a "+base {
		t.Errorf("last commit = %q; want subject \"add a\" on parent %s", got, base)
	}
	if paths, err := Status(repo); err != nil || !slices.Equal(paths, []string{"b.txt"}) {
		t.Errorf("Status after Commit = %q, %v; want only b.txt left", paths, err)
	}
	if got := gittest.Git(t, repo, "diff", "--cached", "--name-only"); got != "b.txt" {
		t.Errorf("staged after Commit = %q; want b.txt still staged", got)
	}
}

func TestDiff(t *testing.T) {
	repo := gittest.InitWithCommit(t)
	for name, content := range map[string]string{"a.txt": "one\n", "b.txt": "same\n"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gittest.Git(t, repo, "add", ".")
	gittest.Git(t, repo, "commit", "-q", "-m", "files")
	commit := gittest.Git(t, repo, "rev-parse", "--short", "HEAD")

	if got, err := Diff(repo, commit, "a.txt", "b.txt"); err != nil || got != "" {
		t.Errorf("Diff with no changes = %q, %v; want empty", got, err)
	}
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Diff(repo, commit, "a.txt", "b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "diff --git a/a.txt b/a.txt") || !strings.Contains(got, "-one\n+two\n") || strings.Contains(got, "b.txt") {
		t.Errorf("Diff = %q; want only a.txt changed from one to two", got)
	}
	if _, err := Diff(repo, "0000000", "a.txt"); err == nil {
		t.Error("Diff against an unknown commit succeeded")
	}
}

func TestMergeBase(t *testing.T) {
	repo := gittest.InitWithCommit(t)
	base := gittest.Git(t, repo, "rev-parse", "--short", "HEAD")
	gittest.Git(t, repo, "switch", "-q", "-c", "topic")
	gittest.Git(t, repo, "commit", "-q", "--allow-empty", "-m", "on topic")
	ref := gittest.Git(t, repo, "rev-parse", "--abbrev-ref", "@{-1}")
	if got, err := MergeBase(repo, ref); err != nil || got != base {
		t.Errorf("MergeBase(%s) = %q, %v; want %s", ref, got, err, base)
	}
	if _, err := MergeBase(repo, "no-such-ref"); err == nil {
		t.Error("MergeBase of an unknown ref succeeded")
	}
}

func TestBranchChanges(t *testing.T) {
	repo := gittest.InitWithCommit(t)
	write := func(name, content string) {
		t.Helper()
		p := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gittest.Git(t, repo, "switch", "-q", "-c", "feat/x")
	if paths, err := BranchChanges(repo, "main"); err != nil || len(paths) != 0 {
		t.Fatalf("BranchChanges of a new branch = %q, %v; want none", paths, err)
	}

	write("src/a.go", "a\n")
	write("both.go", "one\n")
	gittest.Git(t, repo, "add", ".")
	gittest.Git(t, repo, "commit", "-q", "-m", "branch work")
	// A commit on main after the branch point is not the branch's change.
	gittest.Git(t, repo, "switch", "-q", "main")
	write("main-only.go", "m\n")
	gittest.Git(t, repo, "add", ".")
	gittest.Git(t, repo, "commit", "-q", "-m", "main work")
	gittest.Git(t, repo, "switch", "-q", "feat/x")
	write("both.go", "two\n")   // committed on the branch and changed again
	write("dir/new.txt", "n\n") // untracked

	paths, err := BranchChanges(repo, "main")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"both.go", "dir/new.txt", "src/a.go"}
	if !slices.Equal(paths, want) {
		t.Errorf("BranchChanges = %q; want %q", paths, want)
	}
}

func TestCommittedSince(t *testing.T) {
	repo := gittest.InitWithCommit(t)
	since, err := HeadSHA(repo)
	if err != nil {
		t.Fatal(err)
	}
	if paths, err := CommittedSince(repo, since); err != nil || len(paths) != 0 {
		t.Fatalf("CommittedSince HEAD = %q, %v; want none", paths, err)
	}
	for i, files := range [][]string{{"src/a.go", "b.go"}, {"b.go"}} {
		for _, f := range files {
			p := filepath.Join(repo, f)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte{byte('a' + i)}, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		gittest.Git(t, repo, "add", ".")
		gittest.Git(t, repo, "commit", "-q", "-m", "work")
	}
	// Uncommitted changes are not in it.
	if err := os.WriteFile(filepath.Join(repo, "dirty.go"), []byte("d"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := CommittedSince(repo, since)
	if want := []string{"b.go", "src/a.go"}; err != nil || !slices.Equal(paths, want) {
		t.Errorf("CommittedSince = %q, %v; want %q", paths, err, want)
	}
}
