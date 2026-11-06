//go:build !lyceumtest || !lyceumtest_noguard_approvetty

package guard

// ApprovettyEnabled turns on approve's refusal unless stdin and stdout are terminals.
const ApprovettyEnabled = true
