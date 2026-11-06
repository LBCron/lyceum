package main

import (
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

const (
	getTermios = syscall.TCGETS
	setTermios = syscall.TCSETS
)

// hideEOF turns off ECHOCTL. Linux does not echo the end of file character in
// canonical mode either way.
func hideEOF(t *syscall.Termios) { t.Lflag &^= syscall.ECHOCTL }

// unlockPTY unlocks the pty and returns the path of its terminal side.
func unlockPTY(ctrl *os.File) (string, error) {
	var n uint32
	if err := ioctl(ctrl, syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); err != nil {
		return "", err
	}
	var unlock int32
	if err := ioctl(ctrl, syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); err != nil {
		return "", err
	}
	return "/dev/pts/" + strconv.Itoa(int(n)), nil
}
