package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/records"
	"github.com/oritromax/lyceum/internal/unit"
	"github.com/spf13/cobra"
)

// docsDir holds the human-facing docs and exports.
const docsDir = "docs"

// noFreshness is the state of a candidate with nothing to check: a doc, or an
// area file with no cites:.
const noFreshness = "—"

// candidate is one record a lookup lists: an area file, an understand or
// trace record, or a doc. State is its freshness, or noFreshness.
type candidate struct {
	Kind    string   `json:"kind"`
	Path    string   `json:"path"`
	Title   string   `json:"title"`
	State   string   `json:"state"`
	Changed []string `json:"changed,omitempty"`
	Missing []string `json:"missing,omitempty"`
	Reason  string   `json:"reason,omitempty"`

	detail string // records.Entry.Detail
}

// scope is the measured size of the repo: the files git tracks or would
// track, their total lines, and whether either is over threshold.
type scope struct {
	Files     int `json:"files"`
	Lines     int `json:"lines"`
	Threshold struct {
		Files int `json:"files"`
		Lines int `json:"lines"`
	} `json:"threshold"`
	Over bool `json:"over"`
}

// runUnderstand is `lyceum understand [hint]` (records.md, "lyceum understand
// [hint]", "CLI and agent split").
func runUnderstand(cmd *cobra.Command, args []string) error {
	return lookup(cmd, args, records.Understand)
}

// runTrace is `lyceum trace [question]`: the lookup, as for understand, or,
// with --run, a full run (runTraceRun).
func runTrace(cmd *cobra.Command, args []string) error {
	if cmd.Flags().Changed("run") {
		env, _ := cmd.Flags().GetString("run")
		return runTraceRun(cmd, args, env)
	}
	for _, f := range []string{"yes", "trigger"} {
		if cmd.Flags().Changed(f) {
			return out.Refuse(records.Trace, "--"+f+" applies only to a full run, with --run <env>",
				`run lyceum trace --run <env> --trigger "<command>" "<question>"`)
		}
	}
	return lookup(cmd, args, records.Trace)
}

// lookup lists the candidate records for an understand or trace (kind) and
// creates nothing: the registered spec areas first, in registry order
// (AC76), then lyceum/understand/ and lyceum/trace/ records, then the
// .md files under docs/, each with its title and freshness. Matching the hint
// to a candidate is the agent's job, so it prints the hint back. With no
// hint, it also measures the scope and, over understand.warn_threshold,
// tells the agent to ask the user before reading the code (AC75).
func lookup(cmd *cobra.Command, args []string, kind string) error {
	what, noun := kind, "hint"
	if kind == records.Trace {
		noun = "question"
	}
	if len(args) > 1 {
		return out.Refuse(what, fmt.Sprintf("%s takes at most one argument, the %s", kind, noun),
			fmt.Sprintf(`quote the %s: lyceum %s "<%s>"`, noun, kind, noun))
	}
	root, cfg, err := setUpRoot(what)
	if err != nil {
		return err
	}
	cands, err := candidates(root)
	if err != nil {
		return err
	}
	hint := ""
	if len(args) == 1 {
		hint = strings.TrimSpace(args[0])
	}
	var sc *scope
	if hint == "" {
		if sc, err = measure(root, cfg.Understand.WarnThreshold); err != nil {
			return err
		}
	}

	dir := records.Dir(kind)
	write := fmt.Sprintf(`write the record in %s/ with cites: paths only, then run "lyceum record %s/<file>.md"`, dir, dir)
	next := fmt.Sprintf("if a fresh candidate answers the %s, summarise it; if one is drifted or broken, refresh it from the code; otherwise read the code, %s", noun, write)
	switch {
	case hint == "" && kind == records.Understand:
		next = "ask the user what to understand, then run \"lyceum understand \\\"<hint>\\\"\""
	case hint == "":
		next = "ask the user which flow to trace, then run \"lyceum trace \\\"<question>\\\"\""
	}
	if sc != nil && sc.Over {
		next = "ask the user before reading the code: the scope is over understand.warn_threshold; " + next
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	if !asJSON {
		_, _ = io.WriteString(cmd.OutOrStdout(), lookupText(noun, hint, cands, sc))
	}
	out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(out.Result{Next: next, Data: struct {
		Hint       string      `json:"hint"`
		Candidates []candidate `json:"candidates"`
		// Scope is measured only with no hint; null otherwise.
		Scope *scope `json:"scope"`
	}{hint, cands, sc}})
	return nil
}

// lookupText is the human form of a lookup.
func lookupText(noun, hint string, cands []candidate, sc *scope) string {
	var b strings.Builder
	if hint != "" {
		fmt.Fprintf(&b, "%s%s: %s\n", strings.ToUpper(noun[:1]), noun[1:], hint)
	}
	if len(cands) == 0 {
		b.WriteString("Candidates: none\n")
	} else {
		b.WriteString("Candidates:\n")
	}
	for _, c := range cands {
		fmt.Fprintf(&b, "  %s %s %s: %s", c.State, c.Kind, c.Path, c.Title)
		if c.detail != "" {
			fmt.Fprintf(&b, " (%s)", c.detail)
		}
		b.WriteString("\n")
	}
	if sc != nil {
		fmt.Fprintf(&b, "Scope: %d files, %d lines\n", sc.Files, sc.Lines)
		if sc.Over {
			fmt.Fprintf(&b, "The scope is over understand.warn_threshold (%d files or %d lines): ask the user before reading the code\n",
				sc.Threshold.Files, sc.Threshold.Lines)
		}
	}
	return b.String()
}

// candidates lists the registered area files, the understand and trace
// records, and the docs, in lookup order, with their freshness.
func candidates(root string) ([]candidate, error) {
	entries, err := records.All(root)
	if err != nil {
		return nil, err
	}
	byPath := map[string]records.Entry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}
	fromEntry := func(kind, p, title string) candidate {
		c := candidate{Kind: kind, Path: p, Title: title, State: noFreshness}
		if e, ok := byPath[p]; ok {
			c.State, c.Changed, c.Missing, c.Reason, c.detail = string(e.State), e.Changed, e.Missing, e.Reason, e.Detail()
		}
		return c
	}

	cands := []candidate{}
	areas, err := approval.RegisteredAreas(root)
	if err != nil {
		return nil, err
	}
	for _, f := range areas {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		cands = append(cands, fromEntry("area", f, heading(data, f)))
	}
	for _, kind := range []string{records.Understand, records.Trace} {
		for _, e := range entries {
			if e.Kind != kind {
				continue
			}
			title := ""
			if data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(e.Path))); err != nil {
				return nil, err
			} else if r, err := records.Parse(data); err == nil {
				title = r.Title
			}
			cands = append(cands, fromEntry(kind, e.Path, title))
		}
	}
	docs, err := docFiles(root)
	if err != nil {
		return nil, err
	}
	for _, f := range docs {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			return nil, err
		}
		cands = append(cands, candidate{Kind: "docs", Path: f, Title: heading(data, f), State: noFreshness})
	}
	return cands, nil
}

// docFiles returns the .md files under docs/, sorted, or none when it does
// not exist.
func docFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(filepath.Join(root, docsDir), func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	slices.Sort(files)
	return files, err
}

// heading returns the text of the first "# " heading in data, after any
// frontmatter, or file's name without .md.
func heading(data []byte, file string) string {
	if records.HasFrontmatter(data) {
		if _, _, _, body, err := unit.SplitFrontmatter(data); err == nil {
			data = body
		}
	}
	for line := range strings.Lines(string(data)) {
		if t, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "# "); ok {
			return strings.TrimSpace(t)
		}
	}
	return strings.TrimSuffix(path.Base(file), ".md")
}

// measure returns the scope of the repo at root: the files git tracks or
// would track, and their lines, binary files counted as files only.
func measure(root string, threshold config.Threshold) (*scope, error) {
	files, err := gitx.Files(root)
	if err != nil {
		return nil, err
	}
	sc := &scope{}
	sc.Threshold.Files, sc.Threshold.Lines = threshold.Files, threshold.Lines
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		fi, err := os.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) || err == nil && !fi.Mode().IsRegular() {
			continue // deleted from the work tree, a symlink or a submodule
		}
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		sc.Files++
		// git's binary heuristic: a NUL in the first 8000 bytes.
		if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
			continue
		}
		sc.Lines += bytes.Count(data, []byte("\n"))
		if len(data) > 0 && data[len(data)-1] != '\n' {
			sc.Lines++
		}
	}
	sc.Over = sc.Files > threshold.Files || sc.Lines > threshold.Lines
	return sc, nil
}

// setUpRoot returns the root and config of the repo the working directory is
// in, refusing what when it is not a git repo or Lyceum is not set up there.
func setUpRoot(what string) (string, *config.Config, error) {
	if err := gitx.CheckVersion(); err != nil {
		return "", nil, err
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", nil, err
	}
	if !gitx.IsRepo(dir) {
		return "", nil, out.Refuse(what, "this is not a git repository, so Lyceum is not set up here", `run "lyceum init"`)
	}
	root, err := gitx.Root(dir)
	if err != nil {
		return "", nil, err
	}
	cfg, err := config.Read(root)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil, out.Refuse(what, "Lyceum is not set up in this repo: "+config.Path+" is missing", `run "lyceum init"`)
	}
	if err != nil {
		return "", nil, err
	}
	return root, cfg, nil
}
