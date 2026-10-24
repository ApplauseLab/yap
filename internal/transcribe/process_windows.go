//go:build windows

package transcribe

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// statusIllegalInstruction is the exit code of a process that executed an
// instruction the CPU does not support (e.g. AVX2 on an older CPU).
const statusIllegalInstruction = 0xC000001D

// hideChildConsole stops console programs started from Yap's GUI process from
// opening a visible console window.
func hideChildConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

var (
	lifetimeJobOnce sync.Once
	lifetimeJob     windows.Handle
	lifetimeJobErr  error
)

// processLifetimeJob returns a job object that kills its processes when the
// last handle to it closes. The handle is never closed explicitly, so Windows
// closes it, and kills the processes, when Yap exits for any reason.
func processLifetimeJob() (windows.Handle, error) {
	lifetimeJobOnce.Do(func() {
		job, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			lifetimeJobErr = err
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}
		if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			_ = windows.CloseHandle(job)
			lifetimeJobErr = err
			return
		}
		lifetimeJob = job
	})
	return lifetimeJob, lifetimeJobErr
}

// bindToProcessLifetime makes the child die with Yap, including on crash or
// forced termination, when no shutdown code runs.
func bindToProcessLifetime(process *os.Process) error {
	job, err := processLifetimeJob()
	if err != nil {
		return err
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.AssignProcessToJobObject(job, handle)
}

func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		// The process exists but belongs to another user or session.
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(handle)
	event, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && event == uint32(windows.WAIT_TIMEOUT)
}

func isWhisperServerProcess(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	if event, err := windows.WaitForSingleObject(handle, 0); err != nil || event != uint32(windows.WAIT_TIMEOUT) {
		return false
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return false
	}
	name := strings.ToLower(filepath.Base(windows.UTF16ToString(buf[:size])))
	return name == "whisper-server.exe"
}

func isIllegalInstructionExit(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && uint32(exitErr.ExitCode()) == statusIllegalInstruction
}
