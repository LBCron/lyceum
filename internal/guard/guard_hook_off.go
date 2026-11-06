//go:build lyceumtest && lyceumtest_noguard_hook

package guard

// HookEnabled turns on the pre-tool hook's denials: code writes before the code gate clears, and lyceum approve in a shell call.
const HookEnabled = false
