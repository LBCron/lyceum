package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/oritromax/lyceum/internal/codepath"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/harness"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/templates"
)

// initHarness is one of the five v1 harnesses as init detects it
// (distribution.md, "lyceum init" step 4).
type initHarness struct {
	name    string // the --harness value
	display string
	binary  string // detected when on PATH
	folder  string // or when this project folder exists
	// hook is the file holding its pre-tool hook: a JSON settings file Lyceum's
	// entry, with matcher, is merged into, or a shim Lyceum owns.
	hook, matcher, shim string
	trust               string // the trust step its hook needs, if any
	restart             string // the step after init changes its existing hook, if any
}

// initHarnesses are in harness.Names order. Matchers are limited to the
// harness's write and shell tools.
var initHarnesses = []initHarness{
	{name: "claude", display: "Claude Code", binary: "claude", folder: ".claude",
		hook: ".claude/settings.json", matcher: "Write|Edit|MultiEdit|NotebookEdit|Bash"},
	{name: "codex", display: "Codex", binary: "codex", folder: ".codex",
		hook: ".codex/hooks.json", matcher: "Bash|apply_patch",
		trust: "Codex runs project hooks only once you trust them; trust this project in Codex, or the hook silently won't run"},
	{name: "commandcode", display: "Command Code", binary: "commandcode", folder: ".commandcode",
		hook: ".commandcode/settings.json", matcher: "shell|write|edit"},
	{name: "opencode", display: "OpenCode", binary: "opencode", folder: ".opencode",
		hook: ".opencode/plugins/lyceum.js", shim: templates.OpenCodePlugin,
		restart: `restart OpenCode's server ("opencode serve") so it loads the updated hook: a running server keeps the plugin it first loaded for this directory`},
	{name: "pi", display: "Pi", binary: "pi", folder: ".pi",
		hook: ".pi/extensions/lyceum.ts", shim: templates.PiExtension,
		trust: "Pi loads project extensions only in a trusted project; trust this project in Pi, or the hook won't load"},
}

// command is the hook command h's settings file runs.
func (h initHarness) command() string {
	return hookCommandPrefix + " --harness " + h.name
}

// writeHook installs h's pre-tool hook under root: Lyceum's entry merged into
// its settings file, or its shim written.
func writeHook(root string, h initHarness, r *initReport) error {
	content := []byte(h.shim)
	if h.shim == "" {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(h.hook)))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if content, err = mergeHook(data, h.matcher, h.command()); err != nil {
			return out.Refuse("wire "+h.display, h.hook+" is not valid settings JSON: "+err.Error(),
				"fix "+h.hook+", or move it aside, then run \"lyceum init\" again")
		}
	}
	c, err := putFile(root, h.hook, content, 0o644)
	if err != nil {
		return err
	}
	r.add(h.hook, c)
	return nil
}

// wired reports whether init installed h's hook in the repo at root before:
// its shim exists, or its settings file runs Lyceum's hook.
func (h initHarness) wired(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(h.hook)))
	return err == nil && (h.shim != "" || strings.Contains(string(data), hookCommandPrefix))
}

// detected says where h was found in the repo at root, or "" when it wasn't.
func (h initHarness) detected(root string) string {
	if fi, err := os.Stat(filepath.Join(root, h.folder)); err == nil && fi.IsDir() {
		return h.folder + "/"
	}
	if _, err := exec.LookPath(h.binary); err == nil {
		return h.binary + " on PATH"
	}
	return ""
}

// checkHarnessFlags refuses a --harness value that names no v1 harness.
func checkHarnessFlags(names []string) error {
	for _, n := range names {
		if _, ok := harness.Lookup(n); !ok {
			return out.Refuse("set up Lyceum here", fmt.Sprintf("unknown harness %q", n),
				"pass --harness with one of: "+strings.Join(harness.Names, ", "))
		}
	}
	return nil
}

// chooseHarnesses returns the harnesses to wire: those named by --harness and
// those already wired, without asking, and each detected one the user
// accepts, default yes.
func chooseHarnesses(prompt io.Writer, in *bufio.Reader, root string, named []string) []initHarness {
	var wire []initHarness
	for _, h := range initHarnesses {
		if slices.Contains(named, h.name) || h.wired(root) {
			wire = append(wire, h)
			continue
		}
		where := h.detected(root)
		if where != "" && askDefault(prompt, in, fmt.Sprintf("Wire %s (found %s)? [Y/n] ", h.display, where), true) {
			wire = append(wire, h)
		}
	}
	return wire
}

// manifests are the package manifests that, with source beside them, mark a
// brownfield repo (distribution.md, "lyceum init" step 1).
var manifests = []string{
	"package.json", "go.mod", "Cargo.toml", "pyproject.toml", "setup.py", "requirements.txt", "Gemfile", "pom.xml",
	"build.gradle", "build.gradle.kts", "composer.json", "mix.exs", "Package.swift", "deno.json",
}

// lockfiles go with manifests and are not source.
var lockfiles = []string{
	"go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "Cargo.lock", "poetry.lock", "Gemfile.lock", "composer.lock",
}

// brownfield returns the manifests in the repo at root, at any depth, when it
// also holds source: a code file (codepath.IsCode) that is no manifest or
// lockfile and is outside dot files and folders. It returns nil for a
// greenfield repo.
func brownfield(root string) ([]string, error) {
	files, err := gitx.Files(root)
	if err != nil {
		return nil, err
	}
	var found []string
	source := false
	for _, f := range files {
		base := path.Base(f)
		switch {
		case slices.Contains(manifests, base):
			found = append(found, f)
		case slices.Contains(lockfiles, base):
		case codepath.IsCode(f) && !strings.HasPrefix(f, ".") && !strings.Contains(f, "/."):
			source = true
		}
	}
	if !source {
		return nil, nil
	}
	return found, nil
}
