// Package unit holds unit identity: IDs, slugs and branch names.
package unit

import (
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/oritromax/lyceum/internal/testhooks"
)

// MaxSlug is the longest slug a unit ID may carry.
const MaxSlug = 40

// NewHash returns 6 lowercase hex characters read from testhooks.Rand.
func NewHash() string {
	var b [3]byte
	if _, err := io.ReadFull(testhooks.Rand(), b[:]); err != nil {
		// Neither crypto/rand nor the seeded test stream can fail.
		panic("unit: reading random bytes: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// Slug derives a slug from a title: lowercase, every run of characters outside
// [a-z0-9] becomes one "-", no "-" at either end, at most MaxSlug characters.
// Non-ASCII letters count as outside [a-z0-9].
func Slug(title string) (string, error) {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			continue
		}
		dash = true
	}
	s := b.String()
	if len(s) > MaxSlug {
		s = strings.TrimRight(s[:MaxSlug], "-")
	}
	if s == "" {
		return "", fmt.Errorf("title %q has no letters or digits to make a slug from", title)
	}
	return s, nil
}

// ID joins a hash and a slug into a unit ID, e.g. "a3f9c2-csv-export".
func ID(hash, slug string) string { return hash + "-" + slug }

// ParseID splits a unit ID into its hash and slug and validates both.
func ParseID(s string) (hash, slug string, err error) {
	hash, slug, ok := strings.Cut(s, "-")
	if !ok {
		return "", "", fmt.Errorf("unit ID %q has no slug; want <6 hex>-<slug>", s)
	}
	if !validHash(hash) {
		return "", "", fmt.Errorf("unit ID %q: hash %q is not 6 lowercase hex characters", s, hash)
	}
	if got, err := Slug(slug); err != nil || got != slug {
		return "", "", fmt.Errorf("unit ID %q: slug %q is not [a-z0-9-], at most %d characters, without leading, trailing or doubled dashes", s, slug, MaxSlug)
	}
	return hash, slug, nil
}

func validHash(h string) bool {
	if len(h) != 6 {
		return false
	}
	for _, r := range h {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

var branchPrefixes = map[string]string{
	"feat":     "feat/",
	"bug":      "fix/",
	"refactor": "refactor/",
	"chore":    "chore/",
	"doc":      "docs/",
	"test":     "test/",
}

// BranchPrefix returns the branch prefix for a unit type.
func BranchPrefix(unitType string) (string, error) {
	p, ok := branchPrefixes[unitType]
	if !ok {
		return "", fmt.Errorf("unknown unit type %q; want feat, bug, refactor, chore, doc or test", unitType)
	}
	return p, nil
}
