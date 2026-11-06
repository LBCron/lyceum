// Package testspec reads the frontmatter of TESTING.md, the only part of it the
// CLI executes (test-runner.md, "TESTING.md"), and looks up whether
// the file as it is now is the one the owner approved.
package testspec

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/oritromax/lyceum/internal/acmap"
	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/events"
	"go.yaml.in/yaml/v3"
)

// Path is TESTING.md, relative to the repo root.
const Path = approval.TestingPath

// SchemaVersion is the value of the `lyceum` key this build reads.
const SchemaVersion = 1

// DefaultTimeout bounds a run when TESTING.md sets no timeout.
const DefaultTimeout = 15 * time.Minute

// NamePlaceholder is what `single` replaces with the test's name.
const NamePlaceholder = "{name}"

// Schema is the TESTING.md frontmatter format, every key with one example
// line, printed when the file is missing.
const Schema = `TESTING.md starts with this frontmatter (lyceum and suite are required):
---
lyceum: 1
suite: go test -count=1 ./...
single: go test -count=1 ./... -run {name}
test_files: ["**/*_test.go"]
subsets:
  unit: go test -count=1 ./... -short
environments:
  test:
    up: docker compose up -d
timeout: 15m
---
`

// Spec is the TESTING.md frontmatter.
type Spec struct {
	Lyceum    int
	Suite     string            // the full suite: the only command that can clear a gate
	Single    string            // one named test, with NamePlaceholder; "" when unset
	TestFiles []string          // globs for AC mapping and for finding tests
	Subsets   map[string]string // subset name to command
	// Environments is kept as read for trace; nothing uses it yet.
	Environments map[string]map[string]string
	Timeout      time.Duration
	// Unknown are the keys the frontmatter sets that this build does not read,
	// in order. They are warned about, never refused.
	Unknown []string
}

// Read reads and validates the TESTING.md frontmatter under root. A missing
// file is an error that wraps fs.ErrNotExist.
func Read(root string) (*Spec, error) {
	data, err := os.ReadFile(filepath.Join(root, Path))
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse parses and validates TESTING.md data. Every error names the key at fault.
func Parse(data []byte) (*Spec, error) {
	front, err := frontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", Path, err)
	}
	s, err := parse(front)
	if err != nil {
		return nil, fmt.Errorf("%s frontmatter: %w", Path, err)
	}
	return s, nil
}

func parse(front []byte) (*Spec, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(front, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("want a mapping of keys to values")
	}
	m := doc.Content[0]
	s := &Spec{Timeout: DefaultTimeout}
	seen, failed := map[string]bool{}, map[string]bool{}
	var errs []error
	for i := 0; i < len(m.Content); i += 2 {
		key, v := m.Content[i].Value, m.Content[i+1]
		seen[key] = true
		var err error
		switch key {
		case "lyceum":
			err = v.Decode(&s.Lyceum)
		case "suite":
			err = v.Decode(&s.Suite)
		case "single":
			err = v.Decode(&s.Single)
		case "test_files":
			err = v.Decode(&s.TestFiles)
		case "subsets":
			err = v.Decode(&s.Subsets)
		case "environments":
			err = v.Decode(&s.Environments)
		case "timeout":
			var t string
			if err = v.Decode(&t); err == nil {
				s.Timeout, err = time.ParseDuration(t)
			}
		default:
			s.Unknown = append(s.Unknown, key)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", key, err))
			failed[key] = true
		}
	}
	return s, errors.Join(append(errs, s.validate(seen, failed)...)...)
}

// validate returns every problem with s; seen holds the keys the frontmatter
// set, and failed those whose value could not be read, already reported.
func (s *Spec) validate(seen, failed map[string]bool) []error {
	var errs []error
	switch {
	case failed["lyceum"]:
	case !seen["lyceum"]:
		errs = append(errs, fmt.Errorf("lyceum: missing; this build reads lyceum: %d", SchemaVersion))
	case s.Lyceum != SchemaVersion:
		errs = append(errs, fmt.Errorf("lyceum: schema version %d is not supported; this build reads lyceum: %d", s.Lyceum, SchemaVersion))
	}
	if !failed["suite"] && strings.TrimSpace(s.Suite) == "" {
		errs = append(errs, errors.New("suite: missing; it is the command that runs the full suite"))
	}
	if !failed["single"] && seen["single"] && !strings.Contains(s.Single, NamePlaceholder) {
		errs = append(errs, fmt.Errorf("single: %q does not contain %s, where the test's name goes", s.Single, NamePlaceholder))
	}
	if !failed["timeout"] && s.Timeout <= 0 {
		errs = append(errs, fmt.Errorf("timeout: %s is not a positive duration", s.Timeout))
	}
	for _, g := range s.TestFiles {
		if err := checkGlob(g); err != nil {
			errs = append(errs, fmt.Errorf("test_files: %w", err))
		}
	}
	for _, name := range s.SubsetNames() {
		if strings.TrimSpace(name) == "" {
			errs = append(errs, errors.New("subsets: a subset has no name"))
		} else if strings.TrimSpace(s.Subsets[name]) == "" {
			errs = append(errs, fmt.Errorf("subsets: %s has no command", name))
		}
	}
	return errs
}

// checkGlob checks one test_files glob: slash-separated segments, each `**` or a path.Match pattern.
func checkGlob(g string) error {
	if strings.TrimSpace(g) == "" {
		return errors.New("a glob is empty")
	}
	for _, seg := range strings.Split(g, "/") {
		if _, err := path.Match(seg, ""); err != nil {
			return fmt.Errorf("%q is not a valid glob: %w", g, err)
		}
	}
	return nil
}

// SubsetNames returns the defined subset names, sorted.
func (s *Spec) SubsetNames() []string {
	names := make([]string, 0, len(s.Subsets))
	for name := range s.Subsets {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// HasTests reports whether any file under root, outside .git, matches the
// test_files globs. None matching is "no tests".
func (s *Spec) HasTests(root string) (bool, error) {
	found := false
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if acmap.MatchAny(s.TestFiles, filepath.ToSlash(rel)) {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found, err
}

// frontmatter returns the YAML between a leading "---" line and the next "---" line.
func frontmatter(data []byte) ([]byte, error) {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, errors.New(`no frontmatter: the file must start with a "---" line`)
	}
	rest := data[len("---\n"):]
	for i := 0; i < len(rest); {
		line, _, _ := bytes.Cut(rest[i:], []byte("\n"))
		if string(line) == "---" {
			return rest[:i], nil
		}
		i += len(line) + 1
	}
	return nil, errors.New(`malformed frontmatter: no closing "---" line`)
}

// ApprovedHash returns the TESTING.md hash recorded by the last counting
// `approve testing` event in evs, or "" when there is none. The unit an
// approval was given under does not matter: TESTING.md is repo-wide.
func ApprovedHash(evs []events.Event) string {
	last := ""
	for _, e := range evs {
		if e.Type == "approve" && e.Fields["gate"] == string(approval.Testing) && e.Fields["counts"] != false {
			hashes, _ := e.Fields["hashes"].(map[string]any)
			last, _ = hashes[Path].(string)
		}
	}
	return last
}

// Approved reports whether TESTING.md under root has a counting approval, and
// whether it changed since the last one. A missing TESTING.md is an error that
// wraps fs.ErrNotExist.
func Approved(root string) (approved, changed bool, err error) {
	h, err := approval.Hash(filepath.Join(root, Path))
	if err != nil {
		return false, false, err
	}
	evs, err := events.Read(root)
	if err != nil {
		return false, false, err
	}
	last := ApprovedHash(evs)
	return last != "", last != "" && last != h, nil
}
