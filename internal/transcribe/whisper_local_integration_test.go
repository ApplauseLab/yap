package transcribe

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLocalEngineTranscribeWAVReturnsTranscriptOnly(t *testing.T) {
	if os.Getenv("YAP_TEST_WHISPER_WAV") == "" {
		t.Skip("set YAP_TEST_WHISPER_WAV to run")
	}

	engine := NewLocalEngine(os.Getenv("YAP_TEST_WHISPER_MODELS_DIR"))
	model := Model(os.Getenv("YAP_TEST_WHISPER_MODEL"))
	if model == "" {
		model = ModelTinyEn
	}
	engine.SetModel(model)
	engine.SetWhisperBinary(os.Getenv("YAP_TEST_WHISPER_BIN"))
	if serverBin := os.Getenv("YAP_TEST_WHISPER_SERVER"); serverBin != "" {
		engine.SetWhisperServerBinary(serverBin)
	}
	defer engine.Close()

	wavData, err := os.ReadFile(os.Getenv("YAP_TEST_WHISPER_WAV"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for i := 0; i < 2; i++ {
		started := time.Now()
		text, err := engine.TranscribeWAV(ctx, wavData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("transcription %d backend=%s elapsed=%s text=%q", i+1, engine.Backend(), time.Since(started), text)
		if strings.Contains(text, "ggml_backend") || strings.Contains(text, "whisper_model_load") {
			t.Fatalf("transcript includes whisper diagnostics: %q", text)
		}
		if text == "" {
			t.Fatal("empty transcript")
		}
		if want := os.Getenv("YAP_TEST_EXPECT_BACKEND"); want != "" && engine.Backend() != want {
			t.Fatalf("backend = %q, want %q", engine.Backend(), want)
		}
		if want := os.Getenv("YAP_TEST_EXPECT_TEXT"); want != "" && !strings.Contains(strings.ToLower(text), want) {
			t.Fatalf("transcript %q does not contain %q", text, want)
		}
	}
}

// TestLocalEngineLeavesNoWorkerOrTempFiles runs the full local transcription
// path against real whisper.cpp binaries and checks that nothing is left
// behind: no worker process and no temp files.
func TestLocalEngineLeavesNoWorkerOrTempFiles(t *testing.T) {
	if os.Getenv("YAP_TEST_WHISPER_WAV") == "" {
		t.Skip("set YAP_TEST_WHISPER_WAV to run")
	}
	tempDir := t.TempDir()
	t.Setenv("TMP", tempDir)
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("TEMP", tempDir)

	engine := NewLocalEngine(os.Getenv("YAP_TEST_WHISPER_MODELS_DIR"))
	t.Cleanup(func() { _ = engine.Close() })
	if model := Model(os.Getenv("YAP_TEST_WHISPER_MODEL")); model != "" {
		engine.SetModel(model)
	}
	engine.SetWhisperBinary(os.Getenv("YAP_TEST_WHISPER_BIN"))
	if serverBin := os.Getenv("YAP_TEST_WHISPER_SERVER"); serverBin != "" {
		engine.SetWhisperServerBinary(serverBin)
	}
	wavData, err := os.ReadFile(os.Getenv("YAP_TEST_WHISPER_WAV"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := engine.TranscribeWAV(ctx, wavData); err != nil {
		t.Fatal(err)
	}
	workerPID := 0
	engine.workerMu.Lock()
	if engine.worker != nil {
		workerPID = engine.worker.cmd.Process.Pid
	}
	engine.workerMu.Unlock()
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if workerPID != 0 && !waitUntil(3*time.Second, func() bool { return !processAlive(workerPID) }) {
		t.Fatalf("worker pid=%d still running after Close", workerPID)
	}
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Errorf("temp file left behind: %s", entry.Name())
	}
}
