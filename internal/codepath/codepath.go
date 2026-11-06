// Package codepath says which changed paths are code for `lyceum check`
// (enforcement.md, "lyceum check").
package codepath

import (
	"path"
	"strings"
)

// IsCode reports whether p, a path relative to the repo root in git's slash
// form, is code, a manifest, a lockfile or CI config: any path outside
// lyceum/ and docs/ that is not *.md or *.txt, at any depth. CI config such as
// .github/workflows/*.yml counts as code.
func IsCode(p string) bool {
	if strings.HasPrefix(p, "lyceum/") || strings.HasPrefix(p, "docs/") {
		return false
	}
	switch path.Ext(p) {
	case ".md", ".txt":
		return false
	}
	return true
}
