package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/gittest"
)

// TestMeasure_AC75: the scope counts the files git tracks or would track and
// their lines; ignored, deleted and symlinked files don't count, and a binary
// file counts as a file with no lines.
func TestMeasure_AC75(t *testing.T) {
	dir := gittest.InitWithCommit(t)
	write := func(name, content string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", "*.log\n")                  // 1 line
	write("src/a.go", "package a\n\nfunc A() {}\n") // 3 lines
	write("src/b.go", "package a\nvar B = 1")       // 2 lines, no final newline
	write("img.bin", "\x00\x01\n\n\n")              // binary: no lines
	write("gone.go", "package gone\n")
	write("build.log", "ignored\nignored\n")
	gittest.Git(t, dir, "add", ".gitignore", "src/a.go", "gone.go", "img.bin")
	gittest.Git(t, dir, "commit", "-q", "-m", "code")
	if err := os.Remove(filepath.Join(dir, "gone.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("src/a.go", filepath.Join(dir, "link.go")); err != nil {
		t.Fatal(err)
	}

	sc, err := measure(dir, config.Threshold{Files: 4, Lines: 6})
	if err != nil {
		t.Fatal(err)
	}
	if sc.Files != 4 || sc.Lines != 6 || sc.Over {
		t.Errorf("measure = %d files, %d lines, over %v; want 4 files, 6 lines, not over", sc.Files, sc.Lines, sc.Over)
	}
	for _, th := range []config.Threshold{{Files: 3, Lines: 6}, {Files: 4, Lines: 5}} {
		if sc, err := measure(dir, th); err != nil || !sc.Over {
			t.Errorf("measure with threshold %+v: over = %v, %v; want over", th, sc != nil && sc.Over, err)
		}
	}
}
