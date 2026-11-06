//go:build unix

package runner

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/oritromax/lyceum/internal/testhooks"
)

// waitDelay bounds the wait for the output pipes once the command has ended:
// a process it left behind may still hold them open.
const waitDelay = 5 * time.Second

// Exec runs command with `sh -c` in dir. The command's stdout goes live to
// stdout and its stderr to stderr, and both go to capture as they arrive. When
// it runs longer than timeout, its whole process group is killed.
func Exec(dir, command string, timeout time.Duration, stdout, stderr, capture io.Writer) (Outcome, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	var mu sync.Mutex
	cmd.Stdout = tee{&mu, capture, stdout}
	cmd.Stderr = tee{&mu, capture, stderr}
	// Its own process group, so a timeout kills everything the command started.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = waitDelay

	start := testhooks.Now()
	err := cmd.Run()
	o := Outcome{Duration: testhooks.Now().Sub(start), TimedOut: errors.Is(ctx.Err(), context.DeadlineExceeded)}
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
	case err == nil, errors.Is(err, exec.ErrWaitDelay):
	default:
		return Outcome{}, err
	}
	if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		o.Exit = 128 + int(ws.Signal())
	} else {
		o.Exit = cmd.ProcessState.ExitCode()
	}
	if o.TimedOut && o.Exit == 0 {
		// It ended as the limit passed: over the limit is failed.
		o.Exit = 128 + int(syscall.SIGKILL)
	}
	return o, nil
}
