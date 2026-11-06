package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// repoPlan is a plan written by hand, used as a real-world fixture. Tests copy it; they never write it.
const repoPlan = "testdata/a07bb8-m1-core.md"

func copyFile(t *testing.T, src string) string {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	return writeTemp(t, string(data))
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPlanRoundTripRepoPlan(t *testing.T) {
	path := copyFile(t, repoPlan)
	before := readString(t, path)
	p, err := ReadPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "a07bb8-m1-core" || p.Type != "feat" || p.Branch != "feat/a07bb8-m1-core" {
		t.Errorf("identity = %q %q %q", p.ID, p.Type, p.Branch)
	}
	if len(p.Areas) == 0 || len(p.Files) == 0 || len(p.ACs) == 0 {
		t.Errorf("agent fields not read: areas %v, files %v, acs %v", p.Areas, p.Files, p.ACs)
	}
	if len(p.Approvals) != 1 || p.Approvals[0].Gate != "spec" || len(p.Approvals[0].Hashes) != 1 {
		t.Errorf("approvals = %+v", p.Approvals)
	}
	if !strings.HasPrefix(string(p.Body), "\n## What and why\n") {
		t.Errorf("body starts %q", string(p.Body)[:min(40, len(p.Body))])
	}
	if err := WritePlan(path, p); err != nil {
		t.Fatal(err)
	}
	if after := readString(t, path); after != before {
		t.Errorf("round trip changed the file:\n%s", after)
	}
}

func TestSetStatusChangesOnlyThatLine(t *testing.T) {
	path := copyFile(t, repoPlan)
	before := strings.Split(readString(t, path), "\n")
	p, err := ReadPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	next := StatusImplemented
	if p.Status == next {
		next = StatusClosed
	}
	if err := p.SetCLIField("status", next); err != nil {
		t.Fatal(err)
	}
	if p.Status != next {
		t.Errorf("p.Status = %q after SetCLIField, want %q", p.Status, next)
	}
	if err := WritePlan(path, p); err != nil {
		t.Fatal(err)
	}
	after := strings.Split(readString(t, path), "\n")
	if len(after) != len(before) {
		t.Fatalf("line count %d -> %d", len(before), len(after))
	}
	var changed []string
	for i := range before {
		if before[i] != after[i] {
			changed = append(changed, before[i]+" -> "+after[i])
		}
	}
	if len(changed) != 1 || !strings.HasSuffix(changed[0], " -> status: "+next) {
		t.Errorf("changed lines = %q, want only the status line", changed)
	}
}

// specExample has column-aligned comments, unknown keys with comments, and a body with "---" lines.
const specExample = `---
# head comment
id: a3f9c2-csv-export          # CLI-owned
type: feat                     # CLI-owned
title: CSV export
branch: feat/a3f9c2-csv-export # CLI-owned
status: planned                # CLI-owned: planned | in-progress | implemented | closed

# gate comment
gate: plan                     # CLI-owned: current gate
areas: [orders]                # agent-written
files:
  - src/orders/export.ts   # first
  - src/orders/routes.ts
custom_key: {b: 2, a: 1}       # unknown, flow map, keys out of order
nested:
    deep: "kept"               # odd indent
    list: [x,   y]
# trailing comment
---
## What and why
---
body text with no trailing newline`

func TestSetCLIFieldKeepsOtherKeysAndComments(t *testing.T) {
	path := writeTemp(t, specExample)
	p, err := ReadPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.SetCLIField("status", StatusInProgress); err != nil {
		t.Fatal(err)
	}
	if err := p.SetCLIField("gate", "build"); err != nil {
		t.Fatal(err)
	}
	if err := WritePlan(path, p); err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer(
		"status: planned                # CLI-owned: planned", "status: in-progress # CLI-owned: planned",
		"gate: plan                     # CLI-owned", "gate: build # CLI-owned",
	).Replace(specExample)
	if got := readString(t, path); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetCLIFieldApprovals(t *testing.T) {
	path := writeTemp(t, specExample)
	p, err := ReadPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 23, 10, 12, 0, 0, time.UTC)
	a := Approval{Gate: "plan", Hashes: map[string]string{"lyceum/plan/a3f9c2-csv-export.md": "sha256:ab"}, Commit: "1a2b3c4", By: "owner@example.com", At: at}

	// Absent key: appended after everything else.
	if err := p.SetCLIField("approvals", []Approval{a}); err != nil {
		t.Fatal(err)
	}
	if err := WritePlan(path, p); err != nil {
		t.Fatal(err)
	}
	got := readString(t, path)
	added := "approvals:\n  - gate: plan\n    hashes:\n      lyceum/plan/a3f9c2-csv-export.md: sha256:ab\n" +
		"    commit: 1a2b3c4\n    by: owner@example.com\n    at: 2026-10-23T10:12:00Z\n"
	if want := strings.Replace(specExample, "# trailing comment\n", "# trailing comment\n"+added, 1); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	// Present key: replaced in place, the next key's comments untouched.
	p, err = ReadPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	b := a
	b.Gate = "build"
	if err := p.SetCLIField("approvals", []Approval{a, b}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetCLIField("title", "CSV: export"); err != nil {
		t.Fatal(err)
	}
	if err := WritePlan(path, p); err != nil {
		t.Fatal(err)
	}
	p, err = ReadPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Approvals) != 2 || p.Approvals[1].Gate != "build" || !p.Approvals[1].At.Equal(at) {
		t.Errorf("approvals = %+v", p.Approvals)
	}
	if p.Title != "CSV: export" {
		t.Errorf("title = %q", p.Title)
	}
	got = readString(t, path)
	for _, line := range strings.Split(specExample, "\n") {
		if strings.HasPrefix(line, "title:") {
			continue
		}
		if !strings.Contains(got, line) {
			t.Errorf("line %q lost:\n%s", line, got)
		}
	}
}

func TestSetCLIFieldRefusesAgentKeys(t *testing.T) {
	p, err := ReadPlan(writeTemp(t, specExample))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"areas", "files", "acs", "custom_key", "nested", "Status", ""} {
		if err := p.SetCLIField(key, "x"); err == nil || !strings.Contains(err.Error(), "not a CLI-owned") {
			t.Errorf("SetCLIField(%q) error = %v, want a CLI-owned refusal", key, err)
		}
	}
	if string(p.front) != strings.SplitN(specExample, "---\n", 3)[1] {
		t.Errorf("frontmatter changed after refusals:\n%s", p.front)
	}
}

func TestSetCLIFieldRejectsBadStatus(t *testing.T) {
	p, err := ReadPlan(writeTemp(t, specExample))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.SetCLIField("status", "done"); err == nil || !strings.Contains(err.Error(), `status "done"`) {
		t.Errorf("error = %v, want a status error", err)
	}
	if p.Status != StatusPlanned {
		t.Errorf("status = %q after a refused set", p.Status)
	}
}

func TestReadPlanStatus(t *testing.T) {
	for _, s := range []string{StatusPlanned, StatusInProgress, StatusImplemented, StatusClosed} {
		p, err := ReadPlan(writeTemp(t, "---\nid: a\nstatus: "+s+"\n---\n"))
		if err != nil || p.Status != s {
			t.Errorf("status %q: %v, %+v", s, err, p)
		}
	}
	for _, fm := range []string{"status: done", "status: Planned", "status: ''", "id: a"} {
		_, err := ReadPlan(writeTemp(t, "---\n"+fm+"\n---\n"))
		if err == nil || !strings.Contains(err.Error(), "is not one of planned, in-progress, implemented, closed") {
			t.Errorf("%q: error = %v, want a status error", fm, err)
		}
	}
}

func TestReadPlanMalformed(t *testing.T) {
	tests := map[string]string{
		"empty file":          "",
		"no opening line":     "id: a\nstatus: planned\n---\n",
		"opening not first":   "\n---\nstatus: planned\n---\n",
		"no closing line":     "---\nstatus: planned\n",
		"invalid yaml":        "---\nstatus: planned\nareas: [a, b\n---\n",
		"bad indentation":     "---\nstatus: planned\n  id: a\n---\n",
		"empty frontmatter":   "---\n---\nbody\n",
		"scalar frontmatter":  "---\njust text\n---\n",
		"list frontmatter":    "---\n- status: planned\n---\n",
		"duplicate key":       "---\nstatus: planned\nstatus: closed\n---\n",
		"wrong type for area": "---\nstatus: planned\nareas: {a: 1}\n---\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			if p, err := ReadPlan(writeTemp(t, content)); err == nil {
				t.Errorf("ReadPlan = %+v, want an error", p)
			} else if !strings.Contains(err.Error(), "frontmatter") && !strings.Contains(err.Error(), "status") {
				t.Errorf("error %q does not say what is wrong", err)
			}
		})
	}
}

func TestPlanBodyBytePreserved(t *testing.T) {
	bodies := []string{
		"",
		"\n",
		"no newline",
		"\n\n## Leading blanks\n---\n---\ntrailing\n\n",
		"crlf\r\nbody\r\n---\r\n",
		"  ---  \n...\n\ttabs\n",
	}
	for _, body := range bodies {
		for _, crlf := range []bool{false, true} {
			head := "---\nstatus: planned\n---\n"
			if crlf {
				head = "---\r\nstatus: planned\r\n---\r\n"
			}
			path := writeTemp(t, head+body)
			p, err := ReadPlan(path)
			if err != nil {
				t.Fatalf("%q: %v", body, err)
			}
			if string(p.Body) != body {
				t.Errorf("Body = %q, want %q", p.Body, body)
			}
			if err := p.SetCLIField("gate", "plan"); err != nil {
				t.Fatal(err)
			}
			if err := WritePlan(path, p); err != nil {
				t.Fatal(err)
			}
			if got := readString(t, path); !strings.HasSuffix(got, "---"+map[bool]string{false: "\n", true: "\r\n"}[crlf]+body) {
				t.Errorf("after write, file = %q, want body %q intact", got, body)
			}
		}
	}
}

func TestNewPlan(t *testing.T) {
	p, err := NewPlan("a3f9c2-csv-export", "feat", "CSV export: v2 #1", "feat/a3f9c2-csv-export", StatusPlanned, "spec")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plan.md")
	if err := WritePlan(path, p); err != nil {
		t.Fatal(err)
	}
	want := `---
id: a3f9c2-csv-export
type: feat
title: 'CSV export: v2 #1'
branch: feat/a3f9c2-csv-export
status: planned
gate: spec
approvals: []
---

## What and why

## Spec delta

## Steps

## Progress

## Log
`
	if got := readString(t, path); got != want {
		t.Errorf("new plan file:\n%s\nwant:\n%s", got, want)
	}
	r, err := ReadPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "a3f9c2-csv-export" || r.Type != "feat" || r.Title != "CSV export: v2 #1" ||
		r.Branch != "feat/a3f9c2-csv-export" || r.Status != StatusPlanned || r.Gate != "spec" || len(r.Approvals) != 0 {
		t.Errorf("read back = %+v", r)
	}
	// The CLI can set fields on it like on any other plan.
	if err := r.SetCLIField("status", StatusInProgress); err != nil {
		t.Fatal(err)
	}
}

// TestRefactorFields: goal: and public_surface: are read as agent fields, and
// Fields lists every key, unknown ones included.
func TestRefactorFields(t *testing.T) {
	p, err := ParsePlan([]byte("---\nid: a3f9c2-split\ntype: refactor\nstatus: planned\ngoal: performance\nbenchmark: sh ./bench.sh\npublic_surface: [GET /orders, Export()]\nextra: 1\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Goal != "performance" || p.Benchmark != "sh ./bench.sh" || len(p.PublicSurface) != 2 || p.PublicSurface[1] != "Export()" {
		t.Errorf("goal %q, benchmark %q, public_surface %q", p.Goal, p.Benchmark, p.PublicSurface)
	}
	fields, err := p.Fields()
	if err != nil {
		t.Fatal(err)
	}
	if fields["goal"] != "performance" || fields["extra"] != 1 || len(fields) != 7 {
		t.Errorf("Fields = %v", fields)
	}
}

// TestAppendLog: the line goes after the Log section's last entry, before any
// section after it; a body with no Log section gets one at the end.
func TestAppendLog(t *testing.T) {
	const line = "- 2026-01-01: amended: AC2 changes"
	for _, c := range []struct{ name, body, want string }{
		{"empty last section", "\n## Steps\n\n## Log\n", "\n## Steps\n\n## Log\n\n" + line + "\n"},
		{"after entries", "\n## Log\n\n- 2025-12-31: note\n\n", "\n## Log\n\n- 2025-12-31: note\n" + line + "\n"},
		{"section after", "\n## Log\n\n- a\n\n## Notes\nx\n", "\n## Log\n\n- a\n" + line + "\n\n## Notes\nx\n"},
		{"empty, section after", "\n## Log\n\n## Notes\n", "\n## Log\n\n" + line + "\n\n## Notes\n"},
		{"no section", "\n## Steps\n- s\n", "\n## Steps\n- s\n\n## Log\n\n" + line + "\n"},
	} {
		p, err := ParsePlan([]byte("---\nid: a3f9c2-x\nstatus: planned\n---\n" + c.body))
		if err != nil {
			t.Fatal(err)
		}
		p.AppendLog(line)
		if got := string(p.Body); got != c.want {
			t.Errorf("%s: body %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSection(t *testing.T) {
	const front = "---\nid: a3f9c2-x\nstatus: planned\n---\n"
	for _, c := range []struct{ name, body, want string }{
		{"to next section", "\n## What and why\n\nWhy.\n\n## Steps\n- s\n", "Why."},
		{"to the end", "\n## What and why\n\nWhy.\nAnd more.\n", "Why.\nAnd more."},
		{"absent", "\n## Steps\n- s\n", ""},
		{"empty", "\n## What and why\n\n## Steps\n", ""},
		{"twice", "\n## What and why\nA\n## Steps\n## What and why\nB\n", "A\nB"},
		{"deeper heading kept", "\n## What and why\nA\n### Detail\nB\n## Log\n", "A\n### Detail\nB"},
		{"name must match", "\n## What and why not\nA\n", ""},
	} {
		got, err := Section([]byte(front+c.body), "What and why")
		if err != nil || string(got) != c.want {
			t.Errorf("%s: Section = %q, %v; want %q", c.name, got, err, c.want)
		}
	}
	if _, err := Section([]byte("no frontmatter\n"), "What and why"); err == nil {
		t.Error("Section of a file without frontmatter succeeded")
	}
}
