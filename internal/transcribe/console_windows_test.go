//go:build windows

package transcribe

import (
	"os"

	"golang.org/x/sys/windows"
)

var procGetConsoleWindow = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")

func writeConsoleState(path string) int {
	if path == "" {
		return 0
	}
	state := "none"
	if hwnd, _, _ := procGetConsoleWindow.Call(); hwnd != 0 {
		state = "window"
	}
	if err := os.WriteFile(path, []byte(state), 0o644); err != nil {
		return 2
	}
	return 0
}
