package models

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const (
	// BaseURL is the base URL for downloading models
	BaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main"
)

// ModelDownloadInfo contains download information for a model
type ModelDownloadInfo struct {
	Name     string `json:"name"`
	Filename string `json:"filename"`
	URL      string `json:"url"`
	Size     int64  `json:"size"` // bytes
}

// AvailableModels returns download info for all available models
func AvailableModels() []ModelDownloadInfo {
	return []ModelDownloadInfo{
		{"tiny", "ggml-tiny.bin", BaseURL + "/ggml-tiny.bin", 77691713},
		{"tiny.en", "ggml-tiny.en.bin", BaseURL + "/ggml-tiny.en.bin", 77704715},
		{"base", "ggml-base.bin", BaseURL + "/ggml-base.bin", 147951465},
		{"base.en", "ggml-base.en.bin", BaseURL + "/ggml-base.en.bin", 147964211},
		{"small", "ggml-small.bin", BaseURL + "/ggml-small.bin", 487601967},
		{"small.en", "ggml-small.en.bin", BaseURL + "/ggml-small.en.bin", 487614499},
		{"medium", "ggml-medium.bin", BaseURL + "/ggml-medium.bin", 1533763389},
		{"medium.en", "ggml-medium.en.bin", BaseURL + "/ggml-medium.en.bin", 1533774781},
		{"large-v3", "ggml-large-v3.bin", BaseURL + "/ggml-large-v3.bin", 3095033483},
	}
}

// Manager handles model downloads and management
type Manager struct {
	modelsDir string
}

// NewManager creates a new model manager
func NewManager(modelsDir string) (*Manager, error) {
	// Ensure models directory exists
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create models directory: %w", err)
	}
	return &Manager{modelsDir: modelsDir}, nil
}

// GetModelsDir returns the models directory path
func (m *Manager) GetModelsDir() string {
	return m.modelsDir
}

// IsModelDownloaded checks if a model is already downloaded
func (m *Manager) IsModelDownloaded(modelName string) bool {
	filename := fmt.Sprintf("ggml-%s.bin", modelName)
	path := filepath.Join(m.modelsDir, filename)
	_, err := os.Stat(path)
	return err == nil
}

// GetModelPath returns the full path to a model file
func (m *Manager) GetModelPath(modelName string) string {
	filename := fmt.Sprintf("ggml-%s.bin", modelName)
	return filepath.Join(m.modelsDir, filename)
}

// DownloadProgress is called during download with progress info
type DownloadProgress func(downloaded, total int64)

// DownloadModel downloads a model from HuggingFace
func (m *Manager) DownloadModel(modelName string, progress DownloadProgress) error {
	// Find model info
	var modelInfo *ModelDownloadInfo
	for _, info := range AvailableModels() {
		if info.Name == modelName {
			modelInfo = &info
			break
		}
	}
	if modelInfo == nil {
		return fmt.Errorf("unknown model: %s", modelName)
	}

	// Create target path
	targetPath := filepath.Join(m.modelsDir, modelInfo.Filename)

	// Check if already exists
	if _, err := os.Stat(targetPath); err == nil {
		return nil // Already downloaded
	}

	// Create temp file
	tmpPath := targetPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Download
	resp, err := http.Get(modelInfo.URL)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		os.Remove(tmpPath)
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// Get total size
	total := resp.ContentLength
	if total < 0 {
		total = modelInfo.Size
	}

	// Copy with progress
	var downloaded int64
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				os.Remove(tmpPath)
				return fmt.Errorf("failed to write: %w", writeErr)
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("download error: %w", err)
		}
	}

	// Rename to final path
	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to finalize download: %w", err)
	}

	return nil
}

// DeleteModel removes a downloaded model
func (m *Manager) DeleteModel(modelName string) error {
	path := m.GetModelPath(modelName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Already deleted
	}
	return os.Remove(path)
}

// ListDownloadedModels returns a list of downloaded model names
func (m *Manager) ListDownloadedModels() ([]string, error) {
	entries, err := os.ReadDir(m.modelsDir)
	if err != nil {
		return nil, err
	}

	var models []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) > 9 && name[:5] == "ggml-" && name[len(name)-4:] == ".bin" {
			// Extract model name
			modelName := name[5 : len(name)-4]
			models = append(models, modelName)
		}
	}
	return models, nil
}
