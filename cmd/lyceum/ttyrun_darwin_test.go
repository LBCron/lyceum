package main

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"
)

const (
	getTermios = syscall.TIOCGETA
	setTermios = syscall.TIOCSETA
)

// hideEOF turns off ECHOCTL, which stops the end of file character echoing as
// ^D and two backspaces, and sets ONOEOT, because without ECHOCTL the kernel
// echoes it as the raw byte instead (ttyecho and ttyoutput in XNU's tty.c).
func hideEOF(t *syscall.Termios) {
	t.Lflag &^= syscall.ECHOCTL
	t.Oflag |= syscall.ONOEOT
}

// unlockPTY grants and unlocks the pty and returns the path of its terminal side.
func unlockPTY(ctrl *os.File) (string, error) {
	// The ioctl encodes its buffer's length in the low 13 bits of the top word.
	name := make([]byte, (syscall.TIOCPTYGNAME>>16)&0x1fff)
	if err := ioctl(ctrl, syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))); err != nil {
		return "", err
	}
	if err := ioctl(ctrl, syscall.TIOCPTYGRANT, 0); err != nil {
		return "", err
	}
	if err := ioctl(ctrl, syscall.TIOCPTYUNLK, 0); err != nil {
		return "", err
	}
	return string(name[:bytes.IndexByte(name, 0)]), nil
}
