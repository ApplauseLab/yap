//go:build windows

package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

func captureCrashes(dir string) error {
	path := filepath.Join(dir, fmt.Sprintf("yap-%s-%d-crash.log", time.Now().Format("2006-01-02"), os.Getpid()))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	// Go duplicates the handle; closing f does not disable crash output.
	return debug.SetCrashOutput(f, debug.CrashOptions{})
}
