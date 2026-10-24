//go:build darwin

package system

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework ApplicationServices -framework Carbon -framework Cocoa

#include <ApplicationServices/ApplicationServices.h>
#include <Carbon/Carbon.h>
#include <Cocoa/Cocoa.h>
#include <os/lock.h>
#include <unistd.h>

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

// Check if we have accessibility permissions
int hasAccessibilityForPaste(void) {
    // Check without prompting
    BOOL trusted = AXIsProcessTrusted();
    NSLog(@"hasAccessibilityForPaste: trusted=%d", trusted);
    return trusted ? 1 : 0;
}

// Store the previous frontmost app (before our overlay appears).
// This file is compiled without ARC, so the reference is retained manually
// and guarded because recording can start from several goroutines.
static NSRunningApplication *gPreviousFrontApp = nil;
static os_unfair_lock gPreviousFrontAppLock = OS_UNFAIR_LOCK_INIT;

// Save the current frontmost app (call before showing overlay)
void saveFrontmostApp(void) {
    NSRunningApplication *frontApp = [[NSWorkspace sharedWorkspace] frontmostApplication];
    if (frontApp && ![frontApp.bundleIdentifier isEqualToString:@"com.wails.Yap"]) {
        [frontApp retain];
        os_unfair_lock_lock(&gPreviousFrontAppLock);
        NSRunningApplication *old = gPreviousFrontApp;
        gPreviousFrontApp = frontApp;
        os_unfair_lock_unlock(&gPreviousFrontAppLock);
        [old release];
        NSLog(@"saveFrontmostApp: saved %@", frontApp.localizedName);
    }
}

// Returns a retained reference to the saved app (or nil); release it after use.
static NSRunningApplication *copyPreviousFrontApp(void) {
    os_unfair_lock_lock(&gPreviousFrontAppLock);
    NSRunningApplication *app = [gPreviousFrontApp retain];
    os_unfair_lock_unlock(&gPreviousFrontAppLock);
    return app;
}

// Get the frontmost application name
NSString* getFrontmostAppName(void) {
    NSRunningApplication *frontApp = [[NSWorkspace sharedWorkspace] frontmostApplication];
    return frontApp ? frontApp.localizedName : @"(unknown)";
}

// Activate the previously saved app
void activatePreviousApp(void) {
    NSRunningApplication *previousApp = copyPreviousFrontApp();
    if (previousApp) {
        NSLog(@"activatePreviousApp: activating %@", previousApp.localizedName);
        [previousApp activateWithOptions:NSApplicationActivateIgnoringOtherApps];
        [previousApp release];
    }
}

// Simulate Cmd+V keystroke using CGEvent
int simulatePasteKeystroke(void) {
    NSLog(@"simulatePasteKeystroke: starting");
    NSLog(@"simulatePasteKeystroke: current frontmost = %@", getFrontmostAppName());

    // Check accessibility first
    if (!AXIsProcessTrusted()) {
        NSLog(@"simulatePasteKeystroke: ERROR - No accessibility permissions!");
        return 0;
    }

    // Activate the previous app first (in case our app took focus)
    NSRunningApplication *previousApp = copyPreviousFrontApp();
    if (previousApp) {
        NSLog(@"simulatePasteKeystroke: activating previous app %@", previousApp.localizedName);
        [previousApp activateWithOptions:NSApplicationActivateIgnoringOtherApps];
        [previousApp release];
        usleep(100000); // 100ms for activation to complete
        NSLog(@"simulatePasteKeystroke: after activation, frontmost = %@", getFrontmostAppName());
    }

    // Create key down event for 'v' with Command modifier
    CGEventRef keyDown = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)kVK_ANSI_V, true);
    CGEventRef keyUp = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)kVK_ANSI_V, false);

    if (keyDown == NULL || keyUp == NULL) {
        NSLog(@"simulatePasteKeystroke: failed to create events");
        if (keyDown) CFRelease(keyDown);
        if (keyUp) CFRelease(keyUp);
        return 0;
    }

    // Set Command modifier flag
    CGEventSetFlags(keyDown, kCGEventFlagMaskCommand);
    CGEventSetFlags(keyUp, kCGEventFlagMaskCommand);

    // Post events
    NSLog(@"simulatePasteKeystroke: posting keyDown");
    CGEventPost(kCGHIDEventTap, keyDown);
    usleep(50000); // 50ms delay
    NSLog(@"simulatePasteKeystroke: posting keyUp");
    CGEventPost(kCGHIDEventTap, keyUp);

    // Release
    CFRelease(keyDown);
    CFRelease(keyUp);
    NSLog(@"simulatePasteKeystroke: done");
    return 1;
}

#pragma clang diagnostic pop
*/
import "C"

import (
	"fmt"

	"yap/internal/logger"
)

// SaveFrontmostApp saves the current frontmost app (call before showing overlay)
func SaveFrontmostApp() {
	C.saveFrontmostApp()
}

// simulatePasteMacOSNative uses CGEvent to simulate Cmd+V (more reliable than AppleScript)
func simulatePasteMacOSNative() error {
	logger.Debug("simulatePasteMacOSNative: checking accessibility")
	hasAccess := C.hasAccessibilityForPaste()
	logger.Debug(fmt.Sprintf("simulatePasteMacOSNative: hasAccessibility=%d", hasAccess))

	if hasAccess == 0 {
		logger.Error("No accessibility permissions for paste")
		return fmt.Errorf("no accessibility permissions")
	}

	logger.Debug("simulatePasteMacOSNative: calling C function")
	result := C.simulatePasteKeystroke()
	logger.Debug(fmt.Sprintf("simulatePasteMacOSNative: result=%d", result))

	if result == 0 {
		return fmt.Errorf("paste keystroke failed")
	}
	return nil
}
