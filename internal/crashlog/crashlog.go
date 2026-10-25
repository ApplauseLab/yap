// Package crashlog keeps evidence of abnormal exits.
//
// Go writes unrecovered panics, fatal runtime errors and crashes in native
// code (reported as "signal arrived during cgo execution") to stderr and then
// exits with status 2. macOS writes no crash report for these exits, and an
// app started from Finder, the Dock or a launcher has its stderr discarded, so
// the cause is lost. This package sends stderr to a file in the Yap logs
// directory, logs panics in Yap goroutines with their stack, and records
// sessions that end without a clean shutdown.
package crashlog

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"yap/internal/logger"
)

// StderrLogPrefix names the daily files that receive the process stderr.
const StderrLogPrefix = "yap-stderr-"

// RedirectStderr points the process stderr at a daily file in logsDir so that
// Go runtime crash output survives when nobody is reading stderr. It also
// asks the runtime to print the stacks of all goroutines on a crash.
//
// Stderr is left alone when it is a terminal (wails dev, go run), where the
// output is already visible. It returns the log path, or "" when stderr was
// not redirected.
func RedirectStderr(logsDir, version string) (string, error) {
	debug.SetTraceback("all")

	if stderrIsTerminal() {
		return "", nil
	}
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return "", err
	}

	path := filepath.Join(logsDir, fmt.Sprintf("%s%s.log", StderrLogPrefix, time.Now().Format("2006-01-02")))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(file, "=== Yap %s pid=%d started %s; stderr follows ===\n", version, os.Getpid(), time.Now().Format(time.RFC3339))

	if err := redirectStderrTo(file); err != nil {
		file.Close()
		return "", err
	}
	// The descriptor now backs stderr for the rest of the process lifetime.
	os.Stderr = file
	return path, nil
}

// LogPanic logs a panic in the named goroutine with its stack trace and then
// re-panics, so the process still crashes but the cause is in the Yap log.
// Use it where state may be left inconsistent (for example a mutex held while
// panicking). It must be called directly by defer:
//
//	defer crashlog.LogPanic("transcribe")
func LogPanic(name string) {
	if r := recover(); r != nil {
		logPanic(name, r, "process will exit")
		panic(r)
	}
}

// Recover logs a panic in the named goroutine with its stack trace and lets
// the goroutine end without crashing the app. Use it only for self-contained
// work that holds no shared locks. It must be called directly by defer:
//
//	defer crashlog.Recover("auto-paste")
func Recover(name string) {
	if r := recover(); r != nil {
		logPanic(name, r, "recovered")
	}
}

// Go runs fn in a new goroutine that logs a panic and then crashes.
func Go(name string, fn func()) {
	go func() {
		defer LogPanic(name)
		fn()
	}()
}

func logPanic(name string, value any, outcome string) {
	logger.Error(fmt.Sprintf("panic in %s (%s): %v\n%s", name, outcome, value, debug.Stack()))
}
