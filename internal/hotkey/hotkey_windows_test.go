//go:build windows

package hotkey

import (
	"testing"
	"time"
)

func TestWindowsHookStartup(t *testing.T) {
	// Resolve before calling: LazyProc.Call panics if an export is absent,
	// including when called on Register's background goroutine.
	if err := procGetCurrentThreadId.Find(); err != nil {
		t.Fatalf("hotkey startup cannot resolve thread ID function: %v", err)
	}
	m := NewManager()
	if err := m.Register(func() {}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Unregister() })
	if !m.IsRegistered() {
		t.Fatal("hook did not start")
	}
	if err := m.Unregister(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for m.IsRegistered() {
		if time.Now().After(deadline) {
			t.Fatal("hook message loop did not stop")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHeldRecordingKeyOnlyTriggersOnce(t *testing.T) {
	m := NewManager()
	calls := make(chan struct{}, 100)
	m.hotkeyCallback = func() { calls <- struct{}{} }
	for i := 0; i < 60; i++ {
		m.handleKey(VK_RMENU, WM_SYSKEYDOWN)
	}
	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("first press was ignored")
	}
	select {
	case <-calls:
		t.Fatal("held key repeated the recording toggle")
	case <-time.After(50 * time.Millisecond):
	}
	m.handleKey(VK_RMENU, WM_SYSKEYUP)
	m.handleKey(VK_RMENU, WM_SYSKEYDOWN)
	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("second press was ignored")
	}
}
