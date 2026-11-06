//go:build lyceumtest && lyceumtest_noguard_checkintegrity

package guard

// CheckintegrityEnabled turns on check's approval integrity check: approved files re-hashed against the approval.
const CheckintegrityEnabled = false
