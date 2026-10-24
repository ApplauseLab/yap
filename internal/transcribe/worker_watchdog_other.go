//go:build !darwin

package transcribe

// RunWorkerWatchdogIfRequested is a no-op: the worker watchdog is only used
// on macOS, which lacks a parent-death signal.
func RunWorkerWatchdogIfRequested() {}

func startWorkerWatchdog(int) error {
	return nil
}
