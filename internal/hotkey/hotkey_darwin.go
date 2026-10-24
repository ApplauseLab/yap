//go:build darwin

package hotkey

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework Carbon -framework ApplicationServices

#import <Cocoa/Cocoa.h>
#import <Carbon/Carbon.h>
#import <ApplicationServices/ApplicationServices.h>

static id gEventMonitor = nil;
static id gKeyEventMonitor = nil;
static id gLocalKeyEventMonitor = nil;
static id gLocalFlagsMonitor = nil;
static BOOL gHotkeyKeyDown = NO;
static BOOL gCancelKeyEnabled = NO;
static UInt16 gCurrentHotkeyCode = 0x3D;  // Default: Right Option
static UInt16 gCurrentCancelCode = 53;     // Default: Escape
static BOOL gHotkeyIsModifier = YES;       // Is the hotkey a modifier key?
static BOOL gCancelIsModifier = NO;        // Is the cancel key a modifier?

extern void goHotkeyPressed(void);
extern void goCancelPressed(void);

// Common key codes
#define kVK_RightOption 0x3D
#define kVK_LeftOption 0x3A
#define kVK_RightCommand 0x36
#define kVK_LeftCommand 0x37
#define kVK_RightShift 0x3C
#define kVK_LeftShift 0x38
#define kVK_RightControl 0x3E
#define kVK_LeftControl 0x3B
#define kVK_Function 0x3F
#define kVK_CapsLock 0x39
#define kVK_Escape 0x35
#define kVK_Space 0x31
#define kVK_Tab 0x30
#define kVK_Return 0x24

// Check if accessibility permissions are granted (with optional prompt)
static int checkAccessibilityPermissionsWithPrompt(int shouldPrompt) {
    NSDictionary *options = @{(__bridge NSString *)kAXTrustedCheckOptionPrompt: @(shouldPrompt ? YES : NO)};
    BOOL trusted = AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)options);
    if (!trusted) {
        NSLog(@"Accessibility permissions not granted - please enable in System Preferences > Privacy & Security > Accessibility");
    }
    return trusted ? 1 : 0;
}

// Check if accessibility permissions are granted (no prompt)
static BOOL hasAccessibilityPermissions(void) {
    return checkAccessibilityPermissionsWithPrompt(0) ? YES : NO;
}

// Request accessibility permissions explicitly (always shows prompt)
static int requestAccessibilityPermissions(void) {
    NSLog(@"Requesting accessibility permissions...");
    int trusted = checkAccessibilityPermissionsWithPrompt(1);
    if (!trusted) {
        NSLog(@"IMPORTANT: Please grant Accessibility permissions to enable hotkey features");
        NSLog(@"Go to: System Preferences > Privacy & Security > Accessibility");
        NSLog(@"Add and enable this application");
        dispatch_async(dispatch_get_main_queue(), ^{
            NSURL *url = [NSURL URLWithString:@"x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"];
            [[NSWorkspace sharedWorkspace] openURL:url];
        });
    }
    return trusted;
}

// Check if a key code is a modifier key
static BOOL isModifierKeyCode(UInt16 keyCode) {
    switch (keyCode) {
        case kVK_RightOption:
        case kVK_LeftOption:
        case kVK_RightCommand:
        case kVK_LeftCommand:
        case kVK_RightShift:
        case kVK_LeftShift:
        case kVK_RightControl:
        case kVK_LeftControl:
        case kVK_Function:
        case kVK_CapsLock:
            return YES;
        default:
            return NO;
    }
}

// Get the modifier flag for a modifier key code
static NSEventModifierFlags getModifierFlag(UInt16 keyCode) {
    switch (keyCode) {
        case kVK_RightOption:
        case kVK_LeftOption:
            return NSEventModifierFlagOption;
        case kVK_RightCommand:
        case kVK_LeftCommand:
            return NSEventModifierFlagCommand;
        case kVK_RightShift:
        case kVK_LeftShift:
            return NSEventModifierFlagShift;
        case kVK_RightControl:
        case kVK_LeftControl:
            return NSEventModifierFlagControl;
        case kVK_Function:
            return NSEventModifierFlagFunction;
        case kVK_CapsLock:
            return NSEventModifierFlagCapsLock;
        default:
            return 0;
    }
}

static void stopAllMonitoring(void) {
    if (gEventMonitor != nil) {
        [NSEvent removeMonitor:gEventMonitor];
        gEventMonitor = nil;
    }
    if (gKeyEventMonitor != nil) {
        [NSEvent removeMonitor:gKeyEventMonitor];
        gKeyEventMonitor = nil;
    }
    if (gLocalKeyEventMonitor != nil) {
        [NSEvent removeMonitor:gLocalKeyEventMonitor];
        gLocalKeyEventMonitor = nil;
    }
    if (gLocalFlagsMonitor != nil) {
        [NSEvent removeMonitor:gLocalFlagsMonitor];
        gLocalFlagsMonitor = nil;
    }
    gHotkeyKeyDown = NO;
    gCancelKeyEnabled = NO;
}

static void startMonitoring(void) {
    stopAllMonitoring();

    // Check accessibility permissions first
    if (!hasAccessibilityPermissions()) {
        NSLog(@"Cannot start monitoring without accessibility permissions");
        return;
    }

    // Monitor for modifier keys (flagsChanged events)
    gEventMonitor = [NSEvent addGlobalMonitorForEventsMatchingMask:NSEventMaskFlagsChanged
        handler:^(NSEvent *event) {
            UInt16 keyCode = [event keyCode];
            NSEventModifierFlags flags = [event modifierFlags];

            // Check hotkey (if it's a modifier)
            if (gHotkeyIsModifier && keyCode == gCurrentHotkeyCode) {
                NSEventModifierFlags modFlag = getModifierFlag(keyCode);
                if (flags & modFlag) {
                    if (!gHotkeyKeyDown) {
                        gHotkeyKeyDown = YES;
                        goHotkeyPressed();
                    }
                } else {
                    gHotkeyKeyDown = NO;
                }
            }

            // Check cancel key (if it's a modifier)
            if (gCancelKeyEnabled && gCancelIsModifier && keyCode == gCurrentCancelCode) {
                NSEventModifierFlags modFlag = getModifierFlag(keyCode);
                if (flags & modFlag) {
                    goCancelPressed();
                }
            }
        }];

    // Monitor for regular keys (keyDown events)
    gKeyEventMonitor = [NSEvent addGlobalMonitorForEventsMatchingMask:NSEventMaskKeyDown
        handler:^(NSEvent *event) {
            UInt16 keyCode = [event keyCode];

            // Check hotkey (if it's NOT a modifier)
            if (!gHotkeyIsModifier && keyCode == gCurrentHotkeyCode) {
                goHotkeyPressed();
            }

            // Check cancel key (if it's NOT a modifier)
            if (gCancelKeyEnabled && !gCancelIsModifier && keyCode == gCurrentCancelCode) {
                goCancelPressed();
            }
        }];

    // Local monitor for regular keys when this app has focus
    gLocalKeyEventMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown
        handler:^NSEvent *(NSEvent *event) {
            UInt16 keyCode = [event keyCode];

            // Check hotkey (if it's NOT a modifier)
            if (!gHotkeyIsModifier && keyCode == gCurrentHotkeyCode) {
                goHotkeyPressed();
                return nil; // Consume event
            }

            // Check cancel key (if it's NOT a modifier)
            if (gCancelKeyEnabled && !gCancelIsModifier && keyCode == gCurrentCancelCode) {
                goCancelPressed();
                return nil; // Consume event
            }

            return event;
        }];

    // Also add local monitor for modifier keys (flagsChanged) when app has focus
    // This ensures modifier hotkeys work even when the app window is focused
    if (gLocalFlagsMonitor != nil) {
        [NSEvent removeMonitor:gLocalFlagsMonitor];
        gLocalFlagsMonitor = nil;
    }
    gLocalFlagsMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskFlagsChanged
        handler:^NSEvent *(NSEvent *event) {
            UInt16 keyCode = [event keyCode];
            NSEventModifierFlags flags = [event modifierFlags];

            // Check hotkey (if it's a modifier)
            if (gHotkeyIsModifier && keyCode == gCurrentHotkeyCode) {
                NSEventModifierFlags modFlag = getModifierFlag(keyCode);
                if (flags & modFlag) {
                    if (!gHotkeyKeyDown) {
                        gHotkeyKeyDown = YES;
                        goHotkeyPressed();
                    }
                } else {
                    gHotkeyKeyDown = NO;
                }
            }

            // Check cancel key (if it's a modifier)
            if (gCancelKeyEnabled && gCancelIsModifier && keyCode == gCurrentCancelCode) {
                NSEventModifierFlags modFlag = getModifierFlag(keyCode);
                if (flags & modFlag) {
                    goCancelPressed();
                }
            }

            return event;
        }];

    NSLog(@"Key monitoring started - hotkey: %d, cancel: %d", gCurrentHotkeyCode, gCurrentCancelCode);
}

static void setHotkeyCode(UInt16 keyCode) {
    gCurrentHotkeyCode = keyCode;
    gHotkeyIsModifier = isModifierKeyCode(keyCode);
    gHotkeyKeyDown = NO;
    NSLog(@"Hotkey set to keyCode: %d (isModifier: %d)", keyCode, gHotkeyIsModifier);
}

static void setCancelCode(UInt16 keyCode) {
    gCurrentCancelCode = keyCode;
    gCancelIsModifier = isModifierKeyCode(keyCode);
    NSLog(@"Cancel key set to keyCode: %d (isModifier: %d)", keyCode, gCancelIsModifier);
}

static void enableCancelKey(void) {
    gCancelKeyEnabled = YES;
    NSLog(@"Cancel key enabled");
}

static void disableCancelKey(void) {
    gCancelKeyEnabled = NO;
    NSLog(@"Cancel key disabled");
}
*/
import "C"

import (
	"fmt"
	"strings"
	"sync"

	"yap/internal/logger"
)

var (
	callbackMu       sync.Mutex
	hotkeyCallback   func()
	hotkeyC          = make(chan struct{}, 1)
	cancelCallbackMu sync.Mutex
	cancelCallback   func()
)

//export goHotkeyPressed
func goHotkeyPressed() {
	select {
	case hotkeyC <- struct{}{}:
	default:
		// Channel full, dropping event
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

// Callback is the function type for hotkey events
type Callback func()

// Manager handles global hotkey registration
type Manager struct {
	mu        sync.Mutex
	running   bool
	stopC     chan struct{}
	hotkeyStr string
	cancelStr string
}

// NewManager creates a new hotkey manager
func NewManager() *Manager {
	return &Manager{
		hotkeyStr: "rightoption",
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

	if !HasAccessibilityPermissions() {
		return fmt.Errorf("accessibility permission not granted")
	}

	callbackMu.Lock()
	hotkeyCallback = cb
	callbackMu.Unlock()

	// Set the hotkey code
	keyCode := KeyNameToCode(m.hotkeyStr)
	C.setHotkeyCode(C.UInt16(keyCode))

	// Set the cancel key code
	cancelCode := KeyNameToCode(m.cancelStr)
	C.setCancelCode(C.UInt16(cancelCode))

	C.startMonitoring()

	m.running = true
	m.stopC = make(chan struct{})

	// Start goroutine to handle hotkey events safely
	go func() {
		for {
			select {
			case <-hotkeyC:
				callbackMu.Lock()
				cb := hotkeyCallback
				callbackMu.Unlock()
				if cb != nil {
					cb()
				}
			case <-m.stopC:
				return
			}
		}
	}()

	return nil
}

// Unregister removes the hotkey
func (m *Manager) Unregister() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	close(m.stopC)
	C.stopAllMonitoring()
	m.running = false

	return nil
}

// SetHotkeyType sets the recording hotkey by name
func (m *Manager) SetHotkeyType(hotkeyName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.hotkeyStr = strings.ToLower(hotkeyName)
	keyCode := KeyNameToCode(m.hotkeyStr)
	C.setHotkeyCode(C.UInt16(keyCode))

	if m.running {
		C.startMonitoring()
	}

	logger.Info(fmt.Sprintf("Hotkey set to: %s (code: %d)", m.hotkeyStr, keyCode))
}

// SetCancelKey sets the cancel hotkey by name
func (m *Manager) SetCancelKey(keyName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cancelStr = strings.ToLower(keyName)
	cancelCode := KeyNameToCode(m.cancelStr)
	C.setCancelCode(C.UInt16(cancelCode))

	logger.Info(fmt.Sprintf("Cancel key set to: %s (code: %d)", m.cancelStr, cancelCode))
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

	C.enableCancelKey()
}

// DisableCancelKey stops monitoring for the cancel key
func (m *Manager) DisableCancelKey() {
	C.disableCancelKey()

	cancelCallbackMu.Lock()
	cancelCallback = nil
	cancelCallbackMu.Unlock()
}

// GetHotkeyDisplayName returns the display name for a hotkey
func GetHotkeyDisplayName(hotkeyName string) string {
	return KeyNameToDisplayName(hotkeyName)
}

// RequestAccessibilityPermissions prompts user for accessibility permissions
func RequestAccessibilityPermissions() bool {
	return C.requestAccessibilityPermissions() != 0
}

// HasAccessibilityPermissions checks if accessibility permissions are granted (no prompt)
func HasAccessibilityPermissions() bool {
	return C.checkAccessibilityPermissionsWithPrompt(0) != 0
}

// KeyNameToCode converts a key name to a macOS key code
func KeyNameToCode(name string) uint16 {
	switch strings.ToLower(name) {
	// Modifier keys
	case "rightoption", "rightalt":
		return 0x3D
	case "leftoption", "leftalt":
		return 0x3A
	case "rightcommand", "rightcmd":
		return 0x36
	case "leftcommand", "leftcmd":
		return 0x37
	case "rightshift":
		return 0x3C
	case "leftshift":
		return 0x38
	case "rightcontrol", "rightctrl":
		return 0x3E
	case "leftcontrol", "leftctrl":
		return 0x3B
	case "fn", "function":
		return 0x3F
	case "capslock":
		return 0x39

	// Special keys
	case "escape", "esc":
		return 0x35
	case "space":
		return 0x31
	case "tab":
		return 0x30
	case "return", "enter":
		return 0x24
	case "delete", "backspace":
		return 0x33
	case "forwarddelete":
		return 0x75

	// Arrow keys
	case "left", "arrowleft":
		return 0x7B
	case "right", "arrowright":
		return 0x7C
	case "up", "arrowup":
		return 0x7E
	case "down", "arrowdown":
		return 0x7D

	// Function keys
	case "f1":
		return 0x7A
	case "f2":
		return 0x78
	case "f3":
		return 0x63
	case "f4":
		return 0x76
	case "f5":
		return 0x60
	case "f6":
		return 0x61
	case "f7":
		return 0x62
	case "f8":
		return 0x64
	case "f9":
		return 0x65
	case "f10":
		return 0x6D
	case "f11":
		return 0x67
	case "f12":
		return 0x6F

	// Letter keys
	case "a":
		return 0x00
	case "b":
		return 0x0B
	case "c":
		return 0x08
	case "d":
		return 0x02
	case "e":
		return 0x0E
	case "f":
		return 0x03
	case "g":
		return 0x05
	case "h":
		return 0x04
	case "i":
		return 0x22
	case "j":
		return 0x26
	case "k":
		return 0x28
	case "l":
		return 0x25
	case "m":
		return 0x2E
	case "n":
		return 0x2D
	case "o":
		return 0x1F
	case "p":
		return 0x23
	case "q":
		return 0x0C
	case "r":
		return 0x0F
	case "s":
		return 0x01
	case "t":
		return 0x11
	case "u":
		return 0x20
	case "v":
		return 0x09
	case "w":
		return 0x0D
	case "x":
		return 0x07
	case "y":
		return 0x10
	case "z":
		return 0x06

	// Number keys
	case "0":
		return 0x1D
	case "1":
		return 0x12
	case "2":
		return 0x13
	case "3":
		return 0x14
	case "4":
		return 0x15
	case "5":
		return 0x17
	case "6":
		return 0x16
	case "7":
		return 0x1A
	case "8":
		return 0x1C
	case "9":
		return 0x19

	default:
		return 0x3D // Default to right option
	}
}

// KeyNameToDisplayName converts a key name to a display-friendly name
func KeyNameToDisplayName(name string) string {
	switch strings.ToLower(name) {
	case "rightoption", "rightalt":
		return "Right Option (⌥)"
	case "leftoption", "leftalt":
		return "Left Option (⌥)"
	case "rightcommand", "rightcmd":
		return "Right Command (⌘)"
	case "leftcommand", "leftcmd":
		return "Left Command (⌘)"
	case "rightshift":
		return "Right Shift (⇧)"
	case "leftshift":
		return "Left Shift (⇧)"
	case "rightcontrol", "rightctrl":
		return "Right Control (⌃)"
	case "leftcontrol", "leftctrl":
		return "Left Control (⌃)"
	case "fn", "function":
		return "Fn"
	case "capslock":
		return "Caps Lock"
	case "escape", "esc":
		return "Escape"
	case "space":
		return "Space"
	case "tab":
		return "Tab"
	case "return", "enter":
		return "Return"
	case "delete", "backspace":
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
