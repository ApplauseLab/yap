//go:build windows

package overlay

import (
	"image"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"yap/internal/logger"
)

var user32 = windows.NewLazySystemDLL("user32.dll")
var gdi32 = windows.NewLazySystemDLL("gdi32.dll")

func call(name string, args ...uintptr) uintptr {
	r, _, _ := user32.NewProc(name).Call(args...)
	return r
}

type rect struct{ left, top, right, bottom int32 }
type point struct{ x, y int32 }
type message struct {
	hwnd           uintptr
	id             uint32
	wparam, lparam uintptr
	time           uint32
	pt             point
	private        uint32
}
type windowClass struct {
	size, style                        uint32
	proc                               uintptr
	classExtra, windowExtra            int32
	instance, icon, cursor, background uintptr
	menu, name                         *uint16
	smallIcon                          uintptr
}

// Overlay owns a non-activating tool window on a dedicated Windows UI thread.
// Desired state is read on a timer so audio callbacks never wait on the UI.
type Overlay struct {
	mu                 sync.Mutex
	visible, destroyed bool
	status             string
	level              float32
	stop, cancel       func()
	hwnd               uintptr
	done               chan struct{}
	history            [waveformBars]float32
	frameErr           error
	stopKey, cancelKey string
	// Button rectangles from the last drawn frame, used for hit testing.
	stopRect, cancelRect image.Rectangle
}

func New() *Overlay {
	o := &Overlay{status: "Recording...", done: make(chan struct{})}
	ready := make(chan struct{})
	go o.run(ready)
	<-ready
	return o
}

func (o *Overlay) Show() {
	o.mu.Lock()
	o.visible = true
	o.status = "Recording..."
	o.level = 0
	o.history = [waveformBars]float32{}
	o.mu.Unlock()
}
func (o *Overlay) Hide()                       { o.mu.Lock(); o.visible = false; o.mu.Unlock() }
func (o *Overlay) SetStatus(s string)          { o.mu.Lock(); o.status = s; o.mu.Unlock() }
func (o *Overlay) SetAudioLevel(v float32)     { o.mu.Lock(); o.level = v; o.mu.Unlock() }
func (o *Overlay) SetStopCallback(cb func())   { o.mu.Lock(); o.stop = cb; o.mu.Unlock() }
func (o *Overlay) SetCancelCallback(cb func()) { o.mu.Lock(); o.cancel = cb; o.mu.Unlock() }

// SetHotkeyLabels sets the key labels shown on the Stop and Cancel buttons.
func (o *Overlay) SetHotkeyLabels(stop, cancel string) {
	o.mu.Lock()
	o.stopKey, o.cancelKey = stop, cancel
	o.mu.Unlock()
}
func (o *Overlay) Destroy() {
	o.mu.Lock()
	o.destroyed = true
	o.mu.Unlock()
	<-o.done
}

func (o *Overlay) run(ready chan struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(o.done)
	var renderer *overlayRenderer
	var surface *layeredSurface
	defer func() {
		if renderer != nil {
			renderer.face.Close()
		}
		if surface != nil {
			surface.close()
		}
	}()
	name, _ := windows.UTF16PtrFromString("YapRecordingOverlay")
	instance, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	proc := windows.NewCallback(func(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
		switch msg {
		case 0x21: // WM_MOUSEACTIVATE: accept clicks without stealing input focus.
			return 3 // MA_NOACTIVATE
		case 0x0f:
			call("ValidateRect", hwnd, 0)
			return 0
		case 0x202: // WM_LBUTTONUP
			scale := float64(call("GetDpiForWindow", hwnd)) / 96
			if scale == 0 {
				scale = 1
			}
			p := image.Pt(int(float64(int16(lp))/scale), int(float64(int16(lp>>16))/scale))
			o.mu.Lock()
			var cb func()
			if o.status == "Recording..." {
				if p.In(o.stopRect) {
					cb = o.stop
				}
				if p.In(o.cancelRect) {
					cb = o.cancel
				}
			}
			o.mu.Unlock()
			if cb != nil {
				go cb()
			}
			return 0
		case 2: // WM_DESTROY
			call("PostQuitMessage", 0)
			return 0
		}
		return call("DefWindowProcW", hwnd, uintptr(msg), wp, lp)
	})
	wc := windowClass{proc: proc, instance: instance, name: name}
	wc.size = uint32(unsafe.Sizeof(wc))
	if call("RegisterClassExW", uintptr(unsafe.Pointer(&wc))) == 0 {
		logger.Error("Could not register Windows recording overlay")
		close(ready)
		return
	}
	defer call("UnregisterClassW", uintptr(unsafe.Pointer(name)), instance)
	// WS_EX_TOPMOST | WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE, WS_POPUP.
	hwnd := call("CreateWindowExW", 0x08080088, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(name)), 0x80000000, 0, 0, overlayWidth, overlayHeight, 0, 0, instance, 0)
	o.hwnd = hwnd
	close(ready)
	if hwnd == 0 {
		logger.Error("Could not create Windows recording overlay")
		return
	}
	defer call("DestroyWindow", hwnd)
	if call("SetTimer", hwnd, 1, 33, 0) == 0 {
		logger.Error("Could not start Windows overlay timer")
		return
	}
	defer call("KillTimer", hwnd, 1)
	shown := false
	var msg message
	for {
		result := call("GetMessageW", uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if result == 0 || int32(result) == -1 {
			return
		}
		o.mu.Lock()
		visible, destroyed := o.visible, o.destroyed
		o.mu.Unlock()
		if destroyed {
			call("DestroyWindow", hwnd)
			continue // Consume WM_QUIT before releasing this OS thread.
		}
		if visible != shown {
			if visible {
				// Place on the foreground application's monitor, inside its work area.
				monitor := call("MonitorFromWindow", call("GetForegroundWindow"), 2)
				info := struct {
					size          uint32
					monitor, work rect
					flags         uint32
				}{}
				info.size = uint32(unsafe.Sizeof(info))
				call("GetMonitorInfoW", monitor, uintptr(unsafe.Pointer(&info)))
				// Move first so GetDpiForWindow reports the destination monitor's DPI.
				call("SetWindowPos", hwnd, ^uintptr(0), uintptr(info.work.left), uintptr(info.work.top+40), 0, 0, 0x11)
				scale := float64(call("GetDpiForWindow", hwnd)) / 96
				if scale == 0 {
					scale = 1
				}
				if renderer == nil || renderer.scale != scale {
					if renderer != nil {
						renderer.face.Close()
						surface.close()
					}
					fontData, _ := os.ReadFile(filepath.Join(os.Getenv("WINDIR"), "Fonts", "seguisb.ttf"))
					var err error
					renderer, err = newOverlayRenderer(scale, fontData)
					if err != nil {
						logger.Error("Could not load overlay font: " + err.Error())
						return
					}
					surface, err = newLayeredSurface(renderer.background.Bounds().Size())
					if err != nil {
						logger.Error(err.Error())
						return
					}
				}
				width, height := renderer.background.Rect.Dx(), renderer.background.Rect.Dy()
				x := info.work.left + (info.work.right-info.work.left-int32(width))/2
				call("SetWindowPos", hwnd, ^uintptr(0), uintptr(x), uintptr(info.work.top+int32(40*scale)), uintptr(width), uintptr(height), 0x10)
				o.drawFrame(hwnd, renderer, surface, false)
				call("ShowWindow", hwnd, 4) // SW_SHOWNOACTIVATE
			} else {
				call("ShowWindow", hwnd, 0)
			}
			shown = visible
		}
		if shown && msg.id == 0x113 {
			o.drawFrame(hwnd, renderer, surface, true)
		}
		call("TranslateMessage", uintptr(unsafe.Pointer(&msg)))
		call("DispatchMessageW", uintptr(unsafe.Pointer(&msg)))
	}
}

func (o *Overlay) drawFrame(hwnd uintptr, renderer *overlayRenderer, surface *layeredSurface, advance bool) {
	o.mu.Lock()
	if advance {
		copy(o.history[:], o.history[1:])
		o.history[waveformBars-1] = o.level
	}
	status, history := o.status, o.history
	stopText, cancelText := buttonText("Stop", o.stopKey), buttonText("Cancel", o.cancelKey)
	o.mu.Unlock()
	stopRect, cancelRect := renderer.buttons(stopText, cancelText)
	var cursor point
	call("GetCursorPos", uintptr(unsafe.Pointer(&cursor)))
	call("ScreenToClient", hwnd, uintptr(unsafe.Pointer(&cursor)))
	p := image.Pt(int(float64(cursor.x)/renderer.scale), int(float64(cursor.y)/renderer.scale))
	hover := 0
	if p.In(stopRect) {
		hover = 1
	}
	if p.In(cancelRect) {
		hover = 2
	}
	err := surface.present(hwnd, renderer.render(status, stopText, cancelText, history, hover))
	o.mu.Lock()
	o.stopRect, o.cancelRect = stopRect, cancelRect
	if err != nil && o.frameErr == nil {
		logger.Error(err.Error())
	}
	o.frameErr = err
	o.mu.Unlock()
}
