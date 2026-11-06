//go:build !lyceumtest || !lyceumtest_noguard_reopen

package guard

// ReopenEnabled turns on sending a bug at cause back to fix when a code commit follows its verify clear.
const ReopenEnabled = true
