//go:build darwin

package overlay

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework QuartzCore

#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>

// Global references
static NSPanel *gOverlayPanel = nil;
static NSImageView *gWaveformImageView = nil;
static NSTimer *gAnimationTimer = nil;
static NSTextField *gStatusLabel = nil;
static NSButton *gStopButton = nil;
static NSButton *gCancelButton = nil;
static float gWaveformPhase = 0.0;
static float gCurrentAudioLevel = 0.0;
static float gLevelHistory[50];
static int gLevelHistoryIndex = 0;
static id gButtonMonitor = nil;

// Callbacks
extern void goOverlayStopClicked(void);
extern void goOverlayCancelClicked(void);

// Draw waveform to an NSImage using real audio levels
static NSImage* drawWaveformImage(float phase, CGFloat width, CGFloat height) {
    NSImage *image = [[NSImage alloc] initWithSize:NSMakeSize(width, height)];
    [image lockFocus];
    
    CGFloat centerY = height / 2.0;
    
    // Clear background
    [[NSColor clearColor] setFill];
    NSRectFill(NSMakeRect(0, 0, width, height));
    
    // Draw waveform bars
    NSColor *barColor = [NSColor colorWithRed:0.0 green:1.0 blue:0.31 alpha:1.0]; // #00ff4e
    [barColor setFill];
    
    CGFloat barWidth = 3.0;
    CGFloat gap = 3.0;
    CGFloat totalBarWidth = barWidth + gap;
    int numBars = (int)(width / totalBarWidth);
    int historySize = 50;
    
    for (int i = 0; i < numBars; i++) {
        CGFloat x = i * totalBarWidth;
        
        // Map bar index to level history
        int historyIndex = (gLevelHistoryIndex - numBars + i + historySize) % historySize;
        float level = gLevelHistory[historyIndex];
        
        // Add slight animation for visual interest
        CGFloat wave = sin((CGFloat)i * 0.3 + phase) * 0.05;
        CGFloat amplitude = level + wave + 0.1; // Minimum bar height
        
        if (amplitude < 0.1) amplitude = 0.1;
        if (amplitude > 1.0) amplitude = 1.0;
        
        CGFloat barHeight = amplitude * (height - 8);
        CGFloat y = centerY - barHeight / 2.0;
        
        NSBezierPath *bar = [NSBezierPath bezierPathWithRoundedRect:NSMakeRect(x, y, barWidth, barHeight) 
                                                            xRadius:1.5 
                                                            yRadius:1.5];
        [bar fill];
    }
    
    [image unlockFocus];
    return image;
}

// Update audio level (called from background thread, must be thread-safe)
static void updateAudioLevel(float level) {
    // Simple atomic-style update (these are just floats, safe to write)
    gCurrentAudioLevel = level;
    gLevelHistory[gLevelHistoryIndex] = level;
    gLevelHistoryIndex = (gLevelHistoryIndex + 1) % 50;
}

// Create the overlay panel
static void createOverlayPanel(void) {
    if (gOverlayPanel != nil) {
        return;
    }
    
    // Get screen dimensions
    NSScreen *screen = [NSScreen mainScreen];
    NSRect screenFrame = [screen frame];
    
    // Panel size and position (top center)
    CGFloat panelWidth = 380;
    CGFloat panelHeight = 100;
    CGFloat x = (screenFrame.size.width - panelWidth) / 2;
    CGFloat y = screenFrame.size.height - panelHeight - 80; // 80px from top
    
    NSRect panelFrame = NSMakeRect(x, y, panelWidth, panelHeight);
    
    // Create panel
    gOverlayPanel = [[NSPanel alloc] initWithContentRect:panelFrame
                                               styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
                                                 backing:NSBackingStoreBuffered
                                                   defer:NO];
    
    [gOverlayPanel setLevel:NSStatusWindowLevel + 1]; // Above everything
    [gOverlayPanel setBackgroundColor:[NSColor clearColor]];
    [gOverlayPanel setOpaque:NO];
    [gOverlayPanel setHasShadow:YES];
    [gOverlayPanel setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces | 
                                          NSWindowCollectionBehaviorStationary |
                                          NSWindowCollectionBehaviorIgnoresCycle];
    
    // Allow panel to receive key events
    [gOverlayPanel setBecomesKeyOnlyIfNeeded:NO];
    [gOverlayPanel setFloatingPanel:YES];
    
    // Create content view with rounded corners
    NSView *contentView = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, panelWidth, panelHeight)];
    contentView.wantsLayer = YES;
    contentView.layer.backgroundColor = [[NSColor colorWithRed:0.12 green:0.12 blue:0.12 alpha:0.95] CGColor];
    contentView.layer.cornerRadius = 16;
    contentView.layer.masksToBounds = YES;
    
    // Add border
    contentView.layer.borderColor = [[NSColor colorWithWhite:1.0 alpha:0.1] CGColor];
    contentView.layer.borderWidth = 1.0;
    
    [gOverlayPanel setContentView:contentView];
    
    // Waveform image view
    gWaveformImageView = [[NSImageView alloc] initWithFrame:NSMakeRect(20, 50, panelWidth - 40, 35)];
    [gWaveformImageView setImageScaling:NSImageScaleNone];
    [contentView addSubview:gWaveformImageView];
    
    // Status label
    gStatusLabel = [[NSTextField alloc] initWithFrame:NSMakeRect(20, 15, 200, 24)];
    [gStatusLabel setBezeled:NO];
    [gStatusLabel setDrawsBackground:NO];
    [gStatusLabel setEditable:NO];
    [gStatusLabel setSelectable:NO];
    [gStatusLabel setTextColor:[NSColor colorWithWhite:0.7 alpha:1.0]];
    [gStatusLabel setFont:[NSFont systemFontOfSize:13 weight:NSFontWeightMedium]];
    [gStatusLabel setStringValue:@"Recording..."];
    [contentView addSubview:gStatusLabel];
    
    // Button styling
    NSMutableParagraphStyle *style = [[NSMutableParagraphStyle alloc] init];
    [style setAlignment:NSTextAlignmentCenter];
    
    // Stop button
    gStopButton = [[NSButton alloc] initWithFrame:NSMakeRect(panelWidth - 170, 12, 70, 28)];
    [gStopButton setButtonType:NSButtonTypeMomentaryPushIn];
    [gStopButton setBordered:NO];
    gStopButton.wantsLayer = YES;
    gStopButton.layer.backgroundColor = [[NSColor colorWithRed:0.2 green:0.2 blue:0.2 alpha:1.0] CGColor];
    gStopButton.layer.cornerRadius = 6;
    
    NSDictionary *stopAttrs = @{
        NSForegroundColorAttributeName: [NSColor whiteColor],
        NSFontAttributeName: [NSFont systemFontOfSize:12 weight:NSFontWeightMedium],
        NSParagraphStyleAttributeName: style
    };
    NSAttributedString *stopTitle = [[NSAttributedString alloc] initWithString:@"Stop ⌥" attributes:stopAttrs];
    [gStopButton setAttributedTitle:stopTitle];
    [contentView addSubview:gStopButton];
    
    // Cancel button
    gCancelButton = [[NSButton alloc] initWithFrame:NSMakeRect(panelWidth - 90, 12, 75, 28)];
    [gCancelButton setButtonType:NSButtonTypeMomentaryPushIn];
    [gCancelButton setBordered:NO];
    gCancelButton.wantsLayer = YES;
    gCancelButton.layer.backgroundColor = [[NSColor colorWithRed:0.15 green:0.15 blue:0.15 alpha:1.0] CGColor];
    gCancelButton.layer.cornerRadius = 6;
    
    NSDictionary *cancelAttrs = @{
        NSForegroundColorAttributeName: [NSColor colorWithWhite:0.6 alpha:1.0],
        NSFontAttributeName: [NSFont systemFontOfSize:12 weight:NSFontWeightMedium],
        NSParagraphStyleAttributeName: style
    };
    NSAttributedString *cancelTitle = [[NSAttributedString alloc] initWithString:@"Cancel esc" attributes:cancelAttrs];
    [gCancelButton setAttributedTitle:cancelTitle];
    [contentView addSubview:gCancelButton];
}

// Animation tick
static void animationTick(void) {
    if (gWaveformImageView == nil) return;
    
    gWaveformPhase += 0.15;
    NSImage *img = drawWaveformImage(gWaveformPhase, 340, 35);
    [gWaveformImageView setImage:img];
}

// Show the overlay
static void showOverlay(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        createOverlayPanel();
        
        // Initialize level history
        for (int i = 0; i < 50; i++) {
            gLevelHistory[i] = 0.1;
        }
        gLevelHistoryIndex = 0;
        
        // Initial waveform
        NSImage *img = drawWaveformImage(0, 340, 35);
        [gWaveformImageView setImage:img];
        
        // Start animation timer
        if (gAnimationTimer == nil) {
            gAnimationTimer = [NSTimer scheduledTimerWithTimeInterval:0.033 // ~30fps
                                                              repeats:YES
                                                                block:^(NSTimer *timer) {
                animationTick();
            }];
        }
        
        // Set up global event monitor for button clicks (works even when other apps have focus)
        if (gButtonMonitor == nil) {
            gButtonMonitor = [NSEvent addGlobalMonitorForEventsMatchingMask:NSEventMaskLeftMouseUp
                                                                   handler:^(NSEvent *event) {
                if (gOverlayPanel != nil) {
                    NSPoint screenPoint = [NSEvent mouseLocation];
                    NSRect panelFrame = [gOverlayPanel frame];
                    
                    if (NSPointInRect(screenPoint, panelFrame)) {
                        // Convert to panel coordinates
                        NSPoint panelPoint = NSMakePoint(screenPoint.x - panelFrame.origin.x,
                                                         screenPoint.y - panelFrame.origin.y);
                        
                        // Check Stop button
                        if (gStopButton != nil) {
                            NSRect stopFrame = [gStopButton frame];
                            if (NSPointInRect(panelPoint, stopFrame)) {
                                goOverlayStopClicked();
                                return;
                            }
                        }
                        
                        // Check Cancel button
                        if (gCancelButton != nil) {
                            NSRect cancelFrame = [gCancelButton frame];
                            if (NSPointInRect(panelPoint, cancelFrame)) {
                                goOverlayCancelClicked();
                                return;
                            }
                        }
                    }
                }
            }];
        }
        
        // Show panel without taking focus from other apps
        [gOverlayPanel orderFrontRegardless];
        [gStatusLabel setStringValue:@"Recording..."];
    });
}

// Hide the overlay
static void hideOverlay(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (gAnimationTimer != nil) {
            [gAnimationTimer invalidate];
            gAnimationTimer = nil;
        }
        
        if (gButtonMonitor != nil) {
            [NSEvent removeMonitor:gButtonMonitor];
            gButtonMonitor = nil;
        }
        
        if (gOverlayPanel != nil) {
            [gOverlayPanel orderOut:nil];
        }
    });
}

// Update status text
static void updateOverlayStatus(const char *status) {
    NSString *statusStr = [NSString stringWithUTF8String:status];
    dispatch_async(dispatch_get_main_queue(), ^{
        if (gStatusLabel != nil) {
            [gStatusLabel setStringValue:statusStr];
        }
    });
}

// Cleanup
static void destroyOverlay(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (gAnimationTimer != nil) {
            [gAnimationTimer invalidate];
            gAnimationTimer = nil;
        }
        
        if (gButtonMonitor != nil) {
            [NSEvent removeMonitor:gButtonMonitor];
            gButtonMonitor = nil;
        }
        
        if (gOverlayPanel != nil) {
            [gOverlayPanel close];
            gOverlayPanel = nil;
        }
        
        gWaveformImageView = nil;
        gStatusLabel = nil;
        gStopButton = nil;
        gCancelButton = nil;
    });
}
*/
import "C"

import (
	"sync"
	"unsafe"
)

var (
	stopCallbackMu   sync.Mutex
	stopCallback     func()
	cancelCallbackMu sync.Mutex
	cancelCallback   func()
)

//export goOverlayStopClicked
func goOverlayStopClicked() {
	stopCallbackMu.Lock()
	cb := stopCallback
	stopCallbackMu.Unlock()
	
	if cb != nil {
		go cb() // Run in goroutine to avoid blocking
	}
}

//export goOverlayCancelClicked
func goOverlayCancelClicked() {
	cancelCallbackMu.Lock()
	cb := cancelCallback
	cancelCallbackMu.Unlock()
	
	if cb != nil {
		go cb() // Run in goroutine to avoid blocking
	}
}

// Overlay manages the native floating recording overlay
type Overlay struct {
	mu sync.Mutex
}

// New creates a new overlay manager
func New() *Overlay {
	return &Overlay{}
}

// Show displays the recording overlay
func (o *Overlay) Show() {
	o.mu.Lock()
	defer o.mu.Unlock()
	C.showOverlay()
}

// Hide hides the recording overlay
func (o *Overlay) Hide() {
	o.mu.Lock()
	defer o.mu.Unlock()
	C.hideOverlay()
}

// SetStatus updates the status text
func (o *Overlay) SetStatus(status string) {
	cStatus := C.CString(status)
	defer C.free(unsafe.Pointer(cStatus))
	C.updateOverlayStatus(cStatus)
}

// SetAudioLevel updates the audio level for waveform visualization (0.0-1.0)
func (o *Overlay) SetAudioLevel(level float32) {
	C.updateAudioLevel(C.float(level))
}

// SetStopCallback sets the callback for when stop is clicked
func (o *Overlay) SetStopCallback(cb func()) {
	stopCallbackMu.Lock()
	stopCallback = cb
	stopCallbackMu.Unlock()
}

// SetCancelCallback sets the callback for when cancel is clicked or Escape is pressed
func (o *Overlay) SetCancelCallback(cb func()) {
	cancelCallbackMu.Lock()
	cancelCallback = cb
	cancelCallbackMu.Unlock()
}

// Destroy cleans up the overlay
func (o *Overlay) Destroy() {
	o.mu.Lock()
	defer o.mu.Unlock()
	C.destroyOverlay()
}
