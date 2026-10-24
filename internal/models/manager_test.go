package models

import (
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type downloadTransport func(*http.Request) (*http.Response, error)

func (f downloadTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type interruptedDownload struct{}

func (interruptedDownload) Read(p []byte) (int, error) {
	return copy(p, "partial model"), io.ErrUnexpectedEOF
}

func TestConcurrentModelDownloadsShareOneTransfer(t *testing.T) {
	previous := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = previous })
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var requests atomic.Int32
	http.DefaultClient = &http.Client{Transport: downloadTransport(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		once.Do(func() { close(entered) })
		<-release
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("complete model fixture")), ContentLength: 22, Header: make(http.Header)}, nil
	})}
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 8)
	for i := 0; i < cap(results); i++ {
		go func() { results <- manager.DownloadModel("tiny", nil) }()
	}
	<-entered
	// Keep the first transfer open while repeated UI requests arrive.
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
	data, err := os.ReadFile(manager.GetModelPath("tiny"))
	if err != nil || string(data) != "complete model fixture" {
		t.Fatalf("model = %q, error = %v", data, err)
	}
}

func TestDownloadModelFinalization(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      io.Reader
		wantError bool
	}{
		{"success", http.StatusOK, strings.NewReader("complete model fixture"), false},
		{"http failure", http.StatusServiceUnavailable, strings.NewReader("unavailable"), true},
		{"interrupted stream", http.StatusOK, interruptedDownload{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Only replace HTTP; exercise the real download/write/rename path.
			previous := http.DefaultClient
			t.Cleanup(func() { http.DefaultClient = previous })
			http.DefaultClient = &http.Client{Transport: downloadTransport(func(r *http.Request) (*http.Response, error) {
				if !strings.HasSuffix(r.URL.Path, "/ggml-tiny.en.bin") {
					t.Fatalf("unexpected URL: %s", r.URL)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(tc.body), ContentLength: -1, Header: make(http.Header)}, nil
			})}
			dir := t.TempDir()
			manager, err := NewManager(dir)
			if err != nil {
				t.Fatal(err)
			}
			err = manager.DownloadModel("tiny.en", nil)
			if (err != nil) != tc.wantError {
				t.Fatalf("DownloadModel error = %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantError {
				if len(entries) != 0 {
					t.Fatalf("failed download left files: %v", entries)
				}
				return
			}
			if len(entries) != 1 || entries[0].Name() != "ggml-tiny.en.bin" {
				t.Fatalf("unexpected files: %v", entries)
			}
			data, err := os.ReadFile(manager.GetModelPath("tiny.en"))
			if err != nil || string(data) != "complete model fixture" {
				t.Fatalf("model = %q, error = %v", data, err)
			}
			if !manager.IsModelDownloaded("tiny.en") {
				t.Fatal("completed model not available")
			}
		})
	}
}
