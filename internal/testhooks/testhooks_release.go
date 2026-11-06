//go:build !lyceumtest

package testhooks

import (
	"crypto/rand"
	"io"
	"time"
)

// ActionPublished returns built, what the release build was told.
func ActionPublished(built bool) bool { return built }

// Now returns the current time in UTC.
func Now() time.Time { return time.Now().UTC() }

// Rand returns the cryptographic random source.
func Rand() io.Reader { return rand.Reader }
