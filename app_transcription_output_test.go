package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"yap/internal/models"
	"yap/internal/paths"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func outputTestApp(t *testing.T) *App {
	t.Helper()
	t.Setenv(paths.ConfigDirEnv, t.TempDir())
	manager, err := models.NewConfigManager()
	if err != nil {
		t.Fatal(err)
	}
	return &App{configManager: manager, state: StateReady}
}

func TestTranscriptionFilesPreserveBytesAndNeverOverwrite(t *testing.T) {
	dir := t.TempDir()
	texts := []string{"  Café — 你好 👋\n\nمرحبا\r\nПривіт\tend  \n", "No trailing newline", ""}
	var wg sync.WaitGroup
	// Simultaneous saves within the same second must all receive unique paths.
	for i := 0; i < 20; i++ {
		for _, text := range texts {
			wg.Add(1)
			go func(text string) {
				defer wg.Done()
				path, err := writeTranscriptionFile(dir, text)
				if err != nil {
					t.Error(err)
					return
				}
				if !strings.HasPrefix(filepath.Base(path), "transcription-") || filepath.Ext(path) != ".txt" {
					t.Errorf("unexpected filename: %q", path)
				}
				data, err := os.ReadFile(path)
				if err != nil || string(data) != text {
					t.Errorf("saved bytes = %q, error = %v; want %q", data, err, text)
				}
			}(text)
		}
	}
	wg.Wait()
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 60 {
		t.Fatalf("saved %d files, error = %v; want 60", len(entries), err)
	}
}

func TestTranscriptionOutputSettingsAndCancellation(t *testing.T) {
	a := outputTestApp(t)
	if err := a.SetSaveTranscriptions(true); err == nil || a.configManager.Get().SaveTranscriptions {
		t.Fatal("enabled automatic saves without an output folder")
	}
	dir := t.TempDir()
	selected, err := a.chooseTranscriptionOutputDir(func(_ context.Context, options runtime.OpenDialogOptions) (string, error) {
		if options.Title != "Choose transcription output folder" || !options.CanCreateDirectories {
			t.Fatalf("dialog options = %+v", options)
		}
		if !a.mu.TryLock() {
			t.Fatal("app mutex held while choosing a directory")
		}
		a.mu.Unlock()
		return dir, nil
	})
	if err != nil || selected != dir || a.configManager.Get().SaveTranscriptions {
		t.Fatalf("select folder = %q, %v; must not implicitly enable saving", selected, err)
	}
	if err := a.SetSaveTranscriptions(true); err != nil {
		t.Fatal(err)
	}
	selected, err = a.chooseTranscriptionOutputDir(func(_ context.Context, options runtime.OpenDialogOptions) (string, error) {
		if options.DefaultDirectory != dir {
			t.Fatalf("default directory = %q, want %q", options.DefaultDirectory, dir)
		}
		return "", nil
	})
	if err != nil || selected != "" {
		t.Fatalf("cancel = %q, %v", selected, err)
	}
	reloaded, err := models.NewConfigManager()
	if err != nil || !reloaded.Get().SaveTranscriptions || reloaded.Get().TranscriptionOutputDir != dir {
		t.Fatalf("settings did not persist: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("folder validation left probe files: %v", entries)
	}
	if err := a.SetSaveTranscriptions(false); err != nil || a.configManager.Get().TranscriptionOutputDir != dir {
		t.Fatalf("disabling must preserve the folder: %v", err)
	}
}

func TestTranscriptionOutputChooserFailures(t *testing.T) {
	a := outputTestApp(t)
	original := t.TempDir()
	if err := a.configManager.SetTranscriptionOutputDir(original); err != nil {
		t.Fatal(err)
	}
	dialogErr := errors.New("dialog unavailable")
	file := filepath.Join(t.TempDir(), "not-a-folder")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		dir  string
		err  error
	}{
		{"dialog failure", "", dialogErr},
		{"not a folder", file, nil},
		{"missing folder", filepath.Join(t.TempDir(), "missing"), nil},
		{"relative folder", "relative", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected, err := a.chooseTranscriptionOutputDir(func(context.Context, runtime.OpenDialogOptions) (string, error) { return tc.dir, tc.err })
			if err == nil || selected != "" || a.configManager.Get().TranscriptionOutputDir != original {
				t.Fatalf("failed choice changed settings: %q, %v", selected, err)
			}
			if tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("lost dialog error: %v", err)
			}
		})
	}
	if err := os.Remove(original); err != nil {
		t.Fatal(err)
	}
	_, err := a.chooseTranscriptionOutputDir(func(_ context.Context, options runtime.OpenDialogOptions) (string, error) {
		if options.DefaultDirectory != "" {
			t.Fatal("removed output folder prevents reopening the picker")
		}
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutomaticOutputUsesCompletionSettingsAndRecovers(t *testing.T) {
	a := outputTestApp(t)
	dir := t.TempDir()
	if err := a.configManager.SetTranscriptionOutputDir(dir); err != nil {
		t.Fatal(err)
	}
	a.saveTranscriptionOutput("Disabled")
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("created a file while automatic saving was disabled")
	}
	if err := a.SetSaveTranscriptions(true); err != nil {
		t.Fatal(err)
	}
	// Settings changed while transcription was running: use the destination
	// selected at completion, not a stale snapshot from recording start.
	newDir := t.TempDir()
	if err := a.configManager.SetTranscriptionOutputDir(newDir); err != nil {
		t.Fatal(err)
	}
	a.saveTranscriptionOutput("First")
	entries, _ = os.ReadDir(newDir)
	if len(entries) != 1 {
		t.Fatal("did not use the current output folder")
	}
	if err := os.RemoveAll(newDir); err != nil {
		t.Fatal(err)
	}
	a.lastTranscript = "Keep this transcription"
	a.history = []HistoryItem{{ID: "latest", Text: a.lastTranscript}}
	a.saveTranscriptionOutput(a.lastTranscript)
	if !strings.Contains(a.transcriptionOutputError, "Could not save transcription") {
		t.Fatalf("missing visible output error: %q", a.transcriptionOutputError)
	}
	if a.state != StateReady || a.lastError != "" || a.lastTranscript != "Keep this transcription" || len(a.history) != 1 {
		t.Fatal("output failure damaged successful transcription state/history")
	}
	if err := os.Mkdir(newDir, 0700); err != nil {
		t.Fatal(err)
	}
	a.saveTranscriptionOutput("Recovered")
	if a.transcriptionOutputError != "" {
		t.Fatal("successful save did not clear the previous error")
	}
	a.transcriptionOutputError = "Previous failure"
	if err := a.SetSaveTranscriptions(false); err != nil || a.transcriptionOutputError != "" {
		t.Fatalf("disabling did not clear the error: %v", err)
	}
}

func TestTranscriptionOutputRejectsInvalidDestinations(t *testing.T) {
	for _, dir := range []string{"", "relative", filepath.Join(t.TempDir(), "missing")} {
		if path, err := writeTranscriptionFile(dir, "keep this"); err == nil || path != "" {
			t.Fatalf("write to %q = %q, %v", dir, path, err)
		}
	}
}
