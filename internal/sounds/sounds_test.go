//go:build !windows

package sounds

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"yap/internal/proctest"
)

// useFakePlayer puts a silent, instantly exiting afplay first on PATH so the
// tests exercise the real spawn/reap path without playing audio.
func useFakePlayer(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "afplay"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(Cleanup)
}

func assertChildrenReaped(t *testing.T) {
	t.Helper()
	remaining, err := proctest.WaitNoChildren(os.Getpid(), 10*time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) > 0 {
		zombies := 0
		for _, child := range remaining {
			if child.Zombie() {
				zombies++
			}
		}
		t.Fatalf("%d sound player processes not reaped (%d zombies), e.g. %+v", len(remaining), zombies, remaining[0])
	}
}

func TestAsyncPlaybackReapsPlayer(t *testing.T) {
	useFakePlayer(t)
	for i := 0; i < 5; i++ {
		PlayStart()
		PlayStop()
	}
	assertChildrenReaped(t)
}

// TestAsyncPlaybackSoakLeavesNoZombies mirrors many recordings in a long
// session: each recording plays a start and a stop sound.
func TestAsyncPlaybackSoakLeavesNoZombies(t *testing.T) {
	if testing.Short() {
		t.Skip("soak test")
	}
	useFakePlayer(t)
	for i := 0; i < 200; i++ {
		PlayStart()
		PlayStop()
	}
	assertChildrenReaped(t)
}

func TestSyncPlaybackReapsPlayer(t *testing.T) {
	useFakePlayer(t)
	PlayStartSync()
	PlayStopSync()
	assertChildrenReaped(t)
}
