//go:build !darwin || !cgo

package crashlog

// LogAppLifecycle is a no-op outside macOS cgo builds.
func LogAppLifecycle() {}
