// Package evidence decides whether a run record is gate evidence: a run a
// mechanical gate may clear on (test-runner.md, "Run records").
package evidence

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/oritromax/lyceum/internal/approval"
	"github.com/oritromax/lyceum/internal/events"
	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/guard"
	"github.com/oritromax/lyceum/internal/runner"
	"github.com/oritromax/lyceum/internal/testspec"
)

// Reason is one condition a run fails to be gate evidence: why, and what to do.
type Reason struct {
	Why, Next string
	// Testing is set when the reason is TESTING.md's approval.
	Testing bool
}

// Check reports whether rec is gate evidence in the repo at root now, and when
// it is not, every reason why. A run is gate evidence iff it ran the full suite
// (no subset, no repro test, not the benchmark) and Repro's conditions hold.
func Check(root string, rec runner.Record) (ok bool, reasons []Reason, err error) {
	if !guard.EvidenceEnabled {
		return true, nil, nil
	}
	if rec.Subset != nil {
		reasons = append(reasons, Reason{Why: fmt.Sprintf("it ran subset %s, not the full suite", *rec.Subset), Next: `run "lyceum test" with no subset`})
	}
	if rec.Repro != nil {
		reasons = append(reasons, Reason{Why: fmt.Sprintf("it ran the repro test %s, not the full suite", *rec.Repro), Next: `run "lyceum test" with no subset`})
	}
	if rec.Benchmark {
		reasons = append(reasons, Reason{Why: "it ran the benchmark, not the full suite", Next: `run "lyceum test" with no subset`})
	}
	more, err := conditions(root, rec)
	if err != nil {
		return false, nil, err
	}
	reasons = append(reasons, more...)
	return len(reasons) == 0, reasons, nil
}

// Repro reports whether rec, a `lyceum test --repro` run, may clear a bug's
// repro gate in the repo at root now, and when it may not, every reason why
// (test-runner.md, "Mechanical gates"): it ran a repro test on a clean
// tree, without timing out; TESTING.md has a counting approval, and both the
// file as the run read it and the file now match that approval's hash; and
// the run's commit is HEAD now. Whether the test failed and its output names
// it is the gate's own condition.
func Repro(root string, rec runner.Record) (ok bool, reasons []Reason, err error) {
	if rec.Repro == nil {
		reasons = append(reasons, Reason{Why: "it did not run a repro test", Next: `run "lyceum test --repro <test>"`})
	}
	more, err := conditions(root, rec)
	if err != nil {
		return false, nil, err
	}
	reasons = append(reasons, more...)
	return len(reasons) == 0, reasons, nil
}

// conditions are the reasons rec, whatever it ran, cannot clear a gate: a
// dirty tree, a timeout, TESTING.md unapproved or changed, or HEAD moved.
func conditions(root string, rec runner.Record) (reasons []Reason, err error) {
	if rec.Dirty {
		reasons = append(reasons, Reason{Why: "the working tree had uncommitted changes", Next: `commit first, then run "lyceum test"`})
	}
	if rec.TimedOut {
		reasons = append(reasons, Reason{Why: "it ran over the timeout and was killed", Next: `fix the code, then run "lyceum test" again`})
	}

	evs, err := events.Read(root)
	if err != nil {
		return nil, err
	}
	approved := testspec.ApprovedHash(evs)
	current, err := approval.Hash(filepath.Join(root, testspec.Path))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	const approve = `run "lyceum approve testing" yourself in a terminal`
	switch {
	case approved == "":
		reasons = append(reasons, Reason{Why: testspec.Path + " is not approved", Next: approve, Testing: true})
	case current != approved || rec.TestingSHA256 != approved:
		reasons = append(reasons, Reason{Why: testspec.Path + " changed since its approval", Next: approve, Testing: true})
	}

	head, err := gitx.HeadSHA(root)
	if err != nil {
		return nil, err
	}
	if rec.Commit != head {
		reasons = append(reasons, Reason{Why: fmt.Sprintf("it ran at commit %s, but HEAD is now %s", rec.Commit, head), Next: `run "lyceum test" again at HEAD`})
	}
	return reasons, nil
}
