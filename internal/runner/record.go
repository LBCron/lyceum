package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/testhooks"
)

// Record is a run record, .lyceum/runs/<ts>-<rand>.json (test-runner.md, "Run
// records"), plus timed_out. Hashes are "sha256:<hex>", as approvals record them.
type Record struct {
	TS      string  `json:"ts"`
	Command string  `json:"command"`
	Subset  *string `json:"subset"` // nil for the full suite
	// Repro is the test a `lyceum test --repro` run ran with TESTING.md single;
	// left out of other runs' records.
	Repro *string `json:"repro,omitempty"`
	// Benchmark is set on the run of a refactor's plan benchmark: command;
	// left out of other runs' records.
	Benchmark     bool   `json:"benchmark,omitempty"`
	Commit        string `json:"commit"` // HEAD's short SHA; "" with no commits
	Dirty         bool   `json:"dirty"`  // the working tree had uncommitted changes
	Exit          int    `json:"exit"`
	TimedOut      bool   `json:"timed_out"`
	DurationMS    int64  `json:"duration_ms"`
	Output        string `json:"output"` // the output file's name in Dir
	OutputSHA256  string `json:"output_sha256"`
	TestingSHA256 string `json:"testing_sha256"` // TESTING.md as it was when the run started
}

// Command is what to run: a TESTING.md command, the subset it came from ("" for
// the full suite), the repro test it runs ("" unless it is TESTING.md single
// for `lyceum test --repro`), whether it is a refactor's benchmark and the
// timeout.
type Command struct {
	Command, Subset, Repro string
	Benchmark              bool
	Timeout                time.Duration
}

// Run runs c in root (Exec), keeps its output in a new file in Dir and writes
// the run record beside it, and returns the record. The commit, dirty flag and
// TESTING.md hash are taken before the command starts: the tree it ran on.
func Run(root string, c Command, stdout, stderr io.Writer) (Record, error) {
	commit, err := gitx.HeadSHA(root)
	if err != nil {
		return Record{}, err
	}
	changed, err := events.TreeChanges(root)
	if err != nil {
		return Record{}, err
	}
	testing, err := approval.Hash(filepath.Join(root, approval.TestingPath))
	if err != nil {
		return Record{}, err
	}
	ts := testhooks.Now().Truncate(time.Second)
	f, name, err := Create(root)
	if err != nil {
		return Record{}, err
	}
	o, err := Exec(root, c.Command, c.Timeout, stdout, stderr, f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Record{}, err
	}
	dir := filepath.Join(root, filepath.FromSlash(Dir))
	outHash, err := approval.Hash(filepath.Join(dir, name+".out"))
	if err != nil {
		return Record{}, err
	}
	r := Record{
		TS:            ts.Format(time.RFC3339),
		Command:       c.Command,
		Commit:        commit,
		Dirty:         len(changed) > 0,
		Exit:          o.Exit,
		TimedOut:      o.TimedOut,
		DurationMS:    o.Duration.Milliseconds(),
		Output:        name + ".out",
		OutputSHA256:  outHash,
		TestingSHA256: testing,
	}
	if c.Subset != "" {
		r.Subset = &c.Subset
	}
	if c.Repro != "" {
		r.Repro = &c.Repro
	}
	r.Benchmark = c.Benchmark
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return Record{}, err
	}
	return r, os.WriteFile(filepath.Join(dir, name+".json"), b.Bytes(), 0o644)
}

// Records returns the run records in Dir under root, in name order: the order
// they were made in, but for runs made in the same second. A missing Dir has none.
func Records(root string) ([]Record, error) {
	dir := filepath.Join(root, filepath.FromSlash(Dir))
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	recs := make([]Record, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var r Record
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("%s/%s: %w", Dir, name, err)
		}
		recs = append(recs, r)
	}
	return recs, nil
}
