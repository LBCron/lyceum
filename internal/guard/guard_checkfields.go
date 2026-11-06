//go:build !lyceumtest || !lyceumtest_noguard_checkfields

package guard

// CheckfieldsEnabled turns on check's CLI-owned field check: plan fields against the event log.
const CheckfieldsEnabled = true
