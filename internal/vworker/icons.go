package vworker

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/vworker/contract"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// Feather-style stroke icons (MIT), one file per contract.ValidIcons key.
//
//go:embed icons/*.svg
var iconFS embed.FS

// prepareLayerAssets materializes icon and card layers as PNG files in
// tempDir and points their Source at those files, letting the render path
// treat them like any other PNG layer input. Must run before
// appendImageLayerInputs. Icon stroke color and card fill/opacity come from
// the layer's style.
func prepareLayerAssets(sc *contract.Scene, canvasW, canvasH int, tempDir string, sceneIdx int) error {
	for j := range sc.Layers {
		l := &sc.Layers[j]
		switch l.Kind {
		case contract.LayerIcon, contract.LayerCard:
		default:
			continue
		}
		_, _, w, h := l.EffectiveBox()
		bw := int(math.Round(w * float64(canvasW)))
		bh := int(math.Round(h * float64(canvasH)))
		fill, opacity, _, _ := l.EffectiveStyle()
		var (
			pngBytes []byte
			err      error
		)
		switch l.Kind {
		case contract.LayerIcon:
			size := min(bw, bh)
			pngBytes, err = renderIconPNG(l.Icon, fill, size)
		case contract.LayerCard:
			radFrac := l.Radius
			if radFrac <= 0 {
				radFrac = 0.018
			}
			pngBytes, err = renderCardPNG(bw, bh, int(math.Round(radFrac*float64(canvasW))), fill, opacity)
		}
		if err != nil {
			return fmt.Errorf("layer %d: %w", j, err)
		}
		p := filepath.Join(tempDir, fmt.Sprintf("layer_%s_%d_%d.png", l.Kind, sceneIdx, j))
		if err := os.WriteFile(p, pngBytes, 0o644); err != nil {
			return fmt.Errorf("layer %d: write png: %w", j, err)
		}
		l.Source = p
	}
	return nil
}

// renderIconPNG rasterizes an embedded icon at size×size pixels with the
// given #RRGGBB stroke color. Returns PNG bytes with a transparent
// background.
func renderIconPNG(icon, hex string, size int) ([]byte, error) {
	if size < 8 || size > 2048 {
		return nil, fmt.Errorf("icon size %d out of range 8..2048", size)
	}
	body, err := iconFS.ReadFile("icons/" + icon + ".svg")
	if err != nil {
		return nil, fmt.Errorf("unknown icon %q", icon)
	}
	// Feather ships stroke="currentColor" on the root <svg> and relies on
	// attribute inheritance — oksvg resolves neither. Extract the shapes and
	// re-issue them inside a <g> carrying explicit stroke styling.
	svg := string(body)
	open := strings.Index(svg, ">")
	close_ := strings.LastIndex(svg, "<")
	if open < 0 || close_ <= open {
		return nil, fmt.Errorf("icon %q: malformed svg", icon)
	}
	inner := svg[open+1 : close_]
	normalized := fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="%d" height="%d">`+
			`<g fill="none" stroke="#%s" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">%s</g></svg>`,
		size, size, strings.ToUpper(strings.TrimPrefix(hex, "#")), inner)

	parsed, err := oksvg.ReadIconStream(strings.NewReader(normalized))
	if err != nil {
		return nil, fmt.Errorf("icon %q: %w", icon, err)
	}
	parsed.SetTarget(0, 0, float64(size), float64(size))
	rgba := image.NewRGBA(image.Rect(0, 0, size, size))
	scanner := rasterx.NewScannerGV(size, size, rgba, rgba.Bounds())
	raster := rasterx.NewDasher(size, size, scanner)
	// SvgIcon.Draw applies the SetTarget transform — drawing the paths
	// directly would rasterize at the raw 24-unit size.
	parsed.Draw(raster, 1.0)
	var out bytes.Buffer
	if err := png.Encode(&out, rgba); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// renderCardPNG draws a rounded rectangle with baked-in alpha (the card
// fill's coverage × opacity) and returns PNG bytes. Coverage comes from the
// rounded-rect signed distance field, giving exact anti-aliasing without a
// supersample pass.
func renderCardPNG(w, h, r int, hex string, opacity float64) ([]byte, error) {
	if w < 4 || h < 4 || w > 4096 || h > 4096 {
		return nil, fmt.Errorf("card size %dx%d out of range 4..4096", w, h)
	}
	if r < 0 {
		r = 0
	}
	if maxR := min(w, h) / 2; r > maxR {
		r = maxR
	}
	col, err := hexRGB(hex)
	if err != nil {
		return nil, err
	}
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	halfW := float64(w) / 2
	halfH := float64(h) / 2
	rad := float64(r)
	for y := range h {
		dy := math.Abs(float64(y)+0.5-halfH) - (halfH - rad)
		for x := range w {
			dx := math.Abs(float64(x)+0.5-halfW) - (halfW - rad)
			ox := math.Max(dx, 0)
			oy := math.Max(dy, 0)
			d := math.Hypot(ox, oy) + math.Min(math.Max(dx, dy), 0) - rad
			cov := math.Min(math.Max(0.5-d, 0), 1)
			if cov <= 0 {
				continue
			}
			off := rgba.PixOffset(x, y)
			rgba.Pix[off] = col.r
			rgba.Pix[off+1] = col.g
			rgba.Pix[off+2] = col.b
			rgba.Pix[off+3] = uint8(math.Round(cov * opacity * 255))
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, rgba); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

type rgb struct{ r, g, b uint8 }

// hexRGB parses #RRGGBB (with or without the leading #).
func hexRGB(hex string) (rgb, error) {
	s := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(s) != 6 {
		return rgb{}, fmt.Errorf("fill %q is not #RRGGBB", hex)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return rgb{}, fmt.Errorf("fill %q is not #RRGGBB", hex)
	}
	return rgb{uint8(v >> 16), uint8(v >> 8), uint8(v)}, nil
}
