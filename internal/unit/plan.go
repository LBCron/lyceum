package unit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Plan status values (units.md, "The plan file").
const (
	StatusPlanned     = "planned"
	StatusInProgress  = "in-progress"
	StatusImplemented = "implemented"
	StatusClosed      = "closed"
)

// cliOwned lists the frontmatter keys only the CLI writes.
var cliOwned = map[string]bool{
	"id": true, "type": true, "title": true, "branch": true,
	"status": true, "gate": true, "approvals": true,
}

// Approval is one entry of the CLI-owned approvals list, mirroring an approval event.
type Approval struct {
	Gate   string            `yaml:"gate"`
	Hashes map[string]string `yaml:"hashes"`
	Commit string            `yaml:"commit"`
	By     string            `yaml:"by"`
	At     time.Time         `yaml:"at"`
	// Counts is false, and written, only for an approval by someone other than
	// the configured owner, which does not clear the gate. nil means it counts.
	Counts *bool `yaml:"counts,omitempty"`
}

// Plan is a plan file, lyceum/plan/<hash>-<slug>.md (units.md, "The plan file").
//
// The frontmatter fields are a read-only view. CLI-owned fields change only through SetCLIField;
// ParsePlan parses and validates plan file data read from somewhere other than
// the checkout, such as another branch.
func ParsePlan(data []byte) (*Plan, error) {
	return parsePlan(data)
}

// WritePlan writes the frontmatter exactly as read plus those changes, so agent-written keys,
// including ones not listed here, keep their bytes, order and comments.
type Plan struct {
	// CLI-owned.
	ID        string     `yaml:"id"`
	Type      string     `yaml:"type"`
	Title     string     `yaml:"title"`
	Branch    string     `yaml:"branch"`
	Status    string     `yaml:"status"`
	Gate      string     `yaml:"gate"`
	Approvals []Approval `yaml:"approvals"`

	// Agent-written.
	Areas []string `yaml:"areas"`
	Files []string `yaml:"files"`
	ACs   []string `yaml:"acs"`
	// ACsPartial lists ACs this unit proves only in part: verify does not
	// require them, and they stay open until a closed plan lists them in acs.
	ACsPartial []string `yaml:"acs_partial"`
	// Benchmark is a refactor's benchmark command, recorded at intent; verify
	// runs it and requires exit 0.
	Benchmark string `yaml:"benchmark"`
	// Goal is a refactor's stated goal, free text; "performance" makes approve
	// intent require Benchmark.
	Goal string `yaml:"goal"`
	// PublicSurface lists a refactor's public-surface changes; approve plan
	// hashes them with the plan, and verify refuses when they changed since.
	PublicSurface []string `yaml:"public_surface"`

	// Body is everything after the closing "---" line, byte for byte.
	Body []byte `yaml:"-"`

	openLine, front, closeLine []byte
	root                       *yaml.Node
}

// newBody is a new plan's body: the section headings the agent fills in (units.md, "The plan file").
const newBody = "\n## What and why\n\n## Spec delta\n\n## Steps\n\n## Progress\n\n## Log\n"

// NewPlan returns a new unit's plan: the CLI-owned fields with no approvals, and
// a body of empty section headings.
func NewPlan(id, unitType, title, branch, status, gate string) (*Plan, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	err := enc.Encode(struct {
		ID        string     `yaml:"id"`
		Type      string     `yaml:"type"`
		Title     string     `yaml:"title"`
		Branch    string     `yaml:"branch"`
		Status    string     `yaml:"status"`
		Gate      string     `yaml:"gate"`
		Approvals []Approval `yaml:"approvals"`
	}{id, unitType, title, branch, status, gate, []Approval{}})
	if err == nil {
		err = enc.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("new plan %s: %w", id, err)
	}
	p, err := parseFront(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("new plan %s: %w", id, err)
	}
	p.openLine, p.closeLine, p.Body = []byte("---\n"), []byte("---\n"), []byte(newBody)
	return p, nil
}

// Fields returns every frontmatter key and its value, agent-written keys the
// struct does not list included.
func (p *Plan) Fields() (map[string]any, error) {
	fields := map[string]any{}
	if err := p.root.Decode(&fields); err != nil {
		return nil, fmt.Errorf("plan %s: %w", p.ID, err)
	}
	return fields, nil
}

// ReadPlan reads and validates the plan file at path.
func ReadPlan(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p, err := parsePlan(data)
	if err != nil {
		return nil, fmt.Errorf("plan %s: %w", path, err)
	}
	return p, nil
}

// WritePlan writes p to path: the frontmatter as read plus any SetCLIField changes, then Body.
func WritePlan(path string, p *Plan) error {
	return os.WriteFile(path, slices.Concat(p.openLine, p.front, p.closeLine, p.Body), 0o644)
}

// SetCLIField sets one CLI-owned frontmatter key, appending it if absent. Only that key's lines
// are rewritten; a scalar keeps its quoting style and trailing comment. Agent-written keys are refused.
func (p *Plan) SetCLIField(key string, value any) error {
	if !cliOwned[key] {
		return fmt.Errorf("%q is not a CLI-owned plan field; the CLI writes only id, type, title, branch, status, gate and approvals", key)
	}
	if p.root.Style&yaml.FlowStyle != 0 {
		return fmt.Errorf("cannot set %s: frontmatter is a flow mapping, want one key per line", key)
	}
	var val yaml.Node
	if err := val.Encode(value); err != nil {
		return fmt.Errorf("cannot set %s: %w", key, err)
	}

	lines := bytes.SplitAfter(p.front, []byte("\n"))
	start, end := len(lines), len(lines)
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	if i, s, e := keyLines(p.root, lines, key); i >= 0 {
		k, v := p.root.Content[i], p.root.Content[i+1]
		start, end = s, e
		keyNode = &yaml.Node{Kind: yaml.ScalarNode, Tag: k.Tag, Style: k.Style, Value: k.Value, LineComment: k.LineComment}
		if v.Kind == yaml.ScalarNode && val.Kind == yaml.ScalarNode {
			val.Style, val.LineComment = v.Style, v.LineComment
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{keyNode, &val}}); err != nil {
		return fmt.Errorf("cannot set %s: %w", key, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("cannot set %s: %w", key, err)
	}

	front := slices.Concat(bytes.Join(lines[:start], nil), buf.Bytes(), bytes.Join(lines[end:], nil))
	np, err := parseFront(front)
	if err != nil {
		return fmt.Errorf("cannot set %s: %w", key, err)
	}
	np.openLine, np.closeLine, np.Body = p.openLine, p.closeLine, p.Body
	*p = *np
	return nil
}

// WithoutKeys returns plan file data with the lines of each named top-level frontmatter key
// removed: the same lines SetCLIField rewrites for that key. Every other byte is kept. Keys
// that are absent are skipped. The frontmatter must be a block mapping; its values are not validated.
func WithoutKeys(data []byte, keys ...string) ([]byte, error) {
	openLine, front, closeLine, body, err := split(data)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(front, &doc); err != nil {
		return nil, fmt.Errorf("malformed frontmatter: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || doc.Content[0].Kind != yaml.MappingNode || doc.Content[0].Style&yaml.FlowStyle != 0 {
		return nil, errors.New("malformed frontmatter: want one key per line")
	}
	lines := bytes.SplitAfter(front, []byte("\n"))
	drop := make([]bool, len(lines))
	for _, key := range keys {
		if i, start, end := keyLines(doc.Content[0], lines, key); i >= 0 {
			for j := start; j < end; j++ {
				drop[j] = true
			}
		}
	}
	kept := slices.Clone(openLine)
	for j, line := range lines {
		if !drop[j] {
			kept = append(kept, line...)
		}
	}
	return slices.Concat(kept, closeLine, body), nil
}

// WithoutSections returns plan file data with each named body section removed: a section
// runs from its "## <name>" heading line to the next "## " heading or the end of the file.
// The frontmatter and every other byte are kept. Sections that are absent are skipped.
func WithoutSections(data []byte, names ...string) ([]byte, error) {
	openLine, front, closeLine, body, err := split(data)
	if err != nil {
		return nil, err
	}
	kept := slices.Concat(openLine, front, closeLine)
	drop := false
	for i := 0; i < len(body); {
		line := firstLine(body[i:])
		i += len(line)
		if heading := strings.TrimRight(string(line), " \t\r\n"); strings.HasPrefix(heading, "## ") {
			drop = slices.Contains(names, strings.TrimSpace(heading[3:]))
		}
		if !drop {
			kept = append(kept, line...)
		}
	}
	return kept, nil
}

// Section returns the text of plan file data's body section name: the lines after its
// "## <name>" heading line up to the next "## " heading or the end of the file, as
// WithoutSections finds them, with leading and trailing white space trimmed. A section that
// appears more than once returns each occurrence's lines, in order, before the trim. An absent
// section is empty.
func Section(data []byte, name string) ([]byte, error) {
	_, _, _, body, err := split(data)
	if err != nil {
		return nil, err
	}
	var text []byte
	in := false
	for i := 0; i < len(body); {
		line := firstLine(body[i:])
		i += len(line)
		if heading := strings.TrimRight(string(line), " \t\r\n"); strings.HasPrefix(heading, "## ") {
			in = strings.TrimSpace(heading[3:]) == name
			continue
		}
		if in {
			text = append(text, line...)
		}
	}
	return bytes.TrimSpace(text), nil
}

// AppendLog appends line, a "- " list item, to the body's "## Log" section,
// after its last non-blank line, adding the section at the end of the body
// when there is none. Every other byte is kept.
func (p *Plan) AppendLog(line string) {
	start, end := -1, len(p.Body)
	for i := 0; i < len(p.Body); {
		l := firstLine(p.Body[i:])
		heading := strings.TrimRight(string(l), " \t\r\n")
		i += len(l)
		switch {
		case start < 0 && heading == "## Log":
			start = i
		case start >= 0 && strings.HasPrefix(heading, "## "):
			end = i - len(l)
			i = len(p.Body)
		}
	}
	if start < 0 {
		body := bytes.TrimRight(p.Body, "\n")
		p.Body = slices.Concat(body, []byte("\n\n## Log\n\n"+line+"\n"))
		return
	}
	section := bytes.TrimRight(p.Body[start:end], "\n")
	entry := "\n" + line + "\n"
	if len(bytes.TrimSpace(section)) > 0 {
		entry = line + "\n"
		section = append(slices.Clone(section), '\n')
	}
	if end < len(p.Body) {
		entry += "\n"
	}
	p.Body = slices.Concat(p.Body[:start], section, []byte(entry), p.Body[end:])
}

// keyLines finds key in the frontmatter mapping root, whose source is lines. It returns the
// key's index in root.Content, or -1 when absent, and the half-open range of lines holding the
// key and its value: up to the next key, less the blank and top-level comment lines before
// it, which belong to no key.
func keyLines(root *yaml.Node, lines [][]byte, key string) (i, start, end int) {
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value != key {
			continue
		}
		start, end = root.Content[i].Line-1, len(lines)
		if i+2 < len(root.Content) {
			end = root.Content[i+2].Line - 1
		}
		for end > start+1 && (len(bytes.TrimSpace(lines[end-1])) == 0 || lines[end-1][0] == '#') {
			end--
		}
		return i, start, end
	}
	return -1, 0, 0
}

func parsePlan(data []byte) (*Plan, error) {
	openLine, front, closeLine, body, err := split(data)
	if err != nil {
		return nil, err
	}
	p, err := parseFront(front)
	if err != nil {
		return nil, err
	}
	p.openLine, p.closeLine, p.Body = openLine, closeLine, body
	return p, nil
}

// SplitFrontmatter cuts file data with "---" frontmatter, a plan's or any other, into its
// opening "---" line, frontmatter, closing "---" line and body, as plans are cut.
func SplitFrontmatter(data []byte) (openLine, front, closeLine, body []byte, err error) {
	return split(data)
}

// split cuts a plan file into its opening "---" line, frontmatter, closing "---" line and body.
func split(data []byte) (openLine, front, closeLine, body []byte, err error) {
	openLine = firstLine(data)
	if !isDelimiter(openLine) {
		return nil, nil, nil, nil, errors.New(`malformed frontmatter: the file must start with a "---" line`)
	}
	rest := data[len(openLine):]
	for i := 0; i < len(rest); {
		line := firstLine(rest[i:])
		if isDelimiter(line) {
			return openLine, rest[:i], line, rest[i+len(line):], nil
		}
		i += len(line)
	}
	return nil, nil, nil, nil, errors.New(`malformed frontmatter: no closing "---" line`)
}

func parseFront(front []byte) (*Plan, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(front, &doc); err != nil {
		return nil, fmt.Errorf("malformed frontmatter: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("malformed frontmatter: want a mapping of keys to values")
	}
	p := &Plan{front: front, root: doc.Content[0]}
	if err := doc.Decode(p); err != nil {
		return nil, fmt.Errorf("malformed frontmatter: %w", err)
	}
	switch p.Status {
	case StatusPlanned, StatusInProgress, StatusImplemented, StatusClosed:
	default:
		return nil, fmt.Errorf("status %q is not one of planned, in-progress, implemented, closed", p.Status)
	}
	return p, nil
}

// firstLine returns the first line of b, with its line ending.
func firstLine(b []byte) []byte {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return b[:i+1]
	}
	return b
}

func isDelimiter(line []byte) bool {
	return string(bytes.TrimRight(line, "\r\n")) == "---"
}
