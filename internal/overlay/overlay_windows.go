//go:build windows

package overlay

import (
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
type paintStruct struct {
	dc              uintptr
	erase           int32
	area            rect
	restore, update int32
	reserved        [32]byte
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
	o.mu.Unlock()
}
func (o *Overlay) Hide()                       { o.mu.Lock(); o.visible = false; o.mu.Unlock() }
func (o *Overlay) SetStatus(s string)          { o.mu.Lock(); o.status = s; o.mu.Unlock() }
func (o *Overlay) SetAudioLevel(v float32)     { o.mu.Lock(); o.level = v; o.mu.Unlock() }
func (o *Overlay) SetStopCallback(cb func())   { o.mu.Lock(); o.stop = cb; o.mu.Unlock() }
func (o *Overlay) SetCancelCallback(cb func()) { o.mu.Lock(); o.cancel = cb; o.mu.Unlock() }
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
	name, _ := windows.UTF16PtrFromString("YapRecordingOverlay")
	instance, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	proc := windows.NewCallback(func(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
		switch msg {
		case 0x21: // WM_MOUSEACTIVATE: accept clicks without stealing input focus.
			return 3 // MA_NOACTIVATE
		case 0x0f:
			o.paint(hwnd)
			return 0
		case 0x202: // WM_LBUTTONUP
			x, y := int32(int16(lp)), int32(int16(lp>>16))
			o.mu.Lock()
			var cb func()
			if y >= 55 && y <= 94 {
				if x >= 210 && x < 285 && o.status == "Recording..." {
					cb = o.stop
				}
				if x >= 290 && x < 375 && o.status == "Recording..." {
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
	hwnd := call("CreateWindowExW", 0x08000088, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(name)), 0x80000000, 0, 0, 380, 100, 0, 0, instance, 0)
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
			return
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
				x := info.work.left + (info.work.right-info.work.left-380)/2
				call("SetWindowPos", hwnd, ^uintptr(0), uintptr(x), uintptr(info.work.top+40), 380, 100, 0x50) // SHOWWINDOW | NOACTIVATE
			} else {
				call("ShowWindow", hwnd, 0)
			}
			shown = visible
		}
		if shown && msg.id == 0x113 {
			call("InvalidateRect", hwnd, 0, 0)
		}
		call("TranslateMessage", uintptr(unsafe.Pointer(&msg)))
		call("DispatchMessageW", uintptr(unsafe.Pointer(&msg)))
	}
}

func (o *Overlay) paint(hwnd uintptr) {
	var ps paintStruct
	dc := call("BeginPaint", hwnd, uintptr(unsafe.Pointer(&ps)))
	defer call("EndPaint", hwnd, uintptr(unsafe.Pointer(&ps)))
	fill := func(r rect, color uintptr) {
		brush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(color)
		call("FillRect", dc, uintptr(unsafe.Pointer(&r)), brush)
		gdi32.NewProc("DeleteObject").Call(brush)
	}
	fill(rect{0, 0, 380, 100}, 0x202020)
	o.mu.Lock()
	status, level := o.status, o.level
	o.mu.Unlock()
	if level < 0 {
		level = 0
	}
	if level > 1 {
		level = 1
	}
	for i := int32(0); i < 50; i++ {
		h := int32(4 + level*28)
		fill(rect{20 + i*7, 27 - h/2, 23 + i*7, 27 + h/2}, 0x4eff00)
	}
	gdi32.NewProc("SetBkMode").Call(dc, 1)
	gdi32.NewProc("SetTextColor").Call(dc, 0xeeeeee)
	font, _, _ := gdi32.NewProc("GetStockObject").Call(17) // DEFAULT_GUI_FONT
	old, _, _ := gdi32.NewProc("SelectObject").Call(dc, font)
	defer gdi32.NewProc("SelectObject").Call(dc, old)
	text := func(s string, r rect) {
		p, _ := windows.UTF16PtrFromString(s)
		call("DrawTextW", dc, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x24)
	}
	text(status, rect{20, 55, 205, 94})
	if status == "Recording..." {
		fill(rect{210, 55, 285, 94}, 0x383838)
		fill(rect{290, 55, 375, 94}, 0x383838)
		text("Stop", rect{225, 55, 280, 94})
		text("Cancel", rect{303, 55, 370, 94})
	}
}
