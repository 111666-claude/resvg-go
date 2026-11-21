package resvg

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"testing"
)

func decodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	return img
}

func assertDims(t *testing.T, img image.Image, width, height int) {
	t.Helper()
	b := img.Bounds()
	if b.Dx() != width || b.Dy() != height {
		t.Fatalf("png size = %dx%d, want %dx%d", b.Dx(), b.Dy(), width, height)
	}
}

func assertColor(t *testing.T, img image.Image, x, y int, want color.NRGBA) {
	t.Helper()
	got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	tol := 2
	abs := func(a, b uint8) int {
		d := int(a) - int(b)
		if d < 0 {
			return -d
		}
		return d
	}
	if abs(got.R, want.R) > tol || abs(got.G, want.G) > tol ||
		abs(got.B, want.B) > tol || abs(got.A, want.A) > tol {
		t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
	}
}

const (
	redRectSVG = `<svg width="100" height="50" xmlns="http://www.w3.org/2000/svg">
		<rect x="0" y="0" width="100" height="50" fill="#ff0000"/>
	</svg>`
	halfBlueRectSVG = `<svg width="100" height="50" xmlns="http://www.w3.org/2000/svg">
		<rect x="0" y="0" width="100" height="50" fill="rgba(0,0,255,0.5)"/>
	</svg>`
)

func TestFitGeometry(t *testing.T) {
	cases := []struct {
		name       string
		svgW, svgH float32
		canW, canH uint32
		mode       FitMode
		align      Align
		scale      float32
		tx, ty     float32
	}{
		{
			name: "contain landscape svg centered",
			svgW: 100, svgH: 50, canW: 200, canH: 200, mode: FitContain,
			align: Align{}, scale: 2, tx: 0, ty: 50,
		},
		{
			name: "cover landscape svg centered",
			svgW: 100, svgH: 50, canW: 200, canH: 200, mode: FitCover,
			align: Align{}, scale: 4, tx: -100, ty: 0,
		},
		{
			name: "contain portrait svg centered",
			svgW: 50, svgH: 100, canW: 200, canH: 200, mode: FitContain,
			align: Align{}, scale: 2, tx: 50, ty: 0,
		},
		{
			name: "contain aligned top-left",
			svgW: 100, svgH: 50, canW: 200, canH: 200, mode: FitContain,
			align: Align{Horizontal: -1, Vertical: -1}, scale: 2, tx: 0, ty: 0,
		},
		{
			name: "contain aligned bottom-right",
			svgW: 100, svgH: 50, canW: 200, canH: 200, mode: FitContain,
			align: Align{Horizontal: 1, Vertical: 1}, scale: 2, tx: 0, ty: 100,
		},
		{
			name: "align values are clamped",
			svgW: 100, svgH: 50, canW: 200, canH: 200, mode: FitContain,
			align: Align{Horizontal: 5, Vertical: -5}, scale: 2, tx: 0, ty: 0,
		},
		{
			name: "nan align falls back to center",
			svgW: 100, svgH: 50, canW: 200, canH: 200, mode: FitContain,
			align: Align{Horizontal: float32(math.NaN()), Vertical: float32(math.NaN())},
			scale: 2, tx: 0, ty: 50,
		},
		{
			name: "exact aspect ratio has no margin",
			svgW: 100, svgH: 50, canW: 300, canH: 150, mode: FitContain,
			align: Align{}, scale: 3, tx: 0, ty: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fit, err := FitGeometry(tc.svgW, tc.svgH, tc.canW, tc.canH, tc.mode, tc.align)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(float64(fit.ScaleX-tc.scale)) > 1e-5 || fit.ScaleX != fit.ScaleY {
				t.Fatalf("scale = %v/%v, want %v uniform", fit.ScaleX, fit.ScaleY, tc.scale)
			}
			if math.Abs(float64(fit.TranslateX-tc.tx)) > 1e-4 ||
				math.Abs(float64(fit.TranslateY-tc.ty)) > 1e-4 {
				t.Fatalf("translate = (%v,%v), want (%v,%v)", fit.TranslateX, fit.TranslateY, tc.tx, tc.ty)
			}
		})
	}
}

func TestFitGeometryErrors(t *testing.T) {
	if _, err := FitGeometry(100, 50, 200, 200, FitMode(99), Align{}); err != ErrInvalidFitMode {
		t.Fatalf("invalid mode: got %v, want %v", err, ErrInvalidFitMode)
	}
	for _, bad := range []float32{0, -1, float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		if _, err := FitGeometry(bad, 50, 200, 200, FitContain, Align{}); err != ErrInvalidSVGSize {
			t.Fatalf("svg width %v: got %v, want %v", bad, err, ErrInvalidSVGSize)
		}
		if _, err := FitGeometry(100, bad, 200, 200, FitContain, Align{}); err != ErrInvalidSVGSize {
			t.Fatalf("svg height %v: got %v, want %v", bad, err, ErrInvalidSVGSize)
		}
	}
	if _, err := FitGeometry(100, 50, 0, 200, FitContain, Align{}); err != ErrInvalidCanvasSize {
		t.Fatalf("zero canvas: got %v, want %v", err, ErrInvalidCanvasSize)
	}
	// A degenerate SVG size can overflow float32 when computing the scale.
	if _, err := FitGeometry(math.SmallestNonzeroFloat32, 50, 1<<20, 1<<20, FitCover, Align{}); err != ErrNonFiniteScale {
		t.Fatalf("overflow scale: got %v, want %v", err, ErrNonFiniteScale)
	}
}

func newTestWorker(t *testing.T) *Worker {
	t.Helper()
	worker, err := NewDefaultWorker(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { worker.Close() })
	return worker
}

func TestRenderToCanvasContain(t *testing.T) {
	worker := newTestWorker(t)
	// 100x50 SVG fitted into 200x200: scale 2, content 200x100, centered at y=50.
	data, err := worker.RenderToCanvas([]byte(redRectSVG), &CanvasOptions{
		Width: 200, Height: 200, Mode: FitContain,
	})
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, data)
	assertDims(t, img, 200, 200)
	// transparent margin above and below the content
	assertColor(t, img, 100, 0, color.NRGBA{0, 0, 0, 0})
	assertColor(t, img, 100, 49, color.NRGBA{0, 0, 0, 0})
	assertColor(t, img, 100, 199, color.NRGBA{0, 0, 0, 0})
	// content band is opaque red, including its exact boundary rows
	assertColor(t, img, 0, 50, color.NRGBA{255, 0, 0, 255})
	assertColor(t, img, 100, 100, color.NRGBA{255, 0, 0, 255})
	assertColor(t, img, 199, 149, color.NRGBA{255, 0, 0, 255})
	// row 150 is already outside the content band
	assertColor(t, img, 100, 150, color.NRGBA{0, 0, 0, 0})
}

func TestRenderToCanvasCover(t *testing.T) {
	worker := newTestWorker(t)
	// 100x50 SVG covering 200x200: scale 4, content 400x200, horizontally clipped.
	data, err := worker.RenderToCanvas([]byte(redRectSVG), &CanvasOptions{
		Width: 200, Height: 200, Mode: FitCover,
	})
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, data)
	assertDims(t, img, 200, 200)
	// every pixel, corners included, must be opaque red: nothing leaks past the clip
	for _, p := range [][2]int{{0, 0}, {199, 0}, {0, 199}, {199, 199}, {100, 100}} {
		assertColor(t, img, p[0], p[1], color.NRGBA{255, 0, 0, 255})
	}
}

func TestRenderToCanvasCoverAlignment(t *testing.T) {
	worker := newTestWorker(t)
	// two side-by-side halves make the horizontal clip offset observable
	svg := []byte(`<svg width="100" height="50" xmlns="http://www.w3.org/2000/svg">
		<rect x="0"  y="0" width="50" height="50" fill="#ff0000"/>
		<rect x="50" y="0" width="50" height="50" fill="#00ff00"/>
	</svg>`)
	// scale 4 -> content 400x200 in a 200x200 canvas, 200px clipped horizontally
	render := func(align float32) image.Image {
		data, err := worker.RenderToCanvas(svg, &CanvasOptions{
			Width: 200, Height: 200, Mode: FitCover,
			Align: Align{Horizontal: align},
		})
		if err != nil {
			t.Fatal(err)
		}
		return decodePNG(t, data)
	}
	// centered: 100px clipped from each side, each half keeps its middle 100px
	img := render(0)
	assertColor(t, img, 50, 100, color.NRGBA{255, 0, 0, 255})
	assertColor(t, img, 150, 100, color.NRGBA{0, 255, 0, 255})
	// left aligned: nothing clipped on the left, the whole green half is cut off
	img = render(-1)
	assertColor(t, img, 0, 100, color.NRGBA{255, 0, 0, 255})
	assertColor(t, img, 199, 100, color.NRGBA{255, 0, 0, 255})
	// right aligned: nothing clipped on the right, only green remains visible
	img = render(1)
	assertColor(t, img, 0, 100, color.NRGBA{0, 255, 0, 255})
	assertColor(t, img, 199, 100, color.NRGBA{0, 255, 0, 255})
}

func TestRenderToCanvasBackground(t *testing.T) {
	worker := newTestWorker(t)
	data, err := worker.RenderToCanvas([]byte(redRectSVG), &CanvasOptions{
		Width: 200, Height: 200, Mode: FitContain,
		Background: color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	})
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, data)
	assertDims(t, img, 200, 200)
	// the formerly transparent margin is now the opaque background color
	assertColor(t, img, 100, 10, color.NRGBA{255, 255, 255, 255})
	assertColor(t, img, 100, 190, color.NRGBA{255, 255, 255, 255})
	// content keeps its own color
	assertColor(t, img, 100, 100, color.NRGBA{255, 0, 0, 255})
}

func TestRenderToCanvasSemiTransparent(t *testing.T) {
	worker := newTestWorker(t)

	// half-transparent blue over a transparent canvas
	data, err := worker.RenderToCanvas([]byte(halfBlueRectSVG), &CanvasOptions{
		Width: 200, Height: 200, Mode: FitContain,
	})
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, data)
	assertColor(t, img, 100, 100, color.NRGBA{0, 0, 255, 128})
	assertColor(t, img, 100, 10, color.NRGBA{0, 0, 0, 0})

	// the same graphic over an opaque white background blends correctly
	data, err = worker.RenderToCanvas([]byte(halfBlueRectSVG), &CanvasOptions{
		Width: 200, Height: 200, Mode: FitContain,
		Background: color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	})
	if err != nil {
		t.Fatal(err)
	}
	img = decodePNG(t, data)
	assertColor(t, img, 100, 100, color.NRGBA{128, 128, 255, 255})
	assertColor(t, img, 100, 10, color.NRGBA{255, 255, 255, 255})
}

func TestRenderToCanvasAlignment(t *testing.T) {
	worker := newTestWorker(t)
	opts := &CanvasOptions{Width: 200, Height: 200, Mode: FitContain, Align: Align{Vertical: -1}}
	data, err := worker.RenderToCanvas([]byte(redRectSVG), opts)
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, data)
	// top aligned: content starts at the first row, margin is at the bottom
	assertColor(t, img, 100, 0, color.NRGBA{255, 0, 0, 255})
	assertColor(t, img, 100, 99, color.NRGBA{255, 0, 0, 255})
	assertColor(t, img, 100, 100, color.NRGBA{0, 0, 0, 0})

	opts.Align = Align{Vertical: 1}
	data, err = worker.RenderToCanvas([]byte(redRectSVG), opts)
	if err != nil {
		t.Fatal(err)
	}
	img = decodePNG(t, data)
	// bottom aligned: margin is on top, content occupies the last 100 rows
	assertColor(t, img, 100, 99, color.NRGBA{0, 0, 0, 0})
	assertColor(t, img, 100, 100, color.NRGBA{255, 0, 0, 255})
	assertColor(t, img, 100, 199, color.NRGBA{255, 0, 0, 255})
}

func TestRenderToCanvasRepeatable(t *testing.T) {
	worker := newTestWorker(t)
	opts := &CanvasOptions{Width: 123, Height: 77, Mode: FitContain}
	first, err := worker.RenderToCanvas([]byte(redRectSVG), opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := worker.RenderToCanvas([]byte(redRectSVG), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("repeated RenderToCanvas calls must produce identical output")
	}
	assertDims(t, decodePNG(t, first), 123, 77)

	// the same holds through a parsed Tree, which stays usable across renders
	tree, err := worker.NewTreeFromData([]byte(redRectSVG), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()
	a, err := tree.RenderToCanvas(opts)
	if err != nil {
		t.Fatal(err)
	}
	b, err := tree.RenderToCanvas(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("repeated Tree.RenderToCanvas calls must produce identical output")
	}
	if !bytes.Equal(a, first) {
		t.Fatal("worker and tree entry points must render identically")
	}
}

func TestRenderToCanvasErrors(t *testing.T) {
	worker := newTestWorker(t)

	if _, err := worker.RenderToCanvas([]byte(redRectSVG), nil); err != ErrInvalidCanvasOptions {
		t.Fatalf("nil options: got %v, want %v", err, ErrInvalidCanvasOptions)
	}
	if _, err := worker.RenderToCanvas([]byte(redRectSVG), &CanvasOptions{Width: 0, Height: 10}); err != ErrInvalidCanvasSize {
		t.Fatalf("zero width: got %v, want %v", err, ErrInvalidCanvasSize)
	}
	if _, err := worker.RenderToCanvas([]byte(redRectSVG), &CanvasOptions{Width: 10, Height: 0}); err != ErrInvalidCanvasSize {
		t.Fatalf("zero height: got %v, want %v", err, ErrInvalidCanvasSize)
	}
	if _, err := worker.RenderToCanvas([]byte(redRectSVG), &CanvasOptions{Width: maxCanvasSide + 1, Height: 1}); err != ErrInvalidCanvasSize {
		t.Fatalf("oversized side: got %v, want %v", err, ErrInvalidCanvasSize)
	}
	if _, err := worker.RenderToCanvas([]byte(redRectSVG), &CanvasOptions{Width: 1 << 15, Height: 1 << 15}); err != ErrInvalidCanvasSize {
		t.Fatalf("oversized pixel count: got %v, want %v", err, ErrInvalidCanvasSize)
	}
	// unparsable SVG data must surface the parser error, not a panic
	if _, err := worker.RenderToCanvas([]byte("not an svg"), &CanvasOptions{Width: 10, Height: 10}); err == nil {
		t.Fatal("invalid SVG data must return an error")
	}
	// a closed Tree must be rejected up front
	tree, err := worker.NewTreeFromData([]byte(redRectSVG), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := tree.RenderToCanvas(&CanvasOptions{Width: 10, Height: 10}); err != ErrPointerIsNil {
		t.Fatalf("closed tree: got %v, want %v", err, ErrPointerIsNil)
	}
	// after any failed call the worker must still be usable (lock released, no leak)
	if _, err := worker.RenderToCanvas([]byte(redRectSVG), &CanvasOptions{Width: 16, Height: 16}); err != nil {
		t.Fatalf("worker unusable after error: %v", err)
	}
}

func TestRenderToCanvasTreeLifecycle(t *testing.T) {
	worker := newTestWorker(t)
	svg, err := os.ReadFile("./testdata/beach.svg")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := worker.NewTreeFromData(svg, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 512x512 SVG contained into 128x256: scale 0.25, content 128x128, 64px bands
	data, err := tree.RenderToCanvas(&CanvasOptions{Width: 128, Height: 256, Mode: FitContain})
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, data)
	assertDims(t, img, 128, 256)
	if err := tree.Close(); err != nil {
		t.Fatal(err)
	}
	// default Render path and worker lifecycle are untouched
	pngData, err := worker.Render([]byte(redRectSVG))
	if err != nil {
		t.Fatal(err)
	}
	defaultImg := decodePNG(t, pngData)
	assertDims(t, defaultImg, 100, 50)
}

func TestRenderToCanvasSizedSVGFallback(t *testing.T) {
	worker := newTestWorker(t)
	// an SVG without width/height attributes still resolves a usable size
	svg := []byte(`<svg viewBox="0 0 40 20" xmlns="http://www.w3.org/2000/svg">
		<rect width="40" height="20" fill="#00ff00"/>
	</svg>`)
	data, err := worker.RenderToCanvas(svg, &CanvasOptions{Width: 80, Height: 80, Mode: FitContain})
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, data)
	assertDims(t, img, 80, 80)
	assertColor(t, img, 40, 40, color.NRGBA{0, 255, 0, 255})
}

func TestPixmapFill(t *testing.T) {
	worker := newTestWorker(t)
	pixmap, err := worker.NewPixmap(4, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer pixmap.Close()
	if err := pixmap.Fill(color.NRGBA{1, 2, 3, 200}); err != nil {
		t.Fatal(err)
	}
	data, err := pixmap.EncodePNG()
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, data)
	assertColor(t, img, 2, 2, color.NRGBA{1, 2, 3, 200})
	// nil color clears back to transparent
	if err := pixmap.Fill(nil); err != nil {
		t.Fatal(err)
	}
	data, err = pixmap.EncodePNG()
	if err != nil {
		t.Fatal(err)
	}
	img = decodePNG(t, data)
	assertColor(t, img, 2, 2, color.NRGBA{0, 0, 0, 0})
}
