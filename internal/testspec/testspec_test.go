package testspec

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gittest"
)

const full = `---
lyceum: 1
suite: go test ./...
single: go test ./... -run {name}
test_files: ["**/*_test.go", "testdata/*.txtar"]
subsets:
  unit: go test ./... -short
  integration: go test ./... -run Integration
environments:
  test:
    url: http://localhost:8080
    up: docker compose up -d
timeout: 90s
---

# TESTING.md

Body text is guidance, not executed.
`

func TestParseFull(t *testing.T) {
	s, err := Parse([]byte(full))
	if err != nil {
		t.Fatal(err)
	}
	if s.Lyceum != 1 || s.Suite != "go test ./..." || s.Single != "go test ./... -run {name}" {
		t.Errorf("lyceum, suite, single = %d, %q, %q", s.Lyceum, s.Suite, s.Single)
	}
	if want := []string{"**/*_test.go", "testdata/*.txtar"}; !slices.Equal(s.TestFiles, want) {
		t.Errorf("TestFiles = %q; want %q", s.TestFiles, want)
	}
	if s.Subsets["unit"] != "go test ./... -short" || len(s.Subsets) != 2 {
		t.Errorf("Subsets = %q", s.Subsets)
	}
	if got := s.SubsetNames(); !slices.Equal(got, []string{"integration", "unit"}) {
		t.Errorf("SubsetNames = %q", got)
	}
	if s.Environments["test"]["url"] != "http://localhost:8080" || s.Environments["test"]["up"] != "docker compose up -d" {
		t.Errorf("Environments = %q", s.Environments)
	}
	if s.Timeout != 90*time.Second {
		t.Errorf("Timeout = %s; want 90s", s.Timeout)
	}
	if s.Unknown != nil {
		t.Errorf("Unknown = %q; want none", s.Unknown)
	}
}

// TestParseUnknownKeys: keys this build does not read are listed, in order,
// and never refused.
func TestParseUnknownKeys(t *testing.T) {
	s, err := Parse([]byte("---\nlyceum: 1\nsuite: x\nretries: 2\nsuit: y\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"retries", "suit"}; !slices.Equal(s.Unknown, want) {
		t.Errorf("Unknown = %q; want %q", s.Unknown, want)
	}
}

func TestParseDefaults(t *testing.T) {
	s, err := Parse([]byte("---\r\nlyceum: 1\r\nsuite: make test\r\n---\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Timeout != DefaultTimeout || s.Single != "" || s.TestFiles != nil || s.Subsets != nil || s.SubsetNames() == nil {
		t.Errorf("defaults = %+v", s)
	}
}

func TestParseErrorsNameTheKey(t *testing.T) {
	for _, tc := range []struct {
		name, front, want string
	}{
		{"no frontmatter", "# TESTING.md\n", "TESTING.md: no frontmatter"},
		{"unterminated", "---\nlyceum: 1\n", `TESTING.md: malformed frontmatter: no closing "---" line`},
		{"empty", "---\n---\n", "TESTING.md frontmatter: want a mapping"},
		{"not a mapping", "---\n- a\n---\n", "TESTING.md frontmatter: want a mapping"},
		{"lyceum missing", "---\nsuite: x\n---\n", "TESTING.md frontmatter: lyceum: missing; this build reads lyceum: 1"},
		{"lyceum 2", "---\nlyceum: 2\nsuite: x\n---\n", "TESTING.md frontmatter: lyceum: schema version 2 is not supported"},
		{"lyceum not a number", "---\nlyceum: one\nsuite: x\n---\n", "TESTING.md frontmatter: lyceum: "},
		{"suite missing", "---\nlyceum: 1\n---\n", "TESTING.md frontmatter: suite: missing"},
		{"suite blank", "---\nlyceum: 1\nsuite: '  '\n---\n", "TESTING.md frontmatter: suite: missing"},
		{"suite not a string", "---\nlyceum: 1\nsuite: [a]\n---\n", "TESTING.md frontmatter: suite: "},
		{"single without name", "---\nlyceum: 1\nsuite: x\nsingle: go test\n---\n", `TESTING.md frontmatter: single: "go test" does not contain {name}`},
		{"test_files not a list", "---\nlyceum: 1\nsuite: x\ntest_files: '*_test.go'\n---\n", "TESTING.md frontmatter: test_files: "},
		{"test_files bad glob", "---\nlyceum: 1\nsuite: x\ntest_files: ['[a']\n---\n", `TESTING.md frontmatter: test_files: "[a" is not a valid glob`},
		{"test_files empty glob", "---\nlyceum: 1\nsuite: x\ntest_files: ['']\n---\n", "TESTING.md frontmatter: test_files: a glob is empty"},
		{"subsets not a map", "---\nlyceum: 1\nsuite: x\nsubsets: [unit]\n---\n", "TESTING.md frontmatter: subsets: "},
		{"subset no command", "---\nlyceum: 1\nsuite: x\nsubsets:\n  unit: ''\n---\n", "TESTING.md frontmatter: subsets: unit has no command"},
		{"environments not a map", "---\nlyceum: 1\nsuite: x\nenvironments: test\n---\n", "TESTING.md frontmatter: environments: "},
		{"timeout not a duration", "---\nlyceum: 1\nsuite: x\ntimeout: soon\n---\n", "TESTING.md frontmatter: timeout: "},
		{"timeout zero", "---\nlyceum: 1\nsuite: x\ntimeout: 0s\n---\n", "TESTING.md frontmatter: timeout: 0s is not a positive duration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.front))
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("Parse = %v; want an error starting %q", err, tc.want)
			}
		})
	}
}

// AC141: every problem is reported in one pass, and a value that could not be
// read is reported once, not again as missing.
func TestParseReportsEveryError_AC141(t *testing.T) {
	_, err := Parse([]byte("---\nlyceum: 2\nsingle: go test\ntimeout: soon\ntest_files: ['']\n---\n"))
	if err == nil {
		t.Fatal("Parse = nil; want errors")
	}
	got := err.Error()
	for _, want := range []string{"lyceum: schema version 2", "suite: missing", "single: ", "timeout: ", "test_files: a glob is empty"} {
		if !strings.Contains(got, want) {
			t.Errorf("Parse = %q; want it to name %q", got, want)
		}
	}
	if strings.Contains(got, "not a positive duration") {
		t.Errorf("Parse = %q; an unreadable timeout is reported once", got)
	}
}

func TestReadMissing(t *testing.T) {
	if _, err := Read(t.TempDir()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read = %v; want fs.ErrNotExist", err)
	}
}

// approve appends an approve testing event for TESTING.md's current hash.
func approve(t *testing.T, root string, counts bool) {
	t.Helper()
	h, err := approval.Hash(filepath.Join(root, Path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := events.Append(root, events.Event{Type: "approve", Fields: map[string]any{
		"gate": "testing", "hashes": map[string]string{Path: h}, "counts": counts,
	}}); err != nil {
		t.Fatal(err)
	}
}

func TestApproved(t *testing.T) {
	root := gittest.InitWithCommit(t)
	if err := os.Mkdir(filepath.Join(root, "lyceum"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Approved(root); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Approved without TESTING.md = %v; want fs.ErrNotExist", err)
	}
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, Path), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	check := func(wantApproved, wantChanged bool) {
		t.Helper()
		approved, changed, err := Approved(root)
		if err != nil {
			t.Fatal(err)
		}
		if approved != wantApproved || changed != wantChanged {
			t.Errorf("Approved = %t, %t; want %t, %t", approved, changed, wantApproved, wantChanged)
		}
	}

	write(full)
	check(false, false)
	approve(t, root, false) // a non-owner approval does not count
	check(false, false)
	approve(t, root, true)
	check(true, false)
	write(full + "edited\n")
	check(true, true)
	approve(t, root, false)
	check(true, true)
	approve(t, root, true)
	check(true, false)
}

func TestHasTests(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{".git/x_test.go", "src/main.go"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := &Spec{TestFiles: []string{"**/*_test.go"}}
	if found, err := s.HasTests(root); err != nil || found {
		t.Errorf("HasTests = %t, %v; want false: .git is not searched", found, err)
	}
	if found, err := (&Spec{}).HasTests(root); err != nil || found {
		t.Errorf("HasTests with no test_files = %t, %v; want false", found, err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main_test.go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if found, err := s.HasTests(root); err != nil || !found {
		t.Errorf("HasTests = %t, %v; want true", found, err)
	}
}
