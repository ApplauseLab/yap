//go:build windows

package sounds

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"yap/internal/logger"
)

//go:embed start.wav stop.wav
var soundFiles embed.FS

var tempDir string
var playSound = windows.NewLazySystemDLL("winmm.dll").NewProc("PlaySoundW")

func Init() error {
	if err := playSound.Find(); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "yap-sounds-")
	if err != nil {
		return err
	}
	for _, name := range []string{"start.wav", "stop.wav"} {
		data, err := soundFiles.ReadFile(name)
		if err == nil {
			err = os.WriteFile(filepath.Join(dir, name), data, 0600)
		}
		if err != nil {
			_ = os.RemoveAll(dir)
			return err
		}
	}
	tempDir = dir
	return nil
}

func play(name string, async bool) {
	if tempDir == "" {
		return
	}
	path, err := windows.UTF16PtrFromString(filepath.Join(tempDir, name))
	if err != nil {
		return
	}
	flags := uintptr(0x00020000 | 0x0002) // SND_FILENAME | SND_NODEFAULT
	if async {
		flags |= 0x0001
	} // SND_ASYNC: Windows owns playback, no child process.
	if result, _, _ := playSound.Call(uintptr(unsafe.Pointer(path)), 0, flags); result == 0 {
		logger.Warning(fmt.Sprintf("Could not play %s", name))
	}
}

func PlayStart()     { play("start.wav", true) }
func PlayStop()      { play("stop.wav", true) }
func PlayStartSync() { play("start.wav", false) }
func PlayStopSync()  { play("stop.wav", false) }
func Cleanup() {
	if tempDir != "" {
		playSound.Call(0, 0, 0) // Stop playback before removing the files.
		_ = os.RemoveAll(tempDir)
	}
}
