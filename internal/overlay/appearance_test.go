package overlay

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font"
)

func TestOverlayAppearanceAtDisplayScales(t *testing.T) {
	fontData, _ := os.ReadFile(filepath.Join(os.Getenv("WINDIR"), "Fonts", "seguisb.ttf"))
	for _, scale := range []float64{1, 1.5, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			r, err := newOverlayRenderer(scale, fontData)
			if err != nil {
				t.Fatal(err)
			}
			defer r.face.Close()
			var history [waveformBars]float32
			for i := range history {
				history[i] = float32(i%15) / 20
			}
			for _, status := range []string{"Recording...", "Transcribing..."} {
				frame := r.render(status, "Stop Alt", "Cancel Esc", history, 0)
				if frame.RGBAAt(0, 0).A != 0 {
					t.Fatal("outer corner is not transparent")
				}
				if frame.RGBAAt(int(12*scale), int(10*scale)).A > 100 {
					t.Fatal("panel corners are square")
				}
				if frame.RGBAAt(int(200*scale), int(50*scale)).A < 240 {
					t.Fatal("panel body missing")
				}
				// Layered windows require premultiplied color, including antialiased text.
				for i := 0; i < len(frame.Pix); i += 4 {
					if frame.Pix[i] > frame.Pix[i+3] || frame.Pix[i+1] > frame.Pix[i+3] || frame.Pix[i+2] > frame.Pix[i+3] {
						t.Fatal("invalid premultiplied pixel")
					}
				}
				if dir := os.Getenv("YAP_TEST_OVERLAY_ARTIFACTS"); dir != "" {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
					f, err := os.Create(filepath.Join(dir, fmt.Sprintf("overlay-%s-%gx.png", status[:len(status)-3], scale)))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(f, frame)
					closeErr := f.Close()
					if err != nil {
						t.Fatal(err)
					}
					if closeErr != nil {
						t.Fatal(closeErr)
					}
				}
			}
		})
	}
}

func TestButtonText(t *testing.T) {
	for _, c := range []struct{ action, key, want string }{
		{"Stop", "Alt", "Stop Alt"},
		{"Stop", "⌘", "Stop ⌘"},
		{"Cancel", "Esc", "Cancel Esc"},
		{"Stop", "", "Stop"},
	} {
		if got := buttonText(c.action, c.key); got != c.want {
			t.Errorf("buttonText(%q, %q) = %q, want %q", c.action, c.key, got, c.want)
		}
	}
}

func TestOverlayButtonsFitHotkeyLabels(t *testing.T) {
	fontData, _ := os.ReadFile(filepath.Join(os.Getenv("WINDIR"), "Fonts", "seguisb.ttf"))
	for _, scale := range []float64{1, 1.5, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			r, err := newOverlayRenderer(scale, fontData)
			if err != nil {
				t.Fatal(err)
			}
			defer r.face.Close()

			// Bare captions keep the original fixed layout and click targets.
			stop, cancel := r.buttons("Stop", "Cancel")
			if want := image.Rect(220, 68, 290, 96); stop != want {
				t.Errorf("stop = %v, want %v", stop, want)
			}
			if want := image.Rect(300, 68, 375, 96); cancel != want {
				t.Errorf("cancel = %v, want %v", cancel, want)
			}

			// Captions with keys widen the buttons instead of overflowing them.
			for _, labels := range [][2]string{{"Stop Alt", "Cancel Esc"}, {"Stop Shift", "Cancel Space"}, {"Stop Super", "Cancel Enter"}} {
				stop, cancel := r.buttons(labels[0], labels[1])
				for i, b := range []image.Rectangle{stop, cancel} {
					textWidth := float64(font.MeasureString(r.face, labels[i])) / 64 / scale
					if float64(b.Dx()) < textWidth+2*buttonPadding {
						t.Errorf("%q does not fit in %v", labels[i], b)
					}
				}
				if cancel.Max.X != buttonRight || stop.Max.X+buttonGap != cancel.Min.X {
					t.Errorf("buttons not right-aligned: %v %v", stop, cancel)
				}
				if stop.Min.X < 130 {
					t.Errorf("stop button %v overlaps the status text", stop)
				}
			}

			// The configured key is actually drawn on the buttons.
			var history [waveformBars]float32
			ink := func(texts [2]string) [2]int {
				frame := r.render("Recording...", texts[0], texts[1], history, 0)
				stop, cancel := r.buttons(texts[0], texts[1])
				var counts [2]int
				for i, b := range []image.Rectangle{stop, cancel} {
					px := image.Rect(int(float64(b.Min.X)*scale), int(float64(b.Min.Y)*scale), int(float64(b.Max.X)*scale), int(float64(b.Max.Y)*scale))
					for y := px.Min.Y; y < px.Max.Y; y++ {
						for x := px.Min.X; x < px.Max.X; x++ {
							if frame.RGBAAt(x, y).R > 100 { // text, not the button fill
								counts[i]++
							}
						}
					}
				}
				return counts
			}
			plain, labeled := ink([2]string{"Stop", "Cancel"}), ink([2]string{"Stop Alt", "Cancel F9"})
			for i := range plain {
				if labeled[i] <= plain[i] {
					t.Errorf("button %d does not show its key label: ink %d <= %d", i, labeled[i], plain[i])
				}
			}
		})
	}
}
