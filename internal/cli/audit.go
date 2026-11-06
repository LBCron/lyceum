package cli

import (
	"bytes"
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

	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/records"
	"github.com/oritromax/lyceum/internal/runner"
	"github.com/oritromax/lyceum/internal/testhooks"
	"github.com/oritromax/lyceum/internal/testspec"
	"github.com/oritromax/lyceum/internal/unit"
	"github.com/spf13/cobra"
)

// auditTool is an ecosystem's standard audit tool (records.md, "lyceum
// audit"): the manifests that detect the ecosystem at the repo root,
// the command run, the binaries it needs and how to install them.
type auditTool struct {
	Name      string
	Manifests []string
	Command   string
	Binaries  []string
	Install   string
}

// auditTools are the ecosystems audit detects, in the order it runs them.
var auditTools = []auditTool{
	{"npm", []string{"package.json"}, "npm audit --json", []string{"npm"}, "install Node.js, which includes npm: https://nodejs.org/en/download"},
	{"govulncheck", []string{"go.mod"}, "govulncheck ./...", []string{"govulncheck"}, "go install golang.org/x/vuln/cmd/govulncheck@latest"},
	{"pip-audit", []string{"requirements.txt", "pyproject.toml"}, "pip-audit -r requirements.txt", []string{"pip-audit"}, "pip install pip-audit"},
	{"cargo-audit", []string{"Cargo.toml"}, "cargo audit", []string{"cargo", "cargo-audit"}, "cargo install cargo-audit"},
	{"composer", []string{"composer.json"}, "composer audit", []string{"composer"}, "install Composer: https://getcomposer.org/download/"},
	{"bundler-audit", []string{"Gemfile"}, "bundle audit", []string{"bundle", "bundler-audit"}, "gem install bundler-audit"},
}

// runAudit is `lyceum audit` (records.md, "lyceum audit"). It detects each ecosystem by its manifests at the repo root, runs
// that ecosystem's audit tool there, and writes lyceum/audit/<date>-<hash>.md:
// each tool's command, exit code and raw output, parsing nothing, with an
// empty ## Summary for the agent; the record cites the manifests. It commits
// the record through the record path. A missing tool is a refusal naming how
// to install it, before anything runs or is written (AC135). It never
// changes a manifest, a lockfile or any code.
func runAudit(cmd *cobra.Command, args []string) error {
	what := "audit"
	if len(args) > 0 {
		return out.Refuse(what, "audit takes no arguments", `run "lyceum audit"`)
	}
	root, _, err := setUpRoot(what)
	if err != nil {
		return err
	}

	type found struct {
		tool      auditTool
		manifests []string
		command   string
	}
	var detected []found
	for _, t := range auditTools {
		f := found{tool: t}
		for _, m := range t.Manifests {
			if fi, err := os.Stat(filepath.Join(root, m)); err == nil && !fi.IsDir() {
				f.manifests = append(f.manifests, m)
			} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
		if len(f.manifests) > 0 {
			f.command = t.Command
			// pip-audit audits the project, never the installed environment a
			// bare run would: requirements.txt if present, else pyproject.toml.
			if t.Name == "pip-audit" && !slices.Contains(f.manifests, "requirements.txt") {
				f.command = "pip-audit ."
			}
			detected = append(detected, f)
		}
	}
	if len(detected) == 0 {
		var names []string
		for _, t := range auditTools {
			names = append(names, t.Manifests...)
		}
		return out.Refuse(what, "no manifest audit knows is at the repo root ("+strings.Join(names, ", ")+"), so there is nothing to audit",
			`run "lyceum audit" in a repo with one of them at its root`)
	}
	for _, f := range detected {
		for _, b := range f.tool.Binaries {
			if _, err := exec.LookPath(b); err != nil {
				return out.Refuse(what, fmt.Sprintf("%s found %s, but %s is not installed (%s is not on PATH)", what, strings.Join(f.manifests, " and "), f.tool.Name, b),
					fmt.Sprintf("ask the user to install it, or to allow you to: %s", f.tool.Install),
					`then run "lyceum audit" again`)
			}
		}
	}
	if staged, err := gitx.Staged(root); err != nil {
		return err
	} else if len(staged) > 0 {
		return out.Refuse(what, "files are staged, and audit commits only its record:\n  "+strings.Join(staged, "\n  "),
			"unstage them: git restore --staged -- "+strings.Join(staged, " ")+`, then run "lyceum audit" again`)
	}

	now := testhooks.Now()
	date := now.Format("2006-01-02")
	taken, err := recordHashes(root)
	if err != nil {
		return err
	}
	auditFiles, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(records.Dir(records.Audit))))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, de := range auditFiles {
		taken[strings.TrimSuffix(de.Name()[strings.LastIndex(de.Name(), "-")+1:], ".md")] = true
	}
	hash := unit.NewHash()
	for taken[hash] {
		hash = unit.NewHash()
	}
	title := "Dependency audit " + date
	slug, err := unit.Slug(title)
	if err != nil {
		return err
	}
	rel := path.Join(records.Dir(records.Audit), date+"-"+hash+".md")

	var front, body strings.Builder
	fmt.Fprintf(&front, "---\nid: %s\nkind: audit\ntitle: %s\ncites:\n", unit.ID(hash, slug), title)
	fmt.Fprintf(&body, "# %s\n\nThe raw output of each ecosystem's audit tool, run by `lyceum audit` at the repo root. Nothing in it is parsed.\n", title)
	type ranTool struct {
		Tool    string `json:"tool"`
		Command string `json:"command"`
		Exit    int    `json:"exit"`
	}
	var ran []ranTool
	var summary []string
	for _, f := range detected {
		for _, m := range f.manifests {
			fmt.Fprintf(&front, "  - path: %s\n", m)
		}
		var capture bytes.Buffer
		o, err := runner.Exec(root, f.command, testspec.DefaultTimeout, io.Discard, io.Discard, &capture)
		if err != nil {
			return err
		}
		timedOut := ""
		if o.TimedOut {
			timedOut = fmt.Sprintf(" (killed after %s)", testspec.DefaultTimeout)
		}
		fence := strings.Repeat("`", max(3, longestRun(capture.String(), '`')+1))
		output := capture.String()
		if output != "" && !strings.HasSuffix(output, "\n") {
			output += "\n"
		}
		fmt.Fprintf(&body, "\n## %s\n\nManifests: %s\nCommand: `%s`\nExit: %d%s\n\n%stext\n%s%s\n",
			f.tool.Name, strings.Join(f.manifests, ", "), f.command, o.Exit, timedOut, fence, output, fence)
		ran = append(ran, ranTool{f.tool.Name, f.command, o.Exit})
		summary = append(summary, fmt.Sprintf("%s exited %d", f.command, o.Exit))
	}
	body.WriteString("\n## Summary\n\n")
	front.WriteString("---\n")

	dir := filepath.Join(root, filepath.FromSlash(records.Dir(records.Audit)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.WriteFile(file, []byte(front.String()+body.String()), 0o644); err != nil {
		return err
	}
	if _, _, _, err := recordFile(root, file, rel, records.Audit); err != nil {
		return err
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	if !asJSON {
		_, _ = io.WriteString(cmd.OutOrStdout(), "Ran: "+strings.Join(summary, "; ")+"\nRecorded "+rel+"\n")
	}
	out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(out.Result{
		Next: fmt.Sprintf(`read the raw output in %s, write the summary under its ## Summary, then run "lyceum record %s"; each finding the owner acts on becomes a chore (upgrade) or "lyceum add bug" (defect)`, rel, rel),
		Data: struct {
			Record string    `json:"record"`
			Ran    []ranTool `json:"ran"`
		}{rel, ran},
	})
	return nil
}

// longestRun is the length of the longest run of c in s.
func longestRun(s string, c byte) int {
	longest, n := 0, 0
	for i := range len(s) {
		if s[i] == c {
			n++
			longest = max(longest, n)
		} else {
			n = 0
		}
	}
	return longest
}
