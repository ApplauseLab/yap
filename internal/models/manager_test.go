package models

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testModel swaps the tiny.en catalog size for a small payload.
func withSmallCatalogEntry(t *testing.T, name string, size int64) ModelSpec {
	t.Helper()
	original := make([]ModelSpec, len(catalog))
	copy(original, catalog)
	t.Cleanup(func() { catalog = original })
	for i := range catalog {
		if catalog[i].Name == name {
			catalog[i].SizeBytes = size
			return catalog[i]
		}
	}
	t.Fatalf("model %s not in catalog", name)
	return ModelSpec{}
}

func newTestManager(t *testing.T, baseURL string) *Manager {
	t.Helper()
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m.SetBaseURL(baseURL)
	return m
}

func payload(n int) []byte {
	return bytes.Repeat([]byte("y"), n)
}

func serveBytes(t *testing.T, filename string, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+filename {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func assertNoTemp(t *testing.T, m *Manager, spec ModelSpec) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(m.GetModelsDir(), spec.Filename+"*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) > 0 {
		t.Fatalf("temporary files left behind: %v", matches)
	}
}

func TestCatalogIsComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, spec := range Catalog() {
		if seen[spec.Name] {
			t.Fatalf("duplicate model %s", spec.Name)
		}
		seen[spec.Name] = true
		if spec.DisplayName == "" || spec.Filename == "" || spec.Size == "" || spec.UseCase == "" || spec.Provider != ProviderLocal {
			t.Fatalf("incomplete metadata for %s: %+v", spec.Name, spec)
		}
		if spec.SizeBytes <= 0 || spec.Speed < 1 || spec.Speed > 5 || spec.Accuracy < 1 || spec.Accuracy > 5 {
			t.Fatalf("invalid ratings or size for %s: %+v", spec.Name, spec)
		}
		if spec.Filename != "ggml-"+spec.Name+".bin" {
			t.Fatalf("unexpected filename for %s: %s", spec.Name, spec.Filename)
		}
	}
	if len(seen) != 9 {
		t.Fatalf("expected 9 models, got %d", len(seen))
	}
}

func TestDownloadSuccessReportsProgressAndFinalizes(t *testing.T) {
	body := payload(200 * 1024)
	spec := withSmallCatalogEntry(t, "tiny.en", int64(len(body)))
	m := newTestManager(t, serveBytes(t, spec.Filename, body).URL)

	var calls []int64
	err := m.DownloadModel(context.Background(), "tiny.en", func(downloaded, total int64) {
		if total != spec.SizeBytes {
			t.Errorf("total = %d, want %d", total, spec.SizeBytes)
		}
		calls = append(calls, downloaded)
	})
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if len(calls) < 2 || calls[0] != 0 || calls[len(calls)-1] != spec.SizeBytes {
		t.Fatalf("unexpected progress calls: %v", calls)
	}
	if !m.IsModelDownloaded("tiny.en") {
		t.Fatal("model should be downloaded")
	}
	got, err := os.ReadFile(m.GetModelPath("tiny.en"))
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("final file mismatch (err=%v)", err)
	}
	assertNoTemp(t, m, spec)
}

func TestDownloadCancellationRemovesTempFile(t *testing.T) {
	spec := withSmallCatalogEntry(t, "tiny.en", 1<<20)
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(1<<20))
		w.Write(payload(1024))
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	m := newTestManager(t, srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- m.DownloadModel(ctx, "tiny.en", nil) }()
	<-started
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("download did not stop after cancellation")
	}
	if m.IsModelDownloaded("tiny.en") {
		t.Fatal("cancelled model must not be downloaded")
	}
	assertNoTemp(t, m, spec)
}

func TestDownloadHTTPFailure(t *testing.T) {
	spec := withSmallCatalogEntry(t, "tiny.en", 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	m := newTestManager(t, srv.URL)

	err := m.DownloadModel(context.Background(), "tiny.en", nil)
	if err == nil || err.Error() != "server returned HTTP 500" {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.IsModelDownloaded("tiny.en") {
		t.Fatal("failed model must not be downloaded")
	}
	assertNoTemp(t, m, spec)
}

func TestDownloadStreamFailure(t *testing.T) {
	spec := withSmallCatalogEntry(t, "tiny.en", 4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4096")
		w.Write(payload(1000))
		w.(http.Flusher).Flush()
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	t.Cleanup(srv.Close)
	m := newTestManager(t, srv.URL)

	if err := m.DownloadModel(context.Background(), "tiny.en", nil); err == nil {
		t.Fatal("expected stream failure")
	}
	if m.IsModelDownloaded("tiny.en") {
		t.Fatal("truncated model must not be downloaded")
	}
	assertNoTemp(t, m, spec)
}

func TestDownloadRejectsIncorrectContentSize(t *testing.T) {
	spec := withSmallCatalogEntry(t, "tiny.en", 2048)
	m := newTestManager(t, serveBytes(t, spec.Filename, payload(1024)).URL)

	err := m.DownloadModel(context.Background(), "tiny.en", nil)
	if !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("expected size mismatch, got %v", err)
	}
	assertNoTemp(t, m, spec)
}

func TestDownloadRejectsUnreportedWrongSize(t *testing.T) {
	spec := withSmallCatalogEntry(t, "tiny.en", 2048)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Chunked response: no Content-Length, body shorter than expected.
		w.Write(payload(512))
		w.(http.Flusher).Flush()
		w.Write(payload(512))
	}))
	t.Cleanup(srv.Close)
	m := newTestManager(t, srv.URL)

	err := m.DownloadModel(context.Background(), "tiny.en", nil)
	if !errors.Is(err, ErrSizeMismatch) {
		t.Fatalf("expected size mismatch, got %v", err)
	}
	if m.IsModelDownloaded("tiny.en") {
		t.Fatal("short model must not be downloaded")
	}
	assertNoTemp(t, m, spec)
}

func TestExistingFilesAreValidatedBySize(t *testing.T) {
	spec := withSmallCatalogEntry(t, "tiny.en", 100)
	requests := 0
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.Header().Set("Content-Length", "100")
		w.Write(payload(100))
	}))
	t.Cleanup(srv.Close)
	m := newTestManager(t, srv.URL)
	path := m.GetModelPath("tiny.en")

	if err := os.WriteFile(path, payload(10), 0644); err != nil {
		t.Fatal(err)
	}
	if m.IsModelDownloaded("tiny.en") {
		t.Fatal("truncated existing file must not count as downloaded")
	}

	if err := m.DownloadModel(context.Background(), "tiny.en", nil); err != nil {
		t.Fatalf("re-download failed: %v", err)
	}
	if !m.IsModelDownloaded("tiny.en") || requests != 1 {
		t.Fatalf("expected one re-download, requests=%d", requests)
	}

	if err := m.DownloadModel(context.Background(), "tiny.en", nil); err != nil {
		t.Fatalf("valid existing file should succeed: %v", err)
	}
	if requests != 1 {
		t.Fatalf("valid existing file should not be re-downloaded, requests=%d", requests)
	}
	assertNoTemp(t, m, spec)
}

func TestUnknownModelsAreRejected(t *testing.T) {
	m := newTestManager(t, "http://127.0.0.1:1")
	if err := m.DownloadModel(context.Background(), "../../etc/passwd", nil); !errors.Is(err, ErrUnknownModel) {
		t.Fatalf("expected unknown model, got %v", err)
	}
	if err := m.DeleteModel("nope"); !errors.Is(err, ErrUnknownModel) {
		t.Fatalf("expected unknown model, got %v", err)
	}
	if m.IsModelDownloaded("nope") {
		t.Fatal("unknown model cannot be downloaded")
	}
}

func TestDeleteModelRemovesFileAndPartial(t *testing.T) {
	spec := withSmallCatalogEntry(t, "base", 4)
	m := newTestManager(t, "http://127.0.0.1:1")
	path := m.GetModelPath("base")
	os.WriteFile(path, payload(4), 0644)
	os.WriteFile(path+".tmp", payload(2), 0644)
	os.WriteFile(path+"-123.tmp", payload(2), 0644)

	if got := m.ListDownloadedModels(); len(got) != 1 || got[0] != "base" {
		t.Fatalf("ListDownloadedModels = %v", got)
	}
	if err := m.DeleteModel("base"); err != nil {
		t.Fatal(err)
	}
	if m.IsModelDownloaded("base") {
		t.Fatal("model should be deleted")
	}
	assertNoTemp(t, m, spec)
	if err := m.DeleteModel("base"); err != nil {
		t.Fatalf("deleting a missing model should be a no-op: %v", err)
	}
}

func TestFinalizedFileIsClosedAndReplaceable(t *testing.T) {
	body := payload(4096)
	spec := withSmallCatalogEntry(t, "tiny.en", int64(len(body)))
	m := newTestManager(t, serveBytes(t, spec.Filename, body).URL)

	if err := m.DownloadModel(context.Background(), "tiny.en", nil); err != nil {
		t.Fatal(err)
	}
	// The finalized file must be fully written and removable, which fails on
	// Windows when the handle is left open.
	if err := m.DeleteModel("tiny.en"); err != nil {
		t.Fatalf("finalized file could not be removed: %v", err)
	}
	if err := m.DownloadModel(context.Background(), "tiny.en", nil); err != nil {
		t.Fatalf("second download failed: %v", err)
	}
	if !m.IsModelDownloaded("tiny.en") {
		t.Fatal("second download should be finalized")
	}
}

func TestConcurrentSameModelDownloadsShareOneTransfer(t *testing.T) {
	body := payload(64)
	spec := withSmallCatalogEntry(t, "tiny", int64(len(body)))
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		once.Do(func() { close(entered) })
		<-release
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	m := newTestManager(t, srv.URL)

	results := make(chan error, 8)
	for i := 0; i < cap(results); i++ {
		go func() { results <- m.DownloadModel(context.Background(), "tiny", nil) }()
	}
	<-entered
	// Keep the first transfer open while repeated requests arrive.
	time.Sleep(100 * time.Millisecond)
	close(release)
	for i := 0; i < cap(results); i++ {
		if err := <-results; err != nil {
			t.Errorf("concurrent download: %v", err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("HTTP transfers = %d, want 1", got)
	}
	if !m.IsModelDownloaded("tiny") {
		t.Fatal("model should be downloaded")
	}
	assertNoTemp(t, m, spec)
}

func TestDifferentModelsDownloadIndependentlyAndFailureCanRetry(t *testing.T) {
	body := payload(16)
	withSmallCatalogEntry(t, "tiny", int64(len(body)))
	withSmallCatalogEntry(t, "tiny.en", int64(len(body)))
	entered := make(chan string, 2)
	release := make(chan struct{})
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) <= 2 {
			entered <- r.URL.Path
			<-release
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	m := newTestManager(t, srv.URL)

	results := make(chan error, 2)
	for _, name := range []string{"tiny", "tiny.en"} {
		go func(name string) { results <- m.DownloadModel(context.Background(), name, nil) }(name)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("different model downloads blocked each other")
		}
	}
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-results; err == nil {
			t.Error("expected HTTP failure")
		}
	}
	for _, name := range []string{"tiny", "tiny.en"} {
		if err := m.DownloadModel(context.Background(), name, nil); err != nil {
			t.Fatalf("retry %s: %v", name, err)
		}
	}
}

func TestStalePartialsAreRemoved(t *testing.T) {
	body := payload(32)
	spec := withSmallCatalogEntry(t, "tiny", int64(len(body)))
	dir := t.TempDir()
	stale := []string{spec.Filename + "-123.tmp", spec.Filename + ".tmp", "ggml-base.en.bin-9.tmp"}
	old := time.Now().Add(-time.Hour)
	writeStale := func(name string) {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, payload(8), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range stale {
		writeStale(name)
	}
	unrelated := filepath.Join(dir, "notes.tmp")
	os.WriteFile(unrelated, payload(1), 0644)
	// A fresh partial may belong to a live download in another Yap instance.
	live := filepath.Join(dir, "ggml-small.bin-77.tmp")
	os.WriteFile(live, payload(1), 0644)

	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range stale {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s survived startup sweep", name)
		}
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatal("unrelated files must be kept")
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatal("recently written partials must be kept")
	}

	// A stale partial appearing after startup is removed before the next transfer.
	writeStale(spec.Filename + "-456.tmp")
	m.SetBaseURL(serveBytes(t, spec.Filename, body).URL)
	if err := m.DownloadModel(context.Background(), "tiny", nil); err != nil {
		t.Fatal(err)
	}
	assertNoTemp(t, m, spec)
}
