package transcribe

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testProcessRoleEnv makes the test binary act as a helper process. Unlike the
// shell-script launchers it works on Windows, where the binary is copied to
// whisper-server.exe and invoked with real whisper-server arguments.
const testProcessRoleEnv = "YAP_TEST_PROCESS_ROLE"

func TestMain(m *testing.M) {
	RunWorkerWatchdogIfRequested()
	switch os.Getenv(testProcessRoleEnv) {
	case "whisper-server":
		os.Exit(runFakeWhisperServer(os.Args[1:]))
	case "sleep":
		time.Sleep(60 * time.Second)
		os.Exit(0)
	case "exit":
		code, _ := strconv.ParseInt(os.Getenv("YAP_TEST_EXIT_CODE"), 0, 64)
		os.Exit(int(code))
	case "console-probe":
		os.Exit(writeConsoleState(os.Getenv("YAP_TEST_CONSOLE_LOG")))
	case "console-launcher":
		os.Exit(runConsoleLauncher())
	case "owner":
		os.Exit(runFakeOwner())
	case "plain-owner":
		os.Exit(runPlainOwner())
	}
	os.Exit(m.Run())
}

// runFakeWhisperServer mimics the whisper-server endpoints Yap uses.
func runFakeWhisperServer(args []string) int {
	if path := os.Getenv("YAP_TEST_FAKE_ARGS_LOG"); path != "" {
		record := strings.Join(args, "\n") + "\nGGML_BACKEND_PATH=" + os.Getenv("GGML_BACKEND_PATH") + "\n"
		if err := os.WriteFile(path, []byte(record), 0o644); err != nil {
			return 2
		}
	}
	if code := writeConsoleState(os.Getenv("YAP_TEST_CONSOLE_LOG")); code != 0 {
		return code
	}
	host, port, requestPath := argValue(args, "--host"), argValue(args, "--port"), argValue(args, "--request-path")
	if host == "" || port == "" || requestPath == "" {
		return 2
	}
	mux := http.NewServeMux()
	mux.HandleFunc(requestPath+"/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Server", "whisper.cpp")
	})
	mux.HandleFunc(requestPath+"/inference", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "whisper.cpp")
		file, _, err := r.FormFile("file")
		if err != nil || r.FormValue("response_format") != "text" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_, _ = io.Copy(io.Discard, file)
		_ = file.Close()
		_, _ = io.WriteString(w, "fake transcript\n")
	})
	if err := http.ListenAndServe(net.JoinHostPort(host, port), mux); err != nil {
		return 1
	}
	return 0
}

// runFakeOwner stands in for a Yap process that starts a worker and is then
// terminated without running any shutdown code.
func runFakeOwner() int {
	if err := os.Setenv(testProcessRoleEnv, "whisper-server"); err != nil {
		return 2
	}
	engine := NewLocalEngine(os.Getenv("YAP_TEST_OWNER_MODELS_DIR"))
	engine.SetWhisperServerBinary(os.Getenv("YAP_TEST_OWNER_SERVER"))
	if err := engine.Prewarm(context.Background()); err != nil {
		return 3
	}
	engine.workerMu.Lock()
	pid := engine.worker.cmd.Process.Pid
	engine.workerMu.Unlock()
	if err := os.WriteFile(os.Getenv("YAP_TEST_OWNER_PID_FILE"), []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return 4
	}
	time.Sleep(60 * time.Second)
	return 0
}

// runPlainOwner starts a child without binding it to its lifetime, as a
// control for runFakeOwner.
func runPlainOwner() int {
	cmd := exec.Command(os.Getenv("YAP_TEST_OWNER_SERVER"))
	cmd.Env = append(os.Environ(), testProcessRoleEnv+"=sleep")
	if err := cmd.Start(); err != nil {
		return 3
	}
	if err := os.WriteFile(os.Getenv("YAP_TEST_OWNER_PID_FILE"), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		return 4
	}
	time.Sleep(60 * time.Second)
	return 0
}
