package transcribe

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// copyTestBinary copies the test binary under name (plus .exe on Windows) so
// helper processes have a realistic image name.
func copyTestBinary(t *testing.T, dir string, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	data, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// startRoleProcess starts bin as a test helper with the given role.
func startRoleProcess(t *testing.T, bin string, role string, env ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = append(append(os.Environ(), testProcessRoleEnv+"="+role), env...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return cmd
}

func waitUntil(timeout time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func exitedPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), testProcessRoleEnv+"=exit", "YAP_TEST_EXIT_CODE=0")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

func TestWorkerProcessBindsLoopbackAndStopsOnClose(t *testing.T) {
	tmpDir := t.TempDir()
	modelsDir := newTestModelsDir(t, tmpDir)
	argsLog := filepath.Join(tmpDir, "args.log")
	consoleLog := filepath.Join(tmpDir, "console.log")
	t.Setenv(testProcessRoleEnv, "whisper-server")
	t.Setenv("YAP_TEST_FAKE_ARGS_LOG", argsLog)
	t.Setenv("YAP_TEST_CONSOLE_LOG", consoleLog)
	t.Setenv("GGML_BACKEND_PATH", "inherited-from-user-environment")

	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperServerBinary(copyTestBinary(t, tmpDir, "whisper-server"))
	t.Cleanup(func() { _ = engine.Close() })

	text, err := engine.TranscribeWAV(context.Background(), []byte("wav"))
	if err != nil {
		t.Fatal(err)
	}
	if text != "fake transcript" || engine.Backend() != "cpu-worker" {
		t.Fatalf("transcript = %q backend = %q", text, engine.Backend())
	}

	data, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(data)), "\n")
	if host := argValue(args, "--host"); host != "127.0.0.1" {
		t.Fatalf("worker host = %q, want 127.0.0.1; args=%q", host, args)
	}
	if strings.Contains(string(data), "0.0.0.0") {
		t.Fatalf("worker args mention 0.0.0.0: %q", args)
	}
	if runtime.GOOS != "darwin" {
		// Static Windows/Linux builds link the CPU backend in; no backend
		// library path or --no-gpu flag is needed or allowed to leak in.
		if slices.Contains(args, "--no-gpu") {
			t.Fatalf("unexpected --no-gpu: %q", args)
		}
		if !slices.Contains(args, "GGML_BACKEND_PATH=") {
			t.Fatalf("GGML_BACKEND_PATH leaked into worker env: %q", args)
		}
	}
	if state, err := os.ReadFile(consoleLog); err != nil || string(state) != "none" {
		t.Fatalf("worker console state = %q (%v), want none", state, err)
	}

	engine.workerMu.Lock()
	pid := engine.worker.cmd.Process.Pid
	engine.workerMu.Unlock()
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if !waitUntil(3*time.Second, func() bool { return !processAlive(pid) }) {
		t.Fatalf("worker pid=%d still running after Close", pid)
	}
	if _, err := os.Stat(engine.workerPIDPath()); !os.IsNotExist(err) {
		t.Fatalf("pid file not removed after Close: %v", err)
	}
}

func TestProcessAliveAndWhisperServerDetection(t *testing.T) {
	tmpDir := t.TempDir()
	if !processAlive(os.Getpid()) {
		t.Fatal("current process reported dead")
	}
	if pid := exitedPID(t); processAlive(pid) {
		t.Fatalf("exited pid=%d reported alive", pid)
	}

	server := startRoleProcess(t, copyTestBinary(t, tmpDir, "whisper-server"), "sleep")
	other := startRoleProcess(t, copyTestBinary(t, tmpDir, "other-tool"), "sleep")
	if !waitUntil(5*time.Second, func() bool { return isWhisperServerProcess(server.Process.Pid) }) {
		t.Fatal("whisper-server process not detected")
	}
	if !processAlive(other.Process.Pid) {
		t.Fatal("running process reported dead")
	}
	if isWhisperServerProcess(other.Process.Pid) || isWhisperServerProcess(os.Getpid()) {
		t.Fatal("unrelated process detected as whisper-server")
	}
}

func TestIllegalInstructionExitIsExplained(t *testing.T) {
	var crash *exec.Cmd
	if runtime.GOOS == "windows" {
		crash = exec.Command(os.Args[0])
		crash.Env = append(os.Environ(), testProcessRoleEnv+"=exit", "YAP_TEST_EXIT_CODE=0xC000001D")
	} else {
		crash = exec.Command("sh", "-c", "kill -ILL $$")
	}
	err := crash.Run()
	if !isIllegalInstructionExit(err) || !strings.Contains(illegalInstructionHint(err), "AVX2") {
		t.Fatalf("illegal instruction exit not recognised: %v", err)
	}

	failure := exec.Command(os.Args[0])
	failure.Env = append(os.Environ(), testProcessRoleEnv+"=exit", "YAP_TEST_EXIT_CODE=1")
	if err := failure.Run(); err == nil || illegalInstructionHint(err) != "" {
		t.Fatalf("ordinary failure explained as illegal instruction: %v", err)
	}
}
