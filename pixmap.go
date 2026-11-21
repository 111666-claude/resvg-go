package resvg

import (
	"image/color"

	"github.com/kanrichan/resvg-go/internal"
)

// Pixmap tinyskia pixmap
type Pixmap struct {
	wk  *Worker
	ptr int32
}

// NewPixmap allocates a new `Pixmap`.
// Pixmap's width is limited by int32::MAX/4.
// `Pixmap` are not goroutine-safe, don't forget to close!
func (wk *Worker) NewPixmap(width uint32, height uint32) (*Pixmap, error) {
	if !wk.used.CompareAndSwap(false, true) {
		return nil, ErrWorkerIsBeingUsed
	}
	defer wk.used.Store(false)
	pm, err := internal.TinySkiaPixmapNew(wk.ctx, wk.mod, width, height)
	if err != nil {
		return nil, err
	}
	return &Pixmap{wk, pm}, nil
}

// NewPixmapDecodePNG decodes a PNG data  into a `Pixmap`.
func (wk *Worker) NewPixmapDecodePNG(data []byte) (*Pixmap, error) {
	if !wk.used.CompareAndSwap(false, true) {
		return nil, ErrWorkerIsBeingUsed
	}
	defer wk.used.Store(false)
	pm, err := internal.TinySkiaPixmapDecodePNG(wk.ctx, wk.mod, data)
	if err != nil {
		return nil, err
	}
	return &Pixmap{wk, pm}, nil
}

// Close cloes the `Pixmap` and recovers memory.
func (pm *Pixmap) Close() error {
	if !pm.wk.used.CompareAndSwap(false, true) {
		return ErrWorkerIsBeingUsed
	}
	defer pm.wk.used.Store(false)
	if pm.ptr == 0 {
		return ErrPointerIsNil
	}
	err := internal.TinySkiaPixmapDelete(pm.wk.ctx, pm.wk.mod, pm.ptr)
	if err != nil {
		return err
	}
	pm.ptr = 0
	return nil
}

// EncodePNG encodes pixmap into a PNG data.
func (pm *Pixmap) EncodePNG() ([]byte, error) {
	if !pm.wk.used.CompareAndSwap(false, true) {
		return nil, ErrWorkerIsBeingUsed
	}
	defer pm.wk.used.Store(false)
	if pm.ptr == 0 {
		return nil, ErrPointerIsNil
	}
	return internal.TinySkiaPixmapEncodePng(pm.wk.ctx, pm.wk.mod, pm.ptr)
}

// Fill fills the whole pixmap with the specified color.
// A nil color clears the pixmap to transparent black.
func (pm *Pixmap) Fill(c color.Color) error {
	if !pm.wk.used.CompareAndSwap(false, true) {
		return ErrWorkerIsBeingUsed
	}
	defer pm.wk.used.Store(false)
	if pm.ptr == 0 {
		return ErrPointerIsNil
	}
	r, g, b, a := uint8(0), uint8(0), uint8(0), uint8(0)
	if c != nil {
		r32, g32, b32, a32 := c.RGBA()
		r, g, b, a = uint8(r32>>8), uint8(g32>>8), uint8(b32>>8), uint8(a32>>8)
	}
	return internal.TinySkiaPixmapFill(pm.wk.ctx, pm.wk.mod, pm.ptr, r, g, b, a)
}
