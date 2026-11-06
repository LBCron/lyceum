package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/templates"
	"github.com/oritromax/lyceum/internal/testhooks"
	"github.com/oritromax/lyceum/internal/testspec"
	"github.com/spf13/cobra"
)

// actionPublished is set at release, with -ldflags, to "true" once the CI
// Action is published.
var actionPublished = "false"

// ciPublished reports whether this build's CI Action can be resolved.
func ciPublished() bool {
	return testhooks.ActionPublished(actionPublished == "true")
}

// The states of `lyceum doctor`.
const (
	doctorNoRepo   = "no-repo"
	doctorNotSetUp = "not-set-up"
	doctorSetUp    = "set-up"
)

// doctorFinding is one thing `lyceum doctor` found wrong with the setup.
type doctorFinding struct {
	Check   string `json:"check"`
	Message string `json:"message"`
	Next    string `json:"next"`
}

// runDoctor is `lyceum doctor` (commands.md, "lyceum doctor"). It checks setup, never work: the tool everywhere, and in a Lyceum
// repo the integration. It changes nothing, except that an interactive run in
// a repo that is not set up offers to start `lyceum init`. Findings are a
// report, not a refusal, so it exits 0.
func runDoctor(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return out.Refuse("check the setup", "doctor takes no arguments", `run "lyceum doctor"`)
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	findings := []doctorFinding{}
	if err := gitx.CheckVersion(); err != nil {
		findings = append(findings, doctorFinding{"git", err.Error(), "install git 2.30 or newer"})
	}
	state, next := doctorNoRepo, `run "lyceum init" in your project`
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	if gitx.IsRepo(dir) {
		root, err := gitx.Root(dir)
		if err != nil {
			return err
		}
		_, err = config.Read(root)
		if errors.Is(err, fs.ErrNotExist) {
			state = doctorNotSetUp
			findings = append(findings, doctorFinding{"setup", "Lyceum is not set up in this repo", `run "lyceum init"`})
		} else {
			state, next = doctorSetUp, `run "lyceum status"`
			if err != nil {
				findings = append(findings, doctorFinding{"config", err.Error(), "fix " + config.Path})
			}
			more, err := doctorRepo(root)
			if err != nil {
				return err
			}
			findings = append(findings, more...)
		}
	}
	if !asJSON {
		var b strings.Builder
		fmt.Fprintf(&b, "State: %s\n", state)
		if len(findings) == 0 {
			b.WriteString("No findings\n")
		}
		for _, f := range findings {
			fmt.Fprintf(&b, "%s: %s\n  to fix: %s\n", f.Check, f.Message, f.Next)
		}
		_, _ = io.WriteString(cmd.OutOrStdout(), b.String())
	}
	if len(findings) > 0 {
		next = findings[0].Next
	}
	if state == doctorNotSetUp && !asJSON && isTerminal(cmd.InOrStdin()) && isTerminal(cmd.OutOrStdout()) {
		if ask(cmd.ErrOrStderr(), bufio.NewReader(cmd.InOrStdin()), `Run "lyceum init" now? [y/N] `) {
			self, err := os.Executable()
			if err != nil {
				return err
			}
			init := exec.Command(self, "init")
			init.Stdin, init.Stdout, init.Stderr = cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
			return init.Run()
		}
	}
	out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(out.Result{
		Next:  next,
		Extra: map[string]any{"state": state, "findings": findings},
	})
	return nil
}

// doctorRepo checks the Lyceum integration of the repo at root.
func doctorRepo(root string) ([]doctorFinding, error) {
	var findings []doctorFinding
	dirs := []string{".agents/skills"}
	for _, h := range initHarnesses {
		if _, err := os.Stat(filepath.Join(root, h.folder)); err != nil {
			continue
		}
		if h.wired(root) {
			if h.name == "claude" {
				dirs = append(dirs, ".claude/skills")
			}
			continue
		}
		findings = append(findings, doctorFinding{"hook", fmt.Sprintf("%s has no Lyceum hook: %s", h.display, h.hook),
			`run "lyceum init --harness ` + h.name + `"`})
	}
	for _, d := range dirs {
		for _, name := range templates.SkillNames {
			rel := d + "/lyceum-" + name + "/SKILL.md"
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			switch {
			case errors.Is(err, fs.ErrNotExist):
				findings = append(findings, doctorFinding{"skill", "skill lyceum-" + name + " is missing: " + rel, `run "lyceum init"`})
			case err != nil:
				return nil, err
			case string(data) != templates.Skill(name):
				findings = append(findings, doctorFinding{"skill", "skill lyceum-" + name + " differs from this binary's copy: " + rel, `run "lyceum init"`})
			}
		}
	}
	hook, err := gitx.GitPath(root, "hooks/pre-commit")
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(hook) {
		hook = filepath.Join(root, hook)
	}
	data, err := os.ReadFile(hook)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if !hasBlock(string(data)) {
		findings = append(findings, doctorFinding{"pre-commit", "the pre-commit hook does not run lyceum check", `run "lyceum init"`})
	} else if !strings.Contains(string(data), templates.PreCommit) {
		findings = append(findings, doctorFinding{"pre-commit", "the pre-commit hook's Lyceum block is out of date, so commits past a check are not recorded", `run "lyceum init"`})
	}
	missing, err := config.MissingHarnessIgnores(root)
	if err != nil {
		return nil, err
	}
	if len(missing) > 0 {
		findings = append(findings, doctorFinding{"gitignore", ".gitignore does not ignore what harnesses write on their own: " + strings.Join(missing, ", "), `run "lyceum init"`})
	}
	if _, err := testspec.Read(root); err != nil && !errors.Is(err, fs.ErrNotExist) {
		findings = append(findings, doctorFinding{"testing", err.Error(), "fix " + testspec.Path})
	}
	workflow, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(workflowPath)))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if strings.Contains(string(workflow), "lyceum/check-action") && !ciPublished() {
		findings = append(findings, doctorFinding{"ci", workflowPath + " uses a CI Action this build can't resolve: it is not published yet",
			"remove " + workflowPath + " until the Action is published"})
	}
	return findings, nil
}
