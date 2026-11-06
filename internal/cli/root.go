// Package cli builds the lyceum command tree.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/oritromax/lyceum/internal/harness"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/spf13/cobra"
)

// version is set at release with -ldflags "-X github.com/oritromax/lyceum/internal/cli.version=<v>".
var version = "dev"

// Run executes lyceum with args and returns the process exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if args == nil {
		// cobra falls back to os.Args when args is nil.
		args = []string{}
	}
	root := newRoot()
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.Execute()
	if err == nil {
		return out.ExitOK
	}
	json, _ := root.PersistentFlags().GetBool("json")
	if !root.PersistentFlags().Changed("json") {
		// A bad flag stops parsing, so a --json after it was never read.
		json = jsonArg(args)
	}
	return out.Writer{Stdout: stdout, Stderr: stderr, JSON: json}.Fail(err)
}

// jsonArg reports whether args ask for --json before any "--".
func jsonArg(args []string) bool {
	for _, a := range args {
		switch a {
		case "--":
			return false
		case "--json", "--json=true":
			return true
		}
	}
	return false
}

// seeCommands is the next step after an unknown command or a bad flag.
const seeCommands = `run "lyceum" to see the commands`

// unknownCommand refuses any argument to the root: the root takes none, so
// the first one is a command lyceum doesn't have.
func unknownCommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return out.Refuse("run "+cmd.CommandPath()+" "+args[0], fmt.Sprintf("unknown command %q", args[0]), seeCommands)
}

// badFlag turns a flag parse error on any command into a refusal.
func badFlag(cmd *cobra.Command, err error) error {
	return out.Refuse("run "+cmd.CommandPath(), err.Error(), seeCommands)
}

// versionText renders --version: "lyceum <v>", or {"version":"<v>"} with --json.
func versionText(cmd *cobra.Command) (string, error) {
	if asJSON, _ := cmd.Flags().GetBool("json"); !asJSON {
		return "lyceum " + cmd.Version + "\n", nil
	}
	b, err := json.Marshal(struct {
		Version string `json:"version"`
	}{cmd.Version})
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

// notBuilt is the RunE of every command that does not exist yet.
func notBuilt(cmd *cobra.Command, _ []string) error {
	return out.Refuse("run "+cmd.CommandPath(), "not built yet in this version",
		`run "lyceum" to see the available commands`)
}

func newRoot() *cobra.Command {
	// The menu lists commands in commands.md order.
	cobra.EnableCommandSorting = false

	root := &cobra.Command{
		Use:           "lyceum",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		// Bare lyceum prints the menu; any argument is an unknown command.
		Args: unknownCommand,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cobra.AddTemplateFunc("versionText", versionText)
	root.SetVersionTemplate("{{versionText .}}")
	root.SetFlagErrorFunc(badFlag)
	root.PersistentFlags().Bool("json", false, "Print the outcome as one JSON object on stdout")

	root.AddGroup(
		&cobra.Group{ID: "lifecycle", Title: "Lifecycle:"},
		&cobra.Group{ID: "change", Title: "Change:"},
		&cobra.Group{ID: "inspect", Title: "Inspect:"},
	)

	// Lifecycle
	initCmd := stub("lifecycle", "init", "Set up Lyceum in a repo; greenfield interview or brownfield baseline")
	initCmd.RunE = runInit
	initCmd.Flags().StringArray("harness", nil, "Wire this harness without asking (repeatable): "+strings.Join(harness.Names, ", "))
	approve := stub("lifecycle", "approve [gate]", "Owner-only approval from a real terminal; bare form walks every pending gate")
	approve.RunE = runApprove
	approve.Flags().String("no-repro", "", `With "approve cause", while a bug is at repro: skip repro for a bug that can't have a repro test, recording the reason`)
	status := stub("lifecycle", "status [--json]", "Every unit on this branch and others, its gate and the next action; pending approvals; stale records")
	status.RunE = runStatus
	status.Flags().Bool("all", false, "Show closed units too")
	check := stub("lifecycle", "check [--ci]", "Integrity and gate checks; warns locally, fails in CI")
	check.RunE = runCheck
	check.Flags().Bool("ci", false, "Fail on any failing finding and write no events")
	check.Flags().Bool("pre-commit", false, "Run as the commit hook: also record failing findings as bypass events")
	_ = check.Flags().MarkHidden("pre-commit")
	test := stub("lifecycle", "test [subset]", "Run the suite or a subset and record the run")
	test.RunE = runTest
	test.Flags().String("repro", "", "Run one named test with TESTING.md single: to clear a bug's repro gate; it must fail")
	plan := stub("lifecycle", "plan [--new-area <name>]", "Show the plan, add an area, or upgrade a trivial chore")
	plan.RunE = runPlan
	plan.Flags().String("new-area", "", "Register a new spec area: list the registered ones, add its registry row and create its area file")
	amend := stub("lifecycle", `amend [unit] "<reason>"`, "Reopen an approved plan mid-build")
	amend.RunE = runAmend
	amend.Flags().Bool("intent", false, "On a refactor: go back to intent, not plan, when the intent itself changed")
	closeCmd := stub("lifecycle", "close [unit]", "Fold the spec delta, close the unit, open a PR")
	closeCmd.RunE = runClose
	logCmd := stub("lifecycle", "log", "Project history")
	logCmd.RunE = runLog
	logCmd.Flags().String("unit", "", "Only this unit's events and the commits on its branch")
	logCmd.Flags().String("since", "", "Only entries from this date (YYYY-MM-DD, UTC) on")
	logCmd.Flags().String("type", "", "Only events of this type, and no commits")
	eject := stub("lifecycle", "eject", "Remove Lyceum's installed adapters, keep the records")
	eject.RunE = runEject
	doctor := stub("lifecycle", "doctor [--json]", "Check the setup: the tool, and the repo's Lyceum integration")
	doctor.RunE = runDoctor
	root.AddCommand(
		initCmd,
		status,
		check,
		approve,
		plan,
		amend,
		closeCmd,
		test,
		logCmd,
		eject,
		doctor,
	)

	// Change
	add := stub("change", "add [feat|bug|doc|test] [title]", "Start a unit of work: feat, bug, doc or test")
	add.RunE = runAddBare
	for _, t := range []string{"feat", "bug", "doc", "test"} {
		c := stub("", t+" [title]", "")
		c.RunE = runAdd
		add.AddCommand(c)
	}
	refactor := stub("change", "refactor [title]", "Change structure without changing behaviour")
	refactor.RunE = runAdd
	chore := stub("change", "chore [title]", "Maintenance: dependencies, config, README")
	chore.RunE = runAdd
	root.AddCommand(add, refactor, chore)

	// Inspect
	understand := stub("inspect", "understand [hint]", "Explain the project or part of it")
	understand.RunE = runUnderstand
	trace := stub("inspect", "trace [question]", "Follow one flow end to end")
	trace.RunE = runTrace
	trace.Flags().String("run", "", "Full run against this TESTING.md environment: up, seed, the trigger, reset, down")
	trace.Flags().String("trigger", "", "With --run: the command that triggers the flow")
	trace.Flags().Bool("yes", false, "With --run: run the commands; without it they are only printed")
	audit := stub("inspect", "audit", "Report vulnerable or outdated dependencies")
	audit.RunE = runAudit
	export := stub("inspect", "export <record> [path]", "Publish a record to docs/")
	export.RunE = runExport
	root.AddCommand(
		understand,
		trace,
		audit,
		export,
	)

	// Internal: used by skills, hooks and CI, not listed in the menu.
	changedFiles := stub("", "changed-files", "Files changed on this branch, for TESTING.md subset commands")
	changedFiles.RunE = runChangedFiles
	hook := stub("", "hook <event>", "Harness hook entry point")
	hook.RunE = runHook
	preTool := stub("", "pre-tool --harness <name>", "Read a harness's pre-tool payload on stdin and answer allow or deny")
	preTool.RunE = runHookPreTool
	preTool.Flags().String("harness", "", "The harness sending the payload: "+strings.Join(harness.Names, ", "))
	hook.AddCommand(preTool)
	record := stub("", "record <file>", "Fill in a record's id, date and cite hashes, validate it, and commit it")
	record.RunE = runRecord
	for _, c := range []*cobra.Command{
		stub("", "gate <unit> <gate>", "Move a unit to a gate the CLI allows; refuses illegal transitions"),
		hook,
		changedFiles,
		record,
	} {
		c.Hidden = true
		root.AddCommand(c)
	}

	// No help command: cobra lists any command named "help" in the menu, so
	// its default is replaced with an unnamed hidden one. --help still works.
	// The group keeps cobra from printing an empty "Additional Commands:".
	root.SetHelpCommand(&cobra.Command{Hidden: true, GroupID: "lifecycle"})

	return root
}

func stub(group, use, short string) *cobra.Command {
	return &cobra.Command{
		Use:     use,
		Short:   short,
		GroupID: group,
		RunE:    notBuilt,
	}
}
