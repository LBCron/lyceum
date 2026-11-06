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

// WarningPath is the hook warning file, relative to the repo root: local and
// gitignored, like the rest of .lyceum/ (enforcement.md, "Harness hooks").
const WarningPath = ".lyceum/hook-warning.json"

// Warning records the last pre-tool payload a harness's hook could not read.
type Warning struct {
	Harness string    `json:"harness"`
	At      time.Time `json:"at"`
	Error   string    `json:"error"`
}

// WriteWarning records under root that harness's hook could not read a
// payload, failing with cause.
func WriteWarning(root, harness string, cause error) error {
	data, err := json.Marshal(Warning{Harness: harness, At: testhooks.Now().Truncate(time.Second), Error: cause.Error()})
	if err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(WarningPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// ClearWarning removes the warning under root, if there is one.
func ClearWarning(root string) error {
	err := os.Remove(filepath.Join(root, filepath.FromSlash(WarningPath)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// ReadWarning returns the warning under root, or nil when there is none.
func ReadWarning(root string) (*Warning, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(WarningPath)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var w Warning
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	return &w, nil
}
