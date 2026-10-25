package system

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestRevealCommand(t *testing.T) {
	for _, tt := range []struct {
		platform string
		path     string
		want     []string
	}{
		{"darwin", "/Users/Test User/recording.wav", []string{"open", "-R", "/Users/Test User/recording.wav"}},
		{"linux", filepath.Join("recordings", "test.wav"), []string{"xdg-open", "recordings"}},
	} {
		t.Run(tt.platform, func(t *testing.T) {
			cmd, err := revealCommand(tt.platform, tt.path)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cmd.Args, tt.want) {
				t.Fatalf("got %q; want %q", cmd.Args, tt.want)
			}
		})
	}
}

func TestRevealMissingFile(t *testing.T) {
	if err := RevealFile(filepath.Join(t.TempDir(), "missing.wav")); err == nil {
		t.Fatal("missing recording should return an error before launching the file manager")
	}
}
