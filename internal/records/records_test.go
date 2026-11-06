package records

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/oritromax/lyceum/internal/gittest"
)

// write writes content to rel under root, making its folders.
func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// repoWithSrc returns a repo with src/a.ts, src/b.ts, src/sub/c.ts and an
// ignored src/build.log committed.
func repoWithSrc(t *testing.T) string {
	t.Helper()
	root := gittest.InitWithCommit(t)
	write(t, root, ".gitignore", "*.log\n")
	write(t, root, "src/a.ts", "export const a = 1\n")
	write(t, root, "src/b.ts", "export const b = 2\n")
	write(t, root, "src/sub/c.ts", "export const c = 3\n")
	write(t, root, "src/build.log", "noise\n")
	gittest.Git(t, root, "add", "-A")
	gittest.Git(t, root, "commit", "-q", "-m", "src")
	return root
}

// cites returns cites of paths with their current hashes filled in.
func cites(t *testing.T, root string, paths ...string) []Cite {
	t.Helper()
	cur, err := Hash(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	var cs []Cite
	for _, p := range paths {
		h := cur[strings.TrimSuffix(p, "/")]
		if h.Missing() {
			t.Fatalf("%s is missing", p)
		}
		cs = append(cs, Cite{Path: p, Blob: h.Blob, Tree: h.Tree})
	}
	return cs
}

// freshness hashes cs's paths now and checks cs against them.
func freshness(t *testing.T, root string, cs []Cite) Freshness {
	t.Helper()
	var paths []string
	for _, c := range cs {
		paths = append(paths, c.Path)
	}
	cur, err := Hash(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	return Check(cs, cur)
}

func TestHashMatchesGit(t *testing.T) {
	root := repoWithSrc(t)
	cur, err := Hash(root, []string{"src/", "src/a.ts", "src/sub"})
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{
		"src":      gittest.Git(t, root, "rev-parse", "HEAD:src"),
		"src/sub":  gittest.Git(t, root, "rev-parse", "HEAD:src/sub"),
		"src/a.ts": gittest.Git(t, root, "rev-parse", "HEAD:src/a.ts"),
	} {
		if got := cur[p].Tree + cur[p].Blob; got != want {
			t.Errorf("%s: hash %s, want git's %s", p, got, want)
		}
	}
}

func TestHashSHA256Repo(t *testing.T) {
	gittest.Isolate(t)
	root := t.TempDir()
	gittest.Git(t, root, "init", "-q", "--object-format=sha256", "-b", "main")
	gittest.Git(t, root, "config", "user.name", "Test Owner")
	gittest.Git(t, root, "config", "user.email", gittest.Email)
	write(t, root, "src/a.ts", "a\n")
	gittest.Git(t, root, "add", "-A")
	gittest.Git(t, root, "commit", "-q", "-m", "src")
	cur, err := Hash(root, []string{"src/"})
	if err != nil {
		t.Fatal(err)
	}
	if want := gittest.Git(t, root, "rev-parse", "HEAD:src"); cur["src"].Tree != want {
		t.Errorf("tree %s, want git's %s", cur["src"].Tree, want)
	}
}

func TestFreshness_AC69(t *testing.T) {
	root := repoWithSrc(t)
	cs := cites(t, root, "src/a.ts", "src/")
	// An ignored file in the folder and a commit of other files change nothing.
	write(t, root, "src/build.log", "more noise\n")
	write(t, root, "README.md", "readme\n")
	gittest.Git(t, root, "add", "README.md")
	gittest.Git(t, root, "commit", "-q", "-m", "readme")
	if got := freshness(t, root, cs); got.State != Fresh {
		t.Errorf("freshness = %+v, want fresh", got)
	}
}

func TestFreshness_AC70(t *testing.T) {
	root := repoWithSrc(t)
	cs := cites(t, root, "src/a.ts", "src/b.ts")
	write(t, root, "src/a.ts", "export const a = 10\n")
	want := Freshness{State: Drifted, Changed: []string{"src/a.ts"}}
	if got := freshness(t, root, cs); !reflect.DeepEqual(got, want) {
		t.Errorf("uncommitted change: freshness = %+v, want %+v", got, want)
	}
	gittest.Git(t, root, "commit", "-q", "-am", "change a")
	if got := freshness(t, root, cs); !reflect.DeepEqual(got, want) {
		t.Errorf("committed change: freshness = %+v, want %+v", got, want)
	}
	// A changed file inside a cited folder drifts the folder.
	cs = cites(t, root, "src/")
	write(t, root, "src/sub/c.ts", "export const c = 30\n")
	if got := freshness(t, root, cs); got.State != Drifted || !reflect.DeepEqual(got.Changed, []string{"src/"}) {
		t.Errorf("file in folder changed: freshness = %+v, want drifted naming src/", got)
	}
}

func TestFreshness_AC70_PathCitedTwice(t *testing.T) {
	root := repoWithSrc(t)
	cs := cites(t, root, "src/a.ts", "src/b.ts", "src/a.ts")
	cs[0].Symbol, cs[2].Symbol = "a", "a()"
	write(t, root, "src/a.ts", "export const a = 10\n")
	want := Freshness{State: Drifted, Changed: []string{"src/a.ts"}}
	if got := freshness(t, root, cs); !reflect.DeepEqual(got, want) {
		t.Errorf("changed: freshness = %+v, want %+v", got, want)
	}
	if err := os.Remove(filepath.Join(root, "src", "a.ts")); err != nil {
		t.Fatal(err)
	}
	want = Freshness{State: Broken, Missing: []string{"src/a.ts"}}
	if got := freshness(t, root, cs); !reflect.DeepEqual(got, want) {
		t.Errorf("deleted: freshness = %+v, want %+v", got, want)
	}
	// The second cite is stale where the first is not: the path is named once.
	cs = cites(t, root, "src/b.ts", "src/b.ts")
	cs[1].Blob = "stale"
	want = Freshness{State: Drifted, Changed: []string{"src/b.ts"}}
	if got := freshness(t, root, cs); !reflect.DeepEqual(got, want) {
		t.Errorf("one stale cite: freshness = %+v, want %+v", got, want)
	}
}

func TestFreshness_AC71(t *testing.T) {
	root := repoWithSrc(t)
	cs := cites(t, root, "src/a.ts", "src/b.ts", "src/sub/")
	if err := os.Remove(filepath.Join(root, "src", "a.ts")); err != nil {
		t.Fatal(err)
	}
	want := Freshness{State: Broken, Missing: []string{"src/a.ts"}}
	if got := freshness(t, root, cs); !reflect.DeepEqual(got, want) {
		t.Errorf("deleted: freshness = %+v, want %+v", got, want)
	}
	gittest.Git(t, root, "checkout", "--", "src/a.ts")
	gittest.Git(t, root, "mv", "src/b.ts", "src/b2.ts")
	gittest.Git(t, root, "mv", "src/sub", "src/sub2")
	want = Freshness{State: Broken, Missing: []string{"src/b.ts", "src/sub/"}}
	if got := freshness(t, root, cs); !reflect.DeepEqual(got, want) {
		t.Errorf("renamed: freshness = %+v, want %+v", got, want)
	}
}

func TestFreshness_AC72(t *testing.T) {
	root := repoWithSrc(t)
	cs := cites(t, root, "src/")
	write(t, root, "src/sub/d.ts", "export const d = 4\n")
	want := Freshness{State: Drifted, Changed: []string{"src/"}}
	if got := freshness(t, root, cs); !reflect.DeepEqual(got, want) {
		t.Errorf("new untracked file: freshness = %+v, want %+v", got, want)
	}
	gittest.Git(t, root, "add", "-A")
	gittest.Git(t, root, "commit", "-q", "-m", "d")
	if got := freshness(t, root, cs); !reflect.DeepEqual(got, want) {
		t.Errorf("new committed file: freshness = %+v, want %+v", got, want)
	}
	// Removing it again is fresh: the folder's content is what it was.
	gittest.Git(t, root, "rm", "-q", "src/sub/d.ts")
	if got := freshness(t, root, cs); got.State != Fresh {
		t.Errorf("file removed again: freshness = %+v, want fresh", got)
	}
}

func TestFreshness_AC73(t *testing.T) {
	root := repoWithSrc(t)
	gittest.Git(t, root, "switch", "-q", "-c", "feat/abc123-payments")
	write(t, root, "src/a.ts", "export const a = 'branch'\n")
	cs := cites(t, root, "src/a.ts", "src/")
	r := &Record{ID: "abc123-payments", Kind: Understand, Title: "Payments", Created: "2026-01-01T00:00:00Z", Cites: cs}
	write(t, root, "lyceum/understand/abc123-payments.md", recordText(r))
	gittest.Git(t, root, "add", "-A")
	gittest.Git(t, root, "commit", "-q", "-m", "payments and its record")

	gittest.Git(t, root, "switch", "-q", "main")
	gittest.Git(t, root, "merge", "-q", "--squash", "feat/abc123-payments")
	gittest.Git(t, root, "commit", "-q", "-m", "squashed")
	gittest.Git(t, root, "branch", "-q", "-D", "feat/abc123-payments")
	gittest.Git(t, root, "gc", "-q", "--prune=now")

	entries, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{{Path: "lyceum/understand/abc123-payments.md", Kind: Understand, ID: "abc123-payments", Freshness: Freshness{State: Fresh}}}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("All on main after squash merge = %+v, want %+v", entries, want)
	}
}

// recordText is r as a record file with a one-line body.
func recordText(r *Record) string {
	var b strings.Builder
	b.WriteString("---\nid: " + r.ID + "\nkind: " + r.Kind + "\ntitle: " + r.Title + "\ncreated: " + r.Created + "\ncites:\n")
	for _, c := range r.Cites {
		b.WriteString("  - path: " + c.Path + "\n")
		if c.Blob != "" {
			b.WriteString("    blob: \"" + c.Blob + "\"\n")
		}
		if c.Tree != "" {
			b.WriteString("    tree: \"" + c.Tree + "\"\n")
		}
	}
	b.WriteString("---\n\nBody.\n")
	return b.String()
}

func TestAll(t *testing.T) {
	root := repoWithSrc(t)
	cs := cites(t, root, "src/a.ts")
	write(t, root, "lyceum/spec/payments.md", "---\ncites:\n  - path: src/a.ts\n    blob: \""+cs[0].Blob+"\"\n---\n# Payments\n")
	write(t, root, "lyceum/spec/plain.md", "# No frontmatter\n")
	write(t, root, "lyceum/spec/nocites.md", "---\nowner: me\n---\n# No cites\n")
	write(t, root, "lyceum/spec/unhashed.md", "---\ncites:\n  - path: src/a.ts\n---\n")
	write(t, root, "lyceum/trace/bbbbbb-flow.md", recordText(&Record{ID: "bbbbbb-flow", Kind: Trace, Title: "Flow", Created: "2026-01-01T00:00:00Z", Cites: cites(t, root, "src/b.ts")}))
	write(t, root, "lyceum/understand/aaaaaa-draft.md", "---\nkind: understand\ntitle: Draft\ncites:\n  - path: src/a.ts\n---\n")
	write(t, root, "lyceum/audit/cccccc-wrong.md", recordText(&Record{ID: "cccccc-wrong", Kind: Trace, Title: "Wrong", Created: "2026-01-01T00:00:00Z"}))
	write(t, root, "src/b.ts", "changed\n")

	entries, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Path: "lyceum/spec/payments.md", Kind: "area", Freshness: Freshness{State: Fresh}},
		{Path: "lyceum/spec/unhashed.md", Kind: "area", Freshness: Freshness{State: Invalid}, Reason: "field cites[0]: has no blob or tree"},
		{Path: "lyceum/understand/aaaaaa-draft.md", Kind: Understand, Freshness: Freshness{State: Invalid}, Reason: "field id: missing"},
		{Path: "lyceum/trace/bbbbbb-flow.md", Kind: Trace, ID: "bbbbbb-flow", Freshness: Freshness{State: Drifted, Changed: []string{"src/b.ts"}}},
		{Path: "lyceum/audit/cccccc-wrong.md", Kind: Audit, ID: "cccccc-wrong", Freshness: Freshness{State: Invalid}, Reason: `field kind: "trace", but the record is in lyceum/audit/`},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("All =\n%+v\nwant\n%+v", entries, want)
	}
}

func TestValidate(t *testing.T) {
	const ok = "kind: understand\ntitle: T\ncites:\n  - path: src/a.ts\n"
	for _, c := range []struct {
		front, field string
	}{
		{"kind: understand\ncites: []\n", "title"},
		{"title: T\n", "kind"},
		{"kind: notes\ntitle: T\n", "kind"},
		{"id: nope\n" + ok, "id"},
		{"created: yesterday\n" + ok, "created"},
		{"kind: trace\ntitle: T\ncites: src/a.ts\n", "cites"},
		{"kind: trace\ntitle: T\ncites:\n  - src/a.ts\n", "cites[0]"},
		{"kind: trace\ntitle: T\ncites:\n  - symbol: f()\n", "cites[0].path"},
		{"kind: trace\ntitle: T\ncites:\n  - path: /etc/passwd\n", "cites[0].path"},
		{"kind: trace\ntitle: T\ncites:\n  - path: ../x\n", "cites[0].path"},
		{"kind: trace\ntitle: T\ncites:\n  - path: src/./a.ts\n", "cites[0].path"},
		{"kind: trace\ntitle: T\ncites:\n  - path: src/a.ts\n  - path: src/\n    blob: \"1\"\n    tree: \"2\"\n", "cites[1]"},
	} {
		r, err := Parse([]byte("---\n" + c.front + "---\n"))
		if err == nil {
			err = r.Validate()
		}
		var fe *FieldError
		if !errors.As(err, &fe) || fe.Field != c.field {
			t.Errorf("%q: error %v, want a FieldError on %s", c.front, err, c.field)
		}
	}
	r, err := Parse([]byte("---\n" + ok + "---\n"))
	if err != nil || r.Validate() != nil {
		t.Errorf("valid record: %v, %v", err, r.Validate())
	}
	if _, err := Parse([]byte("no frontmatter\n")); err == nil {
		t.Error("a file with no frontmatter parsed")
	}
}

// TestValidateSteps_AC79: every "- " line under a trace's ## Steps, up to the
// next heading, is a step and must be marked [read] or [observed]; indented
// lines, other sections and other kinds are not checked.
func TestValidateSteps_AC79(t *testing.T) {
	const front = "---\nkind: %s\ntitle: T\n---\n"
	for _, c := range []struct {
		kind, body string
		bad        int // the unmarked step's number, 0 for none
	}{
		{"trace", "## Steps\n\n- [read] a\n- [observed] b\n", 0},
		{"trace", "## Steps\n- [read] a\n- b\n", 2},
		{"trace", "## Steps\n- [read] a\n  - detail\n### Sub\n- [observed] b\n", 0},
		{"trace", "## Steps\n- [read] a\n### Sub\n- c\n", 2},
		{"trace", "- intro\n## Steps\n- [read] a\n## Findings\n- spec drift\n", 0},
		{"trace", "# Title\n- intro\n", 0},
		{"understand", "## Steps\n- a\n", 0},
	} {
		r, err := Parse([]byte(fmt.Sprintf(front, c.kind) + c.body))
		if err != nil {
			t.Fatal(err)
		}
		err = r.Validate()
		want := ""
		if c.bad > 0 {
			want = fmt.Sprintf("## Steps: step %d is marked neither [read] nor [observed]", c.bad)
		}
		if got := fmt.Sprint(err); (want == "") != (err == nil) || want != "" && !strings.HasPrefix(got, want) {
			t.Errorf("%s %q: Validate = %v; want %q", c.kind, c.body, err, want)
		}
	}
}

func TestFill(t *testing.T) {
	root := repoWithSrc(t)
	in := "---\n# The agent's comment stays.\nkind: understand # how payments work\ntitle: How payments work\ncites:\n" +
		"  - path: src/a.ts\n    symbol: createPayment()\n    tree: \"stale\"\n  - path: src/sub/\n---\n\n## Summary\n\nBody kept.\n"
	r, err := Parse([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	cur, err := Hash(root, []string{"src/a.ts", "src/sub/"})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Fill("7c01de-how-payments-work", "2026-10-23T10:00:00Z", cur); err != nil {
		t.Fatal(err)
	}
	got, err := r.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	want := "---\nid: 7c01de-how-payments-work\n# The agent's comment stays.\nkind: understand # how payments work\ntitle: How payments work\ncreated: 2026-10-23T10:00:00Z\ncites:\n" +
		"  - path: src/a.ts\n    symbol: createPayment()\n    blob: \"" + cur["src/a.ts"].Blob + "\"\n  - path: src/sub/\n    tree: \"" + cur["src/sub"].Tree + "\"\n---\n\n## Summary\n\nBody kept.\n"
	if string(got) != want {
		t.Errorf("filled record =\n%s\nwant\n%s", got, want)
	}
	back, err := Parse(got)
	if err != nil || back.Validate() != nil || back.Complete() != nil {
		t.Fatalf("filled record does not read back complete: %v", err)
	}
	if f := Check(back.Cites, cur); f.State != Fresh {
		t.Errorf("filled record: freshness %+v, want fresh", f)
	}
}
