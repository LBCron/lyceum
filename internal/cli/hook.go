package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/check"
	"github.com/oritromax/lyceum/internal/config"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/guard"
	"github.com/oritromax/lyceum/internal/harness"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/oritromax/lyceum/internal/unit"
	"github.com/spf13/cobra"
)

// runHook is bare `lyceum hook`, or `lyceum hook <event>` with an event it lacks.
func runHook(cmd *cobra.Command, args []string) error {
	if err := unknownCommand(cmd, args); err != nil {
		return err
	}
	return out.Refuse("run "+cmd.CommandPath(), "hook needs an event", `run "lyceum hook pre-tool --harness <name>"`)
}

// runHookPreTool is `lyceum hook pre-tool --harness <name>` (enforcement.md,
// "Harness hooks"): it reads the harness's pre-tool payload on
// stdin and answers allow or deny in that harness's format, exit 0. A payload
// it can't parse is allowed with a warning on stderr and in the hook warning
// file, which the next payload it parses removes; an unknown harness is
// a refusal.
func runHookPreTool(cmd *cobra.Command, args []string) error {
	what := "run the pre-tool hook"
	if len(args) > 0 {
		return out.Refuse(what, "pre-tool takes no arguments", `run "lyceum hook pre-tool --harness <name>"`)
	}
	name, _ := cmd.Flags().GetString("harness")
	a, ok := harness.Lookup(name)
	if !ok {
		reason := fmt.Sprintf("unknown harness %q", name)
		if name == "" {
			reason = "--harness is missing"
		}
		return out.Refuse(what, reason, "pass --harness with one of: "+strings.Join(harness.Names, ", "))
	}
	payload, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return err
	}
	answer := a.Allow()
	call, err := a.Parse(payload)
	if err != nil {
		// Hooks are friction: a payload Lyceum can't read is let through.
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "lyceum hook: allowed a tool call it could not read: %v\n", err)
		if err := recordWarning(a.Name, err); err != nil {
			return err
		}
	} else if err := clearWarning(); err != nil {
		return err
	} else if guard.HookEnabled {
		reason, err := hookDeny(call)
		if err != nil {
			return err
		}
		if reason != "" {
			answer = a.Deny(reason)
			if ask := promptRoute(a, call); ask != "" {
				answer = a.Ask(ask)
			}
		}
	}
	_, err = io.WriteString(cmd.OutOrStdout(), answer)
	return err
}

// recordWarning writes the hook warning for harness, failing with cause, in
// the repo holding the working directory, when Lyceum is set up there.
func recordWarning(harnessName string, cause error) error {
	root, err := hookRoot()
	if err != nil || root == "" {
		return err
	}
	if _, err := config.Read(root); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return harness.WriteWarning(root, harnessName, cause)
}

// clearWarning removes the hook warning of the repo holding the working
// directory, if it has one.
func clearWarning() error {
	root, err := hookRoot()
	if err != nil || root == "" {
		return err
	}
	return harness.ClearWarning(root)
}

// hookRoot is the root of the git work tree holding the working directory, or
// "" outside one.
func hookRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil || !gitx.IsRepo(dir) {
		return "", err
	}
	return gitx.Root(dir)
}

// hookDeny says why the hook blocks call, or "" when it is allowed: a
// shell call that runs `lyceum approve` or `lyceum eject`, or a write of code before the unit's
// code-opening gate clears (check.CodeBeforeGate).
func hookDeny(call harness.Call) (string, error) {
	switch call.Kind {
	case harness.Shell:
		if countLyceum(call.Command, "approve") > 1 {
			return `Lyceum blocked this command: one approval per command, so one click approves exactly one gate. ` +
				`Next: run each "lyceum approve <gate>" as its own command, one at a time.`, nil
		}
		for _, c := range []string{"approve", "eject"} {
			if runsLyceum(call.Command, c) {
				return `Lyceum blocked this command: "lyceum ` + c + `" is for the owner, typed in their own terminal. ` +
					"Next: stop and ask the owner to run it themselves. Do not retry this command or work around it.", nil
			}
		}
		return "", nil
	case harness.Write:
		return writeDeny(call)
	}
	return "", nil
}

// promptRoute is the harness-prompt approval route (enforcement.md,
// "Harness-prompt approval") for shell call, which runs `lyceum
// approve`: when the command names a gate, harness a can ask the human,
// Lyceum is set up in the repo and approvals.harness_prompt is allow, it
// writes the one-use grant for the gate and returns the reason the harness's
// prompt shows, naming the unit, gate and files. It returns "" when the route
// does not apply, and then the hook denies. So does anything it can't read:
// a denial is the hook's answer when in doubt.
func promptRoute(a harness.Adapter, call harness.Call) string {
	g, ok := promptGate(call.Command)
	// eject has no harness-prompt route, even beside an approve.
	if call.Kind != harness.Shell || !ok || !a.Prompts() || runsLyceum(call.Command, "eject") || countLyceum(call.Command, "approve") > 1 {
		return ""
	}
	dir := call.Cwd
	if dir == "" {
		var err error
		if dir, err = os.Getwd(); err != nil {
			return ""
		}
	}
	if !gitx.IsRepo(dir) {
		return ""
	}
	root, err := gitx.Root(dir)
	if err != nil {
		return ""
	}
	cfg, err := config.Read(root)
	if err != nil || cfg.Approvals.HarnessPrompt != config.HarnessPromptAllow {
		return ""
	}
	branch, err := gitx.Branch(root)
	if err != nil {
		return ""
	}
	p, err := openUnit(root, branch)
	if err != nil {
		return ""
	}
	unitID, of := "", ""
	var typ gate.Type
	planPath := ""
	if g != approval.Testing && g != approval.Project {
		// Only a gate approve would take: the unit's current judgment gate, or tests where it allows them.
		if p == nil {
			return ""
		}
		var current gate.Gate
		var class gate.Class
		typ, current, class, err = unitGate(p)
		if err != nil || g == approval.Tests && testsApprovable(p, typ, current, class) != nil ||
			g != approval.Tests && (g != current || class != gate.Judgment) {
			return ""
		}
		unitID, of, planPath = p.ID, " of unit "+p.ID, path.Join(planDir, p.ID+".md")
	}
	files, err := promptFiles(root, cfg, p, typ, g, planPath)
	if err != nil {
		return ""
	}
	if err := harness.WriteGrant(root, harness.NewGrant(unitID, string(g), a.Name, call.Command, call.PermissionMode)); err != nil {
		return ""
	}
	return fmt.Sprintf("Lyceum owner approval of gate %s%s, approving: %s. "+
		"Allow it only if you are the owner and have reviewed them; Lyceum records that it was approved through this prompt.",
		g, of, strings.Join(files, ", "))
}

// promptFiles are the files `approve g` of p, of type typ, would hash, for
// the harness prompt's reason, as approveGate finds them.
func promptFiles(root string, cfg *config.Config, p *unit.Plan, typ gate.Type, g gate.Gate, planPath string) ([]string, error) {
	switch g {
	case gate.Cause:
		files, _, err := causeApproval(root, cfg.BaseBranch, planPath, p)
		return files, err
	case approval.Tests:
		files, _, err := testsToApprove(root, cfg.BaseBranch, p, typ)
		return files, err
	}
	return approval.FilesFor(root, typ, g, planPath, p)
}

// promptGate returns the gate a shell command line running `lyceum approve`
// names: the first word after approve, in the same command, that is not a
// flag. ok is false when it names none, as bare approve.
func promptGate(line string) (gate.Gate, bool) {
	// Operators end a command; quotes, spaces and $( ) only separate words.
	for _, c := range strings.FieldsFunc(shellCommands(line), func(r rune) bool { return strings.ContainsRune(";&|<>\n", r) }) {
		words := strings.FieldsFunc(c, func(r rune) bool { return strings.ContainsRune(" \t\r()'\"`", r) })
		for i, w := range words {
			if path.Base(w) != "lyceum" {
				continue
			}
			for j, next := range words[i+1:] {
				if next == "approve" {
					for _, arg := range words[i+j+2:] {
						if !strings.HasPrefix(arg, "-") {
							return gate.Gate(arg), true
						}
					}
					return "", false
				}
				if !strings.HasPrefix(next, "-") {
					break
				}
			}
		}
	}
	return "", false
}

// writeDeny says why the hook blocks write call, or "" when it is allowed.
// The repo is the git work tree holding the payload's cwd; when the payload
// has none, the one holding the first absolute path in the call; only then the
// process's, which may be a long-running harness server's. Writes
// outside it, and every write where Lyceum is not set up or the branch has no
// unit, are allowed.
func writeDeny(call harness.Call) (string, error) {
	cwd := call.Cwd
	if cwd == "" {
		var err error
		if cwd, err = os.Getwd(); err != nil {
			return "", err
		}
	}
	dir := call.Cwd
	if dir == "" {
		dir = cwd
		for _, f := range call.Paths {
			if filepath.IsAbs(f) {
				dir = existingDir(filepath.Dir(f))
				break
			}
		}
	}
	if !gitx.IsRepo(dir) {
		return "", nil
	}
	root, err := gitx.Root(dir)
	if err != nil {
		return "", err
	}
	if _, err := config.Read(root); errors.Is(err, fs.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	branch, err := gitx.Branch(root)
	if err != nil {
		return "", err
	}
	p, err := openUnit(root, branch)
	if err != nil {
		return "", err
	}
	evs, err := events.Read(root)
	if err != nil {
		return "", err
	}
	if p == nil {
		if p, err = loggedUnit(root, branch, evs); err != nil || p == nil {
			return "", err
		}
	}
	var rels []string
	for _, f := range call.Paths {
		if !filepath.IsAbs(f) {
			f = filepath.Join(cwd, f)
		}
		rel, err := filepath.Rel(root, realPath(f))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		rels = append(rels, filepath.ToSlash(rel))
	}
	opening, code, err := check.CodeBeforeGate(check.Context{Root: root, Branch: branch, Plan: p, Events: evs}, rels)
	if err != nil || len(code) == 0 {
		return "", err
	}
	next := fmt.Sprintf(`stop and ask the owner to run "lyceum approve %s" in their own terminal`, opening)
	if opening == gate.Repro {
		next = "write the failing repro test first (test files are allowed now); code waits until the repro gate clears"
	}
	is := "is"
	if len(code) > 1 {
		is = "are"
	}
	return fmt.Sprintf("Lyceum blocked this write: %s %s code, and unit %s has not cleared its %s gate. "+
		"Next: %s. Writes under lyceum/ and docs/ are allowed. Do not retry this write or work around it.",
		strings.Join(code, ", "), is, p.ID, opening, next), nil
}

// existingDir is dir, or its nearest ancestor that exists: a write may create
// directories git can't be asked about yet.
func existingDir(dir string) string {
	for {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}

// realPath is p, absolute and clean, with the symlinks in its longest existing
// prefix resolved, so it compares with git's resolved work tree root.
func realPath(p string) string {
	rest := ""
	for dir := p; ; dir = filepath.Dir(dir) {
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(r, rest)
		}
		if filepath.Dir(dir) == dir {
			return p
		}
		rest = filepath.Join(filepath.Base(dir), rest)
	}
}

// runsLyceum reports whether shell command line invokes `lyceum <command>`,
// such as `lyceum approve`: a word naming lyceum, by itself or as a path's
// last element (so `npx lyceum` and `./dist/lyceum` count), followed by
// command with only flags between. Quotes and shell operators separate words,
// so a quoted or chained `lyceum approve` counts too. Text that is only data,
// such as a here-document body, does not (shellCommands).
func runsLyceum(line, command string) bool {
	return countLyceum(line, command) > 0
}

// countLyceum returns how many times shell command line invokes `lyceum
// <command>`, as runsLyceum reads it.
func countLyceum(line, command string) int {
	n := 0
	words := strings.FieldsFunc(shellCommands(line), shellSeparator)
	for i, w := range words {
		if path.Base(w) != "lyceum" {
			continue
		}
		for _, next := range words[i+1:] {
			if next == command {
				n++
				break
			}
			if !strings.HasPrefix(next, "-") {
				break
			}
		}
	}
	return n
}
