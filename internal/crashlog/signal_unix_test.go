//go:build !windows

package crashlog

import (
	"os"
	"syscall"
)

func raiseAbort() {
	_ = syscall.Kill(os.Getpid(), syscall.SIGABRT)
}
