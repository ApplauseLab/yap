//go:build windows

package hotkey

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"yap/internal/logger"

	"golang.org/x/sys/windows"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostThreadMessageW  = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadId  = user32.NewProc("GetCurrentThreadId")
)

const (
	WH_KEYBOARD_LL = 13
	WM_KEYDOWN     = 0x0100
	WM_KEYUP       = 0x0101
	WM_SYSKEYDOWN  = 0x0104
	WM_SYSKEYUP    = 0x0105
	WM_QUIT        = 0x0012
)

// Virtual key codes
const (
	VK_LSHIFT   = 0xA0
	VK_RSHIFT   = 0xA1
	VK_LCONTROL = 0xA2
	VK_RCONTROL = 0xA3
	VK_LMENU    = 0xA4 // Left Alt
	VK_RMENU    = 0xA5 // Right Alt
	VK_LWIN     = 0x5B
	VK_RWIN     = 0x5C
	VK_ESCAPE   = 0x1B
	VK_SPACE    = 0x20
	VK_RETURN   = 0x0D
	VK_TAB      = 0x09
	VK_CAPITAL  = 0x14 // Caps Lock
	VK_BACK     = 0x08 // Backspace
	VK_DELETE   = 0x2E
	VK_LEFT     = 0x25
	VK_UP       = 0x26
	VK_RIGHT    = 0x27
	VK_DOWN     = 0x28
	VK_F1       = 0x70
	VK_F2       = 0x71
	VK_F3       = 0x72
	VK_F4       = 0x73
	VK_F5       = 0x74
	VK_F6       = 0x75
	VK_F7       = 0x76
	VK_F8       = 0x77
	VK_F9       = 0x78
	VK_F10      = 0x79
	VK_F11      = 0x7A
	VK_F12      = 0x7B
)

type KBDLLHOOKSTRUCT struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

// Callback is the function type for hotkey events
type Callback func()

// Manager handles global hotkey registration
type Manager struct {
	mu             sync.Mutex
	running        bool
	hotkeyCode     uint32
	cancelCode     uint32
	hotkeyCallback Callback
	cancelCallback func()
	cancelEnabled  bool
	threadId       uint32
	hotkeyStr      string
	cancelStr      string
}

// Global manager pointer for the callback
var (
	globalManager   *Manager
	globalManagerMu sync.Mutex
)

// NewManager creates a new hotkey manager
func NewManager() *Manager {
	m := &Manager{
		hotkeyStr:  "rightalt",
		cancelStr:  "escape",
		hotkeyCode: VK_RMENU,
		cancelCode: VK_ESCAPE,
	}
	return m
}

// The keyboard hook callback - must be a simple function for syscall.NewCallback
func keyboardProc(nCode int32, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 {
		globalManagerMu.Lock()
		m := globalManager
		globalManagerMu.Unlock()

		if m != nil {
			kbStruct := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))

			// Handle both keydown and syskeydown (for Alt key)
			if wParam == WM_KEYDOWN || wParam == WM_SYSKEYDOWN {
				// Check recording hotkey
				if kbStruct.VkCode == m.hotkeyCode {
					if m.hotkeyCallback != nil {
						go m.hotkeyCallback()
					}
				}

				// Check cancel key
				if m.cancelEnabled && kbStruct.VkCode == m.cancelCode {
					if m.cancelCallback != nil {
						go m.cancelCallback()
					}
				}
			}
		}
	}

	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

// Register registers the global hotkey
func (m *Manager) Register(cb Callback) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.hotkeyCallback = cb
	m.hotkeyCode = KeyNameToCode(m.hotkeyStr)
	m.cancelCode = KeyNameToCode(m.cancelStr)
	m.mu.Unlock()

	// Set global manager for callback
	globalManagerMu.Lock()
	globalManager = m
	globalManagerMu.Unlock()

	// Start the hook in a dedicated goroutine locked to an OS thread
	started := make(chan error, 1)
	go func() {
		// Lock this goroutine to the current OS thread - required for Windows hooks
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// Get current thread ID
		threadId, _, _ := procGetCurrentThreadId.Call()
		m.mu.Lock()
		m.threadId = uint32(threadId)
		m.mu.Unlock()

		// Install the keyboard hook
		hookHandle, _, err := procSetWindowsHookExW.Call(
			WH_KEYBOARD_LL,
			windows.NewCallback(keyboardProc),
			0,
			0,
		)

		if hookHandle == 0 {
			started <- fmt.Errorf("failed to set keyboard hook: %v", err)
			return
		}

		m.mu.Lock()
		m.running = true
		m.mu.Unlock()

		logger.Info(fmt.Sprintf("Keyboard hook installed, hotkey: %s (code: %d)", m.hotkeyStr, m.hotkeyCode))
		started <- nil

		// Message loop - required for the hook to work
		var msg MSG
		for {
			ret, _, _ := procGetMessageW.Call(
				uintptr(unsafe.Pointer(&msg)),
				0, 0, 0,
			)

			// ret == 0 means WM_QUIT, ret == -1 means error
			if ret == 0 || int32(ret) == -1 {
				break
			}

			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}

		// Cleanup
		procUnhookWindowsHookEx.Call(hookHandle)
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	// Wait for hook to be installed
	if err := <-started; err != nil {
		return err
	}

	return nil
}

// Unregister removes the hotkey
func (m *Manager) Unregister() error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	threadId := m.threadId
	m.mu.Unlock()

	// Post WM_QUIT to the message loop to stop it
	if threadId != 0 {
		procPostThreadMessageW.Call(uintptr(threadId), WM_QUIT, 0, 0)
	}

	// Clear global manager
	globalManagerMu.Lock()
	globalManager = nil
	globalManagerMu.Unlock()

	return nil
}

// SetHotkeyType sets the recording hotkey by name
func (m *Manager) SetHotkeyType(hotkeyName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.hotkeyStr = strings.ToLower(hotkeyName)
	m.hotkeyCode = KeyNameToCode(m.hotkeyStr)

	logger.Info(fmt.Sprintf("Hotkey set to: %s (code: %d)", m.hotkeyStr, m.hotkeyCode))
}

// SetCancelKey sets the cancel hotkey by name
func (m *Manager) SetCancelKey(keyName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cancelStr = strings.ToLower(keyName)
	m.cancelCode = KeyNameToCode(m.cancelStr)

	logger.Info(fmt.Sprintf("Cancel key set to: %s (code: %d)", m.cancelStr, m.cancelCode))
}

// IsRegistered returns whether hotkey is registered
func (m *Manager) IsRegistered() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// EnableCancelKey starts monitoring for the cancel key
func (m *Manager) EnableCancelKey(cb func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cancelCallback = cb
	m.cancelEnabled = true
}

// DisableCancelKey stops monitoring for the cancel key
func (m *Manager) DisableCancelKey() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cancelEnabled = false
	m.cancelCallback = nil
}

// GetHotkeyDisplayName returns the display name for a hotkey
func GetHotkeyDisplayName(hotkeyName string) string {
	return KeyNameToDisplayName(hotkeyName)
}

// RequestAccessibilityPermissions is a no-op on Windows
func RequestAccessibilityPermissions() bool {
	return true
}

// HasAccessibilityPermissions always returns true on Windows (no special permissions needed)
func HasAccessibilityPermissions() bool {
	return true
}

// KeyNameToCode converts a key name to a Windows virtual key code
func KeyNameToCode(name string) uint32 {
	switch strings.ToLower(name) {
	// Modifier keys
	case "rightalt", "rightoption":
		return VK_RMENU
	case "leftalt", "leftoption":
		return VK_LMENU
	case "rightctrl", "rightcontrol":
		return VK_RCONTROL
	case "leftctrl", "leftcontrol":
		return VK_LCONTROL
	case "rightshift":
		return VK_RSHIFT
	case "leftshift":
		return VK_LSHIFT
	case "rightwin", "rightsuper", "rightcommand", "rightcmd":
		return VK_RWIN
	case "leftwin", "leftsuper", "leftcommand", "leftcmd":
		return VK_LWIN
	case "capslock":
		return VK_CAPITAL

	// Special keys
	case "escape", "esc":
		return VK_ESCAPE
	case "space":
		return VK_SPACE
	case "tab":
		return VK_TAB
	case "return", "enter":
		return VK_RETURN
	case "backspace":
		return VK_BACK
	case "delete":
		return VK_DELETE

	// Arrow keys
	case "left", "arrowleft":
		return VK_LEFT
	case "right", "arrowright":
		return VK_RIGHT
	case "up", "arrowup":
		return VK_UP
	case "down", "arrowdown":
		return VK_DOWN

	// Function keys
	case "f1":
		return VK_F1
	case "f2":
		return VK_F2
	case "f3":
		return VK_F3
	case "f4":
		return VK_F4
	case "f5":
		return VK_F5
	case "f6":
		return VK_F6
	case "f7":
		return VK_F7
	case "f8":
		return VK_F8
	case "f9":
		return VK_F9
	case "f10":
		return VK_F10
	case "f11":
		return VK_F11
	case "f12":
		return VK_F12

	// Letter keys (A-Z are 0x41-0x5A)
	case "a":
		return 0x41
	case "b":
		return 0x42
	case "c":
		return 0x43
	case "d":
		return 0x44
	case "e":
		return 0x45
	case "f":
		return 0x46
	case "g":
		return 0x47
	case "h":
		return 0x48
	case "i":
		return 0x49
	case "j":
		return 0x4A
	case "k":
		return 0x4B
	case "l":
		return 0x4C
	case "m":
		return 0x4D
	case "n":
		return 0x4E
	case "o":
		return 0x4F
	case "p":
		return 0x50
	case "q":
		return 0x51
	case "r":
		return 0x52
	case "s":
		return 0x53
	case "t":
		return 0x54
	case "u":
		return 0x55
	case "v":
		return 0x56
	case "w":
		return 0x57
	case "x":
		return 0x58
	case "y":
		return 0x59
	case "z":
		return 0x5A

	// Number keys (0-9 are 0x30-0x39)
	case "0":
		return 0x30
	case "1":
		return 0x31
	case "2":
		return 0x32
	case "3":
		return 0x33
	case "4":
		return 0x34
	case "5":
		return 0x35
	case "6":
		return 0x36
	case "7":
		return 0x37
	case "8":
		return 0x38
	case "9":
		return 0x39

	default:
		return VK_RMENU // Default to right alt
	}
}

// KeyNameToDisplayName converts a key name to a display-friendly name
func KeyNameToDisplayName(name string) string {
	switch strings.ToLower(name) {
	case "rightalt", "rightoption":
		return "Right Alt"
	case "leftalt", "leftoption":
		return "Left Alt"
	case "rightctrl", "rightcontrol":
		return "Right Ctrl"
	case "leftctrl", "leftcontrol":
		return "Left Ctrl"
	case "rightshift":
		return "Right Shift"
	case "leftshift":
		return "Left Shift"
	case "rightwin", "rightsuper", "rightcommand", "rightcmd":
		return "Right Win"
	case "leftwin", "leftsuper", "leftcommand", "leftcmd":
		return "Left Win"
	case "capslock":
		return "Caps Lock"
	case "escape", "esc":
		return "Escape"
	case "space":
		return "Space"
	case "tab":
		return "Tab"
	case "return", "enter":
		return "Enter"
	case "backspace":
		return "Backspace"
	case "delete":
		return "Delete"
	case "left", "arrowleft":
		return "←"
	case "right", "arrowright":
		return "→"
	case "up", "arrowup":
		return "↑"
	case "down", "arrowdown":
		return "↓"
	case "f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12":
		return strings.ToUpper(name)
	default:
		return strings.ToUpper(name)
	}
}
