//go:build !lyceumtest || !lyceumtest_noguard_evidence

package guard

// EvidenceEnabled turns on the gate-evidence predicate: only a clean full-suite run at HEAD with TESTING.md approved clears a gate.
const EvidenceEnabled = true
