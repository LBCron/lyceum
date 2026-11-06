// Package events reads and appends the committed event log, lyceum/events.jsonl
// (architecture.md, "Logs"). Each line is one JSON object with at least ts,
// type, unit, actor and commit, plus type-specific fields. The file is
// merged with merge=union, so lines are only ever appended.
package events

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/testhooks"
)

// Path is the event log, relative to the repo root.
const Path = "lyceum/events.jsonl"

// Event is one line of the event log.
type Event struct {
	TS     time.Time // RFC 3339, UTC, second precision
	Type   string
	Unit   string // "" when no unit is involved
	Actor  string // git user.email of whoever ran the command
	Commit string // HEAD short SHA when the event was written; "" in a repo with no commits

	// Fields holds the type-specific fields. Read decodes numbers as json.Number.
	Fields map[string]any
}

// base lists the keys every line has; Fields may not use them.
var base = map[string]bool{"ts": true, "type": true, "unit": true, "actor": true, "commit": true}

// MarshalJSON writes the base keys first, then Fields in key order.
func (e Event) MarshalJSON() ([]byte, error) {
	for k := range e.Fields {
		if base[k] {
			return nil, fmt.Errorf("event field %q clashes with a base key", k)
		}
	}
	head, err := marshal(struct {
		TS     string `json:"ts"`
		Type   string `json:"type"`
		Unit   string `json:"unit"`
		Actor  string `json:"actor"`
		Commit string `json:"commit"`
	}{e.TS.UTC().Format(time.RFC3339), e.Type, e.Unit, e.Actor, e.Commit})
	if err != nil || len(e.Fields) == 0 {
		return head, err
	}
	rest, err := marshal(e.Fields)
	if err != nil {
		return nil, err
	}
	// Splice {"ts":…,"commit":…} and {"k":…} into one object.
	return append(append(head[:len(head)-1], ','), rest[1:]...), nil
}

// UnmarshalJSON reads one event line. type and ts are required; the other base
// keys must be strings when present.
func (e *Event) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return err
	}
	if m == nil {
		return errors.New("want a JSON object")
	}
	var strs [5]string
	for i, k := range []string{"ts", "type", "unit", "actor", "commit"} {
		v, ok := m[k]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("%q is not a string", k)
		}
		strs[i] = s
		delete(m, k)
	}
	if strs[1] == "" {
		return errors.New(`missing "type"`)
	}
	ts, err := time.Parse(time.RFC3339, strs[0])
	if err != nil {
		return fmt.Errorf(`"ts" is not RFC 3339: %w`, err)
	}
	*e = Event{TS: ts.UTC(), Type: strs[1], Unit: strs[2], Actor: strs[3], Commit: strs[4]}
	if len(m) > 0 {
		e.Fields = m
	}
	return nil
}

// Append sets e's TS (testhooks.Now), Actor (git user.email) and Commit (HEAD's
// short SHA), writes it as one line at the end of the event log under repoRoot,
// and returns the event as written. The line goes out in a single write to a file
// opened with O_APPEND, so it is never interleaved with another writer's line.
func Append(repoRoot string, e Event) (Event, error) {
	if e.Type == "" {
		return Event{}, errors.New("event has no type")
	}
	actor, err := gitx.UserEmail(repoRoot)
	if err != nil {
		return Event{}, err
	}
	commit, err := gitx.HeadSHA(repoRoot)
	if err != nil {
		return Event{}, err
	}
	e.TS = testhooks.Now().Truncate(time.Second)
	e.Actor, e.Commit = actor, commit
	e.Fields = maps.Clone(e.Fields)

	line, err := marshal(e)
	if err != nil {
		return Event{}, err
	}
	f, err := os.OpenFile(filepath.Join(repoRoot, Path), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return Event{}, err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return Event{}, err
	}
	return e, f.Close()
}

// Read returns every event in the log under repoRoot, in file order. A missing log
// is empty. Any line that is not a valid event, blank lines included, is an error
// naming its line number.
func Read(repoRoot string) ([]Event, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, Path))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lines := bytes.Split(data, []byte("\n"))
	if len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1] // the final newline ends the last line
	}
	evs := make([]Event, 0, len(lines))
	for i, line := range lines {
		var e Event
		if len(bytes.TrimSpace(line)) == 0 {
			return nil, fmt.Errorf("%s line %d: blank line", Path, i+1)
		}
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", Path, i+1, err)
		}
		evs = append(evs, e)
	}
	return evs, nil
}

// TreeChanges is gitx.Status for the clean-tree checks: without the event log
// when all it differs from HEAD by is bypass lines appended at its end, which
// the commit hook writes and the next Lyceum commit carries. Any other
// change to the log, such as a hand-written approve line or an edited old
// line, stays a change.
func TreeChanges(repoRoot string) ([]string, error) {
	paths, err := gitx.Status(repoRoot)
	if err != nil || !slices.Contains(paths, Path) {
		return paths, err
	}
	head, err := gitx.Show(repoRoot, "HEAD", Path)
	if err != nil {
		return paths, nil // unborn, or the log is not in HEAD: nothing to compare with
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, Path))
	if err != nil {
		return nil, err
	}
	added, ok := bytes.CutPrefix(data, head)
	if !ok || len(added) == 0 || len(head) > 0 && head[len(head)-1] != '\n' {
		return paths, nil
	}
	for line := range bytes.Lines(added) {
		var e Event
		if err := json.Unmarshal(line, &e); err != nil || e.Type != "bypass" {
			return paths, nil
		}
	}
	return slices.DeleteFunc(paths, func(p string) bool { return p == Path }), nil
}

// marshal encodes v as compact JSON without HTML escaping.
func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
