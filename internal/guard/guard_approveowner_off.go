//go:build lyceumtest && lyceumtest_noguard_approveowner

package guard

// ApproveownerEnabled turns on approve's owner check: a non-owner's approval does not count.
const ApproveownerEnabled = false
