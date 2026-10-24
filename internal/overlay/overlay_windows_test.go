//go:build windows

package overlay

import (
	"testing"
	"time"
)

func TestWindowsOverlayLifecycleAndFocus(t *testing.T) {
	o := New()
	t.Cleanup(o.Destroy)
	if o.hwnd == 0 {
		t.Fatal("overlay window was not created")
	}
	style := call("GetWindowLongW", o.hwnd, ^uintptr(19)) // GWL_EXSTYLE = -20
	if style&0x08000088 != 0x08000088 {
		t.Fatalf("missing topmost/toolwindow/noactivate styles: %x", style)
	}
	waitVisible := func(want bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if (call("IsWindowVisible", o.hwnd) != 0) == want {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("overlay visibility did not become %v", want)
	}
	foreground := call("GetForegroundWindow")
	o.Show()
	waitVisible(true)
	if got := call("GetForegroundWindow"); got != foreground {
		t.Fatalf("overlay stole foreground: %x -> %x", foreground, got)
	}
	if result := call("SendMessageW", o.hwnd, 0x21, 0, 0); result != 3 {
		t.Fatal("mouse clicks can activate overlay")
	}
	clicked := make(chan struct{}, 1)
	o.SetStopCallback(func() { clicked <- struct{}{} })
	call("SendMessageW", o.hwnd, 0x202, 0, 230|(70<<16))
	select {
	case <-clicked:
	case <-time.After(time.Second):
		t.Fatal("stop callback missing")
	}
	o.SetAudioLevel(0.8)
	o.SetCancelCallback(func() { clicked <- struct{}{} })
	call("SendMessageW", o.hwnd, 0x202, 0, 320|(70<<16))
	select {
	case <-clicked:
	case <-time.After(time.Second):
		t.Fatal("cancel callback missing")
	}
	o.SetStatus("Transcribing...")
	o.Hide()
	waitVisible(false)
	o.Show()
	waitVisible(true)
	o.Destroy()
	if call("IsWindow", o.hwnd) != 0 {
		t.Fatal("destroy left overlay window alive")
	}
}
