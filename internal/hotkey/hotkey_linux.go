//go:build linux

package hotkey

/*
#cgo LDFLAGS: -lX11

#include <X11/Xlib.h>
#include <X11/keysym.h>
#include <X11/XKBlib.h>
#include <stdlib.h>
#include <stdio.h>

static Display *display = NULL;
static Window root;
static int running = 0;
static KeyCode hotkeyCode = 0;
static KeyCode cancelCode = 0;
static int cancelEnabled = 0;

extern void goHotkeyPressed(void);
extern void goCancelPressed(void);

static int initDisplay(void) {
    if (display != NULL) return 1;

    display = XOpenDisplay(NULL);
    if (display == NULL) {
        fprintf(stderr, "Cannot open X display\n");
        return 0;
    }
    root = DefaultRootWindow(display);
    return 1;
}

static void closeDisplay(void) {
    if (display != NULL) {
        XCloseDisplay(display);
        display = NULL;
    }
}

static void setHotkeyKeysym(KeySym keysym) {
    if (display == NULL) return;
    hotkeyCode = XKeysymToKeycode(display, keysym);
}

static void setCancelKeysym(KeySym keysym) {
    if (display == NULL) return;
    cancelCode = XKeysymToKeycode(display, keysym);
}

static void enableCancel(void) {
    cancelEnabled = 1;
}

static void disableCancel(void) {
    cancelEnabled = 0;
}

static void startMonitoring(void) {
    if (display == NULL) return;
    running = 1;

    // Grab the hotkey
    XGrabKey(display, hotkeyCode, AnyModifier, root, True, GrabModeAsync, GrabModeAsync);

    // Also grab common modifier combinations
    XGrabKey(display, hotkeyCode, Mod2Mask, root, True, GrabModeAsync, GrabModeAsync);
    XGrabKey(display, hotkeyCode, LockMask, root, True, GrabModeAsync, GrabModeAsync);
    XGrabKey(display, hotkeyCode, Mod2Mask | LockMask, root, True, GrabModeAsync, GrabModeAsync);
}

static void stopMonitoring(void) {
    if (display == NULL) return;
    running = 0;

    XUngrabKey(display, hotkeyCode, AnyModifier, root);
    XUngrabKey(display, hotkeyCode, Mod2Mask, root);
    XUngrabKey(display, hotkeyCode, LockMask, root);
    XUngrabKey(display, hotkeyCode, Mod2Mask | LockMask, root);
}

static void processEvents(void) {
    if (display == NULL || !running) return;

    XEvent event;
    while (XPending(display) > 0) {
        XNextEvent(display, &event);

        if (event.type == KeyPress) {
            KeyCode keycode = event.xkey.keycode;

            if (keycode == hotkeyCode) {
                goHotkeyPressed();
            }

            if (cancelEnabled && keycode == cancelCode) {
                goCancelPressed();
            }
        }
    }
}

static int hasEvents(void) {
    if (display == NULL) return 0;
    return XPending(display) > 0;
}
*/
import "C"

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"yap/internal/logger"
)

// Callback is the function type for hotkey events
type Callback func()

// Manager handles global hotkey registration
type Manager struct {
	mu             sync.Mutex
	running        bool
	hotkeyCallback Callback
	cancelCallback func()
	cancelEnabled  bool
	stopCh         chan struct{}
	hotkeyStr      string
	cancelStr      string
}

var (
	callbackMu       sync.Mutex
	hotkeyCallback   Callback
	cancelCallbackMu sync.Mutex
	cancelCallback   func()
)

//export goHotkeyPressed
func goHotkeyPressed() {
	callbackMu.Lock()
	cb := hotkeyCallback
	callbackMu.Unlock()
	if cb != nil {
		go cb()
	}
}

//export goCancelPressed
func goCancelPressed() {
	cancelCallbackMu.Lock()
	cb := cancelCallback
	cancelCallbackMu.Unlock()
	if cb != nil {
		go cb()
	}
}

// NewManager creates a new hotkey manager
func NewManager() *Manager {
	return &Manager{
		hotkeyStr: "rightalt",
		cancelStr: "escape",
	}
}

// Register registers the global hotkey
func (m *Manager) Register(cb Callback) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return nil
	}

	if C.initDisplay() == 0 {
		return fmt.Errorf("failed to open X display")
	}

	callbackMu.Lock()
	hotkeyCallback = cb
	callbackMu.Unlock()
	m.hotkeyCallback = cb

	// Set the hotkey
	keysym := KeyNameToKeysym(m.hotkeyStr)
	C.setHotkeyKeysym(C.KeySym(keysym))

	// Set the cancel key
	cancelKeysym := KeyNameToKeysym(m.cancelStr)
	C.setCancelKeysym(C.KeySym(cancelKeysym))

	C.startMonitoring()

	m.running = true
	m.stopCh = make(chan struct{})

	// Start event processing loop
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopCh:
				return
			case <-ticker.C:
				C.processEvents()
			}
		}
	}()

	logger.Info(fmt.Sprintf("Hotkey registered: %s", m.hotkeyStr))
	return nil
}

// Unregister removes the hotkey
func (m *Manager) Unregister() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	close(m.stopCh)
	C.stopMonitoring()
	C.closeDisplay()
	m.running = false

	return nil
}

// SetHotkeyType sets the recording hotkey by name
func (m *Manager) SetHotkeyType(hotkeyName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.hotkeyStr = strings.ToLower(hotkeyName)
	keysym := KeyNameToKeysym(m.hotkeyStr)

	if m.running {
		C.stopMonitoring()
		C.setHotkeyKeysym(C.KeySym(keysym))
		C.startMonitoring()
	} else {
		C.setHotkeyKeysym(C.KeySym(keysym))
	}

	logger.Info(fmt.Sprintf("Hotkey set to: %s", m.hotkeyStr))
}

// SetCancelKey sets the cancel hotkey by name
func (m *Manager) SetCancelKey(keyName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cancelStr = strings.ToLower(keyName)
	keysym := KeyNameToKeysym(m.cancelStr)
	C.setCancelKeysym(C.KeySym(keysym))

	logger.Info(fmt.Sprintf("Cancel key set to: %s", m.cancelStr))
}

// IsRegistered returns whether hotkey is registered
func (m *Manager) IsRegistered() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// EnableCancelKey starts monitoring for the cancel key
func (m *Manager) EnableCancelKey(cb func()) {
	cancelCallbackMu.Lock()
	cancelCallback = cb
	cancelCallbackMu.Unlock()

	m.mu.Lock()
	m.cancelCallback = cb
	m.cancelEnabled = true
	m.mu.Unlock()

	C.enableCancel()
}

// DisableCancelKey stops monitoring for the cancel key
func (m *Manager) DisableCancelKey() {
	C.disableCancel()

	cancelCallbackMu.Lock()
	cancelCallback = nil
	cancelCallbackMu.Unlock()

	m.mu.Lock()
	m.cancelEnabled = false
	m.cancelCallback = nil
	m.mu.Unlock()
}

// GetHotkeyDisplayName returns the display name for a hotkey
func GetHotkeyDisplayName(hotkeyName string) string {
	return KeyNameToDisplayName(hotkeyName)
}

// RequestAccessibilityPermissions is a no-op on Linux
func RequestAccessibilityPermissions() bool {
	return true
}

// HasAccessibilityPermissions always returns true on Linux (no special permissions needed)
func HasAccessibilityPermissions() bool {
	return true
}

// KeyNameToKeysym converts a key name to an X11 KeySym
func KeyNameToKeysym(name string) uint64 {
	switch strings.ToLower(name) {
	// Modifier keys
	case "rightalt", "rightoption":
		return 0xFFEA // XK_Alt_R
	case "leftalt", "leftoption":
		return 0xFFE9 // XK_Alt_L
	case "rightctrl", "rightcontrol":
		return 0xFFE4 // XK_Control_R
	case "leftctrl", "leftcontrol":
		return 0xFFE3 // XK_Control_L
	case "rightshift":
		return 0xFFE2 // XK_Shift_R
	case "leftshift":
		return 0xFFE1 // XK_Shift_L
	case "rightsuper", "rightwin", "rightcommand", "rightcmd":
		return 0xFFEC // XK_Super_R
	case "leftsuper", "leftwin", "leftcommand", "leftcmd":
		return 0xFFEB // XK_Super_L
	case "capslock":
		return 0xFFE5 // XK_Caps_Lock

	// Special keys
	case "escape", "esc":
		return 0xFF1B // XK_Escape
	case "space":
		return 0x0020 // XK_space
	case "tab":
		return 0xFF09 // XK_Tab
	case "return", "enter":
		return 0xFF0D // XK_Return
	case "backspace":
		return 0xFF08 // XK_BackSpace
	case "delete":
		return 0xFFFF // XK_Delete

	// Arrow keys
	case "left", "arrowleft":
		return 0xFF51 // XK_Left
	case "right", "arrowright":
		return 0xFF53 // XK_Right
	case "up", "arrowup":
		return 0xFF52 // XK_Up
	case "down", "arrowdown":
		return 0xFF54 // XK_Down

	// Function keys
	case "f1":
		return 0xFFBE
	case "f2":
		return 0xFFBF
	case "f3":
		return 0xFFC0
	case "f4":
		return 0xFFC1
	case "f5":
		return 0xFFC2
	case "f6":
		return 0xFFC3
	case "f7":
		return 0xFFC4
	case "f8":
		return 0xFFC5
	case "f9":
		return 0xFFC6
	case "f10":
		return 0xFFC7
	case "f11":
		return 0xFFC8
	case "f12":
		return 0xFFC9

	// Letter keys (lowercase)
	case "a":
		return 0x0061
	case "b":
		return 0x0062
	case "c":
		return 0x0063
	case "d":
		return 0x0064
	case "e":
		return 0x0065
	case "f":
		return 0x0066
	case "g":
		return 0x0067
	case "h":
		return 0x0068
	case "i":
		return 0x0069
	case "j":
		return 0x006A
	case "k":
		return 0x006B
	case "l":
		return 0x006C
	case "m":
		return 0x006D
	case "n":
		return 0x006E
	case "o":
		return 0x006F
	case "p":
		return 0x0070
	case "q":
		return 0x0071
	case "r":
		return 0x0072
	case "s":
		return 0x0073
	case "t":
		return 0x0074
	case "u":
		return 0x0075
	case "v":
		return 0x0076
	case "w":
		return 0x0077
	case "x":
		return 0x0078
	case "y":
		return 0x0079
	case "z":
		return 0x007A

	// Number keys
	case "0":
		return 0x0030
	case "1":
		return 0x0031
	case "2":
		return 0x0032
	case "3":
		return 0x0033
	case "4":
		return 0x0034
	case "5":
		return 0x0035
	case "6":
		return 0x0036
	case "7":
		return 0x0037
	case "8":
		return 0x0038
	case "9":
		return 0x0039

	default:
		return 0xFFEA // Default to right alt
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
	case "rightsuper", "rightwin", "rightcommand", "rightcmd":
		return "Right Super"
	case "leftsuper", "leftwin", "leftcommand", "leftcmd":
		return "Left Super"
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
