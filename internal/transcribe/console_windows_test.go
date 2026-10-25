//go:build windows

package transcribe

import (
	"os"
	"os/exec"

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

// runConsoleLauncher runs without a console, like Yap's GUI-subsystem
// executable, and starts a console probe from there.
func runConsoleLauncher() int {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), testProcessRoleEnv+"=console-probe")
	if os.Getenv("YAP_TEST_HIDE_CONSOLE") == "1" {
		hideChildConsole(cmd)
	}
	if err := cmd.Run(); err != nil {
		return 3
	}
	return 0
}
