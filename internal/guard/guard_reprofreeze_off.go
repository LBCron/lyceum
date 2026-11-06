//go:build lyceumtest && lyceumtest_noguard_reprofreeze

package guard

// ReprofreezeEnabled turns on a bug's frozen repro: verify does not clear while a repro test file changed without approval.
const ReprofreezeEnabled = false
