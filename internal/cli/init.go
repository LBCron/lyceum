package cli

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/templates"
	"github.com/spf13/cobra"
)

// initFiles are the files init writes, relative to the repo root, in the order it reports them.
var initFiles = []string{
	"AGENTS.md", "RULES.md", "lyceum/SPEC.md", config.Path, events.Path, ".gitattributes", ".gitignore",
}

// initNext is the next action after init: add refuses a dirty tree.
const initNext = `commit these files: git add -A && git commit -m "lyceum init", then lyceum add feat "<title>"`

// initNextGreenfield is the next action after init on a greenfield repo whose
// project gate waits: the agent's project interview, then approve project.
const initNextGreenfield = `commit these files: git add -A && git commit -m "lyceum init", ` +
	`then start your agent's lyceum-init skill for the project interview, then run "lyceum approve project" yourself in a terminal`

// runInit is `lyceum init`: it offers git init outside a repo, then writes every
// file in initFiles that is missing (distribution.md, "lyceum init"). An existing
// file is never overwritten; an existing AGENTS.md gets the Lyceum section
// appended only if it has none and the user agrees, and the section between
// its markers replaced if it has one; when Claude Code is wired,
// CLAUDE.md gets a short pointer to AGENTS.md under the same rule.
// TESTING.md is never written here. It then wires the harnesses named by --harness, those
// already wired and the detected ones the user accepts, and offers
// the pre-commit hook and the CI workflow. In a brownfield repo it offers
// understand for a baseline; in a greenfield one whose
// project gate is not approved, it hands over to the agent's project interview
// and approve project. Re-running it updates what it
// installed and never touches lyceum/ content.
func runInit(cmd *cobra.Command, _ []string) error {
	if err := gitx.CheckVersion(); err != nil {
		return err
	}
	named, _ := cmd.Flags().GetStringArray("harness")
	if err := checkHarnessFlags(named); err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	in := bufio.NewReader(cmd.InOrStdin())
	if !gitx.IsRepo(dir) {
		if !ask(cmd.ErrOrStderr(), in, "This is not a git repository. Run git init here? [y/N] ") {
			return out.Refuse("set up Lyceum here", "this is not a git repository",
				`run "git init", then "lyceum init" again`)
		}
		if err := gitx.Init(dir); err != nil {
			return err
		}
	}
	root, err := gitx.Root(dir)
	if err != nil {
		return err
	}
	owner, err := gitx.UserEmail(root)
	if err != nil {
		return err
	}
	if owner == "" {
		return out.Refuse("set up Lyceum here", "git user.email is not set, and it names the owner",
			`run "git config user.email <your email>", then "lyceum init" again`)
	}
	base, err := gitx.DefaultBranch(root)
	if err != nil {
		return err
	}
	if base == "" {
		return out.Refuse("set up Lyceum here", "HEAD is detached, so there is no branch to cut units from",
			`check out the branch units should start from, then run "lyceum init" again`)
	}

	found, err := brownfield(root)
	if err != nil {
		return err
	}

	before, err := snapshot(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "lyceum"), 0o755); err != nil {
		return err
	}
	_, declined, err := writeSection(cmd.ErrOrStderr(), in, root, "AGENTS.md", templates.AgentsSection)
	if err != nil {
		return err
	}
	for _, f := range []struct{ path, content string }{
		{"RULES.md", templates.Rules},
		{"lyceum/SPEC.md", templates.Spec},
		{events.Path, ""},
	} {
		if err := createFile(filepath.Join(root, f.path), f.content); err != nil {
			return err
		}
	}
	if _, ok := before[config.Path]; !ok {
		if err := config.Write(root, config.Default(owner, base)); err != nil {
			return err
		}
	}
	if err := config.EnsureRepoFiles(root); err != nil {
		return err
	}
	wire := chooseHarnesses(cmd.ErrOrStderr(), in, root, named)
	claude := slices.ContainsFunc(wire, func(h initHarness) bool { return h.name == "claude" })
	var claudeChange change
	var claudeDeclined bool
	if claude {
		if claudeChange, claudeDeclined, err = writeSection(cmd.ErrOrStderr(), in, root, "CLAUDE.md", templates.ClaudePointer); err != nil {
			return err
		}
	}
	after, err := snapshot(root)
	if err != nil {
		return err
	}

	data := &initReport{[]string{}, []string{}, []string{}, []string{}, len(found) > 0}
	for _, h := range wire {
		data.Wired = append(data.Wired, h.name)
	}
	for _, p := range initFiles {
		old, had := before[p]
		switch {
		case !had:
			data.add(p, created)
		case string(old) != string(after[p]):
			data.add(p, updated)
		default:
			data.add(p, unchanged)
		}
	}
	if claude {
		data.add("CLAUDE.md", claudeChange)
	}
	if err := writeSkills(root, wire, data); err != nil {
		return err
	}
	for _, h := range wire {
		if err := writeHook(root, h, data); err != nil {
			return err
		}
		if h.name == "codex" {
			if err := writeCodexConfig(root, data); err != nil {
				return err
			}
		}
	}
	var skipped []string
	for _, offer := range []func(io.Writer, *bufio.Reader, string, *initReport) (string, error){writePreCommit, writeWorkflow} {
		line, err := offer(cmd.ErrOrStderr(), in, root, data)
		if err != nil {
			return err
		}
		if line != "" {
			skipped = append(skipped, line)
		}
	}

	next := initNext
	if len(found) == 0 {
		approved, err := projectApproved(root)
		if err != nil {
			return err
		}
		if !approved {
			next = initNextGreenfield
		}
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	if !asJSON {
		var b strings.Builder
		for _, p := range data.Created {
			b.WriteString("Created " + p + "\n")
		}
		for _, p := range data.Updated {
			b.WriteString("Updated " + p + "\n")
		}
		if declined {
			b.WriteString(`Skipped the Lyceum section in AGENTS.md; run "lyceum init" again to add it` + "\n")
		}
		if claudeDeclined {
			b.WriteString(`Skipped the Lyceum section in CLAUDE.md; run "lyceum init" again to add it` + "\n")
		}
		for _, line := range skipped {
			b.WriteString(line + "\n")
		}
		for _, h := range wire {
			b.WriteString("Wired " + h.display + "\n")
			if h.trust != "" {
				b.WriteString("  " + h.trust + "\n")
			}
			if h.name == "codex" {
				b.WriteString("  Codex can now commit in this repo: " + codexConfig + " makes .git writable under workspace-write\n")
			}
			// A re-run that changed the hook it installed before.
			if h.restart != "" && slices.Contains(data.Updated, h.hook) {
				b.WriteString("  " + h.restart + "\n")
			}
		}
		if len(found) > 0 {
			b.WriteString("Brownfield: this repo already has code (" + strings.Join(found, ", ") + "). " +
				`Lyceum offers "lyceum understand" for a baseline of it; ` +
				"the spec then grows with each area a unit touches\n")
		}
		_, _ = io.WriteString(cmd.OutOrStdout(), b.String())
	}
	out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.
		Result(out.Result{Next: next, Data: data})
	return nil
}

// initReport is what init did to each file it writes, and the harnesses it wired.
type initReport struct {
	Created   []string `json:"created"`
	Updated   []string `json:"updated"`
	Unchanged []string `json:"unchanged"`
	Wired     []string `json:"wired"`
	// Brownfield is set when the repo already had code.
	Brownfield bool `json:"brownfield"`
}

// change is how init left one file.
type change int

const (
	unchanged change = iota
	created
	updated
)

// add reports that init left file p as c.
func (r *initReport) add(p string, c change) {
	switch c {
	case created:
		r.Created = append(r.Created, p)
	case updated:
		r.Updated = append(r.Updated, p)
	default:
		r.Unchanged = append(r.Unchanged, p)
	}
}

// putFile writes content to the file rel under root, creating its directories,
// and says whether that created it, updated it or left it as it was.
func putFile(root, rel string, content []byte, perm fs.FileMode) (change, error) {
	path := filepath.Join(root, filepath.FromSlash(rel))
	old, err := os.ReadFile(path)
	switch {
	case err == nil && bytes.Equal(old, content):
		return unchanged, nil
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return unchanged, err
	}
	c := updated
	if err != nil {
		c = created
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return unchanged, err
	}
	return c, os.WriteFile(path, content, perm)
}

// writeSkills writes the Lyceum skills, as embedded in this binary, to
// .agents/skills/, and to .claude/skills/ when Claude Code is wired
// (distribution.md, "Harness support").
func writeSkills(root string, wire []initHarness, r *initReport) error {
	dirs := []string{".agents/skills"}
	if slices.ContainsFunc(wire, func(h initHarness) bool { return h.name == "claude" }) {
		dirs = append(dirs, ".claude/skills")
	}
	for _, dir := range dirs {
		for _, name := range templates.SkillNames {
			p := dir + "/lyceum-" + name + "/SKILL.md"
			c, err := putFile(root, p, []byte(templates.Skill(name)), 0o644)
			if err != nil {
				return err
			}
			r.add(p, c)
		}
	}
	return nil
}

// writeSection creates the file name under root with section, replaces the
// text between the markers of one that has them with section, or shows section
// and asks before appending it to one that has none. It reports
// how it left the file and whether the user declined.
func writeSection(prompt io.Writer, in *bufio.Reader, root, name, section string) (c change, declined bool, err error) {
	path := filepath.Join(root, name)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return created, false, createFile(path, "# "+name+"\n\n"+section)
	}
	if err != nil {
		return unchanged, false, err
	}
	if text := string(data); strings.Contains(text, templates.SectionStart) {
		c, err := replaceSection(path, text, section)
		return c, false, err
	}
	_, _ = io.WriteString(prompt, section)
	if !ask(prompt, in, "Append this Lyceum section to "+name+"? [y/N] ") {
		return unchanged, true, nil
	}
	add := "\n" + section
	if len(data) > 0 && data[len(data)-1] != '\n' {
		add = "\n" + add
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return unchanged, false, err
	}
	if _, err := f.WriteString(add); err != nil {
		_ = f.Close()
		return unchanged, false, err
	}
	return updated, false, f.Close()
}

// replaceSection rewrites the file at path, whose content is text, with its
// first marker-delimited section, markers included, replaced by section; every
// byte outside the markers is kept. A start marker with no end marker after it
// leaves the file as it is.
func replaceSection(path, text, section string) (change, error) {
	i := strings.Index(text, templates.SectionStart)
	j := strings.Index(text[i:], templates.SectionEnd)
	if j < 0 {
		return unchanged, nil
	}
	replaced := text[:i] + strings.TrimSuffix(section, "\n") + text[i+j+len(templates.SectionEnd):]
	if replaced == text {
		return unchanged, nil
	}
	return updated, os.WriteFile(path, []byte(replaced), 0o644)
}

// ask writes question and reads one line of answer from in. Only "y" or "yes"
// (any case) is a yes; no answer, as from a closed or empty stdin, is a no.
func ask(prompt io.Writer, in *bufio.Reader, question string) bool {
	return askDefault(prompt, in, question, false)
}

// askDefault is ask for a question whose default is def: an empty answer, the
// user's Enter alone, takes def. No answer at all is still a no.
func askDefault(prompt io.Writer, in *bufio.Reader, question string, def bool) bool {
	_, _ = io.WriteString(prompt, question)
	answer, err := in.ReadString('\n')
	if err != nil {
		// The answer, if any, was not ended by the user's Enter.
		_, _ = io.WriteString(prompt, "\n")
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	case "":
		return def && err == nil
	}
	return false
}

// createFile writes content to a new file at path and leaves an existing one as is.
func createFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// snapshot reads every file in initFiles under root; a missing file has no key.
func snapshot(root string) (map[string][]byte, error) {
	m := make(map[string][]byte, len(initFiles))
	for _, p := range initFiles {
		data, err := os.ReadFile(filepath.Join(root, p))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		m[p] = data
	}
	return m, nil
}
