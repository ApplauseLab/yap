package transcribe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLocalEngineReusesPersistentWorker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test worker launcher uses a shell script")
	}

	tmpDir := t.TempDir()
	modelsDir := filepath.Join(tmpDir, "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "ggml-base.en.bin"), []byte("model"), 0644); err != nil {
		t.Fatal(err)
	}

	startLog := filepath.Join(tmpDir, "starts.log")
	t.Setenv("YAP_TEST_WORKER_START_LOG", startLog)
	launcher := filepath.Join(tmpDir, "whisper-server")
	script := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestWhisperWorkerHelper -- \"$@\"\n", os.Args[0])
	if err := os.WriteFile(launcher, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperServerBinary(launcher)
	t.Cleanup(func() { _ = engine.Close() })

	for i := 0; i < 2; i++ {
		text, err := engine.TranscribeWAV(context.Background(), []byte("wav"))
		if err != nil {
			t.Fatal(err)
		}
		if text != "persistent transcript" {
			t.Fatalf("transcript = %q", text)
		}
	}

	starts, err := os.ReadFile(startLog)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(strings.Fields(string(starts))); got != 1 {
		t.Fatalf("worker starts = %d, want 1", got)
	}
}

func TestLocalEngineFallsBackToPersistentCPUWorker(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("Metal backend is only used on Apple Silicon")
	}

	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	modelsDir := filepath.Join(tmpDir, "models")
	libexecDir := filepath.Join(tmpDir, "libexec")
	for _, dir := range []string{binDir, modelsDir, libexecDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		filepath.Join(modelsDir, "ggml-base.en.bin"),
		filepath.Join(libexecDir, "libggml-metal.so"),
		filepath.Join(libexecDir, preferredAppleCPUBackend()),
	} {
		if err := os.WriteFile(path, []byte("test"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	startLog := filepath.Join(tmpDir, "starts.log")
	t.Setenv("YAP_TEST_WORKER_START_LOG", startLog)
	t.Setenv("YAP_TEST_WORKER_FAIL_METAL", "1")
	launcher := filepath.Join(binDir, "whisper-server")
	script := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestWhisperWorkerHelper -- \"$@\"\n", os.Args[0])
	if err := os.WriteFile(launcher, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperServerBinary(launcher)
	t.Cleanup(func() { _ = engine.Close() })
	for i := 0; i < 2; i++ {
		text, err := engine.TranscribeWAV(context.Background(), []byte("wav"))
		if err != nil {
			t.Fatal(err)
		}
		if text != "persistent transcript" {
			t.Fatalf("transcript = %q", text)
		}
	}

	data, err := os.ReadFile(startLog)
	if err != nil {
		t.Fatal(err)
	}
	attempts := strings.Fields(string(data))
	want := []string{filepath.Join(libexecDir, "libggml-metal.so"), filepath.Join(libexecDir, preferredAppleCPUBackend())}
	if strings.Join(attempts, "\n") != strings.Join(want, "\n") {
		t.Fatalf("worker starts = %q, want %q", attempts, want)
	}
}

func TestClosedLocalEngineDoesNotStartWorker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test worker launcher uses a shell script")
	}

	tmpDir := t.TempDir()
	modelsDir := filepath.Join(tmpDir, "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "ggml-base.en.bin"), []byte("model"), 0644); err != nil {
		t.Fatal(err)
	}
	startLog := filepath.Join(tmpDir, "starts.log")
	t.Setenv("YAP_TEST_WORKER_START_LOG", startLog)
	launcher := filepath.Join(tmpDir, "whisper-server")
	script := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestWhisperWorkerHelper -- \"$@\"\n", os.Args[0])
	if err := os.WriteFile(launcher, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperServerBinary(launcher)
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if err := engine.Prewarm(context.Background()); err == nil {
		t.Fatal("Prewarm succeeded after Close")
	}
	if _, err := os.Stat(startLog); !os.IsNotExist(err) {
		t.Fatalf("worker started after Close: %v", err)
	}
}

func TestCloseCancelsActivePersistentInference(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test worker launcher uses a shell script")
	}

	tmpDir := t.TempDir()
	modelsDir := filepath.Join(tmpDir, "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "ggml-base.en.bin"), []byte("model"), 0644); err != nil {
		t.Fatal(err)
	}
	startLog := filepath.Join(tmpDir, "starts.log")
	requestLog := filepath.Join(tmpDir, "requests.log")
	t.Setenv("YAP_TEST_WORKER_START_LOG", startLog)
	t.Setenv("YAP_TEST_WORKER_REQUEST_LOG", requestLog)
	t.Setenv("YAP_TEST_WORKER_DELAY", "10s")
	launcher := filepath.Join(tmpDir, "whisper-server")
	script := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestWhisperWorkerHelper -- \"$@\"\n", os.Args[0])
	if err := os.WriteFile(launcher, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperServerBinary(launcher)
	cliLog := filepath.Join(tmpDir, "cli.log")
	cli := filepath.Join(tmpDir, "whisper-cli")
	if err := os.WriteFile(cli, []byte("#!/bin/sh\nprintf called > "+cliLog+"\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	engine.SetWhisperBinary(cli)
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

	started := time.Now()
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("Close took %s", elapsed)
	}
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("transcription did not return after Close")
	}
	if _, err := os.Stat(cliLog); !os.IsNotExist(err) {
		t.Fatalf("CLI fallback started after Close: %v", err)
	}
}

func TestWhisperWorkerHelper(t *testing.T) {
	if os.Getenv("YAP_TEST_WORKER_START_LOG") == "" {
		return
	}

	args := helperArgs(os.Args)
	port := argValue(args, "--port")
	requestPath := argValue(args, "--request-path")
	if port == "" || requestPath == "" {
		os.Exit(2)
	}
	file, err := os.OpenFile(os.Getenv("YAP_TEST_WORKER_START_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		os.Exit(2)
	}
	backend := os.Getenv("GGML_BACKEND_PATH")
	if backend == "" {
		backend = "default"
	}
	_, _ = file.WriteString(backend + "\n")
	_ = file.Close()
	if os.Getenv("YAP_TEST_WORKER_FAIL_METAL") == "1" && strings.Contains(backend, "libggml-metal") {
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(requestPath+"/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Server", "whisper.cpp")
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(requestPath+"/inference", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "whisper.cpp")
		if requestLog := os.Getenv("YAP_TEST_WORKER_REQUEST_LOG"); requestLog != "" {
			_ = os.WriteFile(requestLog, []byte("request\n"), 0644)
		}
		if delay := os.Getenv("YAP_TEST_WORKER_DELAY"); delay != "" {
			if duration, err := time.ParseDuration(delay); err == nil {
				time.Sleep(duration)
			}
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_, _ = io.Copy(io.Discard, file)
		_ = file.Close()
		_, _ = io.WriteString(w, "persistent transcript\n")
	})
	if err := http.ListenAndServe("127.0.0.1:"+port, mux); err != nil {
		os.Exit(1)
	}
}

func helperArgs(args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			return args[i+1:]
		}
	}
	return nil
}

func argValue(args []string, key string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}
