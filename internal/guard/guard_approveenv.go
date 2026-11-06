//go:build !lyceumtest || !lyceumtest_noguard_approveenv

package guard

// ApproveenvEnabled turns on approve's refusal when an agent harness variable is set.
const ApproveenvEnabled = true
