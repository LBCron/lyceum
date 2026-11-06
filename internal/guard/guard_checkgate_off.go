//go:build lyceumtest && lyceumtest_noguard_checkgate

package guard

// CheckgateEnabled turns on check's gate-before-code check: no code before the code-opening gate clears.
const CheckgateEnabled = false
