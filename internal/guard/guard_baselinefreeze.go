//go:build !lyceumtest || !lyceumtest_noguard_baselinefreeze

package guard

// BaselinefreezeEnabled turns on a refactor's frozen tests: verify does not clear while a test file differs from the baseline fingerprint without approval.
const BaselinefreezeEnabled = true
