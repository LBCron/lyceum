package events

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oritromax/lyceum/internal/gittest"
)

// newRepo returns a temp git repo with a lyceum/ dir, with or without a commit.
func newRepo(t *testing.T, commit bool) string {
	t.Helper()
	repo := gittest.Init(t)
	if commit {
		gittest.Git(t, repo, "commit", "-q", "--allow-empty", "-m", "initial")
	}
	if err := os.Mkdir(filepath.Join(repo, "lyceum"), 0o755); err != nil {
		t.Fatal(err)
	}
	return repo
}

func writeLog(t *testing.T, repo, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, Path), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAppendFillsBaseFields(t *testing.T) {
	repo := newRepo(t, true)
	sha := gittest.Git(t, repo, "rev-parse", "--short", "HEAD")
	before := time.Now().Add(-time.Second)

	got, err := Append(repo, Event{
		Type: "add", Unit: "a3f9c2-csv-export",
		Actor: "forged@example.com", Commit: "deadbee", TS: time.Unix(0, 0), // overwritten
		Fields: map[string]any{"title": "CSV <export> & more"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Actor != gittest.Email || got.Commit != sha {
		t.Errorf("actor, commit = %q, %q; want %q, %q", got.Actor, got.Commit, gittest.Email, sha)
	}
	if got.TS.Before(before.Truncate(time.Second)) || got.TS.After(time.Now()) || got.TS.Nanosecond() != 0 {
		t.Errorf("ts = %s; want now, in whole seconds", got.TS)
	}

	data, err := os.ReadFile(filepath.Join(repo, Path))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"ts":"` + got.TS.Format(time.RFC3339) + `","type":"add","unit":"a3f9c2-csv-export","actor":"owner@example.com","commit":"` +
		sha + `","title":"CSV <export> & more"}` + "\n"
	if string(data) != want {
		t.Errorf("log =\n%s\nwant\n%s", data, want)
	}
	if !strings.HasSuffix(got.TS.Format(time.RFC3339), "Z") {
		t.Errorf("ts %s is not UTC", got.TS)
	}
}

func TestAppendNoCommits(t *testing.T) {
	repo := newRepo(t, false)
	got, err := Append(repo, Event{Type: "add"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Commit != "" {
		t.Errorf("commit = %q; want empty in a repo with no commits", got.Commit)
	}
}

func TestAppendRefuses(t *testing.T) {
	repo := newRepo(t, true)
	if _, err := Append(repo, Event{}); err == nil {
		t.Error("Append with no type succeeded")
	}
	if _, err := Append(repo, Event{Type: "add", Fields: map[string]any{"actor": "x"}}); err == nil {
		t.Error("Append with a field clashing with a base key succeeded")
	}
	if _, err := os.Stat(filepath.Join(repo, Path)); !os.IsNotExist(err) {
		t.Errorf("a refused Append wrote the log: %v", err)
	}
}

func TestAppendThenRead(t *testing.T) {
	repo := newRepo(t, true)
	hashes := map[string]any{"lyceum/plan/a3f9c2-csv-export.md": "sha256:ab"}
	var written []Event
	for _, e := range []Event{
		{Type: "add", Unit: "a3f9c2-csv-export"},
		{Type: "approve", Unit: "a3f9c2-csv-export", Fields: map[string]any{"gate": "spec", "hashes": hashes, "n": 12345678901234567}},
		{Type: "bypass"},
	} {
		w, err := Append(repo, e)
		if err != nil {
			t.Fatal(err)
		}
		written = append(written, w)
	}

	got, err := Read(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(written) {
		t.Fatalf("Read returned %d events; want %d", len(got), len(written))
	}
	for i := range got {
		g, w := got[i], written[i]
		if !g.TS.Equal(w.TS) || g.Type != w.Type || g.Unit != w.Unit || g.Actor != w.Actor || g.Commit != w.Commit {
			t.Errorf("event %d = %+v; want %+v", i, g, w)
		}
	}
	f := got[1].Fields
	if f["gate"] != "spec" || f["n"] != json.Number("12345678901234567") {
		t.Errorf("fields = %v", f)
	}
	if h, ok := f["hashes"].(map[string]any); !ok || h["lyceum/plan/a3f9c2-csv-export.md"] != "sha256:ab" {
		t.Errorf("hashes = %v", f["hashes"])
	}
	if got[0].Fields != nil {
		t.Errorf("event without fields read back fields %v", got[0].Fields)
	}
}

// TestAppendConcurrent checks that concurrent appends never interleave: every line
// they wrote reads back whole.
func TestAppendConcurrent(t *testing.T) {
	repo := newRepo(t, true)
	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := Append(repo, Event{Type: "add", Fields: map[string]any{"i": i, "pad": strings.Repeat("x", 4096)}})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := Read(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Errorf("Read returned %d events; want %d", len(got), n)
	}
}

func TestReadMissing(t *testing.T) {
	repo := newRepo(t, false)
	got, err := Read(repo)
	if err != nil || got != nil {
		t.Errorf("Read of a missing log = %v, %v; want nil, nil", got, err)
	}
}

func TestReadMalformed(t *testing.T) {
	good := `{"ts":"2026-01-01T00:00:00Z","type":"add","unit":"","actor":"a@b","commit":""}`
	for _, tc := range []struct{ name, line string }{
		{"not json", `{"ts":`},
		{"not an object", `["add"]`},
		{"null", `null`},
		{"blank", ``},
		{"two values", good + ` {}`},
		{"no type", `{"ts":"2026-01-01T00:00:00Z"}`},
		{"no ts", `{"type":"add"}`},
		{"bad ts", `{"ts":"yesterday","type":"add"}`},
		{"non-string base key", `{"ts":"2026-01-01T00:00:00Z","type":"add","unit":7}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			if err := os.Mkdir(filepath.Join(repo, "lyceum"), 0o755); err != nil {
				t.Fatal(err)
			}
			writeLog(t, repo, good+"\n"+tc.line+"\n"+good+"\n")
			_, err := Read(repo)
			if err == nil || !strings.Contains(err.Error(), "events.jsonl line 2:") {
				t.Errorf("Read = %v; want an error naming line 2", err)
			}
		})
	}
}

func TestReadNoFinalNewline(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "lyceum"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeLog(t, repo, `{"ts":"2026-01-01T00:00:00Z","type":"add"}`)
	got, err := Read(repo)
	if err != nil || len(got) != 1 || got[0].Type != "add" {
		t.Errorf("Read = %v, %v; want one add event", got, err)
	}
}

// AC172: only bypass lines at the end of the log are left out of the tree's changes.
func TestTreeChanges_AC172(t *testing.T) {
	const bypass = `{"ts":"2026-01-01T00:00:00Z","type":"bypass","unit":"","actor":"a","commit":"c","check":"x","message":"m"}` + "\n"
	const approve = `{"ts":"2026-01-01T00:00:00Z","type":"approve","unit":"u","actor":"a","commit":"c","gate":"spec"}` + "\n"
	const first = `{"ts":"2026-01-01T00:00:00Z","type":"add","unit":"u","actor":"a","commit":"c"}` + "\n"
	cases := []struct {
		name, log string
		want      []string
	}{
		{"bypass appended", first + bypass, nil},
		{"two bypass appended", first + bypass + bypass, nil},
		{"approve appended", first + approve, []string{Path}},
		{"approve after bypass", first + bypass + approve, []string{Path}},
		{"bypass before the old line", bypass + first, []string{Path}},
		{"not an event", first + "nope\n", []string{Path}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := newRepo(t, true)
			full := filepath.Join(dir, filepath.FromSlash(Path))
			if err := os.WriteFile(full, []byte(first), 0o644); err != nil {
				t.Fatal(err)
			}
			gittest.Git(t, dir, "add", Path)
			gittest.Git(t, dir, "commit", "-q", "-m", "log")
			if err := os.WriteFile(full, []byte(c.log), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := TreeChanges(dir)
			if err != nil || !slices.Equal(got, c.want) {
				t.Errorf("TreeChanges = %v, %v; want %v", got, err, c.want)
			}
		})
	}
}
