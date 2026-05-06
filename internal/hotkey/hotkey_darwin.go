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
static BOOL gModifierKeyDown = NO;
static BOOL gEscapeEnabled = NO;
static int gCurrentHotkeyType = 0; // 0=rightOption, 1=leftOption, 2=fn, 3=doubleRightOption
static NSTimeInterval gLastRightOptionPress = 0;

extern void goHotkeyPressed(void);
extern void goEscapePressed(void);

// Key codes
#define kVK_RightOption 0x3D
#define kVK_LeftOption 0x3A
#define kVK_Function 0x3F

// Check if accessibility permissions are granted
static BOOL checkAccessibilityPermissions(void) {
    // Check if we have accessibility permissions
    NSDictionary *options = @{(__bridge NSString *)kAXTrustedCheckOptionPrompt: @YES};
    BOOL trusted = AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)options);
    if (!trusted) {
        NSLog(@"Accessibility permissions not granted - please enable in System Preferences");
    }
    return trusted;
}

static void startMonitoringWithType(int hotkeyType) {
    if (gEventMonitor != nil) {
        [NSEvent removeMonitor:gEventMonitor];
        gEventMonitor = nil;
    }
    
    gCurrentHotkeyType = hotkeyType;
    gModifierKeyDown = NO;
    gLastRightOptionPress = 0;
    
    // Check accessibility permissions first
    if (!checkAccessibilityPermissions()) {
        NSLog(@"Cannot start monitoring without accessibility permissions");
    }
    
    // Monitor for flagsChanged events (modifier keys)
    gEventMonitor = [NSEvent addGlobalMonitorForEventsMatchingMask:NSEventMaskFlagsChanged
        handler:^(NSEvent *event) {
            UInt16 keyCode = [event keyCode];
            NSEventModifierFlags flags = [event modifierFlags];
            
            switch (gCurrentHotkeyType) {
                case 0: // rightOption
                    if (keyCode == kVK_RightOption) {
                        if (flags & NSEventModifierFlagOption) {
                            if (!gModifierKeyDown) {
                                gModifierKeyDown = YES;
                                goHotkeyPressed();
                            }
                        } else {
                            gModifierKeyDown = NO;
                        }
                    }
                    break;
                    
                case 1: // leftOption
                    if (keyCode == kVK_LeftOption) {
                        if (flags & NSEventModifierFlagOption) {
                            if (!gModifierKeyDown) {
                                gModifierKeyDown = YES;
                                goHotkeyPressed();
                            }
                        } else {
                            gModifierKeyDown = NO;
                        }
                    }
                    break;
                    
                case 2: // fn
                    if (keyCode == kVK_Function) {
                        if (flags & NSEventModifierFlagFunction) {
                            if (!gModifierKeyDown) {
                                gModifierKeyDown = YES;
                                goHotkeyPressed();
                            }
                        } else {
                            gModifierKeyDown = NO;
                        }
                    }
                    break;
                    
                case 3: // doubleRightOption (double tap)
                    if (keyCode == kVK_RightOption) {
                        if (flags & NSEventModifierFlagOption) {
                            NSTimeInterval now = [[NSDate date] timeIntervalSince1970];
                            if (now - gLastRightOptionPress < 0.4) { // 400ms window for double tap
                                goHotkeyPressed();
                                gLastRightOptionPress = 0; // Reset to prevent triple-tap
                            } else {
                                gLastRightOptionPress = now;
                            }
                        }
                    }
                    break;
            }
        }];
    
    NSString *hotkeyName;
    switch (hotkeyType) {
        case 0: hotkeyName = @"Right Option"; break;
        case 1: hotkeyName = @"Left Option"; break;
        case 2: hotkeyName = @"Fn"; break;
        case 3: hotkeyName = @"Double-tap Right Option"; break;
        default: hotkeyName = @"Unknown"; break;
    }
    NSLog(@"%@ key monitoring started", hotkeyName);
}

static void startMonitoring(void) {
    startMonitoringWithType(0); // Default to right option
}

static void stopMonitoring(void) {
    if (gEventMonitor != nil) {
        [NSEvent removeMonitor:gEventMonitor];
        gEventMonitor = nil;
        gModifierKeyDown = NO;
    }
}

static void startEscapeMonitoring(void) {
    if (gKeyEventMonitor != nil) {
        return;
    }
    
    gEscapeEnabled = YES;
    NSLog(@"Starting escape key monitoring");
    
    // Global monitor for when other apps have focus
    gKeyEventMonitor = [NSEvent addGlobalMonitorForEventsMatchingMask:NSEventMaskKeyDown
        handler:^(NSEvent *event) {
            if (gEscapeEnabled && [event keyCode] == 53) { // 53 = Escape
                NSLog(@"Escape key pressed (global)");
                goEscapePressed();
            }
        }];
    
    // Local monitor for when this app has focus
    gLocalKeyEventMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown
        handler:^NSEvent *(NSEvent *event) {
            if (gEscapeEnabled && [event keyCode] == 53) { // 53 = Escape
                NSLog(@"Escape key pressed (local)");
                goEscapePressed();
                return nil; // Consume event
            }
            return event;
        }];
}

static void stopEscapeMonitoring(void) {
    gEscapeEnabled = NO;
    if (gKeyEventMonitor != nil) {
        [NSEvent removeMonitor:gKeyEventMonitor];
        gKeyEventMonitor = nil;
    }
    if (gLocalKeyEventMonitor != nil) {
        [NSEvent removeMonitor:gLocalKeyEventMonitor];
        gLocalKeyEventMonitor = nil;
    }
    NSLog(@"Escape key monitoring stopped");
}
*/
import "C"

import (
	"sync"
)

var (
	callbackMu       sync.Mutex
	callback         func()
	hotkeyC          = make(chan struct{}, 1)
	escapeCallbackMu sync.Mutex
	escapeCallback   func()
)

//export goHotkeyPressed
func goHotkeyPressed() {
	select {
	case hotkeyC <- struct{}{}:
	default:
		// Channel full, dropping event
	}
}

//export goEscapePressed
func goEscapePressed() {
	escapeCallbackMu.Lock()
	cb := escapeCallback
	escapeCallbackMu.Unlock()
	if cb != nil {
		go cb()
	}
}

// Callback is the function type for hotkey events
type Callback func()

// Manager handles global hotkey registration
type Manager struct {
	mu      sync.Mutex
	running bool
	stopC   chan struct{}
}

// NewManager creates a new hotkey manager
func NewManager() *Manager {
	return &Manager{}
}

// Register registers the global hotkey (Right Option key)
func (m *Manager) Register(cb Callback) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return nil
	}

	callbackMu.Lock()
	callback = cb
	callbackMu.Unlock()

	C.startMonitoring()

	m.running = true
	m.stopC = make(chan struct{})

	// Start goroutine to handle hotkey events safely
	go func() {
		for {
			select {
			case <-hotkeyC:
				callbackMu.Lock()
				cb := callback
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
	C.stopMonitoring()
	m.running = false

	return nil
}

// SetHotkeyType changes the hotkey type
// Types: "rightOption", "leftOption", "fn", "doubleRightOption"
func (m *Manager) SetHotkeyType(hotkeyType string) {
	var typeInt C.int
	switch hotkeyType {
	case "leftOption":
		typeInt = 1
	case "fn":
		typeInt = 2
	case "doubleRightOption":
		typeInt = 3
	default: // "rightOption"
		typeInt = 0
	}
	C.startMonitoringWithType(typeInt)
}

// GetHotkeyDisplayName returns the display name for a hotkey type
func GetHotkeyDisplayName(hotkeyType string) string {
	switch hotkeyType {
	case "leftOption":
		return "Left Option (⌥)"
	case "fn":
		return "Fn"
	case "doubleRightOption":
		return "Double-tap Right Option"
	default:
		return "Right Option (⌥)"
	}
}

// IsRegistered returns whether hotkey is registered
func (m *Manager) IsRegistered() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// EnableEscapeCancel starts monitoring for Escape key to cancel recording
func (m *Manager) EnableEscapeCancel(cb func()) {
	escapeCallbackMu.Lock()
	escapeCallback = cb
	escapeCallbackMu.Unlock()
	
	C.startEscapeMonitoring()
}

// DisableEscapeCancel stops monitoring for Escape key
func (m *Manager) DisableEscapeCancel() {
	C.stopEscapeMonitoring()
	escapeCallbackMu.Lock()
	escapeCallback = nil
	escapeCallbackMu.Unlock()
}
