package resvg

import (
	"errors"
	"image/color"
	"math"
)

// FitMode selects how the SVG is proportionally fitted into the target canvas.
type FitMode int

const (
	// FitContain uniformly scales the SVG so that it is fully visible inside
	// the canvas. The whole SVG is rendered; any leftover canvas area stays
	// transparent or is painted with the background color.
	FitContain FitMode = iota
	// FitCover uniformly scales the SVG so that it fills the whole canvas.
	// The canvas is fully covered; the parts of the SVG that overflow the
	// canvas are clipped.
	FitCover
)

// Align positions the fitted SVG inside the canvas.
// Both fields are clamped to the range [-1, 1]; non-finite values are treated
// as 0. The zero value Align{} centers the SVG on both axes.
//
//	Horizontal: -1 = left,   0 = center, 1 = right
//	Vertical:   -1 = top,    0 = center, 1 = bottom
type Align struct {
	Horizontal float32
	Vertical   float32
}

// CanvasOptions controls the proportional canvas fitting of an SVG.
type CanvasOptions struct {
	// Width and Height define the exact output PNG size in pixels.
	Width  uint32
	Height uint32

	// Mode selects FitContain or FitCover behavior.
	Mode FitMode

	// Align positions the fitted SVG within the canvas.
	// The zero value centers it.
	Align Align

	// Background fills the canvas before the SVG is rendered.
	// A nil background leaves the canvas fully transparent.
	Background color.Color
}

// FitTransform is the uniform scale-and-translate transform computed by
// FitGeometry. It maps SVG user units onto target canvas pixels.
type FitTransform struct {
	ScaleX     float32
	ScaleY     float32
	TranslateX float32
	TranslateY float32
}

const (
	// maxCanvasSide bounds one edge of a fitted canvas.
	maxCanvasSide = 1 << 20
	// maxCanvasPixels bounds the number of pixels of a fitted canvas.
	// A pixmap stores 4 bytes per pixel, so this caps a single canvas
	// allocation at 1 GiB before any WASM memory is touched.
	maxCanvasPixels = 1 << 28
)

var (
	// ErrInvalidCanvasOptions is returned when the canvas options are missing.
	ErrInvalidCanvasOptions = errors.New("canvas options must not be nil")
	// ErrInvalidCanvasSize is returned when the target size is zero or exceeds
	// the built-in canvas memory limits.
	ErrInvalidCanvasSize = errors.New("canvas width and height must be non-zero and within the memory limit")
	// ErrInvalidSVGSize is returned when the parsed SVG has no usable size.
	ErrInvalidSVGSize = errors.New("svg width and height must be positive and finite")
	// ErrInvalidFitMode is returned when the fit mode is neither FitContain nor FitCover.
	ErrInvalidFitMode = errors.New("fit mode must be FitContain or FitCover")
	// ErrNonFiniteScale is returned when the computed scale is not a finite number.
	ErrNonFiniteScale = errors.New("computed canvas scale is not a finite number")
)

// RenderToCanvas parses the SVG and renders it onto a canvas of exactly the
// requested size, fitting it proportionally according to the options.
// The default Render behavior is unchanged.
func (wk *Worker) RenderToCanvas(svg []byte, opts *CanvasOptions) ([]byte, error) {
	if opts == nil {
		return nil, ErrInvalidCanvasOptions
	}
	tree, err := wk.NewTreeFromData(svg, nil)
	if err != nil {
		return nil, err
	}
	defer tree.Close()
	return tree.RenderToCanvas(opts)
}

// RenderToCanvas renders the parsed Tree onto a canvas of exactly the requested
// size, fitting it proportionally according to the options. It can be called
// repeatedly; every call allocates and frees its own pixmap, so the Tree and
// Worker lifecycles are unaffected.
func (t *Tree) RenderToCanvas(opts *CanvasOptions) ([]byte, error) {
	if opts == nil {
		return nil, ErrInvalidCanvasOptions
	}
	if t.ptr == 0 {
		return nil, ErrPointerIsNil
	}
	return renderTreeToCanvas(t, opts)
}

func renderTreeToCanvas(t *Tree, opts *CanvasOptions) ([]byte, error) {
	if err := validateCanvasSize(opts.Width, opts.Height); err != nil {
		return nil, err
	}
	svgWidth, svgHeight, err := t.GetSize()
	if err != nil {
		return nil, err
	}
	fit, err := FitGeometry(svgWidth, svgHeight, opts.Width, opts.Height, opts.Mode, opts.Align)
	if err != nil {
		return nil, err
	}
	pixmap, err := t.wk.NewPixmap(opts.Width, opts.Height)
	if err != nil {
		return nil, err
	}
	defer pixmap.Close()
	if opts.Background != nil {
		if err := pixmap.Fill(opts.Background); err != nil {
			return nil, err
		}
	}
	if err := t.Render(
		TransformFromRow(fit.ScaleX, 0, 0, fit.ScaleY, fit.TranslateX, fit.TranslateY),
		pixmap,
	); err != nil {
		return nil, err
	}
	return pixmap.EncodePNG()
}

func validateCanvasSize(width uint32, height uint32) error {
	if width == 0 || height == 0 || width > maxCanvasSide || height > maxCanvasSide {
		return ErrInvalidCanvasSize
	}
	if uint64(width)*uint64(height) > maxCanvasPixels {
		return ErrInvalidCanvasSize
	}
	return nil
}

// FitGeometry computes the uniform scale and translation that fits an SVG of
// svgWidth by svgHeight into a canvas of canvasWidth by canvasHeight using the
// given mode and alignment.
func FitGeometry(svgWidth, svgHeight float32, canvasWidth, canvasHeight uint32, mode FitMode, align Align) (FitTransform, error) {
	if mode != FitContain && mode != FitCover {
		return FitTransform{}, ErrInvalidFitMode
	}
	cw, ch := float32(canvasWidth), float32(canvasHeight)
	if !isFiniteF32(svgWidth) || !isFiniteF32(svgHeight) || svgWidth <= 0 || svgHeight <= 0 {
		return FitTransform{}, ErrInvalidSVGSize
	}
	if !isFiniteF32(cw) || !isFiniteF32(ch) || cw <= 0 || ch <= 0 {
		return FitTransform{}, ErrInvalidCanvasSize
	}
	scaleW := cw / svgWidth
	scaleH := ch / svgHeight
	scale := scaleW
	if mode == FitCover {
		if scaleH > scale {
			scale = scaleH
		}
	} else if scaleH < scale {
		scale = scaleH
	}
	if !isFiniteF32(scale) || scale <= 0 {
		return FitTransform{}, ErrNonFiniteScale
	}
	fittedWidth := svgWidth * scale
	fittedHeight := svgHeight * scale
	h := clampAlign(align.Horizontal)
	v := clampAlign(align.Vertical)
	tx := (cw - fittedWidth) * (h + 1) * 0.5
	ty := (ch - fittedHeight) * (v + 1) * 0.5
	if !isFiniteF32(tx) || !isFiniteF32(ty) {
		return FitTransform{}, ErrNonFiniteScale
	}
	return FitTransform{
		ScaleX:     scale,
		ScaleY:     scale,
		TranslateX: tx,
		TranslateY: ty,
	}, nil
}

func isFiniteF32(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
}

// clampAlign restricts an alignment factor to [-1, 1], treating NaN as centered.
func clampAlign(v float32) float32 {
	if !isFiniteF32(v) {
		return 0
	}
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
}
