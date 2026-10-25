package overlay

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
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
				frame := r.render(status, history, 0)
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
