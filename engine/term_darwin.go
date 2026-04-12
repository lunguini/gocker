package engine

import (
	"os"
	"syscall"
	"unsafe"
)

type termState struct {
	termios syscall.Termios
}

func saveTermState() *termState {
	fd := int(os.Stdin.Fd())
	var t syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&t)), 0, 0, 0)
	if errno != 0 {
		return nil
	}
	return &termState{termios: t}
}

func restoreTermState(state *termState) {
	if state == nil {
		return
	}
	fd := int(os.Stdin.Fd())
	_, _, _ = syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCSETA), uintptr(unsafe.Pointer(&state.termios)), 0, 0, 0)
}

// IsTerminal reports whether stdin is connected to a terminal.
func IsTerminal() bool {
	fd := int(os.Stdin.Fd())
	var t syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&t)), 0, 0, 0)
	return errno == 0
}
