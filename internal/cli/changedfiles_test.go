package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oritromax/lyceum/internal/gittest"
)

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChangedFiles(t *testing.T) {
	root := gittest.InitWithCommit(t)
	writeFile(t, root, "lyceum/config.yaml", "lyceum: 1\nowner: owner@example.com\nbase_branch: main\n")
	writeFile(t, root, "kept.txt", "on main\n")
	gittest.Git(t, root, "add", "-A")
	gittest.Git(t, root, "commit", "-q", "-m", "setup")
	gittest.Git(t, root, "switch", "-q", "-c", "feat/x")
	writeFile(t, root, "src/b.go", "package src\n")
	writeFile(t, root, "a.txt", "a\n")
	gittest.Git(t, root, "add", "-A")
	gittest.Git(t, root, "commit", "-q", "-m", "work")
	writeFile(t, root, "a.txt", "a, changed again\n") // committed on the branch and uncommitted: listed once
	writeFile(t, root, "z.txt", "untracked\n")
	t.Chdir(filepath.Join(root, "src"))

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"changed-files"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d; stderr:\n%s", code, stderr.String())
	}
	if want := "a.txt\nsrc/b.go\nz.txt\n"; stdout.String() != want {
		t.Errorf("stdout = %q; want %q", stdout.String(), want)
	}

	stdout.Reset()
	if code := Run([]string{"changed-files", "--json"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("--json: exit %d", code)
	}
	if want := `{"ok":true,"unit":null,"gate":null,"next":"run \"lyceum test\"","data":{"files":["a.txt","src/b.go","z.txt"]}}` + "\n"; stdout.String() != want {
		t.Errorf("--json stdout = %s; want %s", stdout.String(), want)
	}
}

func TestChangedFilesOnBase(t *testing.T) {
	root := gittest.InitWithCommit(t)
	writeFile(t, root, "lyceum/config.yaml", "lyceum: 1\nowner: owner@example.com\nbase_branch: main\n")
	gittest.Git(t, root, "add", "-A")
	gittest.Git(t, root, "commit", "-q", "-m", "setup")
	t.Chdir(root)

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"changed-files"}, strings.NewReader(""), &stdout, &stderr); code != 0 || stdout.Len() != 0 {
		t.Errorf("exit %d, stdout %q; want 0 and nothing on the base branch", code, stdout.String())
	}
}

func TestChangedFilesBaseMissing(t *testing.T) {
	root := gittest.InitWithCommit(t)
	writeFile(t, root, "lyceum/config.yaml", "lyceum: 1\nowner: owner@example.com\nbase_branch: nope\n")
	t.Chdir(root)

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"changed-files"}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d; want 2", code)
	}
	want := "Can't list changed files: the base branch nope is missing: neither nope nor origin/nope exists here\nNext: git fetch origin nope\n"
	if stdout.Len() != 0 || stderr.String() != want {
		t.Errorf("stdout %q, stderr %q; want nothing and %q", stdout.String(), stderr.String(), want)
	}
}
