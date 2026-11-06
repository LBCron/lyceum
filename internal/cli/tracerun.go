package cli

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/runner"
	"github.com/oritromax/lyceum/internal/testhooks"
	"github.com/oritromax/lyceum/internal/testspec"
	"github.com/spf13/cobra"
)

// traceRunsDir holds the logs of trace full runs, one folder per run. It is
// under the gitignored runtime dir .lyceum/.
const traceRunsDir = ".lyceum/traces"

// traceSteps are the steps of a full run, in the order they run: an
// environment's up and seed, the trigger, then its reset and down.
var traceSteps = []string{"up", "seed", "trigger", "reset", "down"}

// cleanupSteps run even when an earlier step failed.
var cleanupSteps = []string{"reset", "down"}

// traceStep is one command of a full run and, once run, how it ended.
type traceStep struct {
	Step    string `json:"step"`
	Command string `json:"command"`
	// Skipped is set when the step did not run because up or seed failed.
	Skipped      bool   `json:"skipped,omitempty"`
	Exit         int    `json:"exit"`
	TimedOut     bool   `json:"timed_out"`
	DurationMS   int64  `json:"duration_ms"`
	Output       string `json:"output,omitempty"` // the output file, relative to the repo root
	OutputSHA256 string `json:"output_sha256,omitempty"`
}

// traceRun is the run log of a full run, <traceRunsDir>/<ts>-<rand>/run.json.
type traceRun struct {
	TS            string      `json:"ts"`
	Question      string      `json:"question"`
	Env           string      `json:"env"`
	URL           string      `json:"url,omitempty"`
	Commit        string      `json:"commit"`
	Dirty         bool        `json:"dirty"`
	TestingSHA256 string      `json:"testing_sha256"`
	Steps         []traceStep `json:"steps"`
}

// runTraceRun is `lyceum trace --run <env> --trigger "<cmd>" [--yes]
// "<question>"`, a full run (records.md, "lyceum trace", step 4).
// The environment must be declared in the approved TESTING.md; with none, or
// an unknown one, it refuses and offers code-only (AC77). Without --yes it
// prints the commands it would run and runs nothing (AC134). With --yes it
// runs up, seed, the trigger, reset and down, whichever exist, in that order,
// reset and down even when an earlier one failed, and records every command
// in a run log under .lyceum/, then prints a block for the trace record
// (AC78).
func runTraceRun(cmd *cobra.Command, args []string, env string) error {
	what := "run the trace"
	codeOnly := `or trace code-only: lyceum trace "<question>", then read the flow and mark each step [read]`
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return out.Refuse(what, "a full run takes one argument, the question it answers",
			fmt.Sprintf(`run lyceum trace --run %s --trigger "<command>" "<question>"`, env))
	}
	question := strings.TrimSpace(args[0])
	root, _, err := setUpRoot(what)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(filepath.Join(root, testspec.Path))
	if errors.Is(err, fs.ErrNotExist) {
		return out.Refuse(what, "a full run needs an environment declared in an approved "+testspec.Path+", and it does not exist",
			"the agent drafts "+testspec.Path+" with environments:, then run \"lyceum approve testing\" yourself in a terminal", codeOnly)
	} else if err != nil {
		return err
	}
	spec, err := testspec.Parse(data)
	if err != nil {
		return out.Refuse(what, err.Error(),
			`fix the frontmatter of `+testspec.Path+`, then run "lyceum approve testing" yourself in a terminal`, codeOnly)
	}
	approved, changed, err := testspec.Approved(root)
	if err != nil {
		return err
	}
	switch {
	case !approved:
		return out.Refuse(what, testspec.Path+" is not approved, so its environments can't be run",
			`run "lyceum approve testing" yourself in a terminal`, codeOnly)
	case changed:
		return out.Refuse(what, testspec.Path+" changed after it was approved, so its environments can't be run",
			`run "lyceum approve testing" yourself in a terminal`, codeOnly)
	}
	vars, ok := spec.Environments[env]
	if !ok {
		names := make([]string, 0, len(spec.Environments))
		for name := range spec.Environments {
			names = append(names, name)
		}
		slices.Sort(names)
		reason := testspec.Path + " defines no environments, so there is nothing to run the trace against"
		next := []string{codeOnly, "or the agent adds an environment under environments: in " + testspec.Path + `, then run "lyceum approve testing" yourself in a terminal`}
		if len(names) > 0 {
			reason = fmt.Sprintf("%s defines no environment %q; the defined environments are: %s", testspec.Path, env, strings.Join(names, ", "))
			next = append([]string{fmt.Sprintf(`run lyceum trace --run <env> with one of: %s`, strings.Join(names, ", "))}, next...)
		}
		return out.Refuse(what, reason, next...)
	}
	trigger, _ := cmd.Flags().GetString("trigger")
	if strings.TrimSpace(trigger) == "" {
		return out.Refuse(what, "a full run needs the command that triggers the flow: --trigger is missing",
			fmt.Sprintf(`run lyceum trace --run %s --trigger "<command>" %q`, env, question))
	}

	var steps []traceStep
	for _, s := range traceSteps {
		c := vars[s]
		if s == "trigger" {
			c = trigger
		}
		if strings.TrimSpace(c) != "" {
			steps = append(steps, traceStep{Step: s, Command: c})
		}
	}
	yes, _ := cmd.Flags().GetBool("yes")
	asJSON, _ := cmd.Flags().GetBool("json")
	stdout, stderr := cmd.OutOrStdout(), cmd.ErrOrStderr()
	if !yes {
		if !asJSON {
			_, _ = io.WriteString(stdout, tracePlanText(env, vars["url"], steps))
		}
		again := fmt.Sprintf("lyceum trace --run %s --trigger %s --yes %s", env, shellQuote(trigger), shellQuote(question))
		out.Writer{Stdout: stdout, Stderr: stderr, JSON: asJSON}.Result(out.Result{
			Next: "tell the user these commands and their side effects and ask; when they agree, run: " + again,
			Data: struct {
				Env   string      `json:"env"`
				URL   string      `json:"url"`
				Steps []traceStep `json:"steps"`
				Ran   bool        `json:"ran"`
			}{env, vars["url"], steps, false},
		})
		return nil
	}

	commit, err := gitx.HeadSHA(root)
	if err != nil {
		return err
	}
	dirty, err := gitx.Status(root)
	if err != nil {
		return err
	}
	testing, err := approval.Hash(filepath.Join(root, testspec.Path))
	if err != nil {
		return err
	}
	ts := testhooks.Now().Truncate(time.Second)
	dir, err := newTraceRunDir(root, ts)
	if err != nil {
		return err
	}
	live := stdout
	if asJSON {
		// stdout holds only the one JSON object.
		live = stderr
	}
	failed := false
	for i := range steps {
		s := &steps[i]
		if failed && !slices.Contains(cleanupSteps, s.Step) {
			s.Skipped = true
			continue
		}
		s.Output = path.Join(dir, s.Step+".out")
		f, err := os.Create(filepath.Join(root, filepath.FromSlash(s.Output)))
		if err != nil {
			return err
		}
		o, err := runner.Exec(root, s.Command, spec.Timeout, live, stderr, f)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		if s.OutputSHA256, err = approval.Hash(filepath.Join(root, filepath.FromSlash(s.Output))); err != nil {
			return err
		}
		s.Exit, s.TimedOut, s.DurationMS = o.Exit, o.TimedOut, o.Duration.Milliseconds()
		if s.Exit != 0 && (s.Step == "up" || s.Step == "seed") {
			failed = true
		}
	}
	run := traceRun{TS: ts.Format(time.RFC3339), Question: question, Env: env, URL: vars["url"], Commit: commit,
		Dirty: len(dirty) > 0, TestingSHA256: testing, Steps: steps}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(run); err != nil {
		return err
	}
	log := path.Join(dir, "run.json")
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(log)), b.Bytes(), 0o644); err != nil {
		return err
	}

	if !asJSON {
		_, _ = io.WriteString(stdout, "Recorded: "+log+"\n"+
			"Paste this into the trace record, then mark each step [observed] where this run shows it, [read] otherwise:\n\n"+
			traceRunBlock(run, log)+"\n")
	}
	out.Writer{Stdout: stdout, Stderr: stderr, JSON: asJSON}.Result(out.Result{
		Next: `write the trace in lyceum/trace/ with the run block, each step under ## Steps marked [read] or [observed], and a Mermaid sequenceDiagram, then run "lyceum record lyceum/trace/<file>.md"`,
		Data: struct {
			Log string `json:"log"`
			traceRun
		}{log, run},
	})
	return nil
}

// tracePlanText is what a full run without --yes prints: the commands it
// would run, in order, and that none ran.
func tracePlanText(env, url string, steps []traceStep) string {
	var b strings.Builder
	at := ""
	if url != "" {
		at = " (" + url + ")"
	}
	fmt.Fprintf(&b, "A full run against environment %s%s will run, in order:\n", env, at)
	for _, s := range steps {
		fmt.Fprintf(&b, "  %s: %s\n", s.Step, s.Command)
	}
	b.WriteString("Side effects: whatever these commands change in " + env + "; reset and down run even if an earlier command fails.\n")
	b.WriteString("Nothing ran: add --yes to run them.\n")
	return b.String()
}

// traceRunBlock is the block the agent pastes into the trace record: every
// command run, its exit code, duration and output file.
func traceRunBlock(run traceRun, log string) string {
	var b strings.Builder
	b.WriteString("## Run\n\n")
	fmt.Fprintf(&b, "Environment %s, run %s at commit %s; log %s\n\n", run.Env, run.TS, run.Commit, log)
	for _, s := range run.Steps {
		if s.Skipped {
			fmt.Fprintf(&b, "- %s: `%s`: skipped, an earlier step failed\n", s.Step, s.Command)
			continue
		}
		timedOut := ""
		if s.TimedOut {
			timedOut = ", timed out"
		}
		fmt.Fprintf(&b, "- %s: `%s`: exit %d%s, %d ms, output %s\n", s.Step, s.Command, s.Exit, timedOut, s.DurationMS, s.Output)
	}
	return b.String()
}

// newTraceRunDir makes a new folder for a full run's log under
// traceRunsDir, named <ts>-<rand>, and returns it relative to root.
func newTraceRunDir(root string, ts time.Time) (string, error) {
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(traceRunsDir)), 0o755); err != nil {
		return "", err
	}
	// Two runs in the same second get different names; a collision draws again.
	for range 10 {
		r := make([]byte, 4)
		if _, err := io.ReadFull(testhooks.Rand(), r); err != nil {
			return "", err
		}
		dir := path.Join(traceRunsDir, ts.Format("20060102T150405Z")+"-"+hex.EncodeToString(r))
		err := os.Mkdir(filepath.Join(root, filepath.FromSlash(dir)), 0o755)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return dir, err
	}
	return "", fmt.Errorf("no free run name in %s", traceRunsDir)
}
