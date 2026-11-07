package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SetSaveTranscriptions enables automatic text files for future transcriptions.
func (a *App) SetSaveTranscriptions(enabled bool) error {
	if a.configManager == nil {
		return fmt.Errorf("configuration is not ready")
	}
	if enabled {
		if err := checkTranscriptionOutputDir(a.configManager.Get().TranscriptionOutputDir); err != nil {
			return err
		}
	}
	if err := a.configManager.SetSaveTranscriptions(enabled); err != nil {
		return fmt.Errorf("could not save transcription settings: %w", err)
	}
	a.clearTranscriptionOutputError()
	return nil
}

// ChooseTranscriptionOutputDir selects and persists the automatic-save folder.
// Cancellation returns an empty path and leaves the settings untouched.
func (a *App) ChooseTranscriptionOutputDir() (string, error) {
	return a.chooseTranscriptionOutputDir(runtime.OpenDirectoryDialog)
}

func (a *App) chooseTranscriptionOutputDir(choose func(context.Context, runtime.OpenDialogOptions) (string, error)) (string, error) {
	if a.configManager == nil {
		return "", fmt.Errorf("configuration is not ready")
	}
	options := runtime.OpenDialogOptions{Title: "Choose transcription output folder", CanCreateDirectories: true}
	previous := a.configManager.Get().TranscriptionOutputDir
	// A removed or disconnected folder must not prevent choosing its replacement.
	if info, err := os.Stat(previous); err == nil && info.IsDir() {
		options.DefaultDirectory = previous
	}
	dir, err := choose(a.ctx, options)
	if err != nil {
		return "", fmt.Errorf("could not choose transcription output folder: %w", err)
	}
	if dir == "" {
		return "", nil
	}
	if err := checkTranscriptionOutputDir(dir); err != nil {
		return "", err
	}
	if err := a.configManager.SetTranscriptionOutputDir(dir); err != nil {
		return "", fmt.Errorf("could not save transcription output folder: %w", err)
	}
	a.clearTranscriptionOutputError()
	return dir, nil
}

func checkTranscriptionOutputDir(dir string) error {
	if dir == "" || !filepath.IsAbs(dir) {
		return fmt.Errorf("choose a transcription output folder first")
	}
	// Test actual access instead of relying on mode bits (ACLs and mounted
	// volumes can make those misleading). The probe never contains user text.
	f, err := os.CreateTemp(dir, ".yap-write-check-*")
	if err != nil {
		return fmt.Errorf("transcription output folder is not writable: %w", err)
	}
	closeErr := f.Close()
	removeErr := os.Remove(f.Name())
	if closeErr != nil {
		return fmt.Errorf("could not check transcription output folder: %w", closeErr)
	}
	if removeErr != nil {
		return fmt.Errorf("could not clean up transcription output folder check: %w", removeErr)
	}
	return nil
}

func (a *App) clearTranscriptionOutputError() {
	a.mu.Lock()
	a.transcriptionOutputError = ""
	a.mu.Unlock()
	a.emitState()
}

// saveTranscriptionOutput runs only after successful transcription, outside the
// app mutex. Failure is separate from recording state: History and clipboard
// remain available even if the destination is full or disconnected.
func (a *App) saveTranscriptionOutput(text string) {
	config := a.configManager.Get()
	if !config.SaveTranscriptions {
		return
	}
	_, err := writeTranscriptionFile(config.TranscriptionOutputDir, text)
	a.mu.Lock()
	defer a.mu.Unlock()
	current := a.configManager.Get()
	if !current.SaveTranscriptions || current.TranscriptionOutputDir != config.TranscriptionOutputDir {
		return
	}
	if err != nil {
		a.transcriptionOutputError = fmt.Sprintf("Could not save transcription to %q: %v", config.TranscriptionOutputDir, err)
	} else {
		a.transcriptionOutputError = ""
	}
}

func writeTranscriptionFile(dir, text string) (string, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return "", fmt.Errorf("choose a transcription output folder in Settings")
	}
	// CreateTemp uses exclusive creation: even simultaneous recordings never
	// overwrite each other or an existing file. Keep the useful .txt extension.
	f, err := os.CreateTemp(dir, "transcription-"+time.Now().Format("2006-01-02-150405")+"-*.txt")
	if err != nil {
		return "", err
	}
	path := f.Name()
	// Preserve every UTF-8 byte, including whitespace and original line endings.
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}
