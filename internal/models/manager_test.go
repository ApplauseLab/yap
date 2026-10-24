package models

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type downloadTransport func(*http.Request) (*http.Response, error)

func (f downloadTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type interruptedDownload struct{}

func (interruptedDownload) Read(p []byte) (int, error) {
	return copy(p, "partial model"), io.ErrUnexpectedEOF
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
