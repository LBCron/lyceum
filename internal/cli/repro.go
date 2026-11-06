package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/evidence"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/guard"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/runner"
	"github.com/oritromax/lyceum/internal/testspec"
	"github.com/spf13/cobra"
)

// reproNext is the next step for a bug at repro.
const reproNext = `write a test that fails because of the bug, commit it, then run "lyceum test --repro <test>"`

// runRepro is `lyceum test --repro <name>` (test-runner.md, "Mechanical gates"): it clears a bug's repro gate test-first. It refuses
// unless the branch's unit is a bug at repro, TESTING.md sets single:, the
// tree is clean and the branch diff against base, resolved as check
// resolves it, touches no code but test files (AC103). It then runs
// single with {name} replaced by the shell-quoted name and records the run,
// with repro set. The gate clears when the run may clear repro
// (evidence.Repro), exited non-zero and its output mentions name (AC104): the
// gate-clear event inlines the run and fingerprints every test file in the
// branch diff. A run that does not clear exits 0 and says why.
func runRepro(cmd *cobra.Command, root, base string, spec *testspec.Spec, name string, args []string) error {
	what := "run the repro test"
	if len(args) > 0 {
		return out.Refuse(what, "--repro runs one named test and takes no subset", `run "lyceum test --repro <test>"`)
	}
	if name == "" {
		return out.Refuse(what, "--repro needs the name of the test that reproduces the bug", `run "lyceum test --repro <test>"`)
	}
	branch, err := gitx.Branch(root)
	if err != nil {
		return err
	}
	p, err := openUnit(root, branch)
	if err != nil {
		return err
	}
	if p == nil {
		return out.Refuse(what, "this branch has no open unit; --repro clears a bug's repro gate",
			`start a bug: lyceum add bug "<title>"`)
	}
	typ, current, class, err := unitGate(p)
	if err != nil {
		return out.Refuse(what, fmt.Sprintf("the plan of unit %s has type %s and gate %s, which do not go together", p.ID, p.Type, p.Gate),
			`run "lyceum check" to see what changed in the plan`)
	}
	if typ != gate.Bug || current != gate.Repro {
		return out.Refuse(what, fmt.Sprintf("unit %s is a %s at gate %s; --repro clears only a bug's repro gate", p.ID, p.Type, current),
			gateNext(class, current))
	}
	if spec.Single == "" {
		return out.Refuse(what, testspec.Path+" sets no single:, the command that runs one named test",
			fmt.Sprintf(`add single: to the frontmatter of %s with %s where the test's name goes (for example: go test ./... -run %s), then run "lyceum approve testing" yourself in a terminal`,
				testspec.Path, testspec.NamePlaceholder, testspec.NamePlaceholder))
	}
	dirty, err := events.TreeChanges(root)
	if err != nil {
		return err
	}
	if len(dirty) > 0 {
		return dirtyRefusal(root, what, "the working tree has uncommitted changes:\n  "+strings.Join(dirty, "\n  "), dirty,
			fmt.Sprintf(`commit first: git add <paths> && git commit -m "<message>", then run "lyceum test --repro %s"`, name))
	}
	tests, code, _, err := branchTests(root, base, spec.TestFiles, what)
	if err != nil {
		return err
	}
	if guard.ReprocodeEnabled && len(code) > 0 {
		return out.Refuse(what, "the bug is not reproduced yet, but the branch changes files that are not tests:\n  "+strings.Join(code, "\n  "),
			"take those changes off this branch, keeping only test files (test_files in "+testspec.Path+`); the fix comes after repro clears`)
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	stdout, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	live := stdout
	if asJSON {
		// stdout holds only the one JSON object.
		live = stderr
	}
	c := runner.Command{Command: strings.ReplaceAll(spec.Single, testspec.NamePlaceholder, shellQuote(name)), Repro: name, Timeout: spec.Timeout}
	rec, err := runner.Run(root, c, live, stderr)
	if err != nil {
		return err
	}
	summary, err := runSummary(root, spec.Timeout, rec)
	if err != nil {
		return err
	}
	_, reasons, err := evidence.Repro(root, rec)
	if err != nil {
		return err
	}
	output, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(runPath(rec))+".out"))
	if err != nil {
		return err
	}

	res := out.Result{Unit: p.ID, Gate: p.Gate, Next: reproNext, Data: rec}
	var line string
	switch {
	case len(reasons) > 0:
		for _, r := range reasons {
			line += "This run can't clear a gate: " + r.Why + "\n"
		}
		res.Next = reasons[0].Next
		for _, r := range reasons {
			if r.Testing {
				res.Next = r.Next
			}
		}
	case rec.Exit == 0:
		line = fmt.Sprintf("Not cleared: %s: test %s passed, so it does not reproduce the bug; a repro test must fail before the fix exists\n", gate.Repro, name)
		res.Next = fmt.Sprintf(`make %s fail because of the bug, commit it, then run "lyceum test --repro %s"`, name, name)
	case !bytes.Contains(output, []byte(name)):
		line = fmt.Sprintf("Not cleared: %s: the run failed, but its output does not mention %s, so it may not have run that test\n", gate.Repro, name)
		res.Next = fmt.Sprintf(`check that %s is the test's exact name and that single: in %s runs the test named %s, then run "lyceum test --repro %s"`,
			name, testspec.Path, testspec.NamePlaceholder, name)
	default:
		fingerprint, err := fingerprint(root, tests)
		if err != nil {
			return err
		}
		if line, err = clearGate(root, p, typ, gate.Repro, map[string]any{"run": rec, "fingerprint": fingerprint}, &res); err != nil {
			return err
		}
	}
	if !asJSON {
		_, _ = io.WriteString(stdout, summary+line)
	}
	out.Writer{Stdout: stdout, Stderr: stderr, JSON: asJSON}.Result(res)
	return nil
}

// fingerprint is the sha256 of each of files under root, by path. A
// file the branch deleted has none.
func fingerprint(root string, files []string) (map[string]string, error) {
	fp := map[string]string{}
	for _, f := range files {
		h, err := approval.Hash(filepath.Join(root, filepath.FromSlash(f)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		fp[f] = h
	}
	return fp, nil
}

// shellQuote quotes s as one sh word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
