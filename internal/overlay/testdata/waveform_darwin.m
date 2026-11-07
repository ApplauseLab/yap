#include "overlay_native.h"
#include <stdio.h>

void goOverlayStopClicked(void) {}
void goOverlayCancelClicked(void) {}

int main(void) {
    @autoreleasepool {
        const int bars = 340 / 6;
        // Start beyond the old negative-index region to detect duplicated
        // peaks first, then exercise every cursor over several full wraps.
        for (int i = 0; i < 6; i++) updateAudioLevel(0);
        for (int cycle = 0; cycle < 3; cycle++) {
            updateAudioLevel(1);
            for (int age = 0; age <= bars; age++) {
                NSImage *image = drawWaveformImage(0, 340, 35);
                NSBitmapImageRep *bitmap = [NSBitmapImageRep imageRepWithData:[image TIFFRepresentation]];
                int peaks = 0;
                int peakBar = -1;
                for (int bar = 0; bar < bars; bar++) {
                    NSInteger x = (bar * 6 + 1.5) * bitmap.pixelsWide / 340.0;
                    NSInteger y = 25 * bitmap.pixelsHigh / 35.0;
                    NSColor *color = [[bitmap colorAtX:x y:y] colorUsingColorSpace:[NSColorSpace deviceRGBColorSpace]];
                    if (color.alphaComponent > 0.5 && color.greenComponent > 0.8) {
                        peaks++;
                        peakBar = bar;
                    }
                }
                [image release];
                int expected = age < bars ? 1 : 0;
                if (peaks != expected || (expected && peakBar != bars - 1 - age)) {
                    fprintf(stderr, "cycle=%d age=%d: expected %d peak at bar %d; got %d at %d\n",
                            cycle, age, expected, bars - 1 - age, peaks, peakBar);
                    return 1;
                }
                updateAudioLevel(0);
            }
        }
        puts("PASS: one impulse moves once across all 56 bars and expires; three wraps, ASan/UBSan enabled");
    }
    return 0;
}
