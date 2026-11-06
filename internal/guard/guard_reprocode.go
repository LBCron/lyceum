//go:build !lyceumtest || !lyceumtest_noguard_reprocode

package guard

// ReprocodeEnabled turns on --repro's refusal of a branch diff with code that is not a test.
const ReprocodeEnabled = true
