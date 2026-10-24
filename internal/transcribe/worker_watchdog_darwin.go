//go:build darwin

package transcribe

import (
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// workerWatchdogEnv switches the Yap executable into watchdog mode for the
// whisper-server with the given PID.
const workerWatchdogEnv = "YAP_WHISPER_WORKER_WATCHDOG_PID"

// RunWorkerWatchdogIfRequested turns this process into a worker watchdog when
// Yap launched it as one, and never returns in that case. main must call it
// before any other initialisation.
//
// macOS has no parent-death signal, so a whisper-server would outlive a Yap
// that is SIGKILLed or crashes. The watchdog holds the read end of a pipe
// whose only writer is Yap; the kernel closes it when Yap dies for any reason,
// and the watchdog then stops the worker. It exits on its own as soon as the
// worker exits, so a normal worker stop needs no extra coordination.
func RunWorkerWatchdogIfRequested() {
	value, ok := os.LookupEnv(workerWatchdogEnv)
	if !ok {
		return
	}
	pid, err := strconv.Atoi(value)
	if err != nil || pid <= 0 {
		os.Exit(2)
	}
	os.Exit(runWorkerWatchdog(pid))
}

func runWorkerWatchdog(workerPID int) int {
	// Terminal job-control signals (Ctrl-C under wails dev) also reach this
	// process; it must keep guarding the worker until Yap itself is gone.
	signal.Ignore(os.Interrupt, syscall.SIGHUP)

	kq, err := unix.Kqueue()
	if err != nil {
		return 1
	}
	var procEvent unix.Kevent_t
	unix.SetKevent(&procEvent, workerPID, unix.EVFILT_PROC, unix.EV_ADD|unix.EV_ONESHOT)
	procEvent.Fflags = unix.NOTE_EXIT
	if _, err := unix.Kevent(kq, []unix.Kevent_t{procEvent}, nil, nil); err != nil {
		// ESRCH: the worker is already gone.
		return 0
	}
	// Any later exit of this PID is now reported as NOTE_EXIT. Make sure it is
	// still the worker Yap started and not an already reused PID.
	if info, err := unix.SysctlKinfoProc("kern.proc.pid", workerPID); err != nil || int(info.Eproc.Ppid) != os.Getppid() {
		return 0
	}

	var stdinEvent unix.Kevent_t
	unix.SetKevent(&stdinEvent, 0, unix.EVFILT_READ, unix.EV_ADD)
	if _, err := unix.Kevent(kq, []unix.Kevent_t{stdinEvent}, nil, nil); err != nil {
		return 1
	}

	events := make([]unix.Kevent_t, 4)
	buf := make([]byte, 512)
	for {
		n, err := unix.Kevent(kq, nil, events, nil)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return 1
		}
		ownerGone := false
		for _, event := range events[:n] {
			switch event.Filter {
			case unix.EVFILT_PROC:
				return 0
			case unix.EVFILT_READ:
				if event.Flags&unix.EV_EOF != 0 {
					ownerGone = true
				} else {
					_, _ = unix.Read(0, buf)
				}
			}
		}
		if ownerGone {
			stopOrphanedWorker(kq, workerPID, events)
			return 0
		}
	}
}

// stopOrphanedWorker asks the worker to exit like Yap's own stop path does,
// then kills it if it has not exited within workerStopTimeout.
func stopOrphanedWorker(kq int, workerPID int, events []unix.Kevent_t) {
	var stdinEvent unix.Kevent_t
	unix.SetKevent(&stdinEvent, 0, unix.EVFILT_READ, unix.EV_DELETE)
	_, _ = unix.Kevent(kq, []unix.Kevent_t{stdinEvent}, nil, nil)

	_ = unix.Kill(workerPID, unix.SIGINT)
	timeout := unix.NsecToTimespec(int64(workerStopTimeout))
	for {
		n, err := unix.Kevent(kq, nil, events, &timeout)
		if err == unix.EINTR {
			continue
		}
		if err == nil {
			for _, event := range events[:n] {
				if event.Filter == unix.EVFILT_PROC {
					return
				}
			}
		}
		// Timed out without NOTE_EXIT, so workerPID still names the worker.
		_ = unix.Kill(workerPID, unix.SIGKILL)
		return
	}
}

// startWorkerWatchdog launches a watchdog for workerPID. The watchdog reaps
// itself once the worker exits; Yap waits for it so it never lingers as a
// zombie.
func startWorkerWatchdog(workerPID int) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe)
	// Shown by ps/Activity Monitor command lines in place of a second "Yap".
	cmd.Args = []string{"yap-whisper-watchdog"}
	cmd.Env = append(os.Environ(), workerWatchdogEnv+"="+strconv.Itoa(workerPID))
	// Yap keeps the write end open until the watchdog exits (Wait closes it);
	// it is never written to. EOF means Yap has died.
	if _, err := cmd.StdinPipe(); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}
