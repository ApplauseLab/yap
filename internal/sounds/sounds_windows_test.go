//go:build windows

package sounds

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsSoundsUseNativeWAVPlayback(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(Cleanup)
	for _, name := range []string{"start.wav", "stop.wav"} {
		data, err := os.ReadFile(filepath.Join(tempDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(data) < 44 || !bytes.Equal(data[:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WAVE")) {
			t.Fatalf("%s is not a playable PCM WAV", name)
		}
	}
}
