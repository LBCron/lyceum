package harness

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/oritromax/lyceum/internal/testhooks"
)

// GrantPath is the one-use harness-prompt approval grant, relative to the repo
// root: local and gitignored, like the rest of .lyceum/ (enforcement.md,
// "Harness-prompt approval").
const GrantPath = ".lyceum/approve-grant.json"

// GrantTTL is how long a grant stays usable after the hook writes it.
const GrantTTL = 2 * time.Minute

// Grant is what the hook writes before it answers "ask" for `lyceum approve
// <gate>`: approve honours it once, inside the same harness, before it expires.
type Grant struct {
	Unit    string `json:"unit"` // "" for the repo-level gates
	Gate    string `json:"gate"`
	Harness string `json:"harness"`
	Command string `json:"command"` // the shell command, exactly as the payload gave it
	// PermissionMode is the harness's permission mode, when the payload had one.
	PermissionMode string    `json:"permission_mode,omitempty"`
	At             time.Time `json:"at"`
	Expires        time.Time `json:"expires"`
}

// NewGrant is a grant for gate of unit in harness, for command, written now
// and expiring GrantTTL later.
func NewGrant(unit, gate, harness, command, permissionMode string) Grant {
	at := testhooks.Now().Truncate(time.Second)
	return Grant{Unit: unit, Gate: gate, Harness: harness, Command: command, PermissionMode: permissionMode, At: at, Expires: at.Add(GrantTTL)}
}

// Expired reports whether g can no longer be used.
func (g Grant) Expired() bool { return !testhooks.Now().Before(g.Expires) }

// WriteGrant writes g under root, replacing any grant there.
func WriteGrant(root string, g Grant) error {
	data, err := json.Marshal(g)
	if err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(GrantPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// ReadGrant returns the grant under root, or nil when there is none.
func ReadGrant(root string) (*Grant, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(GrantPath)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var g Grant
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// RemoveGrant removes the grant under root, if there is one.
func RemoveGrant(root string) error {
	err := os.Remove(filepath.Join(root, filepath.FromSlash(GrantPath)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
