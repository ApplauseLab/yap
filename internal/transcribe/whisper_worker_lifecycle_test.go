package transcribe

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// startFakeWhisperServer runs a long-lived process whose executable is named
// whisper-server, matching what the reaper looks for.
func startFakeWhisperServer(t *testing.T, dir string) *exec.Cmd {
	t.Helper()
	// Copies of signed system binaries are killed on macOS, so reuse the
	// linker-signed test binary under the whisper-server name.
	data, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(dir, "whisper-server")
	if err := os.WriteFile(fake, data, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(fake, "-test.run=^TestFakeWhisperServerHelper$")
	cmd.Env = append(os.Environ(), "YAP_TEST_FAKE_SERVER=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !isWhisperServerProcess(cmd.Process.Pid) {
		if time.Now().After(deadline) {
			t.Fatal("fake whisper-server did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return cmd
}

func TestFakeWhisperServerHelper(t *testing.T) {
	if os.Getenv("YAP_TEST_FAKE_SERVER") != "1" {
		return
	}
	time.Sleep(60 * time.Second)
	os.Exit(0)
}

func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

func waitForExit(cmd *exec.Cmd, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestReapOrphanedWorkerStopsWorkerOfDeadOwner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses unix sleep binary")
	}
	tmpDir := t.TempDir()
	orphan := startFakeWhisperServer(t, tmpDir)
	engine := NewLocalEngine(tmpDir)
	record := fmt.Sprintf("%d %d", orphan.Process.Pid, deadPID(t))
	if err := os.WriteFile(engine.workerPIDPath(), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}

	engine.ReapOrphanedWorker()

	if !waitForExit(orphan, 3*time.Second) {
		t.Fatal("orphaned whisper-server was not stopped")
	}
	if _, err := os.Stat(engine.workerPIDPath()); !os.IsNotExist(err) {
		t.Fatalf("pid file not removed: %v", err)
	}
}

func TestReapOrphanedWorkerKeepsWorkerOfLiveOwner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses unix sleep binary")
	}
	tmpDir := t.TempDir()
	worker := startFakeWhisperServer(t, tmpDir)
	engine := NewLocalEngine(tmpDir)
	record := fmt.Sprintf("%d %d", worker.Process.Pid, os.Getppid())
	if err := os.WriteFile(engine.workerPIDPath(), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}

	engine.ReapOrphanedWorker()

	if waitForExit(worker, 300*time.Millisecond) {
		t.Fatal("worker owned by a live Yap process was stopped")
	}
	if _, err := os.Stat(engine.workerPIDPath()); err != nil {
		t.Fatalf("pid file of live owner removed: %v", err)
	}
}

func TestReapOrphanedWorkerIgnoresUnrelatedProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses unix sleep binary")
	}
	tmpDir := t.TempDir()
	unrelated := exec.Command("sleep", "60")
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = unrelated.Process.Kill()
		_, _ = unrelated.Process.Wait()
	})
	engine := NewLocalEngine(tmpDir)
	record := fmt.Sprintf("%d %d", unrelated.Process.Pid, deadPID(t))
	if err := os.WriteFile(engine.workerPIDPath(), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}

	engine.ReapOrphanedWorker()

	if waitForExit(unrelated, 300*time.Millisecond) {
		t.Fatal("process that is not whisper-server was stopped")
	}
}

func TestPersistentWorkerRecordsAndClearsPIDFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test worker launcher uses a shell script")
	}
	tmpDir := t.TempDir()
	modelsDir := newTestModelsDir(t, tmpDir)
	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperServerBinary(newTestWorkerLauncher(t, tmpDir))
	t.Cleanup(func() { _ = engine.Close() })

	if err := engine.Prewarm(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(engine.workerPIDPath())
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 || fields[1] != fmt.Sprint(os.Getpid()) {
		t.Fatalf("pid file = %q", data)
	}

	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(engine.workerPIDPath()); !os.IsNotExist(err) {
		t.Fatalf("pid file not removed after Close: %v", err)
	}
}

func TestSetModelWithCurrentModelKeepsActiveInference(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test worker launcher uses a shell script")
	}
	tmpDir := t.TempDir()
	modelsDir := newTestModelsDir(t, tmpDir)
	requestLog := filepath.Join(tmpDir, "requests.log")
	t.Setenv("YAP_TEST_WORKER_REQUEST_LOG", requestLog)
	t.Setenv("YAP_TEST_WORKER_DELAY", "300ms")
	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperServerBinary(newTestWorkerLauncher(t, tmpDir))
	t.Cleanup(func() { _ = engine.Close() })

	result := make(chan error, 1)
	go func() {
		_, err := engine.TranscribeWAV(context.Background(), []byte("wav"))
		result <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(requestLog); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not receive inference request")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := engine.SetModel(engine.GetModel()); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatalf("transcription cancelled by no-op SetModel: %v", err)
	}
}

func TestLockedBufferKeepsMostRecentOutput(t *testing.T) {
	buf := &lockedBuffer{}
	chunk := strings.Repeat("a", workerDiagnosticsLimit/2)
	for i := 0; i < 4; i++ {
		if _, err := buf.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := buf.Write([]byte("tail")); err != nil {
		t.Fatal(err)
	}
	buf.mu.Lock()
	size := buf.buf.Len()
	tail := strings.HasSuffix(buf.buf.String(), "tail")
	buf.mu.Unlock()
	if size > workerDiagnosticsLimit {
		t.Fatalf("buffer size = %d, limit %d", size, workerDiagnosticsLimit)
	}
	if !tail {
		t.Fatal("most recent output was not retained")
	}
}

func newTestModelsDir(t *testing.T, tmpDir string) string {
	t.Helper()
	modelsDir := filepath.Join(tmpDir, "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "ggml-base.en.bin"), []byte("model"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YAP_TEST_WORKER_START_LOG", filepath.Join(tmpDir, "starts.log"))
	return modelsDir
}

func newTestWorkerLauncher(t *testing.T, tmpDir string) string {
	t.Helper()
	launcher := filepath.Join(tmpDir, "whisper-server")
	script := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestWhisperWorkerHelper -- \"$@\"\n", os.Args[0])
	if err := os.WriteFile(launcher, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return launcher
}
