//go:build !unix

package runner

import (
	"io"
	"time"

	"github.com/oritromax/lyceum/internal/out"
)

// Exec refuses: running tests needs a Unix process group, and Windows is out
// of scope.
func Exec(_, _ string, _ time.Duration, _, _, _ io.Writer) (Outcome, error) {
	return Outcome{}, out.Refuse("run tests", "running tests is supported on Linux and macOS only",
		"run lyceum test on Linux or macOS, or in WSL")
}
