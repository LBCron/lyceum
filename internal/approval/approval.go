// Package approval knows what an approval covers: the files each gate hashes
// and how they are hashed (enforcement.md, "lyceum approve [gate]").
package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/oritromax/lyceum/internal/acmap"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/unit"
)

// Gates approved outside a unit's gate list: `project` at greenfield init
// (units.md, "Greenfield"), `testing` for TESTING.md and `tests` for
// changed fingerprinted test files, which moves no gate.
const (
	Project gate.Gate = "project"
	Testing gate.Gate = "testing"
	Tests   gate.Gate = "tests"
)

// TestingPath is TESTING.md, relative to the repo root.
const TestingPath = "TESTING.md"

// specDir holds the area files, relative to the repo root.
const specDir = "lyceum/spec"

// SpecPath is the spec index holding the area registry, relative to the repo root.
const SpecPath = "lyceum/SPEC.md"

// FilesFor returns the files an approval of gate g on a unit of type typ
// hashes, relative to root in git's path form (enforcement.md, "The approved
// files per gate"). planPath is p's plan file. A named area file that does not
// exist is a refusal, as is a plan area not in the registry, an intent
// with goal: performance and no benchmark:, and the approvals not built yet. project is
// repo-level, like testing: it needs no plan and hashes SpecPath, listed
// first, and every area file its registry lists (RegisteredAreas), so the
// registry the agent wrote at greenfield is approved too; an empty
// registry is a refusal. cause and tests need the branch diff and the event
// log, and are listed by the CLI.
func FilesFor(root string, typ gate.Type, g gate.Gate, planPath string, p *unit.Plan) ([]string, error) {
	what := "approve " + string(g)
	switch {
	case g == Project:
		files, err := RegisteredAreas(root)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, out.Refuse(what, "no area is registered in the area registry of "+SpecPath+", so there is nothing to approve",
				`start your agent's lyceum-init skill for the project interview, which writes and registers the areas, then run "lyceum approve project" yourself in a terminal`)
		}
		for _, f := range files {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); errors.Is(err, fs.ErrNotExist) {
				return nil, out.Refuse(what, fmt.Sprintf("the area registry of %s lists %s, but it does not exist", SpecPath, f),
					fmt.Sprintf("write %s, or remove its row from the area registry of %s", f, SpecPath))
			} else if err != nil {
				return nil, err
			}
		}
		return append([]string{SpecPath}, files...), nil
	case g == gate.Spec:
		if err := areasRegistered(root, what, planPath, p); err != nil {
			return nil, err
		}
		// For feat, this one approval covers the plan too.
		files := []string{planPath}
		for _, area := range p.Areas {
			f := path.Join(specDir, area+".md")
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); errors.Is(err, fs.ErrNotExist) {
				return nil, out.Refuse(what, fmt.Sprintf("the plan names area %q, but %s does not exist", area, f),
					fmt.Sprintf("write %s, or remove %s from areas: in %s", f, area, planPath))
			} else if err != nil {
				return nil, err
			}
			files = append(files, f)
		}
		// A feat updates the registry's rows and AC ranges.
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(SpecPath))); err == nil {
			files = append(files, SpecPath)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		return files, nil
	case g == gate.Plan || g == gate.Intent:
		if err := areasRegistered(root, what, planPath, p); err != nil {
			return nil, err
		}
		if g == gate.Intent && p.Goal == "performance" && strings.TrimSpace(p.Benchmark) == "" {
			return nil, out.Refuse(what, "the plan's goal is performance, but it names no metric: benchmark: is empty",
				fmt.Sprintf(`set benchmark: in %s to a command that exits 0 only when the performance target is met, then run "lyceum approve intent" yourself in a terminal`, planPath))
		}
		return []string{planPath}, nil
	case g == Testing:
		return []string{TestingPath}, nil
	case typ == gate.Refactor && g == gate.Verify:
		// Refactor's test-diff approval is approve tests; verify itself
		// clears on a run.
		return nil, out.Refuse(what, "approving "+string(g)+" for a "+string(typ)+" is not built yet in this version",
			`run "lyceum" to see the available commands`)
	}
	return nil, fmt.Errorf("gate %s of %s has no approved files", g, typ)
}

// areasRegistered refuses what unless every area p's plan, at planPath, names
// has its file, lyceum/spec/<area>.md, in the area registry (AC37). The
// refusal lists the registered areas and how to register one.
func areasRegistered(root, what, planPath string, p *unit.Plan) error {
	files, err := RegisteredAreas(root)
	if err != nil {
		return err
	}
	for _, area := range p.Areas {
		if slices.Contains(files, path.Join(specDir, area+".md")) {
			continue
		}
		names := make([]string, 0, len(files))
		for _, f := range files {
			names = append(names, strings.TrimSuffix(path.Base(f), ".md"))
		}
		existing := "no area is registered yet"
		if len(names) > 0 {
			existing = "the registered areas are " + strings.Join(names, ", ")
		}
		return out.Refuse(what, fmt.Sprintf("the plan names area %q, which is not in the area registry of %s; %s", area, SpecPath, existing),
			fmt.Sprintf("register it: lyceum plan --new-area %s", area),
			fmt.Sprintf("or name a registered area in areas: in %s", planPath))
	}
	return nil
}

// RegisteredAreas returns the area files the area registry of SpecPath under
// root lists, in table order, relative to root in git's path form. The
// registry is the table under the "## Area registry" heading; each row's File
// cell, backticks trimmed, is relative to SpecPath's folder, as in
// "spec/orders.md". A missing SpecPath, or one without the table, lists none.
func RegisteredAreas(root string) ([]string, error) {
	rows, err := RegistryRows(root)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(rows))
	for _, r := range rows {
		files = append(files, r.File)
	}
	if len(files) == 0 {
		return nil, nil
	}
	return files, nil
}

// RegistryRow is one row of the area registry.
type RegistryRow struct {
	File string // relative to the repo root in git's path form
	ACs  string // the ACs cell as written; HasACs is false with no such column
	// HasACs reports whether the table has an ACs column.
	HasACs bool
}

// RegistryRows returns the rows of the area registry of SpecPath under root,
// as RegisteredAreas reads them.
func RegistryRows(root string) ([]RegistryRow, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(SpecPath)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rows []RegistryRow
	in, col, acs := false, -1, -1
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			in, col = line == "## Area registry", -1
			continue
		}
		if !in || !strings.HasPrefix(line, "|") {
			if in && col >= 0 && line != "" {
				break // the table ended
			}
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		switch {
		case col < 0:
			col, acs = slices.Index(cells, "File"), slices.Index(cells, "ACs")
			if col < 0 {
				return nil, fmt.Errorf("%s: the area registry has no File column", SpecPath)
			}
		case strings.Trim(line, "|-: ") == "":
			// The header's separator row.
		case col < len(cells):
			if f := strings.Trim(cells[col], "` "); f != "" {
				r := RegistryRow{File: path.Join(path.Dir(SpecPath), f), HasACs: acs >= 0}
				if acs >= 0 && acs < len(cells) {
					r.ACs = cells[acs]
				}
				rows = append(rows, r)
			}
		}
	}
	return rows, nil
}

// RegisterArea adds a row for area name, with File spec/<name>.md and empty
// Covers and ACs, after the last row of the area registry of SpecPath under
// root. A missing SpecPath, or one without the registry table, is an
// error; so is a name the registry already lists.
func RegisterArea(root, name string) error {
	file := filepath.Join(root, filepath.FromSlash(SpecPath))
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	files, err := RegisteredAreas(root)
	if err != nil {
		return err
	}
	if slices.Contains(files, path.Join(specDir, name+".md")) {
		return fmt.Errorf("%s: area %s is already registered", SpecPath, name)
	}
	lines := slices.Collect(strings.Lines(string(data)))
	in, end := false, -1
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") {
			if end >= 0 {
				break
			}
			in = t == "## Area registry"
			continue
		}
		if !in {
			continue
		}
		if strings.HasPrefix(t, "|") {
			end = i
			continue
		}
		if end >= 0 && t != "" {
			break // the table ended
		}
	}
	if end < 0 {
		return fmt.Errorf("%s has no area registry table under \"## Area registry\"", SpecPath)
	}
	if !strings.HasSuffix(lines[end], "\n") {
		lines[end] += "\n"
	}
	row := fmt.Sprintf("| %s | `spec/%s.md` | | |\n", name, name)
	lines = slices.Insert(lines, end+1, row)
	return os.WriteFile(file, []byte(strings.Join(lines, "")), 0o644)
}

// ACFile returns the area file under root that defines AC id (see
// acmap.Definitions), or "" when there is none.
func ACFile(root, id string) (string, error) {
	files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(specDir), "*.md"))
	if err != nil {
		return "", err
	}
	slices.Sort(files)
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		for _, def := range acmap.Definitions(data) {
			if def.ID == id {
				return path.Join(specDir, filepath.Base(file)), nil
			}
		}
	}
	return "", nil
}

// Hash returns the sha256 of the file at path as "sha256:<hex>".
func Hash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

// Hashes returns the hash of each of files, relative to root, for an approval
// of gate g. The plan file, planPath, is hashed on its IntentHashBasis for
// intent and on its PlanHashBasis for every other gate.
func Hashes(root, planPath string, g gate.Gate, files []string) (map[string]string, error) {
	hashes := make(map[string]string, len(files))
	for _, f := range files {
		full := filepath.Join(root, filepath.FromSlash(f))
		if f != planPath {
			h, err := Hash(full)
			if err != nil {
				return nil, err
			}
			hashes[f] = h
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		basisOf := PlanHashBasis
		if g == gate.Intent {
			basisOf = IntentHashBasis
		}
		basis, err := basisOf(data)
		if err != nil {
			return nil, fmt.Errorf("plan %s: %w", f, err)
		}
		hashes[f] = hashBytes(basis)
	}
	return hashes, nil
}

// PlanHashBasis returns the bytes a plan file's hash is computed on: the file
// without its approvals, gate, status and files keys and without its "## Progress"
// and "## Log" sections.
//
// Approving writes to the plan it approves: the approval is mirrored into
// approvals: and gate advances, after the hash was taken. A hash of the raw
// file would never match again. Those keys are written only by the CLI and
// checked against the event log on their own, so the hash leaves them
// out. The agent keeps ticking Progress and appending to Log after the
// approval, so those sections are left out too, and adds a file the build
// needs to files:, the planned scope the scope check warns on. The hash
// still covers every other byte, the rest of the body and the other
// agent-written keys included.
// `check` re-hashes the plan through this same function.
func PlanHashBasis(data []byte) ([]byte, error) {
	basis, err := unit.WithoutKeys(data, "approvals", "gate", "status", "files")
	if err != nil {
		return nil, err
	}
	return unit.WithoutSections(basis, "Progress", "Log")
}

// IntentHashBasis returns the bytes an intent approval hashes a plan file on:
// only the refactor's intent, so that writing ## Steps and
// public_surface: before approve plan, which hashes the whole plan, leaves it
// intact. The basis is three fields, in this order:
//
//	goal:<n>\n<goal>\n
//	benchmark:<n>\n<benchmark>\n
//	what-and-why:<n>\n<section>\n
//
// where goal and benchmark are the frontmatter values as decoded (empty when
// absent), section is the "## What and why" section text (unit.Section), and
// each <n> is the decimal byte length of the value after it, so no value can
// run into the next. `check` re-hashes an intent approval through this same
// function.
func IntentHashBasis(data []byte) ([]byte, error) {
	p, err := unit.ParsePlan(data)
	if err != nil {
		return nil, err
	}
	section, err := unit.Section(data, "What and why")
	if err != nil {
		return nil, err
	}
	var basis []byte
	for _, f := range []struct{ name, value string }{
		{"goal", p.Goal}, {"benchmark", p.Benchmark}, {"what-and-why", string(section)},
	} {
		basis = fmt.Appendf(basis, "%s:%d\n%s\n", f.name, len(f.value), f.value)
	}
	return basis, nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
