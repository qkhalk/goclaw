package vworker

import (
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nextlevelbuilder/goclaw/internal/vworker/contract"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// The worker bundles its own display/body/mono fonts (SIL OFL 1.1, see
// fonts/LICENSE-*.txt) so captions stop depending on whatever the host
// installed — and stop looking like default DejaVu.
//
//go:embed fonts/*.ttf fonts/LICENSE-*.txt
var fontFS embed.FS

// FontSet resolves the extracted bundled font paths.
type FontSet struct {
	Display  string // Be Vietnam Pro Bold — captions (crisp Vietnamese diacritics)
	Body     string // Inter Regular — text layers
	BodyBold string // Inter Bold
	Mono     string // JetBrains Mono Medium — "mono" eyebrow captions
}

// ExtractFonts materializes the embedded TTFs into dir/fonts (idempotent) and
// returns their paths. Called once at worker startup; every render then uses
// plain local paths.
func ExtractFonts(dir string) (FontSet, error) {
	fontsDir := filepath.Join(dir, "fonts")
	if err := os.MkdirAll(fontsDir, 0o755); err != nil {
		return FontSet{}, fmt.Errorf("create fonts dir: %w", err)
	}
	files := []string{
		"BeVietnamPro-Bold.ttf",
		"Inter-Regular.ttf",
		"Inter-Bold.ttf",
		"JetBrainsMono-Medium.ttf",
	}
	for _, name := range files {
		out := filepath.Join(fontsDir, name)
		if _, err := os.Stat(out); err == nil {
			continue
		}
		data, err := fontFS.ReadFile("fonts/" + name)
		if err != nil {
			return FontSet{}, fmt.Errorf("read embedded font %s: %w", name, err)
		}
		if err := os.WriteFile(out, data, 0o644); err != nil {
			return FontSet{}, fmt.Errorf("write font %s: %w", name, err)
		}
	}
	return FontSet{
		Display:  filepath.Join(fontsDir, "BeVietnamPro-Bold.ttf"),
		Body:     filepath.Join(fontsDir, "Inter-Regular.ttf"),
		BodyBold: filepath.Join(fontsDir, "Inter-Bold.ttf"),
		Mono:     filepath.Join(fontsDir, "JetBrainsMono-Medium.ttf"),
	}, nil
}

// captionOverlay is one caption PNG pinned on the frame.
type captionOverlay struct {
	Path     string
	X, Y     int     // top-left placement on the canvas
	RevealAt float64 // seconds into the scene (0 = base strip)
	FadeSec  float64 // fade-in length
}

// captionPlan is the rendered caption of one scene: a base strip (dim in
// karaoke mode, full-color otherwise) plus one bright word overlay per word
// when narration timing is known.
type captionPlan struct {
	Overlays []captionOverlay
}

// faceFor caches parsed faces by path+size — ParseFont is expensive and one
// caption reuses the same face for every word.
var (
	faceCacheMu sync.Mutex
	faceCache   = map[string]font.Face{}
)

func faceFor(path string, size int) (font.Face, error) {
	key := fmt.Sprintf("%s:%d", path, size)
	faceCacheMu.Lock()
	if f, ok := faceCache[key]; ok {
		faceCacheMu.Unlock()
		return f, nil
	}
	faceCacheMu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read font %s: %w", path, err)
	}
	parsed, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse font %s: %w", path, err)
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("new face %s: %w", path, err)
	}
	faceCacheMu.Lock()
	faceCache[key] = face
	faceCacheMu.Unlock()
	return face, nil
}

// wordBox is one laid-out word inside the caption strip (top-left of the
// line box, exclusive of the shadow margin).
type wordBox struct {
	text string
	x, y int // top-left, relative to the strip
	w    int
}

type capLine struct {
	boxes []wordBox
	w     int
}

// renderCaption renders the caption PNGs for one scene into tempDir.
//
// narrSec > 0 switches on the karaoke reveal, mirroring the browser painter
// exactly: word k lights up at k/N of the narration while a dimmed base strip
// stays visible for the whole scene. Without narration the caption renders as
// a single strip that fades in.
//
// Styles: "" plain display text with a soft drop shadow; "chip" puts the text
// on a rounded translucent chip; "mono" uses the bundled monospace.
func renderCaption(sc contract.Scene, canvasW, canvasH int, fs FontSet, narrSec float64, tempDir string, sceneIdx int) (*captionPlan, error) {
	cp := sc.Caption
	if cp == nil || strings.TrimSpace(cp.Text) == "" || fs.Display == "" {
		return nil, nil
	}

	fontPath := fs.Display
	if cp.Style == "mono" && fs.Mono != "" {
		fontPath = fs.Mono
	}
	size := cp.FontSize
	if size <= 0 {
		size = 48
	}

	face, err := faceFor(fontPath, size)
	if err != nil {
		return nil, err
	}
	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	descent := metrics.Descent.Ceil()
	lineH := int(float64(size) * 1.32)

	// --- layout: greedy word wrap into lines within 86% of the canvas ---
	words := strings.Fields(cp.Text)
	maxW := int(float64(canvasW) * 0.86)
	spaceW := font.MeasureString(face, " ").Ceil()

	var lines []capLine
	var cur capLine
	for _, w := range words {
		ww := font.MeasureString(face, w).Ceil()
		pen := cur.w
		if len(cur.boxes) > 0 {
			pen += spaceW
		}
		if pen+ww > maxW && len(cur.boxes) > 0 {
			lines = append(lines, cur)
			cur = capLine{}
			pen = 0
		}
		cur.boxes = append(cur.boxes, wordBox{text: w, w: ww})
		cur.w = pen + ww
	}
	if len(cur.boxes) > 0 {
		lines = append(lines, cur)
	}

	// Strip bounds: widest line + padding (chips breathe wider).
	padX, padY := size/3+8, size/4+6
	if cp.Style == "chip" {
		padX, padY = size*2/3, size/2
	}
	textW := 0
	for _, ln := range lines {
		if ln.w > textW {
			textW = ln.w
		}
	}
	shadow := size/22 + 1
	stripW := textW + padX*2 + shadow
	stripH := len(lines)*lineH + padY*2 + shadow
	lineBoxH := ascent + descent

	// Word positions inside the strip (lines centered horizontally).
	y := padY
	for li := range lines {
		x := padX + (textW-lines[li].w)/2
		for wi := range lines[li].boxes {
			lines[li].boxes[wi].x = x
			lines[li].boxes[wi].y = y
			x += lines[li].boxes[wi].w + spaceW
		}
		y += lineH
	}

	// Placement on the canvas per caption position.
	cx := (canvasW - stripW) / 2
	var cy int
	switch cp.Position {
	case "top":
		cy = 60
	case "center":
		cy = (canvasH - stripH) / 2
	default: // bottom
		cy = canvasH - stripH - 60
	}

	plan := &captionPlan{}
	karaoke := narrSec > 0 && len(words) > 0

	// --- base strip ---
	base := image.NewRGBA(image.Rect(0, 0, stripW, stripH))
	if cp.Style == "chip" {
		fillRoundRect(base, image.Rect(0, 0, stripW, stripH), size*7/10,
			color.RGBA{R: 8, G: 12, B: 22, A: 148})
	}
	textAlpha := uint8(255)
	if karaoke {
		textAlpha = 96 // dim until the voice lights each word up
	}
	for _, ln := range lines {
		for _, wb := range ln.boxes {
			drawWord(base, face, wb.text, wb.x, wb.y, (lineH-lineBoxH)/2+ascent, textAlpha, shadow)
		}
	}
	basePath := filepath.Join(tempDir, fmt.Sprintf("cap_%03d_base.png", sceneIdx))
	if err := writePNG(basePath, base); err != nil {
		return nil, err
	}
	plan.Overlays = append(plan.Overlays, captionOverlay{Path: basePath, X: cx, Y: cy, FadeSec: 0.25})

	// --- karaoke bright words (one small PNG per word) ---
	if karaoke {
		for wi := range words {
			var box wordBox
			k := 0
			for li := range lines {
				for bi := range lines[li].boxes {
					if k == wi {
						box = lines[li].boxes[bi]
					}
					k++
				}
			}
			img := image.NewRGBA(image.Rect(0, 0, box.w+shadow+2, lineBoxH+shadow+2))
			drawWord(img, face, box.text, 1, 1, (lineH-lineBoxH)/2+ascent, 255, shadow)
			wordPath := filepath.Join(tempDir, fmt.Sprintf("cap_%03d_w%02d.png", sceneIdx, wi))
			if err := writePNG(wordPath, img); err != nil {
				return nil, err
			}
			plan.Overlays = append(plan.Overlays, captionOverlay{
				Path:     wordPath,
				X:        cx + box.x - 1,
				Y:        cy + box.y - 1,
				RevealAt: float64(wi) / float64(len(words)) * narrSec,
				FadeSec:  0.12,
			})
		}
	}
	return plan, nil
}

// drawWord renders one word at (x, topOfLineBox + yOff) with a soft shadow.
// The shadow is an offset dark pass under the fill pass — cheap and reads
// well over both photos and gradients.
func drawWord(img *image.RGBA, face font.Face, text string, x, y, yOff int, alpha uint8, shadow int) {
	baseline := y + yOff
	if shadow > 0 {
		d := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(color.RGBA{R: 0, G: 0, B: 0, A: alpha / 2}),
			Face: face,
			Dot:  fixed.P(x+shadow, baseline+shadow),
		}
		d.DrawString(text)
	}
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.RGBA{R: 255, G: 255, B: 255, A: alpha}),
		Face: face,
		Dot:  fixed.P(x, baseline),
	}
	d.DrawString(text)
}

// fillRoundRect fills a rounded rectangle by per-pixel SDF test. Strip-sized
// canvases only (~1000x300) so the loop is cheap.
func fillRoundRect(img *image.RGBA, r image.Rectangle, radius int, c color.RGBA) {
	x0, y0, x1, y1 := r.Min.X, r.Min.Y, r.Max.X, r.Max.Y
	if radius > (x1-x0)/2 {
		radius = (x1 - x0) / 2
	}
	if radius > (y1-y0)/2 {
		radius = (y1 - y0) / 2
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			cx := max(x, x0+radius)
			if cx > x1-1-radius {
				cx = x1 - 1 - radius
			}
			cyy := max(y, y0+radius)
			if cyy > y1-1-radius {
				cyy = y1 - 1 - radius
			}
			dx, dy := x-cx, y-cyy
			if dx*dx+dy*dy <= radius*radius {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

// renderGlowPNG renders a radial glow orb tinted with hexColor ("#RRGGBB"),
// alpha falling off quadratically from the core. The ffmpeg overlay
// expressions drift it behind the scene content.
func renderGlowPNG(tempDir, hexColor string, sceneIdx int) (string, error) {
	const size = 512
	r, g, b, err := parseHexColor(hexColor)
	if err != nil {
		return "", err
	}
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cx, cy := float64(size/2), float64(size/2)
	radius := float64(size / 2)
	const peak = 44 // ~0.17 alpha at the core
	for y := range size {
		for x := range size {
			d := math.Hypot(float64(x)-cx, float64(y)-cy) / radius
			if d >= 1 {
				continue
			}
			a := uint8(float64(peak) * (1 - d) * (1 - d))
			if a == 0 {
				continue
			}
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}
	p := filepath.Join(tempDir, fmt.Sprintf("glow_%03d.png", sceneIdx))
	if err := writePNG(p, img); err != nil {
		return "", err
	}
	return p, nil
}

func parseHexColor(s string) (r, g, b uint8, err error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return 0, 0, 0, fmt.Errorf("bad color %q", s)
	}
	var v [3]uint8
	for i := range 3 {
		var b byte
		if _, err := fmt.Sscanf(s[i*2:i*2+2], "%02x", &b); err != nil {
			return 0, 0, 0, fmt.Errorf("bad color %q: %w", s, err)
		}
		v[i] = b
	}
	return v[0], v[1], v[2], nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return nil
}
