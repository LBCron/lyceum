package cli

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/oritromax/lyceum/internal/check"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gate"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/unit"
)

// reopenBug sends bug p back from cause to fix when a commit after its latest
// verify clear changes a file that is neither a test (TESTING.md test_files)
// nor under lyceum/ (check.Reopened), so verify must clear again before
// approve cause. It writes a gate-reopen event with the reason,
// moves the plan's gate to fix and its status to in-progress and
// commits the plan and the event log. It returns the reason, or "" when p is
// not a bug at cause or no such commit follows the clear.
func reopenBug(root string, p *unit.Plan) (string, error) {
	if p == nil {
		return "", nil
	}
	evs, err := events.Read(root)
	if err != nil {
		return "", err
	}
	reason, err := check.Reopened(root, p, evs)
	if err != nil || reason == "" {
		return "", err
	}
	// The plan is changed in memory before the event is written, so a plan the
	// CLI cannot rewrite leaves no event behind.
	if err := p.SetCLIField("gate", string(gate.Fix)); err != nil {
		return "", err
	}
	if err := p.SetCLIField("status", unit.StatusInProgress); err != nil {
		return "", err
	}
	if _, err := events.Append(root, events.Event{Type: "gate-reopen", Unit: p.ID,
		Fields: map[string]any{"gate": string(gate.Fix), "status": unit.StatusInProgress, "reason": reason}}); err != nil {
		return "", err
	}
	planPath := path.Join(planDir, p.ID+".md")
	if err := unit.WritePlan(filepath.Join(root, filepath.FromSlash(planPath)), p); err != nil {
		return "", err
	}
	if err := gitx.Commit(root, fmt.Sprintf("lyceum: reopen %s %s", gate.Fix, p.ID), planPath, events.Path); err != nil {
		return "", err
	}
	return reason, nil
}

// reopenedLine is the line saying unit id went back to fix for reason.
func reopenedLine(id, reason string) string {
	return fmt.Sprintf("Back to %s for %s: %s; verify must clear again before approve cause\n", gate.Fix, id, reason)
}
