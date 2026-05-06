package transcribe

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LocalEngine uses local whisper.cpp for transcription
type LocalEngine struct {
	model      Model
	modelsDir  string
	whisperBin string // Path to whisper CLI binary
}

// NewLocalEngine creates a new local whisper.cpp engine
func NewLocalEngine(modelsDir string) *LocalEngine {
	return &LocalEngine{
		model:     ModelBaseEn,
		modelsDir: modelsDir,
	}
}

// SetWhisperBinary sets the path to the whisper CLI binary
func (e *LocalEngine) SetWhisperBinary(path string) {
	e.whisperBin = path
}

// Transcribe converts audio samples to text
func (e *LocalEngine) Transcribe(ctx context.Context, samples []float32) (string, error) {
	wavData, err := samplesToWAV(samples)
	if err != nil {
		return "", fmt.Errorf("failed to convert samples: %w", err)
	}
	return e.TranscribeWAV(ctx, wavData)
}

// TranscribeWAV transcribes WAV audio data using whisper CLI
func (e *LocalEngine) TranscribeWAV(ctx context.Context, wavData []byte) (string, error) {
	// Check if whisper binary is available
	whisperBin := e.whisperBin
	if whisperBin == "" {
		// Try to find whisper-cli in common locations
		possiblePaths := []string{
			"/opt/homebrew/bin/whisper-cli",
			"/usr/local/bin/whisper-cli",
			filepath.Join(os.Getenv("HOME"), ".local/bin/whisper-cli"),
		}
		for _, p := range possiblePaths {
			if _, err := os.Stat(p); err == nil {
				whisperBin = p
				break
			}
		}
		// Also try PATH lookup
		if whisperBin == "" {
			if p, err := exec.LookPath("whisper-cli"); err == nil {
				whisperBin = p
			}
		}
	}

	if whisperBin == "" {
		return "", fmt.Errorf("whisper-cli not found. Please install whisper.cpp or set the binary path")
	}

	// Get model path
	modelPath := e.getModelPath()
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return "", fmt.Errorf("model file not found: %s. Please download the model first", modelPath)
	}

	// Create temp file for audio
	tmpFile, err := os.CreateTemp("", "whisper-input-*.wav")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(wavData); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write audio: %w", err)
	}
	tmpFile.Close()

	// Run whisper-cli
	cmd := exec.CommandContext(ctx, whisperBin,
		"-m", modelPath,
		"-f", tmpFile.Name(),
		"--no-timestamps",
		"-otxt",
	)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("whisper failed: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("whisper failed: %w", err)
	}

	// Clean up the output
	text := strings.TrimSpace(string(output))
	return text, nil
}

// SetModel sets the model to use
func (e *LocalEngine) SetModel(model Model) error {
	e.model = model
	return nil
}

// GetModel returns the current model
func (e *LocalEngine) GetModel() Model {
	return e.model
}

// IsAvailable checks if the engine is ready
func (e *LocalEngine) IsAvailable() bool {
	// Check if model file exists
	modelPath := e.getModelPath()
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return false
	}

	// Check if whisper binary is available
	if e.whisperBin != "" {
		if _, err := exec.LookPath(e.whisperBin); err != nil {
			return false
		}
		return true
	}

	// Try common paths
	possiblePaths := []string{
		"/opt/homebrew/bin/whisper-cli",
		"/usr/local/bin/whisper-cli",
	}
	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	// Also try PATH lookup
	if _, err := exec.LookPath("whisper-cli"); err == nil {
		return true
	}
	return false
}

// Name returns the provider name
func (e *LocalEngine) Name() Provider {
	return ProviderLocal
}

// getModelPath returns the full path to the model file
func (e *LocalEngine) getModelPath() string {
	modelFile := fmt.Sprintf("ggml-%s.bin", e.model)
	return filepath.Join(e.modelsDir, modelFile)
}

// GetModelPath returns the expected model file path (for download)
func (e *LocalEngine) GetModelPath() string {
	return e.getModelPath()
}

// GetModelsDir returns the models directory
func (e *LocalEngine) GetModelsDir() string {
	return e.modelsDir
}
