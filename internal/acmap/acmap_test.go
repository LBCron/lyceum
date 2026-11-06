package acmap

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// id is built by concatenation so this file does not itself map that AC.
const id = "AC" + "14"

func TestWholeWord(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{id, true},
		{"// " + id + ": summary", true},
		{"TestFoo_" + id, true},
		{id + "_bar", true},
		{"(" + id + ")", true},
		{id + "0", false},
		{"X" + id, false},
		{"5" + id, false},
		{id + "x", false},
		{strings.ToLower(id), false},
	}
	for _, c := range cases {
		got := false
		for _, found := range wholeWordIDs([]byte(c.in)) {
			if found == id {
				got = true
			}
		}
		if got != c.want {
			t.Errorf("%q maps %s = %v, want %v", c.in, id, got, c.want)
		}
	}
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		glob, path string
		want       bool
	}{
		{"**/*_test.go", "a_test.go", true},
		{"**/*_test.go", "internal/acmap/a_test.go", true},
		{"**/*_test.go", "internal/a.go", false},
		{"**/testdata/script/*.txtar", "cmd/lyceum/testdata/script/ac095_x.txtar", true},
		{"**/testdata/script/*.txtar", "cmd/lyceum/testdata/script/sub/x.txtar", false},
		{"cmd/*.go", "cmd/main.go", true},
		{"cmd/*.go", "cmd/lyceum/main.go", false},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/y/b", true},
	}
	for _, c := range cases {
		if got := MatchAny([]string{c.glob}, c.path); got != c.want {
			t.Errorf("match(%q, %q) = %v, want %v", c.glob, c.path, got, c.want)
		}
	}
}

func TestSpecACs(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.md"), "| ID | Criterion |\n|---|---|\n| AC9010 | ten |\n| AC9002 | two |\nsee AC9003 in prose\n")
	write(t, filepath.Join(dir, "b.md"), "|AC9001| one |\n| AC9002 | dup |\n")
	write(t, filepath.Join(dir, "c.txt"), "| AC9004 | not markdown |\n")

	got, err := SpecACs(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"AC9001", "AC9002", "AC9010"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SpecACs = %v, want %v", got, want)
	}
}

func TestMapped(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "x_test.go"), "func TestA_AC9001(t *testing.T) {} // AC9001 again\n")
	write(t, filepath.Join(root, "sub", "y_test.go"), "// AC9001, AC9002; AC90020\n")
	write(t, filepath.Join(root, "sub", "z.go"), "// AC9003\n")

	got, err := Mapped(root, []string{"**/*_test.go"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"AC9001":  {"sub/y_test.go", "x_test.go"},
		"AC9002":  {"sub/y_test.go"},
		"AC90020": {"sub/y_test.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Mapped = %v, want %v", got, want)
	}
}

func TestTestingGlobs(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "TESTING.md")
	write(t, p, "---\nlyceum: 1\ntest_files: [\"**/*_test.go\", \"**/*.txtar\"]\n---\n\n# TESTING\n")
	got, err := TestingGlobs(p)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"**/*_test.go", "**/*.txtar"}; !reflect.DeepEqual(got, want) {
		t.Errorf("TestingGlobs = %v, want %v", got, want)
	}

	write(t, p, "# no frontmatter\n")
	if _, err := TestingGlobs(p); err == nil {
		t.Error("TestingGlobs without frontmatter: want error")
	}
	write(t, p, "---\nlyceum: 1\n---\n")
	if _, err := TestingGlobs(p); err == nil {
		t.Error("TestingGlobs without test_files: want error")
	}
}

func TestSortIDs(t *testing.T) {
	ids := []string{"AC90100", "AC9009", "AC90010", "AC9001"}
	sortIDs(ids)
	if want := []string{"AC9001", "AC9009", "AC90010", "AC90100"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("sortIDs = %v, want %v", ids, want)
	}
}

func write(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenSpecACs_AC156(t *testing.T) {
	dir := t.TempDir()
	text := "| AC9001 | Given a, then b |\n" +
		"| AC9002 | Withdrawn, D1 |\n" +
		"| AC9003 | Given c, then d (Deferred after v1, D2.) |\n" +
		"| AC9004 | Given a withdrawn AC, then e |\n" +
		"| AC9005 | Given a row ending `(Deferred after v1, …)`, then f |\n"
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := OpenSpecACs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"AC9001", "AC9004", "AC9005"}; !slices.Equal(got, want) {
		t.Errorf("OpenSpecACs = %v; want %v", got, want)
	}
}

// TestDefinitions_AC167: the two forms an AC is defined in, what closes each,
// and what is only a mention.
func TestDefinitions_AC167(t *testing.T) {
	cases := []struct {
		name, text string
		want       []Definition
	}{
		{"table row", "| AC9001 | Given a, then b |\n", []Definition{{"AC9001", false}}},
		{"table row, no spaces", "|AC9001| one |\n", []Definition{{"AC9001", false}}},
		{"dash bullet", "- **AC9001** Given a, then b\n", []Definition{{"AC9001", false}}},
		{"star bullet", "* **AC9001** Given a, then b\n", []Definition{{"AC9001", false}}},
		{"indented bullet", "  - **AC9001** Given a\n", []Definition{{"AC9001", false}}},
		{"bullet, no final newline", "- **AC9001**", []Definition{{"AC9001", false}}},
		{"bullet, colon", "- **AC9001**: Given a\n", []Definition{{"AC9001", false}}},
		{"both forms", "| AC9001 | a |\n\n- **AC9002** b\n", []Definition{{"AC9001", false}, {"AC9002", false}}},
		{"CRLF", "- **AC9001** Given a\r\n  (Deferred after v1, D1)\r\n", []Definition{{"AC9001", true}}},

		{"mid-sentence", "See AC9001 for this.\n", nil},
		{"bold mid-sentence", "See **AC9001** for this.\n", nil},
		{"bold, no bullet", "**AC9001** Given a\n", nil},
		{"bold not first in the bullet", "- Given **AC9001**, then b\n", nil},
		{"plain ID in a bullet", "- AC9001 Given a\n", nil},
		{"longer word", "- **AC9001x** Given a\n", nil},
		{"lowercase", "- **ac9001** Given a\n", nil},
		{"no space after the marker", "-**AC9001** Given a\n", nil},
		{"registry row", "| orders | `spec/orders.md` | Orders | AC9001-AC9002 |\n", nil},
		{"inside another AC", "- **AC9001** Given AC9002 and **AC9003**, then b\n", []Definition{{"AC9001", false}}},

		{"bullet withdrawn", "- **AC9001** Withdrawn, D1\n", []Definition{{"AC9001", true}}},
		{"bullet withdrawn, marked up", "- **AC9001**: ~~Withdrawn~~ D1\n", []Definition{{"AC9001", true}}},
		{"bullet deferred", "- **AC9001** Given a (Deferred after v1, D2.)\n", []Definition{{"AC9001", true}}},
		{"deferred on a continuation", "- **AC9001** Given a,\n  then b.\n  (Deferred after v1, D2.)\n", []Definition{{"AC9001", true}}},
		{"deferred wrapped across lines", "- **AC9001** Given a (Deferred after\n  v1, D2.)\n", []Definition{{"AC9001", true}}},
		{"unindented continuation", "- **AC9001** Given a\n(Deferred after v1, D2.)\n", []Definition{{"AC9001", true}}},

		{"mentions withdrawn", "- **AC9001** Given a withdrawn AC, then b\n", []Definition{{"AC9001", false}}},
		{"mentions deferred", "- **AC9001** Given a note `(Deferred after v1, …)` in text, then b\n", []Definition{{"AC9001", false}}},
		{"text after the marker", "- **AC9001** Given a (Deferred after v1, D2) [DERIVED: x]\n", []Definition{{"AC9001", false}}},
		{"next bullet's marker", "- **AC9001** Given a\n- **AC9002** Given b (Deferred after v1)\n", []Definition{{"AC9001", false}, {"AC9002", true}}},
		{"plain bullet ends the item", "- **AC9001** Given a\n- (Deferred after v1)\n", []Definition{{"AC9001", false}}},
		{"blank line ends the item", "- **AC9001** Given a\n\n  (Deferred after v1)\n", []Definition{{"AC9001", false}}},
		{"heading ends the item", "- **AC9001** Given a\n## (Deferred after v1)\n", []Definition{{"AC9001", false}}},
		{"table row ends the item", "- **AC9001** Given a\n| (Deferred after v1) |\n", []Definition{{"AC9001", false}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Definitions([]byte(c.text)); !slices.Equal(got, c.want) {
				t.Errorf("Definitions(%q) = %v; want %v", c.text, got, c.want)
			}
		})
	}
}

// TestSpecACsBullets_AC167: the readers over a directory take bullets, and
// lyceum/SPEC.md beside the directory is not an area file: its rows define nothing.
func TestSpecACsBullets_AC167(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "spec")
	write(t, filepath.Join(root, "SPEC.md"), "| AC9009 | not an area file |\n| orders | `spec/orders.md` | Orders | AC9001 |\n- **AC9008** nor this\n")
	write(t, filepath.Join(dir, "orders.md"), "- **AC9002** Given a\n- **AC9001** Withdrawn, D1\n* **AC9003** Given b\n  (Deferred after v1, D2.)\n")
	write(t, filepath.Join(dir, "rows.md"), "| AC9004 | Given c |\n")

	got, err := SpecACs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"AC9001", "AC9002", "AC9003", "AC9004"}; !slices.Equal(got, want) {
		t.Errorf("SpecACs = %v; want %v", got, want)
	}
	open, err := OpenSpecACs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"AC9002", "AC9004"}; !slices.Equal(open, want) {
		t.Errorf("OpenSpecACs = %v; want %v", open, want)
	}
}
