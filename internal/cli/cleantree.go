package cli

import (
	"strings"

	"github.com/oritromax/lyceum/internal/gitx"
	"github.com/oritromax/lyceum/internal/out"
)

// dirtyRefusal is the clean-tree refusal over changed, with next as its steps,
// plus one naming the untracked files among them and saying to ignore them if a
// tool wrote them. Clean-tree checks do not skip untracked files themselves.
func dirtyRefusal(root, what, reason string, changed []string, next ...string) error {
	untracked, err := gitx.Untracked(root, changed)
	if err != nil {
		return err
	}
	if len(untracked) > 0 {
		next = append(next, "if a tool writes "+strings.Join(untracked, ", ")+" on its own, ignore it instead: add it to .gitignore and commit that")
	}
	return out.Refuse(what, reason, next...)
}
