//go:build darwin && cgo

package crashlog

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

extern void goAppLifecycleEvent(int event);

static void observeAppLifecycle(void) {
    NSNotificationCenter *center = [NSNotificationCenter defaultCenter];
    NSOperationQueue *queue = [NSOperationQueue mainQueue];
    NSArray<NSNotificationName> *names = @[
        NSApplicationDidBecomeActiveNotification,
        NSApplicationDidResignActiveNotification,
        NSApplicationDidHideNotification,
        NSApplicationDidUnhideNotification,
        NSApplicationWillTerminateNotification,
    ];
    for (NSUInteger i = 0; i < names.count; i++) {
        int event = (int)i;
        [center addObserverForName:names[i] object:nil queue:queue usingBlock:^(NSNotification *note) {
            goAppLifecycleEvent(event);
        }];
    }
}
*/
import "C"

import (
	"sync"

	"yap/internal/logger"
)

var (
	lifecycleEvents = []string{
		"app became active",
		"app resigned active",
		"app hidden",
		"app unhidden",
		"app will terminate",
	}
	observeOnce sync.Once
)

//export goAppLifecycleEvent
func goAppLifecycleEvent(event C.int) {
	if i := int(event); i >= 0 && i < len(lifecycleEvents) {
		logger.Info("Lifecycle: " + lifecycleEvents[i])
	}
}

// LogAppLifecycle writes macOS app activation, hide and terminate events to
// the Yap log so that a crash can be placed relative to them.
func LogAppLifecycle() {
	observeOnce.Do(func() { C.observeAppLifecycle() })
}
