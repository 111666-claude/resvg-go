<div align="center">
  <img src=".github/hua_nobg_512.gif" alt="椛" width = "400">
  <br>

  <h1>resvg-go</h1>
  <img src="https://counter.seku.su/cmoe?name=resvgo&theme=r34" /><br>
  A SVG renderer written in Go & WASM depended on resvg without CGO<br><br>
  
</div>


## Usage

### Render by default
```go
// initialize and don't forget to close!
// worker are not goroutine-safe!
worker, _ := NewDefaultWorker(context.Background())
defer worker.Close()

// render the SVG as a PNG!
png, _ := worker.Render(svg)
```

### Render with options
```go
// initialize and don't forget to close!
// worker are not goroutine-safe!
worker, _ := NewDefaultWorker(context.Background())
defer worker.Close()

// render the SVG as a PNG!
fontdb, _ := worker.NewFontDBDefault()
defer fontdb.Close()
fontdb.LoadFontData(ttf)

pixmap, _ := worker.NewPixmap(512, 512)
defer pixmap.Close()

tree, _ := worker.NewTreeFromData(svg, &Options{})
defer tree.Close()
tree.ConvertText(fontdb)
tree.Render(TransformIdentity(), pixmap)

png, _ := pixmap.EncodePNG()
```

### Render onto a fixed-size canvas
A group of vector badges can share one exact output size without stretching:
the SVG is fitted proportionally with `FitContain` (letterbox, whole SVG
visible) or `FitCover` (fill the canvas, overflow clipped), with configurable
alignment and background color.
```go
// initialize and don't forget to close!
// worker are not goroutine-safe!
worker, _ := NewDefaultWorker(context.Background())
defer worker.Close()

// every badge is rendered as an exact 256x256 PNG
png, _ := worker.RenderToCanvas(svg, &CanvasOptions{
    Width:      256,
    Height:     256,
    Mode:       FitContain, // or FitCover
    Align:      Align{Horizontal: 0, Vertical: 0}, // -1..1, zero means center
    Background: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, // nil keeps it transparent
})

// the geometry (uniform scale + translation) is also available on its own
fit, err := FitGeometry(svgWidth, svgHeight, 256, 256, FitContain, Align{})
```


## Thanks
- [resvg](https://github.com/RazrFalcon/resvg) - an SVG rendering library written in Rust
- [wazero](https://github.com/tetratelabs/wazero) - the zero dependency WebAssembly runtime for Go developers
