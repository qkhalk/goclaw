package vworker

// Multi-form motion-layer primitives (style packs + counter / toggle_grid /
// compare_bars / stack / stamp / cta / highlighted text).
//
// Two rendering strategies:
//   - filter-graph primitives (counter, toggle_grid, compare_bars, stack)
//     chain plain drawtext/drawbox filters with enable windows — they stay
//     inline in both the -vf and filter_complex paths;
//   - generated-PNG overlays (stamp, cta, highlighted text) rasterize once
//     per scene in prepareMotionAssets and ride the existing PNG-layer input
//     machinery (12fps loop inputs, anim chain, enable-windowed overlay).
//
// Every timing/color formula here has a hand-mirrored twin in the browser
// painter (ui/web/src/pages/tools/video/components/render-shared.ts) — keep
// the two in lockstep or the preview drifts from the burn-in.

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/nextlevelbuilder/goclaw/internal/vworker/contract"
)

// ── Style packs ──

// stylePackSpec is one scene-level look preset. A pack expands to the scene's
// color/color2/grid/glow (color scenes only for the backdrop fields) and to
// the default text color / accent color of layers that don't set fill.
type stylePackSpec struct {
	color, color2 string
	grid          bool
	glow          string
	text, accent  string
}

// stylePacks is the canonical pack table — mirrored by STYLE_PACKS in
// render-shared.ts.
var stylePacks = map[string]stylePackSpec{
	"tech_dark": {color: "#0D1117", color2: "#1E293B", grid: true, glow: "#38BDF8", text: "#F8FAFC", accent: "#38BDF8"},
	"neon_lab":  {color: "#07070B", color2: "#1E1B4B", grid: false, glow: "#22D3EE", text: "#F4F4F5", accent: "#22D3EE"},
	// paper_light has no glow — radial orbs wash out on light backdrops.
	"paper_light": {color: "#F8FAFC", color2: "#E2E8F0", grid: true, glow: "", text: "#0F172A", accent: "#2563EB"},
	"bold_red":    {color: "#140404", color2: "#7F1D1D", grid: false, glow: "#EF4444", text: "#FFFFFF", accent: "#F87171"},
}

// resolveStylePack expands the scene's style_pack into the per-field values
// the scene left unset. Individual scene fields always win over the pack; the
// grid flag merges as "pack default unless the scene turned it on" (a pack
// with grid can only be opted out of by dropping the pack — bools carry no
// explicit-off). Non-color scenes only take the pack's glow.
func resolveStylePack(sc contract.Scene) contract.Scene {
	pack, ok := stylePacks[sc.StylePack]
	if !ok {
		return sc
	}
	if sc.Type == contract.SceneColor {
		if sc.Color == "" {
			sc.Color = pack.color
		}
		if sc.Color2 == "" {
			sc.Color2 = pack.color2
		}
		if !sc.Grid {
			sc.Grid = pack.grid
		}
	}
	if sc.Glow == "" {
		sc.Glow = pack.glow
	}
	return sc
}

// sceneTextColor resolves the pack's default text color ("" = plain white).
func sceneTextColor(sc contract.Scene) string {
	if pack, ok := stylePacks[sc.StylePack]; ok {
		return pack.text
	}
	return ""
}

// sceneAccent resolves the pack's accent color ("" = none).
func sceneAccent(sc contract.Scene) string {
	if pack, ok := stylePacks[sc.StylePack]; ok {
		return pack.accent
	}
	return ""
}

// packFillOr resolves a layer's primary color: explicit fill, then the pack
// accent, then fallback.
func packFillOr(sc contract.Scene, l contract.Layer, fallback string) string {
	if l.Fill != "" {
		return l.Fill
	}
	if c := sceneAccent(sc); c != "" {
		return c
	}
	return fallback
}

// ── Generated-PNG overlay kinds (stamp / cta / highlighted text) ──

// prepareMotionAssets materializes the PNG-overlay motion layers (stamp, cta,
// highlighted text) into tempDir and points their Source at the files, letting
// the render path treat them like any other PNG layer input. Must run before
// appendImageLayerInputs. A failure downgrades that layer only: the Source
// stays empty and the compositor skips it (isPNGLayer requires a Source), so
// a font hiccup can never kill a whole render.
func prepareMotionAssets(sc *contract.Scene, canvasW, canvasH int, tempDir string, sceneIdx int, fonts FontSet, fallbackFont string) {
	for j := range sc.Layers {
		l := &sc.Layers[j]
		highlighted := l.Kind == contract.LayerText && len(l.Highlights) > 0
		if !highlighted && l.Kind != contract.LayerStamp && l.Kind != contract.LayerCTA {
			continue
		}
		_, _, fontSize, _ := l.EffectiveStyle()
		_, _, w, h := l.EffectiveBox()
		bw := int(math.Round(w * float64(canvasW)))
		bh := int(math.Round(h * float64(canvasH)))
		// Text layers size themselves (no h default) — only the width matters
		// for the highlighted-text PNG.
		if bw < 8 || bw > 4096 {
			continue
		}
		if l.Kind != contract.LayerText && (bh < 8 || bh > 4096) {
			continue
		}
		// Stamps and pills read best in the bold face; highlighted text keeps
		// the layer's own font selection over the regular body face.
		boldFace := fonts.BodyBold
		if boldFace == "" {
			boldFace = fallbackFont
		}
		textFace := fallbackFont
		if textFace == "" {
			textFace = boldFace
		}
		faceFallback := textFace
		if l.Kind == contract.LayerStamp || l.Kind == contract.LayerCTA {
			faceFallback = boldFace
		}
		facePath := layerFontFor(*l, fonts, faceFallback)
		if facePath == "" {
			continue
		}
		var (
			pngBytes []byte
			err      error
		)
		switch {
		case l.Kind == contract.LayerStamp:
			pngBytes, err = renderStampPNG(*l, facePath, bw, bh)
		case l.Kind == contract.LayerCTA:
			pngBytes, err = renderCTAPNG(*l, facePath, bw, bh)
		default:
			pngBytes, err = renderHighlightTextPNG(*l, facePath, bw, fontSize)
		}
		if err != nil {
			continue // skip this layer, keep the render alive
		}
		p := filepath.Join(tempDir, fmt.Sprintf("motion_%s_%d_%d.png", l.Kind, sceneIdx, j))
		if err := os.WriteFile(p, pngBytes, 0o644); err != nil {
			continue
		}
		l.Source = p
	}
}

// stampFontSize resolves the stamp's face and a shrink-to-fit font size
// (stamps never overflow the box width).
func stampFaceSize(facePath, text string, bw, bh, fontSize int) (font.Face, int, error) {
	if fontSize <= 0 {
		fontSize = int(float64(bh) * 0.38)
	}
	face, err := faceFor(facePath, fontSize)
	if err != nil {
		return nil, 0, err
	}
	maxTextW := bw * 62 / 100
	for font.MeasureString(face, text).Ceil() > maxTextW && fontSize > 10 {
		fontSize = fontSize * 9 / 10
		if face, err = faceFor(facePath, fontSize); err != nil {
			return nil, 0, err
		}
	}
	return face, fontSize, nil
}

// renderStampPNG draws a bordered stamp (border ring + bold text) and rotates
// it by the layer's angle. The stamp is rasterized axis-aligned at 2× then
// nearest-sampled into the destination at 1× — the supersample keeps rotated
// edges from stair-stepping without a full affine resampler.
func renderStampPNG(l contract.Layer, facePath string, bw, bh int) ([]byte, error) {
	angle := l.Angle
	if angle == 0 {
		angle = -8
	}
	text := strings.TrimSpace(l.Text)
	if text == "" {
		return nil, fmt.Errorf("stamp layer needs text")
	}
	col, err := hexRGB(l.Fill)
	if err != nil {
		col = rgb{r: 255, g: 255, b: 255} // explicit fill is the norm; default white
	}
	face, fontSize, err := stampFaceSize(facePath, text, bw, bh, l.FontSize)
	if err != nil {
		return nil, err
	}
	textW := font.MeasureString(face, text).Ceil()
	pad := bh * 22 / 100
	sw := textW + pad*2
	sh := bh

	// Axis-aligned stamp at 2× supersample: filled border block punched out
	// in the middle, bold text centered.
	const ss = 2
	img := image.NewRGBA(image.Rect(0, 0, sw*ss, sh*ss))
	border := max(3, sh*ss*7/100)
	block := func(x0, y0, x1, y1 int, c color.RGBA) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.SetRGBA(x, y, c)
			}
		}
	}
	block(0, 0, sw*ss, sh*ss, color.RGBA{R: col.r, G: col.g, B: col.b, A: 255})
	block(border, border, sw*ss-border, sh*ss-border, color.RGBA{})
	stampFace, err := faceFor(facePath, fontSize*ss)
	if err != nil {
		return nil, err
	}
	metrics := stampFace.Metrics()
	ascent := metrics.Ascent.Ceil()
	baseline := (sh*ss+ascent-metrics.Descent.Ceil())/2 - 1
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.RGBA{R: col.r, G: col.g, B: col.b, A: 255}),
		Face: stampFace,
		Dot:  fixed.P((sw*ss-textW*ss)/2, baseline),
	}
	d.DrawString(text)

	// Rotate about the centers into the destination box, scaled to fit.
	out := image.NewRGBA(image.Rect(0, 0, bw, bh))
	rad := angle * math.Pi / 180
	sin, cos := math.Sin(rad), math.Cos(rad)
	needW := float64(sw)*math.Abs(cos) + float64(sh)*math.Abs(sin)
	needH := float64(sw)*math.Abs(sin) + float64(sh)*math.Abs(cos)
	fit := math.Min(1, math.Min(float64(bw)/needW, float64(bh)/needH))
	for y := 0; y < bh; y++ {
		for x := 0; x < bw; x++ {
			px := (float64(x) - float64(bw)/2) / fit
			py := (float64(y) - float64(bh)/2) / fit
			// Inverse rotation (−θ): q = R(−θ)·p.
			sx := px*cos + py*sin
			sy := -px*sin + py*cos
			ux := int(math.Floor((sx + float64(sw)/2) * ss))
			uy := int(math.Floor((sy + float64(sh)/2) * ss))
			if ux < 0 || uy < 0 || ux >= sw*ss || uy >= sh*ss {
				continue
			}
			off := img.PixOffset(ux, uy)
			o := out.PixOffset(x, y)
			copy(out.Pix[o:o+4], img.Pix[off:off+4])
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// renderCTAPNG draws a horizontal-gradient pill with centered display text —
// the call-to-action bar at the bottom of the frame.
func renderCTAPNG(l contract.Layer, facePath string, bw, bh int) ([]byte, error) {
	text := strings.TrimSpace(l.Text)
	if text == "" {
		return nil, fmt.Errorf("cta layer needs text")
	}
	c0, err := hexRGB(l.Fill)
	if err != nil {
		c0 = rgb{r: 0x38, g: 0xBD, b: 0xF8}
	}
	c1, err := hexRGB(l.FillB)
	if err != nil {
		c1 = rgb{r: 0x8B, g: 0x5C, b: 0xF6}
	}
	fontSize := l.FontSize
	if fontSize <= 0 {
		fontSize = bh * 44 / 100
	}
	face, err := faceFor(facePath, fontSize)
	if err != nil {
		return nil, err
	}
	for font.MeasureString(face, text).Ceil() > bw*78/100 && fontSize > 10 {
		fontSize = fontSize * 93 / 100
		if face, err = faceFor(facePath, fontSize); err != nil {
			return nil, err
		}
	}
	textW := font.MeasureString(face, text).Ceil()

	img := image.NewRGBA(image.Rect(0, 0, bw, bh))
	for y := 0; y < bh; y++ {
		for x := 0; x < bw; x++ {
			cov := pillCoverage(x, y, bw, bh)
			if cov <= 0 {
				continue
			}
			// Horizontal gradient c0 → c1 across the pill.
			t := float64(x) / float64(max(bw-1, 1))
			r := uint8(math.Round(float64(c0.r) + (float64(c1.r)-float64(c0.r))*t))
			g := uint8(math.Round(float64(c0.g) + (float64(c1.g)-float64(c0.g))*t))
			b := uint8(math.Round(float64(c0.b) + (float64(c1.b)-float64(c0.b))*t))
			a := uint8(math.Round(cov * 255))
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(uint16(r) * uint16(a) / 255),
				G: uint8(uint16(g) * uint16(a) / 255),
				B: uint8(uint16(b) * uint16(a) / 255),
				A: a,
			})
		}
	}
	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	baseline := (bh+ascent-metrics.Descent.Ceil())/2 - 1
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.RGBA{R: 255, G: 255, B: 255, A: 235}),
		Face: face,
		Dot:  fixed.P((bw-textW)/2, baseline),
	}
	d.DrawString(text)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// pillCoverage is the anti-aliased coverage of a full pill (radius h/2) at a
// pixel center — same SDF approach as renderCardRGBA.
func pillCoverage(x, y, bw, bh int) float64 {
	rad := float64(bh) / 2
	halfW, halfH := float64(bw)/2, float64(bh)/2
	dx := math.Abs(float64(x)+0.5-halfW) - (halfW - rad)
	dy := math.Abs(float64(y)+0.5-halfH) - (halfH - rad)
	ox, oy := math.Max(dx, 0), math.Max(dy, 0)
	dist := math.Hypot(ox, oy) + math.Min(math.Max(dx, dy), 0) - rad
	return math.Min(math.Max(0.5-dist, 0), 1)
}

// renderHighlightTextPNG lays out a highlighted text layer word-by-word with
// the bundled faces — the same greedy wrap and pen-advance math as the
// browser painter — and bakes each highlighted word in its own color. This
// sidesteps drawtext's single-color limitation while keeping layout parity.
func renderHighlightTextPNG(l contract.Layer, facePath string, bw, fontSize int) ([]byte, error) {
	if fontSize <= 0 {
		fontSize = 48
	}
	base, err := hexRGB(l.Fill)
	if err != nil {
		base = rgb{r: 255, g: 255, b: 255}
	}
	hlColor := map[string]rgb{}
	for _, hl := range l.Highlights {
		if c, err := hexRGB(hl.Color); err == nil {
			hlColor[hl.Word] = c
		}
	}
	face, err := faceFor(facePath, fontSize)
	if err != nil {
		return nil, err
	}
	spaceW := font.MeasureString(face, " ").Ceil()
	lineH := fontSize * 13 / 10
	shadow := max(2, fontSize/22)

	// Greedy wrap (same as the client painter and the caption layout).
	words := strings.Fields(l.Text)
	type hword struct {
		text string
		w    int
		col  rgb
	}
	var lines [][]hword
	var cur []hword
	curW := 0
	for _, w := range words {
		ww := font.MeasureString(face, w).Ceil()
		pen := curW
		if len(cur) > 0 {
			pen += spaceW
		}
		if pen+ww > bw && len(cur) > 0 {
			lines = append(lines, cur)
			cur = nil
			pen = 0
		}
		col, hl := hlColor[w]
		if !hl {
			col = base
		}
		cur = append(cur, hword{text: w, w: ww, col: col})
		curW = pen + ww
	}
	if len(cur) > 0 {
		lines = append(lines, cur)
	}

	imgW := bw + shadow + 2
	imgH := len(lines)*lineH + shadow + 2
	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))
	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	descent := metrics.Descent.Ceil()
	lineBoxH := ascent + descent
	for li, ln := range lines {
		lineW := 0
		for k, w := range ln {
			if k > 0 {
				lineW += spaceW
			}
			lineW += w.w
		}
		// Align within the box (matches layerXExpr / the canvas painter).
		lineX := 1
		switch l.Align {
		case "left":
		case "right":
			lineX = imgW - shadow - 1 - lineW
		default:
			lineX = (imgW-lineW)/2 + 1
		}
		baseline := li*lineH + (lineH-lineBoxH)/2 + ascent
		pen := lineX
		for _, w := range ln {
			// Soft offset shadow under the fill (client: rgba(0,0,0,.5) blur).
			sh := &font.Drawer{
				Dst:  img,
				Src:  image.NewUniform(color.RGBA{A: 96}),
				Face: face,
				Dot:  fixed.P(pen+shadow, baseline+shadow),
			}
			sh.DrawString(w.text)
			d := &font.Drawer{
				Dst:  img,
				Src:  image.NewUniform(color.RGBA{R: w.col.r, G: w.col.g, B: w.col.b, A: 255}),
				Face: face,
				Dot:  fixed.P(pen, baseline),
			}
			d.DrawString(w.text)
			pen += w.w + spaceW
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ── Filter-graph primitives ──

// escapeExpansionText percent-doubles literal percents so the drawtext
// expansion engine renders them literally (colons and quotes are handled by
// the callers' existing escaping).
func escapeExpansionText(s string) string {
	return strings.ReplaceAll(s, "%", "%%")
}

// counterFilter renders a count-up number via drawtext's %{eif:...} time
// expansion: value = from + (to-from)·clamp((t-start)/window). Linear — the
// browser painter uses the same formula.
func counterFilter(sc contract.Scene, l contract.Layer, canvasW, canvasH int, tempDir string, sceneIdx, layerIdx int, fonts FontSet, fontFile string) (string, error) {
	// Unlike plain text layers, a counter renders even without a configured
	// font — ffmpeg falls back to its fontconfig default. Dropping the layer
	// would silently lose the scene's headline number; a default-font render
	// is always better than nothing.
	fontPath := layerFontFor(l, fonts, fontFile)
	_, opacity, fontSize, align := l.EffectiveStyle()
	x, y, w, _ := l.EffectiveBox()
	x0 := int(math.Round(x * float64(canvasW)))
	y0 := int(math.Round(y * float64(canvasH)))
	bw := int(math.Round(w * float64(canvasW)))
	en := layerEnableExpr(l, sc.DurationSec)
	win := l.EffectiveDuration(sc.DurationSec)
	hex := strings.ToUpper(strings.TrimPrefix(l.Fill, "#"))
	if hex == "" {
		hex = strings.ToUpper(strings.TrimPrefix(sceneAccent(sc), "#"))
	}
	if hex == "" {
		hex = "38BDF8"
	}
	expr := fmt.Sprintf("%.4f+(%.4f-%.4f)*min(1,max(0,(t-%.3f)/%.3f))",
		l.From, l.To, l.From, l.Start, win)
	value := counterValueExpansion(expr, l.Decimals)
	var tv string
	if tempDir == "" {
		// Inline text: escapeDrawText performs the single option-level
		// escape pass (colons inside the %{...} block included) — pre-escaping
		// here would double-escape backslashes and break the expansion.
		text := escapeExpansionText(l.Text) + value + escapeExpansionText(l.Suffix)
		tv = "text='" + escapeDrawText(text) + "'"
	} else {
		// File mode: colons inside the expansion block need no escaping.
		content := escapeExpansionText(l.Text) + value + escapeExpansionText(l.Suffix)
		p := filepath.Join(tempDir, fmt.Sprintf("layer_%03d_%02d_counter.txt", sceneIdx, layerIdx))
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return "", fmt.Errorf("write counter text file: %w", err)
		}
		quoted := strings.ReplaceAll(p, `'`, `'\''`)
		tv = "textfile='" + quoted + "'"
	}
	head := "drawtext="
	if ff := fontFileArg(fontPath); ff != "" {
		head += ff + ":"
	}
	return fmt.Sprintf("%s%s:fontsize=%d:fontcolor=0x%s@%.2f:borderw=2:bordercolor=black@0.5:shadowcolor=black@0.35:shadowx=0:shadowy=2:x='%s':y=%d:alpha='min(1,(t-%.2f)/0.30)':%s",
		head, tv, fontSize, hex, opacity, layerXExpr(align, x0, bw), y0, l.Start, en), nil
}

// counterValueExpansion renders the animated count-up value as drawtext eif
// expansions. The eif format parameter only accepts d/u/x/X — no decimal
// count ("expr:1" fails on ffmpeg ≥6 with "Invalid format '1'"), so the value
// is scaled by 10^decimals, rounded, and split into digit groups by pure
// expressions (sub-expressions are duplicated — drawtext's expression eval
// has no usable temp-store inside %{eif}).
func counterValueExpansion(expr string, decimals int) string {
	if decimals < 0 {
		decimals = 0
	}
	if decimals > 2 {
		decimals = 2
	}
	if decimals == 0 {
		return "%{eif:round(" + expr + "):d}"
	}
	scale := 1
	for i := 0; i < decimals; i++ {
		scale *= 10
	}
	s := strconv.Itoa(scale)
	r := fmt.Sprintf("round((%s)*%d)", expr, scale)
	intPart := "%{eif:floor(" + r + "/" + s + "):d}"
	frac := r + "-floor(" + r + "/" + s + ")*" + s
	if decimals == 1 {
		return intPart + ".%{eif:" + frac + ":d}"
	}
	// decimals == 2: split the two fractional digits — eif has no zero-
	// padding, so "05" must be emitted as separate tens/units expansions.
	tens := "%{eif:floor((" + frac + ")/10):d}"
	units := "%{eif:" + frac + "-10*floor((" + frac + ")/10):d}"
	return intPart + "." + tens + units
}

// toggleOnExpr is the shared on/off schedule of a toggle cell — mirrored
// literally by the browser painter: on = ((c*31 + r*17 + step*7) mod 5) < 3,
// step = floor((t-start)/cadence). One arithmetic expression, evaluated per
// output frame — no enable windows needed.
func toggleOnExpr(c, r int, start, cadence float64) string {
	return fmt.Sprintf("lt(mod(%d*31+%d*17+floor((t-%.3f)/%.3f)*7,5),3)", c, r, start, cadence)
}

func toggleOn(c, r, step int) bool {
	return (c*31+r*17+step*7)%5 < 3
}

// toggleGridFilters renders a cols×rows grid of switches: per-cell drawbox
// pairs (dim off-state, bright on-state) with the arithmetic enable
// expression above. ≤4×4 cells keeps the filter chain bounded.
func toggleGridFilters(sc contract.Scene, l contract.Layer, canvasW, canvasH int) string {
	_, opacity, _, _ := l.EffectiveStyle()
	x, y, w, h := l.EffectiveBox()
	x0 := int(math.Round(x * float64(canvasW)))
	y0 := int(math.Round(y * float64(canvasH)))
	bw := int(math.Round(w * float64(canvasW)))
	bh := int(math.Round(h * float64(canvasH)))
	cols, rows := l.EffectiveCols(), l.EffectiveRows()
	cadence := l.EffectiveCadence()
	onHex := strings.ToUpper(strings.TrimPrefix(packFillOr(sc, l, "#22C55E"), "#"))
	offHex := strings.ToUpper(strings.TrimPrefix(l.FillB, "#"))
	if offHex == "" {
		offHex = "334155"
	}
	gap := int(math.Round(float64(bw) * 0.012))
	if gap < 2 {
		gap = 2
	}
	cellW := (bw - gap*(cols-1)) / cols
	cellH := (bh - gap*(rows-1)) / rows
	if cellW < 2 || cellH < 2 {
		return ""
	}
	var parts []string
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			cx := x0 + c*(cellW+gap)
			cy := y0 + r*(cellH+gap)
			state := fmt.Sprintf("mod(%d*31+%d*17+floor((t-%.3f)/%.3f)*7,5)", c, r, l.Start, cadence)
			parts = append(parts,
				fmt.Sprintf("drawbox=x=%d:y=%d:w=%d:h=%d:color=0x%s@%.2f:t=fill:enable='gte(%s,3)'",
					cx, cy, cellW, cellH, offHex, math.Min(opacity, 0.5), state),
				fmt.Sprintf("drawbox=x=%d:y=%d:w=%d:h=%d:color=0x%s@%.2f:t=fill:enable='lt(%s,3)'",
					cx, cy, cellW, cellH, onHex, opacity, state))
		}
	}
	return strings.Join(parts, ",")
}

// compareBarsFilters renders two labeled horizontal bars growing to their
// target widths over an ease-out growth window (8 discrete steps on the
// server; the browser animates the same ease continuously — identical at
// rest).
func compareBarsFilters(sc contract.Scene, l contract.Layer, canvasW, canvasH int, tempDir string, sceneIdx, layerIdx int, fonts FontSet, fontFile string) (string, error) {
	_, opacity, fontSize, _ := l.EffectiveStyle()
	x, y, w, h := l.EffectiveBox()
	x0 := int(math.Round(x * float64(canvasW)))
	y0 := int(math.Round(y * float64(canvasH)))
	bw := int(math.Round(w * float64(canvasW)))
	bh := int(math.Round(h * float64(canvasH)))
	win := l.EffectiveDuration(sc.DurationSec)
	end := l.Start + win
	grow := math.Min(0.8, win*0.6)
	const steps = 8
	fillA := strings.ToUpper(strings.TrimPrefix(packFillOr(sc, l, "#38BDF8"), "#"))
	fillB := strings.ToUpper(strings.TrimPrefix(l.FillB, "#"))
	if fillB == "" {
		fillB = "F97316"
	}
	textHex := strings.ToUpper(strings.TrimPrefix(sceneTextColor(sc), "#"))
	if textHex == "" {
		textHex = "FFFFFF"
	}
	fontPath := layerFontFor(l, fonts, fontFile)

	rowH := bh / 2
	labelH := int(float64(fontSize) * 1.25)
	barH := rowH - labelH - rowH/12
	if barH < 4 {
		barH = 4
	}
	barH = min(barH, rowH/2)

	var parts []string
	emit := func(f string) { parts = append(parts, f) }

	type bar struct {
		label string
		width float64
		fill  string
		row   int
	}
	bars := []bar{
		{l.LabelA, l.EffectiveWidthA(), fillA, 0},
		{l.LabelB, l.EffectiveWidthB(), fillB, 1},
	}
	for bi, b := range bars {
		rowY := y0 + b.row*rowH
		barY := rowY + labelH
		// Dim full-width track.
		emit(fmt.Sprintf("drawbox=x=%d:y=%d:w=%d:h=%d:color=0x94A3B8@%.2f:t=fill:%s",
			x0, barY, bw, barH, math.Min(opacity, 1)*0.25, layerEnableExpr(l, sc.DurationSec)))
		// Stepped ease-out growth.
		target := int(math.Round(float64(bw) * b.width))
		for k := 0; k < steps; k++ {
			p := float64(k+1) / steps
			ease := 1 - (1-p)*(1-p)
			wk := int(math.Round(float64(target) * ease))
			if wk <= 0 {
				continue
			}
			wFrom := l.Start + grow*float64(k)/steps
			var en string
			if k == steps-1 {
				en = fmt.Sprintf("enable='between(t,%.3f,%.3f)'", wFrom, end)
			} else {
				en = fmt.Sprintf("enable='between(t,%.3f,%.3f)'", wFrom, l.Start+grow*float64(k+1)/steps)
			}
			emit(fmt.Sprintf("drawbox=x=%d:y=%d:w=%d:h=%d:color=0x%s@%.2f:t=fill:%s",
				x0, barY, wk, barH, b.fill, opacity, en))
		}
		// Label above the bar. Like the counter, labels render even without
		// a configured worker font (ffmpeg's fontconfig default steps in).
		if b.label != "" {
			tv, err := layerLabelTextValue(tempDir, sceneIdx, layerIdx, bi, b.label)
			if err != nil {
				return "", err
			}
			head := "drawtext="
			if ff := fontFileArg(fontPath); ff != "" {
				head += ff + ":"
			}
			emit(fmt.Sprintf("%s%s:fontsize=%d:fontcolor=0x%s@%.2f:borderw=2:bordercolor=black@0.5:x=%d:y=%d:alpha='min(1,(t-%.2f)/0.30)':%s",
				head, tv, fontSize, textHex, math.Min(opacity, 1), x0, rowY, l.Start, layerEnableExpr(l, sc.DurationSec)))
		}
	}
	return strings.Join(parts, ","), nil
}

// stackTiming resolves the stack slide-in schedule (mirrored by the browser
// painter): per-slab stagger and slide windows derived from the layer
// window so entries always fit.
func stackTiming(n int, win float64) (slideSec, stagSec float64) {
	slideSec = math.Min(0.27, win*0.35)
	if n > 1 {
		stagSec = math.Min(0.18, (win-slideSec)/float64(n-1)*0.8)
	}
	return slideSec, stagSec
}

// stackFilters renders n slabs sliding in top-down with a per-slab stagger
// (3 ease-out steps per slab) and optional labels.
func stackFilters(sc contract.Scene, l contract.Layer, canvasW, canvasH int, tempDir string, sceneIdx, layerIdx int, fonts FontSet, fontFile string) (string, error) {
	_, opacity, fontSize, _ := l.EffectiveStyle()
	x, y, w, h := l.EffectiveBox()
	x0 := int(math.Round(x * float64(canvasW)))
	y0 := int(math.Round(y * float64(canvasH)))
	bw := int(math.Round(w * float64(canvasW)))
	bh := int(math.Round(h * float64(canvasH)))
	n := l.EffectiveN()
	win := l.EffectiveDuration(sc.DurationSec)
	end := l.Start + win
	slideSec, stagSec := stackTiming(n, win)
	const steps = 3
	slideMax := int(math.Round(0.05 * float64(canvasH)))
	gap := bh * 4 / 100
	slabH := (bh - gap*(n-1)) / n
	if slabH < 4 {
		slabH = 4
	}
	c0, errC0 := hexRGB(l.Fill)
	c1, errC1 := hexRGB(l.FillB)
	useGrad := errC0 == nil && errC1 == nil
	fontPath := layerFontFor(l, fonts, fontFile)
	textHex := strings.ToUpper(strings.TrimPrefix(sceneTextColor(sc), "#"))
	if textHex == "" {
		textHex = "FFFFFF"
	}

	var parts []string
	emit := func(f string) { parts = append(parts, f) }
	for k := 0; k < n; k++ {
		enterAt := l.Start + stagSec*float64(k)
		slabY := y0 + k*(slabH+gap)
		hex := "1E293B"
		switch {
		case useGrad:
			t := 0.0
			if n > 1 {
				t = float64(k) / float64(n-1)
			}
			mix := rgb{
				r: uint8(math.Round(float64(c0.r) + (float64(c1.r)-float64(c0.r))*t)),
				g: uint8(math.Round(float64(c0.g) + (float64(c1.g)-float64(c0.g))*t)),
				b: uint8(math.Round(float64(c0.b) + (float64(c1.b)-float64(c0.b))*t)),
			}
			hex = fmt.Sprintf("%02X%02X%02X", mix.r, mix.g, mix.b)
		case errC0 == nil:
			hex = fmt.Sprintf("%02X%02X%02X", c0.r, c0.g, c0.b)
		}
		for s := 0; s < steps; s++ {
			from := enterAt + slideSec*float64(s)/steps
			if from >= end {
				break
			}
			var en string
			if s == steps-1 {
				en = fmt.Sprintf("enable='between(t,%.3f,%.3f)'", from, end)
			} else {
				en = fmt.Sprintf("enable='between(t,%.3f,%.3f)'", from, enterAt+slideSec*float64(s+1)/steps)
			}
			p := float64(s+1) / steps
			ease := 1 - (1-p)*(1-p)
			off := int(math.Round(float64(slideMax) * (1 - ease)))
			emit(fmt.Sprintf("drawbox=x=%d:y=%d:w=%d:h=%d:color=0x%s@%.2f:t=fill:%s",
				x0, slabY+off, bw, slabH, hex, opacity, en))
		}
		if k < len(l.Labels) && strings.TrimSpace(l.Labels[k]) != "" {
			tv, err := layerLabelTextValue(tempDir, sceneIdx, layerIdx, 10+k, l.Labels[k])
			if err != nil {
				return "", err
			}
			labelY := slabY + max(2, (slabH-fontSize*11/10)/2)
			labelEn := fmt.Sprintf("enable='between(t,%.3f,%.3f)'", enterAt+slideSec, end)
			head := "drawtext="
			if ff := fontFileArg(fontPath); ff != "" {
				head += ff + ":"
			}
			emit(fmt.Sprintf("%s%s:fontsize=%d:fontcolor=0x%s@%.2f:borderw=2:bordercolor=black@0.5:x=%d:y=%d:%s",
				head, tv, fontSize, textHex, math.Min(opacity, 1), x0+slabH*35/100, labelY, labelEn))
		}
	}
	return strings.Join(parts, ","), nil
}

// layerLabelTextValue writes one label text file for the multi-filter layer
// primitives (distinct tags keep counters/bars/stack labels from colliding).
func layerLabelTextValue(tempDir string, sceneIdx, layerIdx, tag int, text string) (string, error) {
	if tempDir == "" {
		return "text='" + escapeDrawText(text) + "'", nil
	}
	p := filepath.Join(tempDir, fmt.Sprintf("layer_%03d_%02d_%02d.txt", sceneIdx, layerIdx, tag))
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		return "", fmt.Errorf("write layer label file: %w", err)
	}
	quoted := strings.ReplaceAll(p, `'`, `'\''`)
	return "textfile='" + quoted + "'", nil
}
