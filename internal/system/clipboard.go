package system

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/atotto/clipboard"
)

// CopyToClipboard copies text to the system clipboard
func CopyToClipboard(text string) error {
	return clipboard.WriteAll(text)
}

// ReadFromClipboard reads text from the system clipboard
func ReadFromClipboard() (string, error) {
	return clipboard.ReadAll()
}

// CopyAndPaste copies text to clipboard and simulates Cmd+V / Ctrl+V
func CopyAndPaste(text string) error {
	// First copy to clipboard
	if err := CopyToClipboard(text); err != nil {
		return fmt.Errorf("failed to copy to clipboard: %w", err)
	}

	// Then simulate paste
	return SimulatePaste()
}

// SimulatePaste simulates pressing Cmd+V (macOS) or Ctrl+V (others)
func SimulatePaste() error {
	switch runtime.GOOS {
	case "darwin":
		return simulatePasteMacOS()
	case "linux":
		return simulatePasteLinux()
	case "windows":
		return simulatePasteWindows()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// simulatePasteMacOS uses AppleScript to simulate Cmd+V
func simulatePasteMacOS() error {
	// First, activate the frontmost app (which should be the app the user was typing in)
	// Then send Cmd+V
	script := `
tell application "System Events"
	set frontApp to name of first application process whose frontmost is true
	keystroke "v" using command down
end tell
`
	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to simulate paste: %w, output: %s", err, string(output))
	}
	return nil
}

// simulatePasteLinux uses xdotool to simulate Ctrl+V
func simulatePasteLinux() error {
	cmd := exec.Command("xdotool", "key", "ctrl+v")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to simulate paste (ensure xdotool is installed): %w", err)
	}
	return nil
}

// simulatePasteWindows uses PowerShell to simulate Ctrl+V
func simulatePasteWindows() error {
	script := `Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait("^v")`
	cmd := exec.Command("powershell", "-Command", script)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to simulate paste: %w", err)
	}
	return nil
}
