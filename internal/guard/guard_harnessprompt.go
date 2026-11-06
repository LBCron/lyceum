//go:build !lyceumtest || !lyceumtest_noguard_harnessprompt

package guard

// HarnesspromptEnabled turns on the harness-prompt grant's match: approve honours a grant only for the unit and gate it resolves, unexpired, inside the grant's harness.
const HarnesspromptEnabled = true
