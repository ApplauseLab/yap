//go:build !windows

package transcribe

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"yap/internal/proctest"
)

// childPIDs returns the PIDs of the current children of this test process.
func childPIDs(t *testing.T) map[int]bool {
	t.Helper()
	children, err := proctest.Children(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	pids := make(map[int]bool, len(children))
	for _, child := range children {
		pids[child.PID] = true
	}
	return pids
}

// assertNoNewChildren fails if any child started after before is still
// running or left unreaped.
func assertNoNewChildren(t *testing.T, before map[int]bool) {
	t.Helper()
	remaining, err := proctest.WaitNoChildren(os.Getpid(), 10*time.Second, func(child proctest.Child) bool {
		return !before[child.PID]
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) > 0 {
		t.Fatalf("%d child processes left behind: %+v", len(remaining), remaining)
	}
}

func newFailingWorkerLauncher(t *testing.T, tmpDir string) string {
	t.Helper()
	launcher := filepath.Join(tmpDir, "whisper-server")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return launcher
}

func TestFailedWorkerStartupLeavesNoProcessOrRecord(t *testing.T) {
	tmpDir := t.TempDir()
	modelsDir := newTestModelsDir(t, tmpDir)
	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperServerBinary(newFailingWorkerLauncher(t, tmpDir))
	t.Cleanup(func() { _ = engine.Close() })
	before := childPIDs(t)

	iterations := 200
	if testing.Short() {
		iterations = 5
	}
	for i := 0; i < iterations; i++ {
		if err := engine.Prewarm(context.Background()); err == nil {
			t.Fatal("prewarm succeeded with a failing worker")
		}
	}

	if _, err := os.Stat(engine.workerPIDPath()); !os.IsNotExist(err) {
		t.Fatalf("pid file left after failed startup: %v", err)
	}
	assertNoNewChildren(t, before)
}

func TestWorkerRestartsLeaveNoZombies(t *testing.T) {
	tmpDir := t.TempDir()
	modelsDir := newTestModelsDir(t, tmpDir)
	if err := os.WriteFile(filepath.Join(modelsDir, "ggml-tiny.en.bin"), []byte("model"), 0644); err != nil {
		t.Fatal(err)
	}
	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperServerBinary(newTestWorkerLauncher(t, tmpDir))
	t.Cleanup(func() { _ = engine.Close() })
	before := childPIDs(t)

	iterations := 50
	if testing.Short() {
		iterations = 3
	}
	models := []Model{ModelTinyEn, ModelBaseEn}
	for i := 0; i < iterations; i++ {
		if err := engine.SetModel(models[i%2]); err != nil {
			t.Fatal(err)
		}
		if err := engine.Prewarm(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	zombies, err := proctest.Zombies(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	// Only the live worker (and its watchdog) may remain; anything else from
	// earlier restarts must already be reaped or be on its way out.
	if len(zombies) > 2 {
		t.Fatalf("%d zombie children after %d worker restarts", len(zombies), iterations)
	}

	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(engine.workerPIDPath()); !os.IsNotExist(err) {
		t.Fatalf("pid file left after Close: %v", err)
	}
	assertNoNewChildren(t, before)
}

func TestPersistentWorkerRecordedWhileStarting(t *testing.T) {
	tmpDir := t.TempDir()
	modelsDir := newTestModelsDir(t, tmpDir)
	gate := filepath.Join(tmpDir, "ready-gate")
	t.Setenv("YAP_TEST_WORKER_STARTUP_GATE", gate)
	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperServerBinary(newTestWorkerLauncher(t, tmpDir))
	t.Cleanup(func() { _ = engine.Close() })

	result := make(chan error, 1)
	go func() { result <- engine.Prewarm(context.Background()) }()
	defer func() {
		_ = os.WriteFile(gate, nil, 0644)
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(engine.workerPIDPath())
		if err == nil {
			fields := strings.Fields(string(data))
			if len(fields) != 2 || fields[1] != strconv.Itoa(os.Getpid()) {
				t.Fatalf("pid file = %q", data)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("worker pid not recorded while the worker was starting")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestReapOrphanedWorkerScansEveryOwnerRecord(t *testing.T) {
	tmpDir := t.TempDir()
	var orphans []*exec.Cmd
	for i := 0; i < 2; i++ {
		dir := filepath.Join(tmpDir, fmt.Sprintf("orphan-%d", i))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		orphans = append(orphans, startFakeWhisperServer(t, dir))
	}
	liveDir := filepath.Join(tmpDir, "live")
	if err := os.MkdirAll(liveDir, 0755); err != nil {
		t.Fatal(err)
	}
	live := startFakeWhisperServer(t, liveDir)
	engine := NewLocalEngine(tmpDir)
	deadOwner := deadPID(t)
	liveOwner := os.Getppid()
	liveRecord := fmt.Sprintf("whisper-worker-%d.pid", liveOwner)
	records := map[string]string{
		// Shared record written by Yap 0.4.2.
		"whisper-worker.pid":                            fmt.Sprintf("%d %d", orphans[0].Process.Pid, deadOwner),
		fmt.Sprintf("whisper-worker-%d.pid", deadOwner): fmt.Sprintf("%d %d", orphans[1].Process.Pid, deadOwner),
		liveRecord: fmt.Sprintf("%d %d", live.Process.Pid, liveOwner),
	}
	for name, record := range records {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(record), 0600); err != nil {
			t.Fatal(err)
		}
	}

	engine.ReapOrphanedWorker()

	for i, orphan := range orphans {
		if !waitForExit(orphan, 3*time.Second) {
			t.Fatalf("orphan %d was not stopped", i)
		}
	}
	if waitForExit(live, 300*time.Millisecond) {
		t.Fatal("worker owned by a live Yap process was stopped")
	}
	for name := range records {
		_, err := os.Stat(filepath.Join(tmpDir, name))
		if name == liveRecord && err != nil {
			t.Fatalf("live owner's pid file removed: %v", err)
		}
		if name != liveRecord && !os.IsNotExist(err) {
			t.Fatalf("orphan pid file %s not removed: %v", name, err)
		}
	}
}

// recordedWorkerOwners returns the owner PIDs of every worker record in dir.
func recordedWorkerOwners(t *testing.T, dir string) map[int]bool {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "whisper-worker*.pid"))
	if err != nil {
		t.Fatal(err)
	}
	owners := make(map[int]bool)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		fields := strings.Fields(string(data))
		if len(fields) != 2 {
			t.Fatalf("%s = %q", path, data)
		}
		owner, err := strconv.Atoi(fields[1])
		if err != nil {
			t.Fatal(err)
		}
		owners[owner] = true
	}
	return owners
}

// TestWorkerOwnerHelper runs a Yap stand-in that owns a persistent worker
// until it is killed.
func TestWorkerOwnerHelper(t *testing.T) {
	modelsDir := os.Getenv("YAP_TEST_WORKER_OWNER_MODELS")
	if modelsDir == "" {
		return
	}
	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperServerBinary(os.Getenv("YAP_TEST_WORKER_OWNER_LAUNCHER"))
	if err := engine.Prewarm(context.Background()); err != nil {
		os.Exit(3)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "owner-ready"), nil, 0644); err != nil {
		os.Exit(3)
	}
	time.Sleep(time.Minute)
	os.Exit(0)
}

// startWorkerOwner runs TestWorkerOwnerHelper in a separate process that owns
// a persistent worker in modelsDir, and waits until that worker is ready.
func startWorkerOwner(t *testing.T, tmpDir string, modelsDir string) *exec.Cmd {
	t.Helper()
	owner := exec.Command(os.Args[0], "-test.run=^TestWorkerOwnerHelper$")
	owner.Env = append(os.Environ(),
		"YAP_TEST_WORKER_OWNER_MODELS="+modelsDir,
		"YAP_TEST_WORKER_OWNER_LAUNCHER="+newTestWorkerLauncher(t, tmpDir),
	)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Children of a live owner cannot have reused PIDs. Stop them first so
		// nothing outlives the test on platforms without the watchdog.
		if children, err := proctest.Children(owner.Process.Pid); err == nil {
			for _, child := range children {
				_ = syscall.Kill(child.PID, syscall.SIGKILL)
			}
		}
		_ = owner.Process.Kill()
		_ = owner.Wait()
	})
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(modelsDir, "owner-ready")); err == nil {
			return owner
		}
		if time.Now().After(deadline) {
			t.Fatal("owner did not start its worker")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestInstancesSharingDataDirKeepEachOthersRecords(t *testing.T) {
	tmpDir := t.TempDir()
	modelsDir := newTestModelsDir(t, tmpDir)
	otherPID := startWorkerOwner(t, tmpDir, modelsDir).Process.Pid
	if !recordedWorkerOwners(t, modelsDir)[otherPID] {
		t.Fatal("other instance's worker is not recorded")
	}

	engine := NewLocalEngine(modelsDir)
	engine.SetWhisperServerBinary(newTestWorkerLauncher(t, tmpDir))
	t.Cleanup(func() { _ = engine.Close() })
	engine.ReapOrphanedWorker()
	if err := engine.Prewarm(context.Background()); err != nil {
		t.Fatal(err)
	}
	owners := recordedWorkerOwners(t, modelsDir)
	if !owners[otherPID] || !owners[os.Getpid()] {
		t.Fatalf("recorded owners = %v, want both %d and %d", owners, otherPID, os.Getpid())
	}

	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	owners = recordedWorkerOwners(t, modelsDir)
	if !owners[otherPID] || owners[os.Getpid()] {
		t.Fatalf("recorded owners after Close = %v, want only %d", owners, otherPID)
	}
}

func processGone(pid int) bool {
	if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
		return true
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	return err != nil || strings.HasPrefix(strings.TrimSpace(string(out)), "Z")
}

func TestWorkerStopsWhenOwnerIsKilled(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the worker watchdog is macOS-only")
	}
	tmpDir := t.TempDir()
	modelsDir := newTestModelsDir(t, tmpDir)
	owner := startWorkerOwner(t, tmpDir, modelsDir)
	children, err := proctest.Children(owner.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	gone := make(map[int]bool, len(children))
	t.Cleanup(func() {
		// Only kill children last seen alive, so a reused PID is never hit.
		for _, child := range children {
			if !gone[child.PID] {
				_ = syscall.Kill(child.PID, syscall.SIGKILL)
			}
		}
	})
	if len(children) != 2 {
		t.Errorf("owner children = %+v, want worker and watchdog", children)
	}

	if err := owner.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = owner.Wait()

	deadline := time.Now().Add(2*workerStopTimeout + 3*time.Second)
	for _, child := range children {
		for !processGone(child.PID) {
			if time.Now().After(deadline) {
				t.Fatalf("child %+v of the killed owner is still running", child)
			}
			time.Sleep(50 * time.Millisecond)
		}
		gone[child.PID] = true
	}
}
