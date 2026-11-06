// Package records holds inspect records, lyceum/{understand,trace,audit}/*.md,
// and the cites: frontmatter they share with as-built area files under
// lyceum/spec/ (records.md, "Record format", "As-built spec sections"): parsing, validating and filling the format, and freshness.
package records

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/oritromax/lyceum/internal/unit"
	"go.yaml.in/yaml/v3"
)

// Record kinds. Each kind's records live in Dir(kind).
const (
	Understand = "understand"
	Trace      = "trace"
	Audit      = "audit"
)

// Kinds lists the record kinds, in the order status lists them.
var Kinds = []string{Understand, Trace, Audit}

// Dir returns the folder holding kind's records, relative to the repo root.
func Dir(kind string) string { return "lyceum/" + kind }

// SpecDir holds the area files, which may carry cites:.
const SpecDir = "lyceum/spec"

// Cite is one entry of cites:. Blob is the blob ID of a cited file, Tree the
// tree ID of a cited folder (Hash); Symbol is for display only.
type Cite struct {
	Path   string `yaml:"path"`
	Symbol string `yaml:"symbol"`
	Blob   string `yaml:"blob"`
	Tree   string `yaml:"tree"`
}

// Record is a record file or an area file's frontmatter. The fields are a
// read-only view of the frontmatter; Fill changes them.
type Record struct {
	ID, Kind, Title, Created string
	Cites                    []Cite
	// HasCites reports whether the frontmatter has a cites: key.
	HasCites bool

	openLine, closeLine, body []byte
	root                      *yaml.Node
}

// FieldError is a record field that is missing or invalid.
type FieldError struct {
	Field, Reason string
}

func (e *FieldError) Error() string { return "field " + e.Field + ": " + e.Reason }

// HasFrontmatter reports whether data starts with a "---" line.
func HasFrontmatter(data []byte) bool {
	line, _, _ := bytes.Cut(data, []byte("\n"))
	return string(bytes.TrimRight(line, "\r")) == "---"
}

// Parse parses a record's frontmatter. It checks each field's shape, not
// whether the record is complete: see Validate and Complete.
func Parse(data []byte) (*Record, error) {
	openLine, front, closeLine, body, err := unit.SplitFrontmatter(data)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(front, &doc); err != nil {
		return nil, fmt.Errorf("malformed frontmatter: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("malformed frontmatter: want a mapping of keys to values")
	}
	r := &Record{openLine: openLine, closeLine: closeLine, body: body, root: doc.Content[0]}
	m := r.root.Content
	for i := 0; i+1 < len(m); i += 2 {
		key, v := m[i].Value, m[i+1]
		switch key {
		case "id", "kind", "title", "created":
			if v.Kind != yaml.ScalarNode {
				return nil, &FieldError{key, "want a single value"}
			}
			switch key {
			case "id":
				r.ID = v.Value
			case "kind":
				r.Kind = v.Value
			case "title":
				r.Title = v.Value
			default:
				r.Created = v.Value
			}
		case "cites":
			r.HasCites = true
			if v.Kind == yaml.ScalarNode && v.Tag == "!!null" {
				continue
			}
			if v.Kind != yaml.SequenceNode {
				return nil, &FieldError{"cites", "want a list of {path, symbol, blob or tree}"}
			}
			for j, item := range v.Content {
				field := fmt.Sprintf("cites[%d]", j)
				if item.Kind != yaml.MappingNode {
					return nil, &FieldError{field, "want a mapping with path:"}
				}
				var c Cite
				if err := item.Decode(&c); err != nil {
					return nil, &FieldError{field, err.Error()}
				}
				r.Cites = append(r.Cites, c)
			}
		}
	}
	return r, nil
}

// Validate checks what an agent-written record must have before `lyceum
// record` fills it in: a known kind, a title, and cites whose paths are clean
// paths relative to the repo root, none with both blob and tree, and, for a
// trace, every step marked (validateSteps). An id or created already there
// must be valid. It does not look at the work tree.
func (r *Record) Validate() error {
	if r.ID != "" {
		if _, _, err := unit.ParseID(r.ID); err != nil {
			return &FieldError{"id", strings.Replace(err.Error(), "unit ID", "record ID", 1)}
		}
	}
	if !slices.Contains(Kinds, r.Kind) {
		if r.Kind == "" {
			return &FieldError{"kind", "missing; want understand, trace or audit"}
		}
		return &FieldError{"kind", fmt.Sprintf("%q is not understand, trace or audit", r.Kind)}
	}
	if strings.TrimSpace(r.Title) == "" {
		return &FieldError{"title", "missing"}
	}
	if r.Created != "" {
		if _, err := time.Parse(time.RFC3339, r.Created); err != nil {
			return &FieldError{"created", fmt.Sprintf("%q is not an RFC 3339 time, e.g. 2026-10-23T10:00:00Z", r.Created)}
		}
	}
	if r.Kind == Trace {
		if err := validateSteps(r.body); err != nil {
			return err
		}
	}
	return ValidateCites(r.Cites)
}

// StepsHeading starts a trace's steps section (records.md, "lyceum trace").
const StepsHeading = "## Steps"

// Step marks: a trace step was read from the code or observed in a full run.
var stepMarks = []string{"[read]", "[observed]"}

// validateSteps checks a trace body: every line starting with "- " in its
// StepsHeading section, up to the next "#" or "##" heading, is a step and
// carries [read] or [observed] (AC79).
func validateSteps(body []byte) error {
	in, n := false, 0
	for line := range strings.Lines(string(body)) {
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") {
			in = strings.TrimSpace(line) == StepsHeading
			continue
		}
		if !in || !strings.HasPrefix(line, "- ") {
			continue
		}
		n++
		if !slices.ContainsFunc(stepMarks, func(m string) bool { return strings.Contains(line, m) }) {
			return fmt.Errorf("%s: step %d is marked neither [read] nor [observed]: %q", StepsHeading, n, line)
		}
	}
	return nil
}

// ValidateCites checks each cite's path and that none has both blob and tree.
func ValidateCites(cites []Cite) error {
	for i, c := range cites {
		field := fmt.Sprintf("cites[%d]", i)
		if err := validPath(c.Path); err != nil {
			return &FieldError{field + ".path", err.Error()}
		}
		if c.Blob != "" && c.Tree != "" {
			return &FieldError{field, "has both blob and tree; a cite is a file (blob) or a folder (tree)"}
		}
	}
	return nil
}

// Complete checks that a valid record has what `lyceum record` fills in: an
// id, created, and a blob or tree on every cite.
func (r *Record) Complete() error {
	if r.ID == "" {
		return &FieldError{"id", "missing"}
	}
	if r.Created == "" {
		return &FieldError{"created", "missing"}
	}
	return CitesComplete(r.Cites)
}

// CitesComplete checks that every cite has a blob or a tree.
func CitesComplete(cites []Cite) error {
	for i, c := range cites {
		if c.Blob == "" && c.Tree == "" {
			return &FieldError{fmt.Sprintf("cites[%d]", i), "has no blob or tree"}
		}
	}
	return nil
}

// validPath checks a cited path: relative to the repo root, slash-separated,
// clean, inside the repo and not the repo itself. A trailing "/" may mark a folder.
func validPath(p string) error {
	switch {
	case p == "":
		return errors.New("missing")
	case strings.Contains(p, `\`):
		return fmt.Errorf("%q: use / to separate path elements", p)
	case path.IsAbs(p):
		return fmt.Errorf("%q is absolute; want a path relative to the repo root", p)
	}
	trimmed := strings.TrimSuffix(p, "/")
	switch clean := path.Clean(trimmed); {
	case clean == "." || clean == ".." || strings.HasPrefix(clean, "../"):
		return fmt.Errorf("%q is not inside the repo", p)
	case clean != trimmed:
		return fmt.Errorf("%q is not a clean path; want %q", p, clean)
	}
	return nil
}

// Fill sets the record's id and created, adding each when absent, and every
// cite's blob or tree from hashes, keyed by cite path as Hash returns them,
// dropping the other one. A cite path missing from hashes, or one Hash found
// missing, is an error.
func (r *Record) Fill(id, created string, hashes map[string]Current) error {
	setScalar(r.root, "id", id, 0)
	at := -1
	if i := keyIndex(r.root, "cites"); i >= 0 {
		at = i
	}
	setScalar(r.root, "created", created, at)
	r.ID, r.Created = id, created
	return r.FillCites(hashes)
}

// FillCites sets every cite's blob or tree from hashes, as Fill does, and
// nothing else: what `lyceum record` fills in on an area file.
func (r *Record) FillCites(hashes map[string]Current) error {
	if !r.HasCites {
		return nil
	}
	seq := r.root.Content[keyIndex(r.root, "cites")+1]
	for i, item := range seq.Content {
		h, ok := hashes[key(r.Cites[i].Path)]
		if !ok || h.Missing() {
			return &FieldError{fmt.Sprintf("cites[%d].path", i), fmt.Sprintf("%s does not exist", r.Cites[i].Path)}
		}
		set, drop := "blob", "tree"
		value := h.Blob
		if h.Tree != "" {
			set, drop, value = "tree", "blob", h.Tree
		}
		deleteKey(item, drop)
		setScalar(item, set, value, -1)
		r.Cites[i].Blob, r.Cites[i].Tree = h.Blob, h.Tree
	}
	return nil
}

// Bytes returns the record file: its frontmatter, re-encoded with any Fill
// changes, then its body as read.
func (r *Record) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(r.root); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return slices.Concat(r.openLine, buf.Bytes(), r.closeLine, r.body), nil
}

// keyIndex returns the index of key's key node in mapping m, or -1.
func keyIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// setScalar sets key in mapping m to value, or inserts it
// before the key at index at, or at the end when at is -1.
func setScalar(m *yaml.Node, key, value string, at int) {
	v := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	switch key {
	case "blob", "tree":
		v.Style = yaml.DoubleQuotedStyle
	case "created":
		v.Tag = "!!timestamp" // plain, as in the record format
	}
	if i := keyIndex(m, key); i >= 0 {
		v.LineComment = m.Content[i+1].LineComment
		m.Content[i+1] = v
		return
	}
	kv := []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v}
	if at < 0 {
		m.Content = append(m.Content, kv...)
		return
	}
	m.Content = slices.Insert(m.Content, at, kv...)
}

// deleteKey removes key and its value from mapping m.
func deleteKey(m *yaml.Node, key string) {
	if i := keyIndex(m, key); i >= 0 {
		m.Content = slices.Delete(m.Content, i, i+2)
	}
}

// key is a cite path as Hash keys it: without a trailing "/".
func key(p string) string { return strings.TrimSuffix(p, "/") }
