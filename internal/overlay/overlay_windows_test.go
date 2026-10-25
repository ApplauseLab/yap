//go:build windows

package overlay

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
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
	o.mu.Lock()
	frameErr := o.frameErr
	o.mu.Unlock()
	if frameErr != nil {
		t.Fatal(frameErr)
	}
	if dir := os.Getenv("YAP_TEST_OVERLAY_ARTIFACTS"); dir != "" {
		captureOverlayWindow(t, o.hwnd, dir)
	}
	if got := call("GetForegroundWindow"); got != foreground {
		t.Fatalf("overlay stole foreground: %x -> %x", foreground, got)
	}
	if result := call("SendMessageW", o.hwnd, 0x21, 0, 0); result != 3 {
		t.Fatal("mouse clicks can activate overlay")
	}
	clicked := make(chan struct{}, 1)
	o.SetStopCallback(func() { clicked <- struct{}{} })
	scale := float64(call("GetDpiForWindow", o.hwnd)) / 96
	click := func(x, y int) {
		call("SendMessageW", o.hwnd, 0x202, 0, uintptr(int(float64(x)*scale)|(int(float64(y)*scale)<<16)))
	}
	click(250, 82)
	select {
	case <-clicked:
	case <-time.After(time.Second):
		t.Fatal("stop callback missing")
	}
	o.SetAudioLevel(0.8)
	o.SetCancelCallback(func() { clicked <- struct{}{} })
	click(330, 82)
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

// Capture the actual composed Windows desktop crop as well as renderer previews.
func captureOverlayWindow(t *testing.T, hwnd uintptr, dir string) {
	t.Helper()
	windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmFlush").Call()
	var bounds rect
	if call("GetWindowRect", hwnd, uintptr(unsafe.Pointer(&bounds))) == 0 {
		t.Fatal("cannot locate overlay")
	}
	surface, err := newLayeredSurface(image.Pt(int(bounds.right-bounds.left), int(bounds.bottom-bounds.top)))
	if err != nil {
		t.Fatal(err)
	}
	defer surface.close()
	dc := call("GetDC", 0)
	defer call("ReleaseDC", 0, dc)
	result, _, _ := gdi32.NewProc("BitBlt").Call(surface.dc, 0, 0, uintptr(surface.size.x), uintptr(surface.size.y), dc, uintptr(bounds.left), uintptr(bounds.top), 0x40CC0020) // SRCCOPY | CAPTUREBLT
	if result == 0 {
		t.Fatal("cannot capture overlay")
	}
	frame := image.NewRGBA(image.Rect(0, 0, int(surface.size.x), int(surface.size.y)))
	for i := 0; i < len(frame.Pix); i += 4 {
		frame.Pix[i], frame.Pix[i+1], frame.Pix[i+2], frame.Pix[i+3] = surface.pixels[i+2], surface.pixels[i+1], surface.pixels[i], 255
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "windows-overlay-desktop.png"))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, frame)
	closeErr := f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}
