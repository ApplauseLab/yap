package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// RevealFile opens the platform file manager for an existing file.
func RevealFile(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("audio file unavailable: %w", err)
	}
	return revealFile(path)
}

func revealWithCommand(path string) error {
	cmd, err := revealCommand(runtime.GOOS, path)
	if err != nil {
		return err
	}
	// Do not block the binding on the file manager's lifetime.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open file manager: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func revealCommand(platform, path string) (*exec.Cmd, error) {
	switch platform {
	case "darwin":
		return exec.Command("open", "-R", path), nil
	case "linux":
		return exec.Command("xdg-open", filepath.Dir(path)), nil
	default:
		return nil, fmt.Errorf("show in folder is unsupported on %s", platform)
	}
}
