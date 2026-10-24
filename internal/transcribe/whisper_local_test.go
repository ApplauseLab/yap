package transcribe

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWhisperEnvUsesBackendFile(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	libexecDir := filepath.Join(tmpDir, "libexec")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libexecDir, 0755); err != nil {
		t.Fatal(err)
	}

	backendPath := filepath.Join(libexecDir, preferredAppleCPUBackend())
	if err := os.WriteFile(backendPath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	metalPath := filepath.Join(libexecDir, "libggml-metal.so")
	if err := os.WriteFile(metalPath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	engine := NewLocalEngine(tmpDir)
	env := engine.whisperEnv([]string{}, filepath.Join(binDir, "whisper-cli"))

	got := envValue(env, "GGML_BACKEND_PATH")
	if got != backendPath {
		t.Fatalf("GGML_BACKEND_PATH = %q, want %q", got, backendPath)
	}
}

func TestTranscribeWAVFallsBackFromMetalAndCachesFailure(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("Metal backend is only used on Apple Silicon")
	}

	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	libexecDir := filepath.Join(tmpDir, "libexec")
	modelsDir := filepath.Join(tmpDir, "models")
	for _, dir := range []string{binDir, libexecDir, modelsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	metalPath := filepath.Join(libexecDir, "libggml-metal.so")
	cpuPath := filepath.Join(libexecDir, preferredAppleCPUBackend())
	for _, path := range []string{metalPath, cpuPath, filepath.Join(modelsDir, "ggml-base.en.bin")} {
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	attemptLog := filepath.Join(tmpDir, "attempts.log")
	whisperBin := filepath.Join(binDir, "whisper-cli")
	script := `#!/bin/sh
echo "$GGML_BACKEND_PATH" >> "` + attemptLog + `"
case "$GGML_BACKEND_PATH" in
  *libggml-metal.so) exit 1 ;;
esac
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-of" ]; then
    shift
    printf 'fallback transcript\n' > "$1.txt"
    exit 0
  fi
  shift
done
exit 2
`
	if err := os.WriteFile(whisperBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperBinary(whisperBin)
	engine.SetWhisperServerBinary(filepath.Join(tmpDir, "missing-whisper-server"))

	for i := 0; i < 2; i++ {
		text, err := engine.TranscribeWAV(context.Background(), []byte("wav"))
		if err != nil {
			t.Fatal(err)
		}
		if text != "fallback transcript" {
			t.Fatalf("transcript = %q", text)
		}
	}

	data, err := os.ReadFile(attemptLog)
	if err != nil {
		t.Fatal(err)
	}
	attempts := strings.Fields(string(data))
	want := []string{metalPath, cpuPath, cpuPath}
	if strings.Join(attempts, "\n") != strings.Join(want, "\n") {
		t.Fatalf("backend attempts = %q, want %q", attempts, want)
	}
}
