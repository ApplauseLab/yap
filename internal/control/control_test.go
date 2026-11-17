package control

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// Unix socket paths are length-limited, so tests avoid the long t.TempDir().
func socketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "yap-ctl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "yap.sock")
}

func TestSendRunsCommandAndReturnsReply(t *testing.T) {
	path := socketPath(t)
	var got []string
	server, err := Listen(path, func(command string) (string, error) {
		got = append(got, command)
		if command == CommandCancel {
			return "", errors.New("not\nrecording")
		}
		return "recording", nil
	})
	if err != nil {
		t.Fatal(err)
	}

	reply, err := Send(path, CommandToggle)
	if err != nil || reply != "recording" {
		t.Fatalf("Send(toggle) = %q, %v", reply, err)
	}
	if _, err := Send(path, CommandCancel); err == nil || err.Error() != "not recording" {
		t.Fatalf("Send(cancel) error = %v, want handler error", err)
	}
	if _, err := Send(path, "explode"); err == nil {
		t.Fatal("unknown command succeeded")
	}
	server.Close()
	if len(got) != 2 || got[0] != CommandToggle || got[1] != CommandCancel {
		t.Fatalf("handler saw %v", got)
	}
	if info, err := os.Stat(path); err == nil {
		t.Fatalf("socket %v left behind after Close", info.Name())
	}
	if _, err := Send(path, CommandToggle); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Send after Close = %v, want ErrNotRunning", err)
	}
}

func TestSocketIsPrivateToTheUser(t *testing.T) {
	path := socketPath(t)
	server, err := Listen(path, func(string) (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("socket mode = %o, want 600", perm)
	}
}

func TestListenRefusesSecondInstanceButReplacesStaleSocket(t *testing.T) {
	path := socketPath(t)
	handler := func(string) (string, error) { return "", nil }
	first, err := Listen(path, handler)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(path, handler); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Listen = %v, want ErrAlreadyRunning", err)
	}
	if _, err := Send(path, CommandStatus); err != nil {
		t.Fatalf("first instance stopped answering after the probe: %v", err)
	}
	first.Close()

	// A crashed instance leaves its socket file behind with nobody listening.
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	stale.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stale socket was not left behind: %v", err)
	}
	second, err := Listen(path, handler)
	if err != nil {
		t.Fatalf("Listen over stale socket: %v", err)
	}
	defer second.Close()
	if _, err := Send(path, CommandStatus); err != nil {
		t.Fatal(err)
	}
}

func TestHandlerPanicIsReportedToCaller(t *testing.T) {
	path := socketPath(t)
	server, err := Listen(path, func(string) (string, error) { panic("boom") })
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if _, err := Send(path, CommandToggle); err == nil {
		t.Fatal("panic was reported as success")
	}
	if _, err := Send(path, CommandToggle); err == nil {
		t.Fatal("server stopped answering after a handler panic")
	}
}
