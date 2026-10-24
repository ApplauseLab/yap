package sounds

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"yap/internal/logger"
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
		logger.Debug("PlayStart: no sound path")
		return
	}
	logger.Debug(fmt.Sprintf("PlayStart: playing %s", startSoundPath))
	// Use afplay with reduced volume (0.6)
	cmd := exec.Command("afplay", "-v", "0.6", startSoundPath)
	if err := cmd.Start(); err != nil {
		logger.Warning(fmt.Sprintf("PlayStart error: %v", err))
	}
}

// PlayStop plays the recording stop sound (non-blocking)
func PlayStop() {
	if stopSoundPath == "" {
		logger.Debug("PlayStop: no sound path")
		return
	}
	logger.Debug(fmt.Sprintf("PlayStop: playing %s", stopSoundPath))
	// Use afplay with reduced volume (0.6)
	cmd := exec.Command("afplay", "-v", "0.6", stopSoundPath)
	if err := cmd.Start(); err != nil {
		logger.Warning(fmt.Sprintf("PlayStop error: %v", err))
	}
}

// PlayStartSync plays the recording start sound and waits for it to finish
func PlayStartSync() {
	if startSoundPath == "" {
		logger.Debug("PlayStartSync: no sound path")
		return
	}
	logger.Debug(fmt.Sprintf("PlayStartSync: playing %s", startSoundPath))
	cmd := exec.Command("afplay", "-v", "0.6", startSoundPath)
	if err := cmd.Run(); err != nil {
		logger.Warning(fmt.Sprintf("PlayStartSync error: %v", err))
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
