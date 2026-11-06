//go:build !lyceumtest || !lyceumtest_noguard_selftest

package guard

// SelftestEnabled is the template guard; it protects nothing.
const SelftestEnabled = true
