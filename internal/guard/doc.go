// Package guard holds the switches that let guard tests turn one protection off
// (TESTING.md §4). Every protection is on in every build unless its own test-only
// build tag is set together with lyceumtest, so a release build cannot turn one off.
//
// To add a guard for a protection called <name>:
//
//   - guard_<name>.go, with //go:build !lyceumtest || !lyceumtest_noguard_<name>,
//     defines const <Name>Enabled = true.
//   - guard_<name>_off.go, with //go:build lyceumtest && lyceumtest_noguard_<name>,
//     defines const <Name>Enabled = false.
//   - The protection runs only inside if guard.<Name>Enabled { … }.
//   - testdata/switches prints it, and guard_test.go lists it.
//
// The guard test builds lyceum with -tags lyceumtest,lyceumtest_noguard_<name>
// and runs the protection's attack script, which must then succeed. Selftest is
// the template and protects nothing.
package guard
