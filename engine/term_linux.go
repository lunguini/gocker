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
	// TCGETS ioctl on Linux
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&t)))
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
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&state.termios)))
}

// IsTerminal reports whether stdin is connected to a terminal.
func IsTerminal() bool {
	fd := int(os.Stdin.Fd())
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
