//go:build !darwin

package system

import "fmt"

// SaveFrontmostApp is a no-op on non-macOS platforms
func SaveFrontmostApp() {
	// No-op on non-macOS
}

// simulatePasteMacOSNative is a stub for non-macOS platforms
// This should never be called since SimulatePaste checks runtime.GOOS
func simulatePasteMacOSNative() error {
	return fmt.Errorf("simulatePasteMacOSNative is only available on macOS")
}
