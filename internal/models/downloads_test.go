package models

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeDownload struct {
	mu       sync.Mutex
	started  chan string
	release  chan error
	progress DownloadProgress
	removed  []string
}

func newFakeDownload() *fakeDownload {
	return &fakeDownload{started: make(chan string, 4), release: make(chan error, 4)}
}

func (f *fakeDownload) download(ctx context.Context, model string, progress DownloadProgress) error {
	f.mu.Lock()
	f.progress = progress
	f.mu.Unlock()
	f.started <- model
	select {
	case err := <-f.release:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *fakeDownload) report(downloaded, total int64) {
	f.mu.Lock()
	p := f.progress
	f.mu.Unlock()
	p(downloaded, total)
}

func (f *fakeDownload) remove(model string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, model)
	return nil
}

type recorder struct {
	mu     sync.Mutex
	states []DownloadState
	done   chan DownloadState
}

func newRecorder() *recorder {
	return &recorder{done: make(chan DownloadState, 4)}
}

func (r *recorder) emit(s DownloadState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states = append(r.states, s)
}

func (r *recorder) onDone(s DownloadState) { r.done <- s }

func (r *recorder) statuses() []DownloadStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]DownloadStatus, len(r.states))
	for i, s := range r.states {
		out[i] = s.Status
	}
	return out
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.states)
}

func waitDone(t *testing.T, r *recorder) DownloadState {
	t.Helper()
	select {
	case s := <-r.done:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("download did not finish")
		return DownloadState{}
	}
}

func setup(t *testing.T) (*Downloader, *fakeDownload, *recorder) {
	t.Helper()
	f := newFakeDownload()
	r := newRecorder()
	d := newDownloader(f.download, f.remove, r.emit, r.onDone)
	t.Cleanup(func() { d.Shutdown(time.Second) })
	return d, f, r
}

func TestDownloaderInitialStateIsIdle(t *testing.T) {
	d, _, _ := setup(t)
	if s := d.State(); s.Status != DownloadIdle || s.Model != "" {
		t.Fatalf("unexpected initial state %+v", s)
	}
}

func TestDownloaderCompletesWithoutActivation(t *testing.T) {
	d, f, r := setup(t)
	if err := d.Start("small"); err != nil {
		t.Fatal(err)
	}
	if got := <-f.started; got != "small" {
		t.Fatalf("started %s", got)
	}
	if s := d.State(); s.Status != DownloadDownloading || s.Model != "small" || s.Total == 0 {
		t.Fatalf("unexpected state %+v", s)
	}
	f.report(50, 100)
	f.release <- nil
	final := waitDone(t, r)
	if final.Status != DownloadCompleted || final.Model != "small" || final.Progress != 100 {
		t.Fatalf("unexpected final state %+v", final)
	}
	if d.State() != final {
		t.Fatal("terminal state should remain observable")
	}
	statuses := r.statuses()
	if statuses[0] != DownloadDownloading || statuses[len(statuses)-1] != DownloadCompleted {
		t.Fatalf("unexpected transitions %v", statuses)
	}
}

func TestDownloaderEnforcesSingleDownload(t *testing.T) {
	d, f, r := setup(t)
	if err := d.Start("tiny"); err != nil {
		t.Fatal(err)
	}
	<-f.started
	if err := d.Start("base"); !errors.Is(err, ErrDownloadBusy) {
		t.Fatalf("expected busy error, got %v", err)
	}
	if err := d.Start("tiny"); !errors.Is(err, ErrDownloadBusy) {
		t.Fatalf("expected busy error for same model, got %v", err)
	}
	f.release <- nil
	waitDone(t, r)
	if err := d.Start("base"); err != nil {
		t.Fatalf("a new download should start after completion: %v", err)
	}
	<-f.started
	if s := d.State(); s.Model != "base" || s.Status != DownloadDownloading {
		t.Fatalf("new download should replace terminal state, got %+v", s)
	}
}

func TestDownloaderRejectsUnknownModel(t *testing.T) {
	d, _, r := setup(t)
	if err := d.Start("huge"); !errors.Is(err, ErrUnknownModel) {
		t.Fatalf("expected unknown model, got %v", err)
	}
	if r.count() != 0 {
		t.Fatal("no state should be emitted for invalid requests")
	}
}

func TestDownloaderCancellationMustMatchModel(t *testing.T) {
	d, f, r := setup(t)
	if err := d.Cancel("tiny"); !errors.Is(err, ErrNoMatchingDownload) {
		t.Fatalf("expected no matching download, got %v", err)
	}
	d.Start("tiny")
	<-f.started
	if err := d.Cancel("base"); !errors.Is(err, ErrNoMatchingDownload) {
		t.Fatalf("expected mismatch error, got %v", err)
	}
	if err := d.Cancel("tiny"); err != nil {
		t.Fatal(err)
	}
	final := waitDone(t, r)
	if final.Status != DownloadCancelled || final.Error != "" {
		t.Fatalf("unexpected final state %+v", final)
	}
	statuses := r.statuses()
	want := []DownloadStatus{DownloadDownloading, DownloadCancelling, DownloadCancelled}
	if len(statuses) != len(want) {
		t.Fatalf("transitions %v, want %v", statuses, want)
	}
	for i := range want {
		if statuses[i] != want[i] {
			t.Fatalf("transitions %v, want %v", statuses, want)
		}
	}
	if err := d.Start("tiny"); err != nil {
		t.Fatalf("retry after cancel failed: %v", err)
	}
	<-f.started
}

func TestDownloaderFailureIsRecoverable(t *testing.T) {
	d, f, r := setup(t)
	d.Start("base.en")
	<-f.started
	f.release <- errors.New("server returned HTTP 503")
	final := waitDone(t, r)
	if final.Status != DownloadFailed || final.Error != "server returned HTTP 503" {
		t.Fatalf("unexpected final state %+v", final)
	}
	if err := d.Start("base.en"); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	<-f.started
}

func TestDownloaderThrottlesProgress(t *testing.T) {
	d, f, r := setup(t)
	d.ProgressInterval = time.Hour
	d.Start("tiny")
	<-f.started
	for i := int64(1); i < 1000; i++ {
		f.report(i, 1000)
	}
	f.report(1000, 1000)
	f.release <- nil
	waitDone(t, r)
	// initial + final-progress + completed
	if n := r.count(); n != 3 {
		t.Fatalf("expected 3 emitted states, got %d (%v)", n, r.statuses())
	}
	if s := d.State(); s.Downloaded != s.Total {
		t.Fatalf("state should track latest progress: %+v", s)
	}
}

func TestDownloaderDeleteProtections(t *testing.T) {
	d, f, r := setup(t)
	if err := d.Delete("base.en", "base.en"); !errors.Is(err, ErrModelActive) {
		t.Fatalf("expected active model protection, got %v", err)
	}
	if err := d.Delete("nope", "base.en"); !errors.Is(err, ErrUnknownModel) {
		t.Fatalf("expected unknown model, got %v", err)
	}
	d.Start("small")
	<-f.started
	if err := d.Delete("small", "base.en"); !errors.Is(err, ErrModelDownloading) {
		t.Fatalf("expected downloading protection, got %v", err)
	}
	if err := d.Delete("tiny", "base.en"); err != nil {
		t.Fatalf("deleting another inactive model should work during a download: %v", err)
	}
	f.release <- nil
	waitDone(t, r)
	if err := d.Delete("small", "base.en"); err != nil {
		t.Fatal(err)
	}
	if s := d.State(); s.Status != DownloadIdle {
		t.Fatalf("deleting the completed model should reset state, got %+v", s)
	}
	if len(f.removed) != 2 {
		t.Fatalf("removed %v", f.removed)
	}
}

func TestDownloaderShutdownCancelsAndWaits(t *testing.T) {
	f := newFakeDownload()
	r := newRecorder()
	d := newDownloader(f.download, f.remove, r.emit, r.onDone)
	d.Start("medium")
	<-f.started

	finished := make(chan struct{})
	go func() {
		d.Shutdown(5 * time.Second)
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not return")
	}
	if s := d.State(); s.Status != DownloadCancelled {
		t.Fatalf("expected cancelled after shutdown, got %+v", s)
	}
	if err := d.Start("tiny"); !errors.Is(err, ErrDownloaderClosed) {
		t.Fatalf("expected closed error, got %v", err)
	}
}
