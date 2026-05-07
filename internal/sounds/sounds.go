package sounds

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed start.mp3 stop.mp3
var soundFiles embed.FS

var tempDir string
var startSoundPath string
var stopSoundPath string

// Init extracts embedded sounds to temp directory for playback
func Init() error {
	var err error
	tempDir, err = os.MkdirTemp("", "applause-sounds-")
	if err != nil {
		return err
	}

	// Extract start sound
	startData, err := soundFiles.ReadFile("start.mp3")
	if err != nil {
		return err
	}
	startSoundPath = filepath.Join(tempDir, "start.mp3")
	if err := os.WriteFile(startSoundPath, startData, 0644); err != nil {
		return err
	}

	// Extract stop sound
	stopData, err := soundFiles.ReadFile("stop.mp3")
	if err != nil {
		return err
	}
	stopSoundPath = filepath.Join(tempDir, "stop.mp3")
	if err := os.WriteFile(stopSoundPath, stopData, 0644); err != nil {
		return err
	}

	return nil
}

// Cleanup removes temporary sound files
func Cleanup() {
	if tempDir != "" {
		os.RemoveAll(tempDir)
	}
}

// PlayStart plays the recording start sound (non-blocking)
func PlayStart() {
	if startSoundPath == "" {
		fmt.Println("PlayStart: no sound path")
		return
	}
	fmt.Printf("PlayStart: playing %s\n", startSoundPath)
	// Use afplay with reduced volume (0.6)
	cmd := exec.Command("afplay", "-v", "0.6", startSoundPath)
	if err := cmd.Start(); err != nil {
		fmt.Printf("PlayStart error: %v\n", err)
	}
}

// PlayStop plays the recording stop sound (non-blocking)
func PlayStop() {
	if stopSoundPath == "" {
		fmt.Println("PlayStop: no sound path")
		return
	}
	fmt.Printf("PlayStop: playing %s\n", stopSoundPath)
	// Use afplay with reduced volume (0.6)
	cmd := exec.Command("afplay", "-v", "0.6", stopSoundPath)
	if err := cmd.Start(); err != nil {
		fmt.Printf("PlayStop error: %v\n", err)
	}
}

// PlayStartSync plays the recording start sound and waits for it to finish
func PlayStartSync() {
	if startSoundPath == "" {
		fmt.Println("PlayStartSync: no sound path")
		return
	}
	fmt.Printf("PlayStartSync: playing %s\n", startSoundPath)
	cmd := exec.Command("afplay", "-v", "0.6", startSoundPath)
	if err := cmd.Run(); err != nil {
		fmt.Printf("PlayStartSync error: %v\n", err)
	}
}

// PlayStopSync plays the recording stop sound and waits for it to finish
func PlayStopSync() {
	if stopSoundPath == "" {
		return
	}
	cmd := exec.Command("afplay", "-v", "0.6", stopSoundPath)
	cmd.Run() // Blocking
}
