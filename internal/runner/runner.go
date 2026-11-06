// Package runner runs the commands TESTING.md names and keeps what they print
// under .lyceum/runs/ (test-runner.md, "Run records" and "Gate mechanics").
package runner

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oritromax/lyceum/internal/testhooks"
)

// Dir holds the run records and their output files, relative to the repo root.
// It is gitignored.
const Dir = ".lyceum/runs"

// Outcome is how a command ended.
type Outcome struct {
	// Exit is the command's exit code; 128+n when signal n ended it, as sh reports.
	// A run killed on timeout is never 0.
	Exit     int
	TimedOut bool
	Duration time.Duration
}

// Create makes Dir under root and a new file in it for a run's output, named
// <ts>-<rand>.out, and returns the file and its name without the extension.
func Create(root string) (*os.File, string, error) {
	dir := filepath.Join(root, filepath.FromSlash(Dir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	ts := testhooks.Now().Format("20060102T150405Z")
	// Two runs in the same second get different names; a collision draws again.
	for range 10 {
		b := make([]byte, 4)
		if _, err := io.ReadFull(testhooks.Rand(), b); err != nil {
			return nil, "", err
		}
		name := ts + "-" + hex.EncodeToString(b)
		f, err := os.OpenFile(filepath.Join(dir, name+".out"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return f, name, err
	}
	return nil, "", fmt.Errorf("no free run name in %s for %s", Dir, ts)
}

// tee writes to capture and then to live, one Write at a time across every tee
// sharing mu, so capture holds the command's stdout and stderr interleaved in
// the order they arrived. A failing live stream does not stop the capture.
type tee struct {
	mu            *sync.Mutex
	capture, live io.Writer
}

func (t tee) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.capture.Write(p); err != nil {
		return 0, err
	}
	_, _ = t.live.Write(p)
	return len(p), nil
}
