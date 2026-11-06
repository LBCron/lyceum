package cli

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
	"github.com/spf13/cobra"
)

// logEntry is one line of `lyceum log`: an event, its kind the event type, or
// a commit, its kind "commit".
type logEntry struct {
	TS      string `json:"ts"`
	Kind    string `json:"kind"`
	Unit    string `json:"unit"`
	Summary string `json:"summary"`
	Actor   string `json:"actor"`
	Commit  string `json:"commit"`

	at time.Time
}

// runLog is `lyceum log` (commands.md, "lyceum log"). It
// merges the event log with the commits of every local branch into one list,
// oldest first, events before commits at the same second. Commits whose
// subject starts with "lyceum:" are hidden: each is shown as its event. A
// commit on a unit's branch and not on the base branch is that unit's.
// --unit keeps that unit's events and commits, --since the entries from a
// date (UTC) on, and --type the events of one type and no commits. It
// changes nothing.
func runLog(cmd *cobra.Command, args []string) error {
	what := "show the log"
	if len(args) > 0 {
		return out.Refuse(what, "log takes no arguments", `run "lyceum log"`)
	}
	unitID, _ := cmd.Flags().GetString("unit")
	sinceArg, _ := cmd.Flags().GetString("since")
	typ, _ := cmd.Flags().GetString("type")
	var since time.Time
	if sinceArg != "" {
		var err error
		if since, err = time.Parse(time.DateOnly, sinceArg); err != nil {
			return out.Refuse(what, fmt.Sprintf("--since %q is not a date in the form YYYY-MM-DD", sinceArg),
				`run "lyceum log --since 2026-01-31"`)
		}
	}
	root, cfg, err := setUpRoot(what)
	if err != nil {
		return err
	}
	evs, err := events.Read(root)
	if err != nil {
		return err
	}

	// Each unit's branch, from its last add event.
	branches := map[string]string{}
	for _, e := range evs {
		if b, _ := e.Fields["branch"].(string); e.Type == "add" && b != "" {
			branches[e.Unit] = b
		}
	}
	if _, ok := branches[unitID]; unitID != "" && !ok {
		return out.Refuse(what, fmt.Sprintf("no unit %s is in %s", unitID, events.Path),
			`run "lyceum status --all" to see the units`)
	}

	var entries []logEntry
	for _, e := range evs {
		if (unitID != "" && e.Unit != unitID) || (typ != "" && e.Type != typ) {
			continue
		}
		entries = append(entries, logEntry{Kind: e.Type, Unit: e.Unit, Summary: eventSummary(e),
			Actor: e.Actor, Commit: e.Commit, at: e.TS})
	}
	if typ == "" {
		commits, err := logCommits(root, cfg.BaseBranch, unitID, branches, what)
		if err != nil {
			return err
		}
		entries = append(entries, commits...)
	}
	entries = slices.DeleteFunc(entries, func(e logEntry) bool { return e.at.Before(since) })
	slices.SortStableFunc(entries, func(a, b logEntry) int { return a.at.Compare(b.at) })
	for i := range entries {
		entries[i].TS = entries[i].at.Format(time.RFC3339)
	}

	if entries == nil {
		entries = []logEntry{}
	}
	res := out.Result{Next: `run "lyceum status"`, Data: struct {
		Entries []logEntry `json:"entries"`
	}{entries}}
	asJSON, _ := cmd.Flags().GetBool("json")
	if !asJSON {
		_, _ = io.WriteString(cmd.OutOrStdout(), logText(entries))
	}
	out.Writer{Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JSON: asJSON}.Result(res)
	return nil
}

// logCommits returns the commits log shows, oldest first: with unitID, those
// on its branch and not on the base branch; else every local branch's, each
// marked with the unit whose branch has it and the base branch doesn't. It
// leaves out commits whose subject starts with "lyceum:".
func logCommits(root, base, unitID string, branches map[string]string, what string) ([]logEntry, error) {
	var commits []gitx.LogCommit
	units := map[string]string{} // commit SHA → unit
	if unitID != "" {
		ref, err := baseRef(root, base, what, false)
		if err != nil {
			return nil, err
		}
		if commits, err = branchCommits(root, ref, branches[unitID]); err != nil {
			return nil, err
		}
		for _, c := range commits {
			units[c.SHA] = unitID
		}
	} else {
		var err error
		if commits, err = gitx.Log(root, "--branches"); err != nil {
			return nil, err
		}
		if base == "" {
			if base, err = gitx.DefaultBranch(root); err != nil {
				return nil, err
			}
		}
		ref := ""
		if base != "" {
			if ref, err = gitx.BaseRef(root, base); err != nil {
				return nil, err
			}
		}
		if ref != "" {
			for id, b := range branches {
				own, err := branchCommits(root, ref, b)
				if err != nil {
					return nil, err
				}
				for _, c := range own {
					units[c.SHA] = id
				}
			}
		}
	}
	var entries []logEntry
	for _, c := range slices.Backward(commits) {
		if strings.HasPrefix(c.Subject, "lyceum:") {
			continue
		}
		entries = append(entries, logEntry{Kind: "commit", Unit: units[c.SHA], Summary: c.Short + " " + c.Subject,
			Actor: c.Author, Commit: c.SHA, at: c.Time})
	}
	return entries, nil
}

// branchCommits returns the commits on local branch and not on ref, newest
// first, or none when the branch is gone.
func branchCommits(root, ref, branch string) ([]gitx.LogCommit, error) {
	ok, err := gitx.BranchExists(root, branch)
	if err != nil || !ok {
		return nil, err
	}
	return gitx.Log(root, "refs/heads/"+branch, "--not", ref)
}

// eventSummary is an event's scalar fields as key=value, in key order.
func eventSummary(e events.Event) string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, cmp.Compare)
	var parts []string
	for _, k := range keys {
		switch v := e.Fields[k].(type) {
		case string, bool, fmt.Stringer:
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	return strings.Join(parts, " ")
}

// logText is the human form of the log: one line per entry with its time,
// kind, unit ("-" for none) and summary, or a line saying there are none.
func logText(entries []logEntry) string {
	if len(entries) == 0 {
		return "No entries.\n"
	}
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for _, e := range entries {
		u := e.Unit
		if u == "" {
			u = "-"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.TS, e.Kind, u, e.Summary)
	}
	_ = w.Flush()
	return b.String()
}
