package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/oritromax/lyceum/internal/gittest"
	"github.com/oritromax/lyceum/internal/gitx"
)

func lyceumDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "lyceum"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeConfig(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, Path), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readConfig(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, Path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

const defaultYAML = `lyceum: 1
owner: owner@example.com
base_branch: main
approvals:
  signed: false
  harness_prompt: allow
harnesses: []
hooks: true
understand:
  warn_threshold:
    files: 2000
    lines: 200000
`

func TestWriteDefault(t *testing.T) {
	root := lyceumDir(t)
	if err := Write(root, Default("owner@example.com", "main")); err != nil {
		t.Fatal(err)
	}
	if got := readConfig(t, root); got != defaultYAML {
		t.Errorf("config =\n%s\nwant\n%s", got, defaultYAML)
	}
}

// TestOwnerFromGit_AC13 is the config half of AC13: a config written with the
// repo's git user.email as owner reads back with that owner.
func TestOwnerFromGit_AC13(t *testing.T) {
	repo := gittest.Init(t)
	if err := os.Mkdir(filepath.Join(repo, "lyceum"), 0o755); err != nil {
		t.Fatal(err)
	}
	email, err := gitx.UserEmail(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(repo, Default(email, "main")); err != nil {
		t.Fatal(err)
	}
	c, err := Read(repo)
	if err != nil {
		t.Fatal(err)
	}
	if c.Owner != gittest.Email {
		t.Errorf("owner = %q; want %q", c.Owner, gittest.Email)
	}
}

func TestReadDefaults(t *testing.T) {
	root := lyceumDir(t)
	writeConfig(t, root, "lyceum: 1\nowner: o@example.com\n")
	c, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	want := Default("o@example.com", "")
	if c.Owner != want.Owner || c.BaseBranch != "" || c.Approvals != want.Approvals || c.Hooks != want.Hooks ||
		c.Understand != want.Understand || len(c.Harnesses) != 0 {
		t.Errorf("Read = %+v; want defaults %+v", c, want)
	}
}

func TestReadValues(t *testing.T) {
	root := lyceumDir(t)
	writeConfig(t, root, `lyceum: 1
owner: o@example.com
base_branch: dev
approvals: {signed: true, harness_prompt: deny}
harnesses: [claude-code, codex]
hooks: false
understand: {warn_threshold: {files: 10, lines: 20}}
`)
	c, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseBranch != "dev" || !c.Approvals.Signed || c.Approvals.HarnessPrompt != HarnessPromptDeny || !slices.Equal(c.Harnesses, []string{"claude-code", "codex"}) ||
		c.Hooks || c.Understand.WarnThreshold != (Threshold{Files: 10, Lines: 20}) {
		t.Errorf("Read = %+v", c)
	}
}

func TestReadRefuses(t *testing.T) {
	for _, tc := range []struct{ name, content, want string }{
		{"no version", "owner: o@example.com\n", "lyceum: 0"},
		{"future version", "lyceum: 2\n", "lyceum: 2"},
		{"empty", "", "mapping"},
		{"not a mapping", "- a\n", "mapping"},
		{"bad yaml", "lyceum: [\n", "yaml"},
		{"wrong type", "lyceum: 1\nhooks: maybe\n", "maybe"},
		{"unknown harness_prompt", "lyceum: 1\napprovals:\n  harness_prompt: sometimes\n", "approvals.harness_prompt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := lyceumDir(t)
			writeConfig(t, root, tc.content)
			_, err := Read(root)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), Path) {
				t.Errorf("Read = %v; want an error naming %s and containing %q", err, Path, tc.want)
			}
		})
	}
	if _, err := Read(lyceumDir(t)); !os.IsNotExist(err) {
		t.Errorf("Read of a missing config = %v; want not-exist", err)
	}
}

func TestWriteKeepsUnknownKeys(t *testing.T) {
	root := lyceumDir(t)
	writeConfig(t, root, `# project config
lyceum: 1
owner: old@example.com # the owner
base_branch: main
team: core
approvals:
  signed: false
  quorum: 2
harnesses: [claude-code]
hooks: true
understand:
  warn_threshold:
    files: 2000
    lines: 200000
  extra: yes
`)
	c, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	c.Owner = "new@example.com"
	c.Approvals.Signed = true
	if err := Write(root, c); err != nil {
		t.Fatal(err)
	}
	want := `# project config
lyceum: 1
owner: new@example.com # the owner
base_branch: main
team: core
approvals:
  signed: true
  quorum: 2
  harness_prompt: allow
harnesses: [claude-code]
hooks: true
understand:
  warn_threshold:
    files: 2000
    lines: 200000
  extra: yes
`
	if got := readConfig(t, root); got != want {
		t.Errorf("config =\n%s\nwant\n%s", got, want)
	}
}

func TestWriteAddsMissingKeys(t *testing.T) {
	root := lyceumDir(t)
	writeConfig(t, root, "lyceum: 1\nowner: o@example.com\nbase_branch: main\n")
	c, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(root, c); err != nil {
		t.Fatal(err)
	}
	if got, want := readConfig(t, root), strings.Replace(defaultYAML, "owner@example.com", "o@example.com", 1); got != want {
		t.Errorf("config =\n%s\nwant\n%s", got, want)
	}
}
