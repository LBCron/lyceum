package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/rogpeppe/go-internal/testscript"
)

// ttyHelperArg makes the test binary run ttyHelper instead of the tests.
const ttyHelperArg = "-lyceum-ttyrun-helper"

// ttyRun implements the script command `ttyrun <input> <command>`: it runs the
// shell command on a real pty, with the pty as stdin, stdout and controlling
// terminal and the input file as what is typed, keeps stderr apart, prints
// exit=<code> to stdout, and writes what the terminal showed to $WORK/tty.out.
// Scripts that assert terminal output use it instead of ttyin and ttyout:
// testscript closes the pty when the child exits, before its reader has drained
// it, so ttyout can lose the end of the output. It runs as a child process so
// the command gets the script's environment and directory.
func ttyRun(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 2 {
		ts.Fatalf("usage: ttyrun <input> <command>")
	}
	exe, err := os.Executable()
	ts.Check(err)
	ts.Check(ts.Exec(exe, ttyHelperArg, ts.MkAbs(args[0]), filepath.Join(ts.Getenv("WORK"), "tty.out"), args[1]))
}

// ttyHelper is ttyRun's child process. It reads the pty until every holder of
// the terminal side has closed it, so nothing the command wrote is lost.
func ttyHelper(input, out, command string) int {
	code, err := runOnPTY(input, out, command)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ttyrun:", err)
		return 1
	}
	fmt.Printf("exit=%d\n", code)
	return 0
}

func runOnPTY(input, out, command string) (int, error) {
	typed, err := os.ReadFile(input)
	if err != nil {
		return 0, err
	}
	ctrl, tty, err := openPTY()
	if err != nil {
		return 0, err
	}
	defer func() { _ = ctrl.Close() }()
	// What is typed, then an end of file (^D), as ttyin does. It waits on the pty
	// for the command, and goes first so its echo can't land inside the output.
	if _, err := ctrl.Write(append(typed, 4)); err != nil {
		return 0, err
	}
	// The command is never the shell's last, so the shell stays its parent: a bash
	// sh, as on macOS, would otherwise exec it. The exit code is unchanged.
	cmd := exec.Command("sh", "-c", command+"\nexit $?")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	err = cmd.Start()
	_ = tty.Close() // the command now holds the only terminal side
	if err != nil {
		return 0, err
	}
	var shown strings.Builder
	// Once the terminal side is closed, Linux reports EIO and macOS end of file.
	if _, err := io.Copy(&shown, ctrl); err != nil && !errors.Is(err, syscall.EIO) {
		return 0, err
	}
	if err := os.WriteFile(out, []byte(shown.String()), 0o644); err != nil {
		return 0, err
	}
	var exit *exec.ExitError
	if err := cmd.Wait(); err != nil && !errors.As(err, &exit) {
		return 0, err
	}
	return cmd.ProcessState.ExitCode(), nil
}

// openPTY opens a new pty, as go-internal's testscript does in a package scripts
// can't import. Both sides stay in blocking mode: f.Fd() sets it.
func openPTY() (ctrl, tty *os.File, err error) {
	ctrl, err = os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	name, err := unlockPTY(ctrl)
	if err == nil {
		tty, err = os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	}
	if err == nil {
		err = keepEOFOffScreen(tty)
	}
	if err != nil {
		_ = ctrl.Close()
		return nil, nil, err
	}
	return ctrl, tty, nil
}

// keepEOFOffScreen sets the terminal so the end of file ttyHelper types is not
// echoed into the output. It must run before anything is typed.
func keepEOFOffScreen(tty *os.File) error {
	var t syscall.Termios
	if err := ioctl(tty, getTermios, uintptr(unsafe.Pointer(&t))); err != nil {
		return err
	}
	hideEOF(&t)
	return ioctl(tty, setTermios, uintptr(unsafe.Pointer(&t)))
}

// ioctl runs one ioctl on f.
func ioctl(f *os.File, cmd, arg uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), cmd, arg); errno != 0 {
		return errno
	}
	return nil
}
