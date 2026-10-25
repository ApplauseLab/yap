//go:build darwin || linux

package crashlog

import (
	"os"

	"golang.org/x/sys/unix"
)

func stderrIsTerminal() bool {
	_, err := unix.IoctlGetTermios(int(os.Stderr.Fd()), ioctlReadTermios)
	return err == nil
}

func redirectStderrTo(file *os.File) error {
	return unix.Dup2(int(file.Fd()), int(os.Stderr.Fd()))
}
