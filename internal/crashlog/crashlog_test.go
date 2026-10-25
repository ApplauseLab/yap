package crashlog

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"yap/internal/logger"
)

const childModeEnv = "YAP_CRASHLOG_TEST_CHILD"

// TestMain lets the test binary act as a crashing child process.
func TestMain(m *testing.M) {
	if mode := os.Getenv(childModeEnv); mode != "" {
		runCrashChild(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func runCrashChild(mode string) {
	if err := logger.Init(); err != nil {
		panic(err)
	}
	if _, err := RedirectStderr(logger.GetLogsDir(), "test"); err != nil {
		panic(err)
	}

	switch mode {
	case "goroutine-panic":
		Go("test-worker", func() {
			var items []string
			_ = items[3]
		})
		// Do not let TestMain exit while the worker is still logging its panic.
		time.Sleep(10 * time.Second)
		os.Exit(99)
	case "recovered-panic":
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer Recover("test-leaf")
			panic("leaf failed")
		}()
		<-done
		os.Stderr.WriteString("child still running\n")
	case "fatal-error":
		// A fatal runtime error bypasses recover, like concurrent map writes.
		var mu sync.Mutex
		mu.Unlock()
	case "signal":
		// What an uncaught Objective-C exception (abort) looks like to Go.
		raiseAbort()
		time.Sleep(5 * time.Second)
	}
}

type childResult struct {
	exitCode  int
	stderrLog string
	yapLog    string
}

func runChild(t *testing.T, mode string) childResult {
	t.Helper()
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(),
		childModeEnv+"="+mode,
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"APPDATA="+filepath.Join(home, ".config"),
	)
	// Stderr is a pipe here, as it is /dev/null for an app opened from
	// Finder, so RedirectStderr takes effect.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	output := stderr.Bytes()

	result := childResult{}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		result.exitCode = exitErr.ExitCode()
	default:
		t.Fatalf("running child: %v", err)
	}
	if strings.Contains(string(output), "goroutine ") {
		t.Errorf("crash output leaked to the original stderr:\n%s", output)
	}

	configDir := filepath.Join(home, "Library", "Application Support")
	if _, err := os.Stat(configDir); err != nil {
		configDir = filepath.Join(home, ".config")
	}
	logsDir := filepath.Join(configDir, "yap", "logs")
	result.stderrLog = readMatching(t, logsDir, StderrLogPrefix)
	result.yapLog = readMatching(t, logsDir, "yap-2")
	return result
}

func readMatching(t *testing.T, dir, prefix string) string {
	t.Helper()
	matches, _ := filepath.Glob(filepath.Join(dir, prefix+"*.log"))
	if len(matches) != 1 {
		t.Fatalf("want one %s*.log in %s, found %v", prefix, dir, matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertContains(t *testing.T, name, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("%s does not contain %q:\n%s", name, want, text)
		}
	}
}

func TestGoroutinePanicIsLoggedAndCrashOutputKept(t *testing.T) {
	result := runChild(t, "goroutine-panic")
	if result.exitCode != 2 {
		t.Fatalf("exit code %d, want 2", result.exitCode)
	}
	assertContains(t, "stderr log", result.stderrLog,
		"=== Yap test pid=",
		"panic: runtime error: index out of range [3] with length 0",
		"crashlog.runCrashChild.func1",
	)
	assertContains(t, "Yap log", result.yapLog,
		"panic in test-worker (process will exit): runtime error: index out of range",
		"crashlog.runCrashChild.func1",
	)
}

func TestRecoverLogsPanicAndKeepsRunning(t *testing.T) {
	result := runChild(t, "recovered-panic")
	if result.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", result.exitCode)
	}
	assertContains(t, "Yap log", result.yapLog, "panic in test-leaf (recovered): leaf failed")
	assertContains(t, "stderr log", result.stderrLog, "child still running")
}

func TestFatalRuntimeErrorIsKept(t *testing.T) {
	result := runChild(t, "fatal-error")
	if result.exitCode != 2 {
		t.Fatalf("exit code %d, want 2", result.exitCode)
	}
	assertContains(t, "stderr log", result.stderrLog, "fatal error: sync: unlock of unlocked mutex", "goroutine 1")
}

func TestSignalCrashIsKept(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no SIGABRT on Windows")
	}
	result := runChild(t, "signal")
	if result.exitCode != 2 {
		t.Fatalf("exit code %d, want 2", result.exitCode)
	}
	assertContains(t, "stderr log", result.stderrLog, "SIGABRT: abort", "goroutine 1")
}

func TestStartSessionReportsDeadSessionsOnly(t *testing.T) {
	dir := t.TempDir()

	dead := exec.Command(os.Args[0], "-test.run=^$")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	deadPID := dead.Process.Pid
	livePID := os.Getppid()
	writeMarker(t, dir, deadPID, "version=0.3.0 started=earlier")
	writeMarker(t, dir, livePID, "version=other")

	session, unclean, err := StartSession(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(unclean) != 1 || unclean[0].PID != deadPID || unclean[0].Details != "version=0.3.0 started=earlier" {
		t.Fatalf("unclean sessions = %+v, want only pid %d", unclean, deadPID)
	}
	if fileExists(markerPath(dir, deadPID)) {
		t.Error("dead session marker was not removed")
	}
	if !fileExists(markerPath(dir, livePID)) {
		t.Error("marker of a running process was removed")
	}
	if !fileExists(markerPath(dir, os.Getpid())) {
		t.Fatal("current session marker was not written")
	}

	session.End()
	if fileExists(markerPath(dir, os.Getpid())) {
		t.Error("End did not remove the current session marker")
	}
}

func markerPath(dir string, pid int) string {
	return filepath.Join(dir, sessionMarkerPrefix+strconv.Itoa(pid))
}

func writeMarker(t *testing.T, dir string, pid int, details string) {
	t.Helper()
	if err := os.WriteFile(markerPath(dir, pid), []byte(details), 0644); err != nil {
		t.Fatal(err)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
