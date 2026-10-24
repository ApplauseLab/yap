//go:build !windows

package transcribe

import "os"

func writeConsoleState(path string) int {
	if path == "" {
		return 0
	}
	if err := os.WriteFile(path, []byte("none"), 0o644); err != nil {
		return 2
	}
	return 0
}

func runConsoleLauncher() int { return 2 }
