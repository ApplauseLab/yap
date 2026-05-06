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
static BOOL gRightOptionDown = NO;
static BOOL gEscapeEnabled = NO;

extern void goHotkeyPressed(void);
extern void goEscapePressed(void);

// Key codes
#define kVK_RightOption 0x3D

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

static void startMonitoring(void) {
    if (gEventMonitor != nil) {
        return; // Already monitoring
    }
    
    // Check accessibility permissions first
    if (!checkAccessibilityPermissions()) {
        NSLog(@"Cannot start monitoring without accessibility permissions");
        // Still try to register - it might work if permissions are granted later
    }
    
    // Monitor for flagsChanged events (modifier keys)
    gEventMonitor = [NSEvent addGlobalMonitorForEventsMatchingMask:NSEventMaskFlagsChanged
        handler:^(NSEvent *event) {
            // Check if right option key
            if ([event keyCode] == kVK_RightOption) {
                // Check if key is being pressed (not released)
                if ([event modifierFlags] & NSEventModifierFlagOption) {
                    if (!gRightOptionDown) {
                        gRightOptionDown = YES;
                        goHotkeyPressed();
                    }
                } else {
                    gRightOptionDown = NO;
                }
            }
        }];
    
    NSLog(@"Right Option key monitoring started");
}

static void stopMonitoring(void) {
    if (gEventMonitor != nil) {
        [NSEvent removeMonitor:gEventMonitor];
        gEventMonitor = nil;
        gRightOptionDown = NO;
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
