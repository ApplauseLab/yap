//go:build !windows

package transcribe

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func hideChildConsole(*exec.Cmd) {}

// bindToProcessLifetime is a no-op: macOS has no parent-death signal, so
// orphaned workers are reaped on the next launch instead.
func bindToProcessLifetime(*os.Process) error { return nil }

func processAlive(pid int) bool {
	return exec.Command("ps", "-p", strconv.Itoa(pid)).Run() == nil
}

func isWhisperServerProcess(pid int) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	return err == nil && strings.Contains(strings.ToLower(string(out)), "whisper-server")
}

func isIllegalInstructionExit(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && status.Signaled() && status.Signal() == syscall.SIGILL
}
