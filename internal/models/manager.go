package models

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	ErrUnknownModel = errors.New("unknown model")
	ErrSizeMismatch = errors.New("file size does not match the expected model size")
)

// Manager handles model downloads and management
type Manager struct {
	modelsDir     string
	baseURL       string
	client        *http.Client
	downloadLocks sync.Map // Model name -> *sync.Mutex; different models download independently.
}

// NewManager creates a new model manager
func NewManager(modelsDir string) (*Manager, error) {
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create models directory: %w", err)
	}
	return &Manager{modelsDir: modelsDir, baseURL: defaultBaseURL()}, nil
}

// SetBaseURL changes where model files are downloaded from.
func (m *Manager) SetBaseURL(baseURL string) {
	m.baseURL = baseURL
}

// GetModelsDir returns the models directory path
func (m *Manager) GetModelsDir() string {
	return m.modelsDir
}

// ModelURL returns the download URL for a catalog model.
func (m *Manager) ModelURL(spec ModelSpec) string {
	return m.baseURL + "/" + spec.Filename
}

// IsModelDownloaded reports whether a catalog model exists on disk with its
// exact expected size. Absent, truncated, or oversized files are unavailable.
func (m *Manager) IsModelDownloaded(modelName string) bool {
	spec, ok := LookupModel(modelName)
	if !ok {
		return false
	}
	info, err := os.Stat(filepath.Join(m.modelsDir, spec.Filename))
	return err == nil && info.Mode().IsRegular() && info.Size() == spec.SizeBytes
}

// GetModelPath returns the full path to a model file
func (m *Manager) GetModelPath(modelName string) string {
	if spec, ok := LookupModel(modelName); ok {
		return filepath.Join(m.modelsDir, spec.Filename)
	}
	return filepath.Join(m.modelsDir, fmt.Sprintf("ggml-%s.bin", modelName))
}

// DownloadProgress is called during download with progress info
type DownloadProgress func(downloaded, total int64)

func (m *Manager) httpClient() *http.Client {
	if m.client != nil {
		return m.client
	}
	return http.DefaultClient
}

// DownloadModel downloads a catalog model. The file is streamed to a unique
// temporary path, size-verified, closed, and atomically renamed into place.
// The temporary file is removed on failure or cancellation. Concurrent calls
// for the same model share one transfer; different models are independent.
func (m *Manager) DownloadModel(ctx context.Context, modelName string, progress DownloadProgress) (err error) {
	spec, err := RequireModel(modelName)
	if err != nil {
		return err
	}

	// Waiters recheck the final file after the active download completes.
	lock, _ := m.downloadLocks.LoadOrStore(modelName, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	if m.IsModelDownloaded(modelName) {
		if progress != nil {
			progress(spec.SizeBytes, spec.SizeBytes)
		}
		return nil
	}

	targetPath := filepath.Join(m.modelsDir, spec.Filename)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.ModelURL(spec), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := m.httpClient().Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("could not reach the download server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength >= 0 && resp.ContentLength != spec.SizeBytes {
		return fmt.Errorf("%w (server reported %d bytes, expected %d)", ErrSizeMismatch, resp.ContentLength, spec.SizeBytes)
	}

	out, err := os.CreateTemp(m.modelsDir, spec.Filename+"-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	tmpPath := out.Name()
	closed := false
	defer func() {
		if !closed {
			out.Close()
		}
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	total := spec.SizeBytes
	if progress != nil {
		progress(0, total)
	}

	var downloaded int64
	buf := make([]byte, 64*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("failed to write: %w", writeErr)
			}
			downloaded += int64(n)
			if downloaded > total {
				return fmt.Errorf("%w (received more than %d bytes)", ErrSizeMismatch, total)
			}
			if progress != nil {
				progress(downloaded, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return fmt.Errorf("connection interrupted: %w", readErr)
		}
	}

	if downloaded != total {
		return fmt.Errorf("%w (received %d bytes, expected %d)", ErrSizeMismatch, downloaded, total)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("failed to flush download: %w", err)
	}
	// Close before publishing: Windows cannot rename a file with an open handle.
	closed = true
	if err := out.Close(); err != nil {
		return fmt.Errorf("failed to close download: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("failed to finalize download: %w", err)
	}
	return nil
}

// DeleteModel removes a downloaded catalog model and any partial download.
func (m *Manager) DeleteModel(modelName string) error {
	spec, err := RequireModel(modelName)
	if err != nil {
		return err
	}
	path := filepath.Join(m.modelsDir, spec.Filename)
	m.removePartials(spec)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ListDownloadedModels returns the catalog models that are fully downloaded.
func (m *Manager) ListDownloadedModels() []string {
	var names []string
	for _, spec := range catalog {
		if m.IsModelDownloaded(spec.Name) {
			names = append(names, spec.Name)
		}
	}
	return names
}

// removePartials deletes leftover temporary files for spec, including the
// fixed-name partials written by earlier versions.
func (m *Manager) removePartials(spec ModelSpec) {
	entries, err := os.ReadDir(m.modelsDir)
	if err != nil {
		return
	}
	prefix := spec.Filename + "-"
	for _, entry := range entries {
		name := entry.Name()
		if name == spec.Filename+".tmp" || (strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".tmp")) {
			_ = os.Remove(filepath.Join(m.modelsDir, name))
		}
	}
}
