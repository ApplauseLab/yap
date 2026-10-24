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
	}
}
