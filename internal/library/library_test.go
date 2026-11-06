package library

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRead(t *testing.T) {
	root := t.TempDir()
	if got, err := Read(root); err != nil || got != nil {
		t.Fatalf("missing file: %v, %v", got, err)
	}
	if err := os.MkdirAll(filepath.Join(root, "lyceum"), 0o755); err != nil {
		t.Fatal(err)
	}
	text := "# Roadmap\n\nProse.\n- nope: not a type\n- feat: CSV export (AC9012, AC9013)\n- refactor: Split (the) store\n- bug: Crash\n"
	if err := os.WriteFile(filepath.Join(root, Path), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Type: "feat", Title: "CSV export", ACs: []string{"AC9012", "AC9013"}},
		{Type: "refactor", Title: "Split (the) store"},
		{Type: "bug", Title: "Crash"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Read = %+v; want %+v", got, want)
	}
	for i, cmd := range []string{`lyceum add feat "CSV export"`, `lyceum refactor "Split (the) store"`, `lyceum add bug "Crash"`} {
		if got := want[i].Command(); got != cmd {
			t.Errorf("Command = %q; want %q", got, cmd)
		}
	}
}
