//go:build !darwin && !linux && !windows

package crashlog

import (
	"errors"
	"os"
)

func stderrIsTerminal() bool { return false }

func redirectStderrTo(*os.File) error {
	return errors.New("stderr redirection is not supported on this platform")
}
