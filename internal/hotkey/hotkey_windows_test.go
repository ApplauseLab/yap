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
