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

	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/records"
	"github.com/oritromax/lyceum/internal/unit"
	"github.com/spf13/cobra"
)

// exportMark starts the first line of every export: the header comment
// naming the record id and file it was exported from.
const exportMark = "<!-- lyceum export: record "

// runExport is `lyceum export <record> [path]` (records.md, "Export"). It writes the record's body under docs/, by default
// docs/<kind>/<slug>.md, after a header linking back to the record and naming
// its id, and commits that file, and only it. It refuses an invalid or broken
// record, a trace with no Mermaid sequenceDiagram block (AC81), and a path
// that holds a file not exported from the same record.
func runExport(cmd *cobra.Command, args []string) error {
	what := "export"
	if len(args) < 1 || len(args) > 2 {
		return out.Refuse(what, "export takes the record file and, optionally, the path to write under docs/",
			`run "lyceum export lyceum/<understand|trace|audit>/<file>.md [docs/<path>.md]"`)
	}
	root, _, err := setUpRoot(what)
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	rel, err := repoPath(root, dir, args[0])
	if err != nil {
		return err
	}
	kind := ""
	for _, k := range records.Kinds {
		if path.Dir(rel) == records.Dir(k) && strings.HasSuffix(rel, ".md") {
			kind = k
		}
	}
	if kind == "" {
		return out.Refuse(what, rel+" is not a record: a .md file directly in lyceum/understand/, lyceum/trace/ or lyceum/audit/",
			`run "lyceum export lyceum/<understand|trace|audit>/<file>.md"`)
	}
	entries, err := records.All(root)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(entries, func(e records.Entry) bool { return e.Path == rel })
	if i < 0 {
		return out.Refuse(what, rel+" does not exist", `run "lyceum understand" or "lyceum trace" to list the records`)
	}
	e := entries[i]
	switch e.State {
	case records.Invalid:
		return out.Refuse(what, fmt.Sprintf("%s is invalid: %s", rel, e.Detail()),
			fmt.Sprintf(`fix it, then run "lyceum record %s" and "lyceum export %s"`, rel, rel))
	case records.Broken:
		return out.Refuse(what, fmt.Sprintf("%s is broken: %s, so it no longer matches the code", rel, e.Detail()),
			fmt.Sprintf(`redo the record from the code, run "lyceum record %s", then "lyceum export %s"`, rel, rel))
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return err
	}
	_, _, _, body, err := unit.SplitFrontmatter(data)
	if err != nil {
		return err
	}
	if kind == records.Trace && !hasSequenceDiagram(body) {
		return out.Refuse(what, rel+" has no Mermaid sequenceDiagram block, which an exported trace carries",
			fmt.Sprintf("add a ```mermaid block with a sequenceDiagram of the flow to %s, run \"lyceum record %s\", then \"lyceum export %s\"", rel, rel, rel))
	}

	_, slug, err := unit.ParseID(e.ID)
	if err != nil {
		return err
	}
	target := path.Join(docsDir, kind, slug+".md")
	if len(args) == 2 {
		if target, err = repoPath(root, dir, args[1]); err != nil {
			return err
		}
		if !strings.HasPrefix(target, docsDir+"/") || !strings.HasSuffix(target, ".md") {
			return out.Refuse(what, target+" is not a .md file under docs/", fmt.Sprintf(`run "lyceum export %s docs/<path>.md"`, rel))
		}
	}
	targetFile := filepath.Join(root, filepath.FromSlash(target))
	if old, err := os.ReadFile(targetFile); err == nil {
		line, _, _ := bytes.Cut(old, []byte("\n"))
		if !strings.HasPrefix(string(line), exportMark+e.ID+" ") {
			return out.Refuse(what, target+" exists and was not exported from record "+e.ID+", so export won't overwrite it",
				fmt.Sprintf(`export to another path: lyceum export %s docs/<path>.md`, rel),
				"or move "+target+" away first")
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	staged, err := gitx.Staged(root)
	if err != nil {
		return err
	}
	if others := slices.DeleteFunc(staged, func(p string) bool { return p == target }); len(others) > 0 {
		return out.Refuse(what, "other files are staged, and export commits only the export:\n  "+strings.Join(others, "\n  "),
			"unstage them: git restore --staged -- "+strings.Join(others, " ")+`, then run "lyceum export `+rel+`" again`)
	}

	back, err := filepath.Rel(filepath.Dir(targetFile), filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return err
	}
	header := fmt.Sprintf("%s%s from %s -->\n> Exported from [%s](%s) (record `%s`). Edit the record, then export it again.\n\n",
		exportMark, e.ID, rel, rel, filepath.ToSlash(back), e.ID)
	content := append([]byte(header), bytes.TrimLeft(body, "\n")...)
	if err := os.MkdirAll(filepath.Dir(targetFile), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(targetFile, content, 0o644); err != nil {
		return err
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	line := "Exported " + rel + " to " + target + "\n"
	if head, err := gitx.Show(root, "HEAD", target); err == nil && bytes.Equal(head, content) {
		line = target + " is already exported: nothing changed\n"
	} else if err := gitx.Commit(root, "lyceum: export "+e.ID, target); err != nil {
		return err
	}
	if e.State == records.Drifted {
		line += fmt.Sprintf("Note: %s is drifted (%s); the export is the record as it was written\n", rel, e.Detail())
	}
	if !asJSON {
		_, _ = io.WriteString(cmd.OutOrStdout(), line)
	}
	out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(out.Result{
		Next: "tell the user the record is exported to " + target + "; the record in " + records.Dir(kind) + "/ stays the source",
		Data: struct {
			Record string `json:"record"`
			ID     string `json:"id"`
			Export string `json:"export"`
			State  string `json:"state"`
		}{rel, e.ID, target, string(e.State)},
	})
	return nil
}

// repoPath returns arg, a path relative to dir or absolute, relative to root
// in git's path form.
func repoPath(root, dir, arg string) (string, error) {
	// root is git's, with symlinks resolved.
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	abs := arg
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(dir, abs)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// hasSequenceDiagram reports whether body has a fenced ```mermaid block whose
// content holds a sequenceDiagram.
func hasSequenceDiagram(body []byte) bool {
	fence, mermaid, found := "", false, false
	for line := range strings.Lines(string(body)) {
		t := strings.TrimSpace(line)
		switch {
		case fence == "" && strings.HasPrefix(t, "```"):
			fence = t[:len(t)-len(strings.TrimLeft(t, "`"))]
			mermaid = strings.TrimSpace(t[len(fence):]) == "mermaid"
		case fence != "" && strings.TrimRight(t, "`") == "" && len(t) >= len(fence):
			fence = ""
		case fence != "" && mermaid && strings.HasPrefix(t, "sequenceDiagram"):
			found = true
		}
	}
	return found
}
