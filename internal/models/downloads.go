package models

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"yap/internal/crashlog"
)

// DownloadStatus describes where the model download lifecycle currently is.
type DownloadStatus string

const (
	DownloadIdle        DownloadStatus = "idle"
	DownloadDownloading DownloadStatus = "downloading"
	DownloadCancelling  DownloadStatus = "cancelling"
	DownloadCompleted   DownloadStatus = "completed"
	DownloadFailed      DownloadStatus = "failed"
	DownloadCancelled   DownloadStatus = "cancelled"
)

// DownloadState is the single payload describing model download progress.
// Terminal states remain until the next download starts so a remounted UI can
// still show completion or offer a retry.
type DownloadState struct {
	Model      string         `json:"model"`
	Status     DownloadStatus `json:"status"`
	Downloaded int64          `json:"downloaded"`
	Total      int64          `json:"total"`
	Progress   float64        `json:"progress"`
	Error      string         `json:"error"`
}

// Active reports whether a download is still running.
func (s DownloadState) Active() bool {
	return s.Status == DownloadDownloading || s.Status == DownloadCancelling
}

var (
	ErrDownloadBusy       = errors.New("another model is already downloading")
	ErrNoMatchingDownload = errors.New("that model is not downloading")
	ErrModelActive        = errors.New("the active model cannot be deleted")
	ErrModelDownloading   = errors.New("a model cannot be deleted while it is downloading")
	ErrDownloaderClosed   = errors.New("downloads are unavailable while Yap is shutting down")
)

// DownloadFunc performs one model download.
type DownloadFunc func(ctx context.Context, model string, progress DownloadProgress) error

// Downloader owns the single active model download and its observable state.
type Downloader struct {
	download DownloadFunc
	remove   func(model string) error
	emit     func(DownloadState)
	onDone   func(DownloadState)

	// ProgressInterval throttles progress events; initial and final
	// states are always emitted.
	ProgressInterval time.Duration

	// emitMu serializes "change state, then emit" so events reach the
	// frontend in the order the state changed. Lock order: emitMu, then mu.
	// emit must never call back into the Downloader.
	emitMu sync.Mutex

	mu       sync.Mutex
	state    DownloadState
	cancel   context.CancelFunc
	done     chan struct{}
	lastEmit time.Time
	closed   bool
}

// NewDownloader wires a downloader to the manager. emit receives every
// state change; onDone runs after a download reaches a terminal state.
func NewDownloader(manager *Manager, emit func(DownloadState), onDone func(DownloadState)) *Downloader {
	return newDownloader(manager.DownloadModel, manager.DeleteModel, emit, onDone)
}

func newDownloader(download DownloadFunc, remove func(string) error, emit func(DownloadState), onDone func(DownloadState)) *Downloader {
	if emit == nil {
		emit = func(DownloadState) {}
	}
	if onDone == nil {
		onDone = func(DownloadState) {}
	}
	return &Downloader{
		download:         download,
		remove:           remove,
		emit:             emit,
		onDone:           onDone,
		ProgressInterval: 150 * time.Millisecond,
		state:            DownloadState{Status: DownloadIdle},
	}
}

// State returns the current download state.
func (d *Downloader) State() DownloadState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state
}

// Start validates the request and begins downloading model in the background.
func (d *Downloader) Start(model string) error {
	spec, err := RequireModel(model)
	if err != nil {
		return err
	}

	d.emitMu.Lock()
	defer d.emitMu.Unlock()

	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return ErrDownloaderClosed
	}
	if d.state.Active() {
		active := d.state.Model
		d.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrDownloadBusy, active)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	d.cancel = cancel
	d.done = done
	d.state = DownloadState{Model: model, Status: DownloadDownloading, Total: spec.SizeBytes}
	d.lastEmit = time.Now()
	initial := d.state
	d.mu.Unlock()

	d.emit(initial)
	go d.run(ctx, cancel, done, model)
	return nil
}

func (d *Downloader) run(ctx context.Context, cancel context.CancelFunc, done chan struct{}, model string) {
	defer crashlog.LogPanic("model download")
	defer close(done)
	defer cancel()

	err := d.download(ctx, model, func(downloaded, total int64) {
		d.emitMu.Lock()
		defer d.emitMu.Unlock()
		d.mu.Lock()
		if d.state.Model != model || !d.state.Active() {
			d.mu.Unlock()
			return
		}
		d.state.Downloaded = downloaded
		d.state.Total = total
		if total > 0 {
			d.state.Progress = float64(downloaded) / float64(total) * 100
		}
		now := time.Now()
		shouldEmit := d.state.Status == DownloadDownloading &&
			(downloaded == total || now.Sub(d.lastEmit) >= d.ProgressInterval)
		if shouldEmit {
			d.lastEmit = now
		}
		snapshot := d.state
		d.mu.Unlock()
		if shouldEmit {
			d.emit(snapshot)
		}
	})

	d.emitMu.Lock()
	d.mu.Lock()
	switch {
	case err == nil:
		d.state.Status = DownloadCompleted
		d.state.Downloaded = d.state.Total
		d.state.Progress = 100
		d.state.Error = ""
	case ctx.Err() != nil:
		d.state.Status = DownloadCancelled
		d.state.Error = ""
	default:
		d.state.Status = DownloadFailed
		d.state.Error = err.Error()
	}
	d.cancel = nil
	final := d.state
	closed := d.closed
	d.mu.Unlock()

	if !closed {
		d.emit(final)
	}
	d.emitMu.Unlock()
	if !closed {
		d.onDone(final)
	}
}

// Cancel stops the active download if it is downloading model.
func (d *Downloader) Cancel(model string) error {
	d.emitMu.Lock()
	d.mu.Lock()
	if !d.state.Active() || d.state.Model != model {
		d.mu.Unlock()
		d.emitMu.Unlock()
		return fmt.Errorf("%w: %s", ErrNoMatchingDownload, model)
	}
	if d.state.Status == DownloadCancelling {
		d.mu.Unlock()
		d.emitMu.Unlock()
		return nil
	}
	d.state.Status = DownloadCancelling
	cancel := d.cancel
	snapshot := d.state
	d.mu.Unlock()

	d.emit(snapshot)
	d.emitMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// Delete removes a downloaded model unless it is active or downloading.
func (d *Downloader) Delete(model, activeModel string) error {
	if _, err := RequireModel(model); err != nil {
		return err
	}
	if model == activeModel {
		return ErrModelActive
	}
	d.mu.Lock()
	downloading := d.state.Active() && d.state.Model == model
	d.mu.Unlock()
	if downloading {
		return ErrModelDownloading
	}
	if err := d.remove(model); err != nil {
		return err
	}
	d.mu.Lock()
	if d.state.Model == model && !d.state.Active() {
		d.state = DownloadState{Status: DownloadIdle}
	}
	d.mu.Unlock()
	return nil
}

// Shutdown cancels any active download and waits up to timeout for it to
// clean up. Later Start calls are rejected.
func (d *Downloader) Shutdown(timeout time.Duration) {
	d.mu.Lock()
	d.closed = true
	cancel := d.cancel
	done := d.done
	d.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(timeout):
		}
	}
}
