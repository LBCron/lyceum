// Package library reads lyceum/LIBRARY.md, the agent-kept ordered list of
// likely next units (units.md, "Roadmap").
package library

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Path is the roadmap, relative to the repo root.
const Path = "lyceum/LIBRARY.md"

// Entry is one roadmap entry: `- <type>: <title>`, optionally followed by
// ` (AC12, AC13)` naming the ACs it will claim.
type Entry struct {
	Type, Title string
	ACs         []string
}

var (
	entry = regexp.MustCompile(`^- (feat|bug|doc|test|refactor|chore): (.+?)(?: \((AC[0-9]+(?:, *AC[0-9]+)*)\))?\s*$`)
	acID  = regexp.MustCompile(`AC[0-9]+`)
)

// Read returns the entries of the roadmap under root, in order. A missing
// file has none, and a line that is no entry is not one.
func Read(root string) ([]Entry, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(Path)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for line := range strings.Lines(string(data)) {
		if m := entry.FindStringSubmatch(line); m != nil {
			entries = append(entries, Entry{Type: m[1], Title: m[2], ACs: acID.FindAllString(m[3], -1)})
		}
	}
	return entries, nil
}

// Command is the command that starts e.
func (e Entry) Command() string {
	if e.Type == "refactor" || e.Type == "chore" {
		return fmt.Sprintf("lyceum %s %q", e.Type, e.Title)
	}
	return fmt.Sprintf("lyceum add %s %q", e.Type, e.Title)
}
