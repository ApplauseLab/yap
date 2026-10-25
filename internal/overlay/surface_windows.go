//go:build windows

package overlay

import (
	"fmt"
	"image"
	"unsafe"
)

// A top-down, premultiplied BGRA DIB is the backing store for UpdateLayeredWindow.
// Handles are owned and released only by the overlay's UI thread.
type layeredSurface struct {
	dc, bitmap, previous uintptr
	pixels               []byte
	size                 point
}

func newLayeredSurface(size image.Point) (*layeredSurface, error) {
	s := &layeredSurface{size: point{int32(size.X), int32(size.Y)}}
	s.dc, _, _ = gdi32.NewProc("CreateCompatibleDC").Call(0)
	info := struct {
		size               uint32
		width, height      int32
		planes, bits       uint16
		compression, bytes uint32
		xppm, yppm         int32
		used, important    uint32
	}{size: 40, width: int32(size.X), height: -int32(size.Y), planes: 1, bits: 32}
	var bits unsafe.Pointer
	s.bitmap, _, _ = gdi32.NewProc("CreateDIBSection").Call(s.dc, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if s.dc == 0 || s.bitmap == 0 {
		s.close()
		return nil, fmt.Errorf("could not allocate Windows overlay surface")
	}
	s.previous, _, _ = gdi32.NewProc("SelectObject").Call(s.dc, s.bitmap)
	s.pixels = unsafe.Slice((*byte)(bits), size.X*size.Y*4)
	return s, nil
}

func (s *layeredSurface) present(hwnd uintptr, frame *image.RGBA) error {
	for i := 0; i < len(s.pixels); i += 4 {
		s.pixels[i], s.pixels[i+1], s.pixels[i+2], s.pixels[i+3] = frame.Pix[i+2], frame.Pix[i+1], frame.Pix[i], frame.Pix[i+3]
	}
	var origin point
	blend := [4]byte{0, 0, 255, 1} // AC_SRC_OVER, full opacity, AC_SRC_ALPHA.
	result, _, err := user32.NewProc("UpdateLayeredWindow").Call(hwnd, 0, 0, uintptr(unsafe.Pointer(&s.size)), s.dc, uintptr(unsafe.Pointer(&origin)), 0, uintptr(unsafe.Pointer(&blend)), 2)
	if result == 0 {
		return fmt.Errorf("could not present Windows overlay: %w", err)
	}
	return nil
}

func (s *layeredSurface) close() {
	if s.dc != 0 && s.previous != 0 {
		gdi32.NewProc("SelectObject").Call(s.dc, s.previous)
	}
	if s.bitmap != 0 {
		gdi32.NewProc("DeleteObject").Call(s.bitmap)
	}
	if s.dc != 0 {
		gdi32.NewProc("DeleteDC").Call(s.dc)
	}
	s.dc, s.bitmap, s.previous = 0, 0, 0
	s.pixels = nil
}
