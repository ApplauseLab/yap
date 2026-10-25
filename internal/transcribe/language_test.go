package transcribe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"yap/internal/models"
)

func TestWhisperLanguageFollowsCatalog(t *testing.T) {
	for _, spec := range models.Catalog() {
		want := "auto"
		if spec.EnglishOnly {
			want = "en"
		}
		if got := Model(spec.Name).WhisperLanguage(); got != want {
			t.Errorf("%s: language = %q, want %q", spec.Name, got, want)
		}
	}
	if got := Model("custom.en").WhisperLanguage(); got != "en" {
		t.Errorf("unknown .en model: language = %q, want en", got)
	}
	if got := Model("custom").WhisperLanguage(); got != "auto" {
		t.Errorf("unknown model: language = %q, want auto", got)
	}
}

func TestPersistentWorkerSendsModelLanguage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test worker launcher uses a shell script")
	}
	tmpDir := t.TempDir()
	modelsDir := filepath.Join(tmpDir, "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ggml-base.en.bin", "ggml-small.bin"} {
		if err := os.WriteFile(filepath.Join(modelsDir, name), []byte("model"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("YAP_TEST_WORKER_START_LOG", filepath.Join(tmpDir, "starts.log"))
	languageLog := filepath.Join(tmpDir, "languages.log")
	t.Setenv("YAP_TEST_WORKER_LANGUAGE_LOG", languageLog)
	launcher := filepath.Join(tmpDir, "whisper-server")
	script := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestWhisperWorkerHelper -- \"$@\"\n", os.Args[0])
	if err := os.WriteFile(launcher, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperServerBinary(launcher)
	t.Cleanup(func() { _ = engine.Close() })

	for _, model := range []Model{ModelBaseEn, ModelSmall} {
		if err := engine.SetModel(model); err != nil {
			t.Fatal(err)
		}
		if _, err := engine.TranscribeWAV(context.Background(), []byte("wav")); err != nil {
			t.Fatalf("%s: %v", model, err)
		}
	}

	data, err := os.ReadFile(languageLog)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(string(data)); strings.Join(got, ",") != "en,auto" {
		t.Fatalf("languages sent = %v, want [en auto]", got)
	}
}

func TestCLIFallbackPassesModelLanguage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test whisper-cli is a shell script")
	}
	tmpDir := t.TempDir()
	modelsDir := filepath.Join(tmpDir, "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ggml-tiny.en.bin", "ggml-medium.bin"} {
		if err := os.WriteFile(filepath.Join(modelsDir, name), []byte("model"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	languageLog := filepath.Join(tmpDir, "languages.log")
	whisperBin := filepath.Join(tmpDir, "whisper-cli")
	script := `#!/bin/sh
lang=""
out=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -l) shift; lang="$1" ;;
    -of) shift; out="$1" ;;
  esac
  shift
done
echo "$lang" >> "` + languageLog + `"
printf 'cli transcript\n' > "$out.txt"
`
	if err := os.WriteFile(whisperBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperBinary(whisperBin)
	engine.SetWhisperServerBinary(filepath.Join(tmpDir, "missing-whisper-server"))

	for _, model := range []Model{ModelTinyEn, ModelMedium} {
		if err := engine.SetModel(model); err != nil {
			t.Fatal(err)
		}
		text, err := engine.TranscribeWAV(context.Background(), []byte("wav"))
		if err != nil {
			t.Fatalf("%s: %v", model, err)
		}
		if text != "cli transcript" {
			t.Fatalf("%s: transcript = %q", model, text)
		}
	}

	data, err := os.ReadFile(languageLog)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(data))
	if len(got) == 0 || got[0] != "en" || got[len(got)-1] != "auto" {
		t.Fatalf("languages passed = %v, want tiny.en=en then medium=auto", got)
	}
	for _, lang := range got {
		if lang != "en" && lang != "auto" {
			t.Fatalf("unexpected language %q in %v", lang, got)
		}
	}
}
