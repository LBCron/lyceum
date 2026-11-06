package cli

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/oritromax/lyceum/internal/out"
)

// codexConfig is Codex's project config, where init makes .git writable.
const codexConfig = ".codex/config.toml"

// codexTable is the config table holding writable_roots.
const codexTable = "sandbox_workspace_write"

// codexMarker opens the comment line init writes above writable_roots,
// naming the path it added, so a re-run can tell it from the user's own.
const codexMarker = "# lyceum init added "

// writeCodexConfig merges the repo's absolute .git path into writable_roots
// in Codex's project config (distribution.md, "lyceum init" step 4):
// under workspace-write Codex mounts .git read-only, so the agent can't commit.
func writeCodexConfig(root string, r *initReport) error {
	full := filepath.Join(root, filepath.FromSlash(codexConfig))
	data, err := os.ReadFile(full)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	gitDir := filepath.Join(root, ".git")
	content, err := mergeCodexConfig(data, gitDir)
	if err != nil {
		return out.Refuse("wire Codex", codexConfig+" has a writable_roots that lyceum init can't edit safely: "+err.Error(),
			"add "+tomlString(gitDir)+" to writable_roots under ["+codexTable+"] in "+codexConfig+` yourself, then run "lyceum init" again`)
	}
	c, err := putFile(root, codexConfig, content, 0o644)
	if err != nil {
		return err
	}
	r.add(codexConfig, c)
	return nil
}

// mergeCodexConfig returns the TOML data with path in writable_roots under
// [sandbox_workspace_write], by line editing: every other line is kept. The
// table and key are added when absent; a path a marker says init added before
// is replaced, so a moved repo gets its new path; a path already listed is
// left as it is. Only a one-line array of plain strings is edited; another
// form is an error, unless it already holds path.
func mergeCodexConfig(data []byte, path string) ([]byte, error) {
	text := string(data)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	lines := strings.SplitAfter(text, "\n")
	lines = lines[:len(lines)-1] // the empty string after the last "\n"
	marker := codexMarker + tomlString(path) + " so Codex can commit in this repo\n"
	entry := "writable_roots = [" + tomlString(path) + "]\n"

	header, key, end := -1, -1, len(lines)
	table := ""
	for i, line := range lines {
		t := strings.TrimSpace(line)
		switch {
		case t == "" || strings.HasPrefix(t, "#"):
		case strings.HasPrefix(t, "["):
			name, _, _ := strings.Cut(t, "#")
			table = strings.TrimSpace(strings.Trim(strings.TrimSpace(name), "[]"))
			if table == codexTable {
				header = i
			} else if header >= 0 && end == len(lines) {
				end = i
			}
		case table == "" && (keyOf(t) == codexTable || strings.HasPrefix(keyOf(t), codexTable+".")):
			return nil, errors.New(codexTable + " is set as a dotted key or inline table")
		case table == codexTable && keyOf(t) == "writable_roots":
			key = i
		}
	}

	switch {
	case header < 0:
		if text != "" {
			text += "\n"
		}
		return []byte(text + "[" + codexTable + "]\n" + marker + entry), nil
	case key < 0:
		return []byte(strings.Join(slices.Insert(lines, header+1, marker, entry), "")), nil
	}

	line := lines[key]
	eq := strings.Index(line, "=")
	value := line[eq+1:]
	roots, rest, err := parseRoots(value)
	if err != nil {
		if strings.Contains(strings.Join(lines[key:end], ""), tomlString(path)) {
			return data, nil
		}
		return nil, err
	}
	stale := ""
	if key > 0 {
		if m, ok := strings.CutPrefix(strings.TrimSpace(lines[key-1]), codexMarker); ok {
			if s, _, ok := parseString(m); ok {
				stale = s
			}
		}
	}
	has := slices.ContainsFunc(roots, func(r writableRoot) bool { return r.value == path })
	if has && (stale == "" || stale == path) {
		return data, nil
	}
	roots = slices.DeleteFunc(roots, func(r writableRoot) bool { return stale != "" && r.value == stale })
	if !has {
		roots = append(roots, writableRoot{raw: tomlString(path), value: path})
	}
	raws := make([]string, len(roots))
	for i, r := range roots {
		raws[i] = r.raw
	}
	space := value[:len(value)-len(strings.TrimLeft(value, " \t"))]
	lines[key] = line[:eq+1] + space + "[" + strings.Join(raws, ", ") + "]" + rest
	if stale != "" {
		lines[key-1] = marker
	} else {
		lines = slices.Insert(lines, key, marker)
	}
	return []byte(strings.Join(lines, "")), nil
}

// keyOf is the bare key of TOML line t, "" when t is no key/value line.
func keyOf(t string) string {
	k, _, ok := strings.Cut(t, "=")
	if !ok {
		return ""
	}
	return strings.TrimSpace(k)
}

// writableRoot is one writable_roots element: its source text and its string value.
type writableRoot struct{ raw, value string }

// parseRoots parses s, the rest of a writable_roots line after "=", as a
// one-line array of strings. It returns the elements and what follows the
// closing "]", a comment and the line end.
func parseRoots(s string) ([]writableRoot, string, error) {
	s = strings.TrimLeft(s, " \t")
	if !strings.HasPrefix(s, "[") {
		return nil, "", errors.New("writable_roots is not an array")
	}
	s = s[1:]
	var roots []writableRoot
	for {
		s = strings.TrimLeft(s, " \t,")
		switch {
		case strings.HasPrefix(s, "]"):
			rest := s[1:]
			if t := strings.TrimSpace(rest); t != "" && !strings.HasPrefix(t, "#") {
				return nil, "", errors.New("writable_roots has text after its array")
			}
			return roots, rest, nil
		case s == "" || strings.HasPrefix(s, "\n") || strings.HasPrefix(s, "\r") || strings.HasPrefix(s, "#"):
			return nil, "", errors.New("writable_roots spans several lines")
		}
		v, n, ok := parseString(s)
		if !ok {
			return nil, "", errors.New("writable_roots holds an element that is not a plain string")
		}
		roots = append(roots, writableRoot{raw: s[:n], value: v})
		s = s[n:]
	}
}

// parseString parses the TOML string s starts with: a literal string, or a
// basic one whose only escapes are \\ and \". It returns its value and length.
func parseString(s string) (string, int, bool) {
	switch {
	case strings.HasPrefix(s, "'") && !strings.HasPrefix(s, "'''"):
		if i := strings.IndexAny(s[1:], "'\n"); i >= 0 && s[1+i] == '\'' {
			return s[1 : 1+i], i + 2, true
		}
	case strings.HasPrefix(s, `"`) && !strings.HasPrefix(s, `"""`):
		var b strings.Builder
		for i := 1; i < len(s); i++ {
			switch c := s[i]; {
			case c == '"':
				return b.String(), i + 1, true
			case c == '\\' && i+1 < len(s) && (s[i+1] == '\\' || s[i+1] == '"'):
				i++
				b.WriteByte(s[i])
			case c == '\\' || c == '\n':
				return "", 0, false
			default:
				b.WriteByte(c)
			}
		}
	}
	return "", 0, false
}

// tomlString is s as a TOML basic string.
func tomlString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// unmergeCodexConfig returns the TOML data without what mergeCodexConfig
// added (distribution.md, "Uninstall"): each path a marker line names
// is taken out of the writable_roots on the line below it, with the marker;
// a writable_roots left empty goes, then a [sandbox_workspace_write] left with
// nothing under it, with the blank line before it. Every other line is kept.
// It returns nil when only blank lines are left.
func unmergeCodexConfig(data []byte) ([]byte, error) {
	text := string(data)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	lines := strings.SplitAfter(text, "\n")
	lines = lines[:len(lines)-1]
	for i := 0; i < len(lines); i++ {
		m, ok := strings.CutPrefix(strings.TrimSpace(lines[i]), codexMarker)
		if !ok {
			continue
		}
		path, _, ok := parseString(m)
		if !ok || i+1 == len(lines) || keyOf(strings.TrimSpace(lines[i+1])) != "writable_roots" {
			return nil, errors.New("the marker line " + strings.TrimSpace(lines[i]) + " is not above a writable_roots line")
		}
		line := lines[i+1]
		eq := strings.Index(line, "=")
		value := line[eq+1:]
		roots, rest, err := parseRoots(value)
		if err != nil {
			return nil, err
		}
		roots = slices.DeleteFunc(roots, func(r writableRoot) bool { return r.value == path })
		if len(roots) == 0 {
			lines = slices.Delete(lines, i, i+2)
		} else {
			raws := make([]string, len(roots))
			for j, r := range roots {
				raws[j] = r.raw
			}
			space := value[:len(value)-len(strings.TrimLeft(value, " \t"))]
			lines[i+1] = line[:eq+1] + space + "[" + strings.Join(raws, ", ") + "]" + rest
			lines = slices.Delete(lines, i, i+1)
		}
		i--
	}

	// A [sandbox_workspace_write] with only blank lines under it goes.
	for i, line := range lines {
		t := strings.TrimSpace(line)
		name, _, _ := strings.Cut(t, "#")
		if !strings.HasPrefix(t, "[") || strings.TrimSpace(strings.Trim(strings.TrimSpace(name), "[]")) != codexTable {
			continue
		}
		end := i + 1
		for end < len(lines) && strings.TrimSpace(lines[end]) == "" {
			end++
		}
		if end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), "[") {
			break
		}
		start := i
		if start > 0 && strings.TrimSpace(lines[start-1]) == "" {
			start--
		}
		lines = slices.Delete(lines, start, end)
		break
	}
	if strings.TrimSpace(strings.Join(lines, "")) == "" {
		return nil, nil
	}
	return []byte(strings.Join(lines, "")), nil
}
