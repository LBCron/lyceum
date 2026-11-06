//go:build lyceumtest && lyceumtest_noguard_checknounit

package guard

// ChecknounitEnabled turns on check's no-unit check: no code on a branch with no unit.
const ChecknounitEnabled = false
