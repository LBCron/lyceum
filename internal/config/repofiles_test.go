package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oritromax/lyceum/internal/gittest"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestEnsureRepoFiles_AC9 is the file half of AC9: after EnsureRepoFiles, git
// merges the event log with merge=union and ignores .lyceum/.
func TestEnsureRepoFiles_AC9(t *testing.T) {
	repo := gittest.Init(t)
	if err := EnsureRepoFiles(repo); err != nil {
		t.Fatal(err)
	}
	if got := gittest.Git(t, repo, "check-attr", "merge", "--", "lyceum/events.jsonl"); got != "lyceum/events.jsonl: merge: union" {
		t.Errorf("check-attr = %q; want merge: union", got)
	}
	if got := gittest.Git(t, repo, "check-ignore", ".lyceum/runs/x.json"); got != ".lyceum/runs/x.json" {
		t.Errorf("check-ignore = %q; want .lyceum/ ignored", got)
	}
	if got := readFile(t, filepath.Join(repo, ".gitattributes")); got != "lyceum/events.jsonl merge=union\n" {
		t.Errorf(".gitattributes = %q", got)
	}
	if got := readFile(t, filepath.Join(repo, ".gitignore")); got != ".lyceum/\n.commandcode/taste/\n.claude/settings.local.json\n.claude/worktrees/\n" {
		t.Errorf(".gitignore = %q", got)
	}
}

func TestEnsureRepoFilesKeepsLines(t *testing.T) {
	root := t.TempDir()
	attrs := filepath.Join(root, ".gitattributes")
	ignore := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(attrs, []byte("*.png binary"), 0o644); err != nil { // no final newline
		t.Fatal(err)
	}
	if err := os.WriteFile(ignore, []byte("dist/\n\n# lyceum\n.lyceum/  \nnode_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if err := EnsureRepoFiles(root); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := readFile(t, attrs), "*.png binary\nlyceum/events.jsonl merge=union\n"; got != want {
		t.Errorf(".gitattributes = %q; want %q", got, want)
	}
	if got, want := readFile(t, ignore), "dist/\n\n# lyceum\n.lyceum/  \nnode_modules/\n.commandcode/taste/\n.claude/settings.local.json\n.claude/worktrees/\n"; got != want {
		t.Errorf(".gitignore = %q; want %q (kept, with the harness paths added)", got, want)
	}
}
