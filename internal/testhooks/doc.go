// Package testhooks is the only source of time and randomness for Lyceum.
//
// All production code must take the current time from Now and random bytes from
// Rand, never from time.Now, math/rand or crypto/rand directly. That keeps the
// test-only overrides in one place:
//
//   - Release builds (no lyceumtest tag): Now is the real clock in UTC and Rand is
//     crypto/rand.Reader. No environment variable is read (AC116).
//   - Test builds (-tags lyceumtest): Now returns LYCEUM_NOW (RFC 3339) when set,
//     and Rand is a deterministic ChaCha8 stream seeded from LYCEUM_SEED (an
//     unsigned decimal integer) when set. Unset or empty falls back to the release
//     behaviour; a malformed value panics, since it can only be a test bug.
//
// Rand returns an io.Reader. In seeded test builds every call returns the same
// process-wide stream, so successive reads continue it rather than restart it.
package testhooks
