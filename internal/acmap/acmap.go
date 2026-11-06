// Package acmap finds the acceptance criteria defined in the spec and the test files
// that map them. An AC is mapped when its ID appears as a whole word in a file
// matching the TESTING.md `test_files:` globs: `AC14` maps AC14, `AC140` and `XAC14` do
// not, and an underscore is a boundary, so `TestExport_AC14` maps AC14.
package acmap

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

var (
	specRow      = regexp.MustCompile(`^\|\s*(AC[0-9]+)\s*\|`)
	specBullet   = regexp.MustCompile(`^\s*[-*]\s+\*\*(AC[0-9]+)\*\*(?:\W|$)`)
	acToken      = regexp.MustCompile(`AC[0-9]+`)
	releaseCheck = regexp.MustCompile(`^.+-([0-9]{4}-[0-9]{2}-[0-9]{2})\.md$`)
)

// liveOnly are the ACs only a live run can prove, as listed in TESTING.md §6.
var liveOnly = []string{"AC4", "AC66", "AC68", "AC74", "AC80", "AC83", "AC84", "AC85", "AC89", "AC90", "AC91", "AC93"}

// releaseHarnesses are the five v1 harnesses a live-only AC must pass on.
var releaseHarnesses = []string{"claude", "codex", "commandcode", "opencode", "pi"}

// platformOnly are the live-only packaging ACs, which pass per v1 platform
// (releasePlatforms) instead of per harness.
var platformOnly = []string{"AC83", "AC84", "AC85", "AC93"}

// releasePlatforms are the four v1 platforms a platformOnly AC must pass on.
var releasePlatforms = []string{"linux-x64", "linux-arm64", "darwin-x64", "darwin-arm64"}

// closedRow matches an AC row whose criterion starts "Withdrawn" (SPEC.md) or
// ends "(Deferred after v1, …)". A criterion that only mentions either
// is still open.
var closedRow = regexp.MustCompile(`(?i)^\|\s*AC[0-9]+\s*\|\s*\W*withdrawn|\(Deferred after v1[^)]*\)\W*\|\s*$`)

// closedBullet is closedRow for a whole bullet, its lines joined by one space.
var closedBullet = regexp.MustCompile(`(?i)^\s*[-*]\s+\*\*AC[0-9]+\*\*\W*withdrawn|\(Deferred after v1[^)]*\)\W*$`)

// bulletEnd matches a line a bullet does not continue onto: a blank line,
// another list item, a heading or a table row. A later paragraph of the item,
// after a blank line, is not read, so a marker there leaves the AC open and
// required rather than silently dropped.
var bulletEnd = regexp.MustCompile(`^\s*$|^\s*(?:[-*+]|[0-9]+[.)])\s|^\s*#{1,6}\s|^\s*\|`)

// Definition is one AC an area file defines. Closed is true for one marked
// withdrawn or deferred after v1.
type Definition struct {
	ID     string
	Closed bool
}

// Definitions returns the ACs defined in the text of an area file, in file
// order, one per defining line: a table row `| ACn |` or a bullet led by the
// bold ID, `- **ACn**` or `* **ACn**`. An ID anywhere else is a mention,
// not a definition. This is the only reader of the two forms.
func Definitions(data []byte) []Definition {
	lines := slices.Collect(strings.Lines(string(data)))
	var defs []Definition
	for i, line := range lines {
		if m := specRow.FindStringSubmatch(line); m != nil {
			defs = append(defs, Definition{m[1], closedRow.MatchString(line)})
			continue
		}
		if m := specBullet.FindStringSubmatch(line); m != nil {
			item := strings.TrimSpace(line)
			for _, next := range lines[i+1:] {
				if bulletEnd.MatchString(next) {
					break
				}
				item += " " + strings.TrimSpace(next)
			}
			defs = append(defs, Definition{m[1], closedBullet.MatchString(item)})
		}
	}
	return defs
}

// OpenSpecACs returns the AC IDs SpecACs lists, without one marked withdrawn
// or deferred after v1, sorted numerically.
func OpenSpecACs(specDir string) ([]string, error) {
	ids, err := SpecACs(specDir)
	if err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(specDir, "*.md"))
	if err != nil {
		return nil, err
	}
	closed := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for _, def := range Definitions(data) {
			if def.Closed {
				closed[def.ID] = true
			}
		}
	}
	return slices.DeleteFunc(ids, func(id string) bool { return closed[id] }), nil
}

// SpecACs returns every AC ID defined in specDir/*.md (see Definitions), sorted
// numerically.
func SpecACs(specDir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(specDir, "*.md"))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for _, def := range Definitions(data) {
			seen[def.ID] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sortIDs(ids)
	return ids, nil
}

// Mapped returns, for every AC ID that appears as a whole word in a file under root
// matching one of globs, the slash-separated paths (relative to root) of those files, in walk order.
func Mapped(root string, globs []string) (map[string][]string, error) {
	mapped := map[string][]string{}
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
		rel = filepath.ToSlash(rel)
		if !MatchAny(globs, rel) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, id := range wholeWordIDs(data) {
			mapped[id] = append(mapped[id], rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mapped, nil
}

// ReleaseChecked returns the live-only ACs the latest release check in dir records as
// passed: a PASS line for every v1 harness, or for a platformOnly AC every v1 platform,
// and no FAIL line. The latest file
// is the `<label>-YYYY-MM-DD.md` with the greatest date, then the greatest name. A missing
// dir or no such file yields none.
func ReleaseChecked(dir string) (map[string]bool, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	latest, latestDate := "", ""
	for _, e := range entries {
		m := releaseCheck.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil {
			continue
		}
		if m[1] > latestDate || m[1] == latestDate && e.Name() > latest {
			latest, latestDate = e.Name(), m[1]
		}
	}
	if latest == "" {
		return map[string]bool{}, nil
	}
	p := filepath.Join(dir, latest)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	lines, err := resultLines(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	pass := map[string]map[string]bool{}
	fail := map[string]bool{}
	for _, l := range lines {
		f := strings.Split(l, "|")
		if len(f) != 4 || f[0] != "RESULT" || f[3] != "PASS" && f[3] != "FAIL" {
			return nil, fmt.Errorf("%s: malformed result line %q", p, l)
		}
		h, id := f[1], f[2] // h is a harness, or a platform for a platformOnly AC
		if f[3] == "FAIL" {
			fail[id] = true
			continue
		}
		if pass[id] == nil {
			pass[id] = map[string]bool{}
		}
		pass[id][h] = true
	}
	checked := map[string]bool{}
	for _, id := range liveOnly {
		on := releaseHarnesses
		if slices.Contains(platformOnly, id) {
			on = releasePlatforms
		}
		ok := !fail[id]
		for _, h := range on {
			ok = ok && pass[id][h]
		}
		if ok {
			checked[id] = true
		}
	}
	return checked, nil
}

// resultLines returns the non-blank lines of the first fenced block after the
// `## Results` heading.
func resultLines(data []byte) ([]string, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	inSection, inFence := false, false
	var lines []string
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		switch {
		case !inSection:
			inSection = l == "## Results"
		case !inFence && strings.HasPrefix(l, "## "):
			return nil, errors.New("## Results has no fenced block")
		case strings.HasPrefix(l, "```"):
			if inFence {
				return lines, nil
			}
			inFence = true
		case inFence && l != "":
			lines = append(lines, l)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !inSection {
		return nil, errors.New("no ## Results section")
	}
	if inFence {
		return nil, errors.New("unterminated ## Results block")
	}
	return nil, errors.New("## Results has no fenced block")
}

// wholeWordIDs returns the distinct AC IDs in data not preceded or followed by a
// letter or digit.
func wholeWordIDs(data []byte) []string {
	seen := map[string]bool{}
	var ids []string
	for _, loc := range acToken.FindAllIndex(data, -1) {
		if loc[0] > 0 && isAlnum(data[loc[0]-1]) {
			continue
		}
		if loc[1] < len(data) && isAlnum(data[loc[1]]) {
			continue
		}
		id := string(data[loc[0]:loc[1]])
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

func isAlnum(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// MatchAny reports whether rel, a slash-separated path relative to the repo
// root, matches one of globs, in the TESTING.md `test_files:` glob form.
func MatchAny(globs []string, rel string) bool {
	for _, g := range globs {
		if match(strings.Split(g, "/"), strings.Split(rel, "/")) {
			return true
		}
	}
	return false
}

// match reports whether path segments match pattern segments, where a `**` segment
// matches zero or more path segments and every other segment follows path.Match.
func match(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if match(pat[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	if ok, _ := path.Match(pat[0], segs[0]); !ok {
		return false
	}
	return match(pat[1:], segs[1:])
}

// TestingGlobs reads `test_files:` from the YAML frontmatter of the TESTING.md at testingMD.
func TestingGlobs(testingMD string) ([]string, error) {
	data, err := os.ReadFile(testingMD)
	if err != nil {
		return nil, err
	}
	fm, err := frontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", testingMD, err)
	}
	var meta struct {
		TestFiles []string `yaml:"test_files"`
	}
	if err := yaml.Unmarshal(fm, &meta); err != nil {
		return nil, fmt.Errorf("%s: %w", testingMD, err)
	}
	if len(meta.TestFiles) == 0 {
		return nil, fmt.Errorf("%s: frontmatter has no test_files", testingMD)
	}
	return meta.TestFiles, nil
}

// frontmatter returns the YAML between a leading `---` line and the next `---` line.
func frontmatter(data []byte) ([]byte, error) {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, errors.New("no frontmatter")
	}
	body := data[len("---\n"):]
	if bytes.HasPrefix(body, []byte("---\n")) {
		return nil, nil
	}
	end := bytes.Index(body, []byte("\n---\n"))
	if end < 0 {
		if !bytes.HasSuffix(body, []byte("\n---")) {
			return nil, errors.New("unterminated frontmatter")
		}
		end = len(body) - len("\n---")
	}
	return body[:end+1], nil
}

// sortIDs sorts AC IDs by number.
func sortIDs(ids []string) {
	sort.Slice(ids, func(i, j int) bool {
		return acNum(ids[i]) < acNum(ids[j])
	})
}

func acNum(id string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(id, "AC"))
	return n
}
