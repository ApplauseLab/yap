package crashlog

import (
	"os"

	"golang.org/x/sys/windows"
)

func stderrIsTerminal() bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(os.Stderr.Fd()), &mode) == nil
}

// The Go runtime looks up STD_ERROR_HANDLE each time it writes crash output.
func redirectStderrTo(file *os.File) error {
	return windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(file.Fd()))
}
