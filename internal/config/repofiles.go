package config

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Lines EnsureRepoFiles keeps in the repo's git files.
const (
	unionMergeLine = "lyceum/events.jsonl merge=union" // in .gitattributes
	runtimeIgnore  = ".lyceum/"                        // in .gitignore
)

// HarnessIgnores are the paths harnesses write on their own in a project and
// that are not config to commit. An untracked one would fail a clean-tree
// check for no reason of the agent's: Command Code's taste notes, Claude Code's
// personal settings and its worktrees. Only observed or documented paths go here.
var HarnessIgnores = []string{".commandcode/taste/", ".claude/settings.local.json", ".claude/worktrees/"}

// EnsureRepoFiles makes sure .gitattributes under root merges the event log with
// merge=union and .gitignore ignores the runtime dir .lyceum/ and HarnessIgnores.
// Each file is created if missing; a line already present is not added again, and
// existing lines are kept byte for byte.
func EnsureRepoFiles(root string) error {
	if err := ensureLine(filepath.Join(root, ".gitattributes"), unionMergeLine); err != nil {
		return err
	}
	for _, line := range append([]string{runtimeIgnore}, HarnessIgnores...) {
		if err := ensureLine(filepath.Join(root, ".gitignore"), line); err != nil {
			return err
		}
	}
	return nil
}

// MissingHarnessIgnores returns the HarnessIgnores .gitignore under root lacks.
func MissingHarnessIgnores(root string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	have := map[string]bool{}
	for l := range bytes.Lines(data) {
		have[string(bytes.TrimSpace(l))] = true
	}
	var missing []string
	for _, line := range HarnessIgnores {
		if !have[line] {
			missing = append(missing, line)
		}
	}
	return missing, nil
}

// ensureLine appends line to the file at path unless a line equal to it, ignoring
// surrounding whitespace, is already there.
func ensureLine(path, line string) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for l := range bytes.Lines(data) {
		if string(bytes.TrimSpace(l)) == line {
			return nil
		}
	}
	add := line + "\n"
	if len(data) > 0 && data[len(data)-1] != '\n' {
		add = "\n" + add
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(add); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
