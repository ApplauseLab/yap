//go:build windows

package transcribe

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestHideChildConsoleSuppressesConsoleWindow(t *testing.T) {
	probe := func(hide bool) string {
		log := filepath.Join(t.TempDir(), "console.log")
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), testProcessRoleEnv+"=console-probe", "YAP_TEST_CONSOLE_LOG="+log)
		if hide {
			hideChildConsole(cmd)
		}
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if state := probe(false); state != "window" {
		t.Fatalf("control child console state = %q, want window", state)
	}
	if state := probe(true); state != "none" {
		t.Fatalf("hidden child console state = %q, want none", state)
	}
}

func TestWorkerDiesWhenOwnerIsTerminated(t *testing.T) {
	tmpDir := t.TempDir()
	modelsDir := newTestModelsDir(t, tmpDir)
	pidFile := filepath.Join(tmpDir, "worker.pid")
	owner := startRoleProcess(t, os.Args[0], "owner",
		"YAP_TEST_OWNER_MODELS_DIR="+modelsDir,
		"YAP_TEST_OWNER_SERVER="+copyTestBinary(t, tmpDir, "whisper-server"),
		"YAP_TEST_OWNER_PID_FILE="+pidFile,
	)

	var workerPID int
	if !waitUntil(15*time.Second, func() bool {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return false
		}
		workerPID, err = strconv.Atoi(strings.TrimSpace(string(data)))
		return err == nil
	}) {
		t.Fatal("owner did not start a worker")
	}
	if !isWhisperServerProcess(workerPID) {
		t.Fatalf("worker pid=%d is not running", workerPID)
	}

	// TerminateProcess runs no Go shutdown code, like a crash or End task.
	if err := owner.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if !waitUntil(5*time.Second, func() bool { return !processAlive(workerPID) }) {
		t.Fatalf("worker pid=%d survived its terminated owner", workerPID)
	}

	engine := NewLocalEngine(modelsDir)
	engine.ReapOrphanedWorker()
	if _, err := os.Stat(engine.workerPIDPath()); !os.IsNotExist(err) {
		t.Fatalf("stale pid file not removed: %v", err)
	}
}

func TestReapOrphanedWorkerOnWindows(t *testing.T) {
	writeRecord := func(engine *LocalEngine, pid, owner int) {
		t.Helper()
		if err := os.WriteFile(engine.workerPIDPath(), []byte(fmt.Sprintf("%d %d", pid, owner)), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("dead owner", func(t *testing.T) {
		tmpDir := t.TempDir()
		orphan := startRoleProcess(t, copyTestBinary(t, tmpDir, "whisper-server"), "sleep")
		engine := NewLocalEngine(tmpDir)
		writeRecord(engine, orphan.Process.Pid, exitedPID(t))
		engine.ReapOrphanedWorker()
		if !waitForExit(orphan, 3*time.Second) {
			t.Fatal("orphaned whisper-server was not stopped")
		}
		if _, err := os.Stat(engine.workerPIDPath()); !os.IsNotExist(err) {
			t.Fatalf("pid file not removed: %v", err)
		}
	})

	t.Run("live owner", func(t *testing.T) {
		tmpDir := t.TempDir()
		worker := startRoleProcess(t, copyTestBinary(t, tmpDir, "whisper-server"), "sleep")
		engine := NewLocalEngine(tmpDir)
		writeRecord(engine, worker.Process.Pid, os.Getppid())
		engine.ReapOrphanedWorker()
		if waitForExit(worker, 300*time.Millisecond) {
			t.Fatal("worker owned by a live process was stopped")
		}
		if _, err := os.Stat(engine.workerPIDPath()); err != nil {
			t.Fatalf("pid file of live owner removed: %v", err)
		}
	})

	t.Run("unrelated process", func(t *testing.T) {
		tmpDir := t.TempDir()
		unrelated := startRoleProcess(t, copyTestBinary(t, tmpDir, "other-tool"), "sleep")
		engine := NewLocalEngine(tmpDir)
		writeRecord(engine, unrelated.Process.Pid, exitedPID(t))
		engine.ReapOrphanedWorker()
		if waitForExit(unrelated, 300*time.Millisecond) {
			t.Fatal("process that is not whisper-server was stopped")
		}
	})
}
