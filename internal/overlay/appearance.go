package overlay

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Coordinates are device-independent pixels, matching the macOS 380x100 panel.
const overlayWidth, overlayHeight = 400, 124
const waveformBars = 56

// Buttons are right-aligned and never narrower than the original fixed layout.
const buttonTop, buttonBottom, buttonRight = 68, 96, 375
const stopMinWidth, cancelMinWidth, buttonGap, buttonPadding = 70, 75, 10, 10

// buttonText returns a button caption such as "Stop ⌥" or "Cancel Esc".
func buttonText(action, keyLabel string) string {
	if keyLabel == "" {
		return action
	}
	return action + " " + keyLabel
}

type overlayRenderer struct {
	scale      float64
	face       font.Face
	background *image.RGBA
}

func newOverlayRenderer(scale float64, fontData []byte) (*overlayRenderer, error) {
	if len(fontData) == 0 {
		fontData = gobold.TTF
	}
	f, err := opentype.Parse(fontData)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 13 * scale, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	r := &overlayRenderer{scale: scale, face: face}
	r.background = image.NewRGBA(image.Rect(0, 0, int(overlayWidth*scale), int(overlayHeight*scale)))
	// Soft shadow and antialiased translucent panel, including transparent corners.
	for y := 0; y < r.background.Rect.Dy(); y++ {
		for x := 0; x < r.background.Rect.Dx(); x++ {
			d := roundedDistance((float64(x)+.5)/scale, (float64(y)+.5)/scale, 10, 13, 390, 113, 16)
			a := 0.0
			if d > 0 {
				a = 55 * math.Exp(-d*d/40)
			} else {
				a = 55
			}
			r.background.SetRGBA(x, y, color.RGBA{A: uint8(a)})
		}
	}
	r.round(r.background, 10, 8, 390, 108, 16, color.NRGBA{90, 90, 90, 240})
	r.round(r.background, 11, 9, 389, 107, 15, color.NRGBA{30, 30, 30, 250})
	return r, nil
}

func roundedDistance(x, y, left, top, right, bottom, radius float64) float64 {
	qx := math.Abs(x-(left+right)/2) - ((right-left)/2 - radius)
	qy := math.Abs(y-(top+bottom)/2) - ((bottom-top)/2 - radius)
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - radius
}

func (r *overlayRenderer) round(dst *image.RGBA, left, top, right, bottom, radius float64, c color.NRGBA) {
	s := r.scale
	for y := max(0, int(top*s)-1); y < min(dst.Rect.Dy(), int(bottom*s)+1); y++ {
		for x := max(0, int(left*s)-1); x < min(dst.Rect.Dx(), int(right*s)+1); x++ {
			coverage := math.Max(0, math.Min(1, .5-roundedDistance((float64(x)+.5)/s, (float64(y)+.5)/s, left, top, right, bottom, radius)*s))
			a := uint32(float64(c.A) * coverage)
			i := dst.PixOffset(x, y)
			for j, v := range []uint8{c.R, c.G, c.B} {
				dst.Pix[i+j] = uint8((uint32(v)*a + uint32(dst.Pix[i+j])*(255-a) + 127) / 255)
			}
			dst.Pix[i+3] = uint8(a + (uint32(dst.Pix[i+3])*(255-a)+127)/255)
		}
	}
}

// buttons sizes the Stop and Cancel buttons to fit their captions, in
// device-independent pixels.
func (r *overlayRenderer) buttons(stopText, cancelText string) (stop, cancel image.Rectangle) {
	width := func(text string, minWidth int) int {
		w := int(math.Ceil(float64(font.MeasureString(r.face, text))/64/r.scale)) + 2*buttonPadding
		return max(minWidth, w)
	}
	cancel = image.Rect(buttonRight-width(cancelText, cancelMinWidth), buttonTop, buttonRight, buttonBottom)
	stopRight := cancel.Min.X - buttonGap
	stop = image.Rect(stopRight-width(stopText, stopMinWidth), buttonTop, stopRight, buttonBottom)
	return stop, cancel
}

func (r *overlayRenderer) render(status, stopText, cancelText string, history [waveformBars]float32, hover int) *image.RGBA {
	dst := image.NewRGBA(r.background.Bounds())
	copy(dst.Pix, r.background.Pix)
	for i, level := range history {
		if math.IsNaN(float64(level)) {
			level = 0
		}
		level = max(0, min(1, level))
		h := 3 + float64(level)*25
		x := 30 + float64(i)*6
		r.round(dst, x, 40-h/2, x+3, 40+h/2, 1.5, color.NRGBA{0, 255, 78, 255})
	}
	if status == "Recording..." {
		status = "Recording…"
	}
	if status == "Transcribing..." {
		status = "Transcribing…"
	}
	r.text(dst, status, image.Rect(31, 68, 215, 96), color.NRGBA{180, 180, 180, 255}, false)
	if status == "Recording…" {
		stopBounds, cancelBounds := r.buttons(stopText, cancelText)
		for i, b := range []image.Rectangle{stopBounds, cancelBounds} {
			shade := uint8(40)
			if i == 0 {
				shade = 51
			}
			if hover == i+1 {
				shade += 12
			}
			r.round(dst, float64(b.Min.X), float64(b.Min.Y), float64(b.Max.X), float64(b.Max.Y), 7, color.NRGBA{shade, shade, shade, 255})
		}
		r.text(dst, stopText, stopBounds, color.NRGBA{245, 245, 245, 255}, true)
		r.text(dst, cancelText, cancelBounds, color.NRGBA{160, 160, 160, 255}, true)
	}
	return dst
}

func (r *overlayRenderer) text(dst draw.Image, text string, bounds image.Rectangle, c color.NRGBA, center bool) {
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: r.face}
	x := fixed.Int26_6(float64(bounds.Min.X) * r.scale * 64)
	if center {
		x += (fixed.Int26_6(float64(bounds.Dx())*r.scale*64) - d.MeasureString(text)) / 2
	}
	m := r.face.Metrics()
	y := fixed.Int26_6(float64(bounds.Min.Y+bounds.Max.Y)*r.scale*32) + (m.Ascent-m.Descent)/2
	d.Dot = fixed.Point26_6{X: x, Y: y}
	d.DrawString(text)
}
