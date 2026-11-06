package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/templates"
	"github.com/spf13/cobra"
)

// ejectChange is one file eject changes: rel is its path as reported, full
// its path on disk, and content what it is left with, nil when it goes.
type ejectChange struct {
	rel, full string
	content   []byte
}

// runEject is `lyceum eject` (distribution.md, "Uninstall"), the
// inverse of init. Like approve it is the owner's: refused inside a harness
// and without a terminal, before anything else (ownerOnly), and confirmed by
// typing eject. It removes the skills, the hook entries, merged out of each
// settings file, the shims, Codex's writable root, the CI workflow, the
// pre-commit hook's Lyceum block and the Lyceum sections of AGENTS.md and
// CLAUDE.md. A file goes only when nothing but Lyceum's part was in it. It
// works out every change before making any, so a file it can't edit refuses
// with nothing changed. lyceum/ and the root docs stay; it commits
// nothing.
func runEject(cmd *cobra.Command, args []string) error {
	what, run := "eject", "lyceum eject"
	if err := ownerOnly(cmd, what, run, "eject"); err != nil {
		return err
	}
	if len(args) > 0 {
		return out.Refuse(what, "eject takes no arguments", `run "lyceum eject" yourself in a terminal`)
	}
	if err := gitx.CheckVersion(); err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	if !gitx.IsRepo(dir) {
		return out.Refuse(what, "this is not a git repository, so Lyceum is not set up here",
			`run "lyceum eject" in the repo to remove Lyceum from`)
	}
	root, err := gitx.Root(dir)
	if err != nil {
		return err
	}
	changes, err := ejectChanges(root)
	if err != nil {
		return err
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	data := struct {
		Removed []string `json:"removed"`
		Updated []string `json:"updated"`
	}{[]string{}, []string{}}
	w := out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}
	if len(changes) == 0 {
		w.Result(out.Result{Next: "nothing to do: Lyceum's skills, hooks and checks are not installed here", Data: data})
		return nil
	}

	var summary strings.Builder
	summary.WriteString("Eject removes Lyceum's skills, hooks and checks; lyceum/ and the root docs stay:\n")
	for _, c := range changes {
		verb := "remove"
		if c.content != nil {
			verb = "update"
		}
		fmt.Fprintf(&summary, "  %s %s\n", verb, c.rel)
	}
	summary.WriteString("Type eject to confirm: ")
	if !typedConfirmation(cmd.ErrOrStderr(), bufio.NewReader(cmd.InOrStdin()), summary.String(), "eject") {
		return out.Refuse(what, "confirmation did not match; nothing was removed",
			`run "lyceum eject" again and type eject exactly`)
	}

	for _, c := range changes {
		if c.content != nil {
			fi, err := os.Stat(c.full)
			if err != nil {
				return err
			}
			if err := os.WriteFile(c.full, c.content, fi.Mode().Perm()); err != nil {
				return err
			}
			data.Updated = append(data.Updated, c.rel)
			continue
		}
		if err := os.Remove(c.full); err != nil {
			return err
		}
		data.Removed = append(data.Removed, c.rel)
		if !filepath.IsAbs(c.rel) && !strings.HasPrefix(c.rel, ".git/") {
			// The folders init made for it go too, once empty.
			for d := filepath.Dir(c.full); d != root; d = filepath.Dir(d) {
				if os.Remove(d) != nil {
					break
				}
			}
		}
	}
	if !asJSON {
		var b strings.Builder
		for _, p := range data.Removed {
			b.WriteString("Removed " + p + "\n")
		}
		for _, p := range data.Updated {
			b.WriteString("Updated " + p + "\n")
		}
		_, _ = io.WriteString(cmd.OutOrStdout(), b.String())
	}
	w.Result(out.Result{Next: `review the removal with "git status", then commit it: git add -A && git commit -m "lyceum eject"`, Data: data})
	return nil
}

// ejectChanges works out what eject changes under root, in the order init
// writes it, refusing on a file it can't edit safely.
func ejectChanges(root string) ([]ejectChange, error) {
	var changes []ejectChange
	read := func(rel string) ([]byte, string, bool, error) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		data, err := os.ReadFile(full)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, full, false, nil
		}
		return data, full, err == nil, err
	}
	cantEdit := func(rel string, err error) error {
		return out.Refuse("eject", rel+" can't be edited safely: "+err.Error()+"; nothing was removed",
			"take Lyceum's part out of "+rel+` yourself, then run "lyceum eject" again`)
	}

	for _, dir := range []string{".agents/skills", ".claude/skills"} {
		for _, name := range templates.SkillNames {
			rel := dir + "/lyceum-" + name + "/SKILL.md"
			if _, full, ok, err := read(rel); err != nil {
				return nil, err
			} else if ok {
				changes = append(changes, ejectChange{rel, full, nil})
			}
		}
	}
	for _, h := range initHarnesses {
		data, full, ok, err := read(h.hook)
		if err != nil {
			return nil, err
		}
		switch {
		case !ok:
		case h.shim != "":
			changes = append(changes, ejectChange{h.hook, full, nil})
		case strings.Contains(string(data), hookCommandPrefix):
			content, err := unmergeHook(data)
			if err != nil {
				return nil, cantEdit(h.hook, err)
			}
			changes = append(changes, ejectChange{h.hook, full, content})
		}
	}
	if data, full, ok, err := read(codexConfig); err != nil {
		return nil, err
	} else if ok && strings.Contains(string(data), codexMarker) {
		content, err := unmergeCodexConfig(data)
		if err != nil {
			return nil, cantEdit(codexConfig, err)
		}
		changes = append(changes, ejectChange{codexConfig, full, content})
	}
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		data, full, ok, err := read(name)
		if err != nil {
			return nil, err
		}
		text, found := removeSection(string(data))
		if !ok || !found {
			continue
		}
		// AGENTS.md is a root doc and stays; a CLAUDE.md init
		// created holds only the pointer, and goes.
		content := []byte(text)
		if name == "CLAUDE.md" && text == "# CLAUDE.md\n" {
			content = nil
		}
		changes = append(changes, ejectChange{name, full, content})
	}

	hook, err := gitx.GitPath(root, "hooks/pre-commit")
	if err != nil {
		return nil, err
	}
	full := hook
	if !filepath.IsAbs(full) {
		full = filepath.Join(root, full)
	}
	if data, err := os.ReadFile(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	} else if err == nil && hasBlock(string(data)) {
		var content []byte
		if rest := removeBlock(string(data)); rest != "#!/bin/sh\n" {
			content = []byte(rest)
		}
		changes = append(changes, ejectChange{filepath.ToSlash(hook), full, content})
	}

	// Only the workflow init wrote, by its first line.
	first, _, _ := strings.Cut(templates.Workflow, "\n")
	if data, full, ok, err := read(workflowPath); err != nil {
		return nil, err
	} else if ok && strings.HasPrefix(string(data), first+"\n") {
		changes = append(changes, ejectChange{workflowPath, full, nil})
	}
	return changes, nil
}

// removeSection is text without its marker-delimited Lyceum section, the
// inverse of writeSection: the markers, what is between them, the line end
// after the end marker and the blank line put before an appended section go.
// found is false when text has no whole section.
func removeSection(text string) (string, bool) {
	i := strings.Index(text, templates.SectionStart)
	if i < 0 {
		return text, false
	}
	j := strings.Index(text[i:], templates.SectionEnd)
	if j < 0 {
		return text, false
	}
	head, rest := text[:i], strings.TrimPrefix(text[i+j+len(templates.SectionEnd):], "\n")
	if rest == "" && strings.HasSuffix(head, "\n\n") {
		head = head[:len(head)-1]
	}
	return head + rest, true
}
