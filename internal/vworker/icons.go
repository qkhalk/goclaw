package vworker

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/draw"
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
// the layer's style; icons with Chip render on a tinted tile, cards with
// Border get a contrast ring.
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
			if l.Chip {
				pngBytes, err = renderChipPNG(l.Icon, fill, size)
			} else {
				pngBytes, err = renderIconPNG(l.Icon, fill, size)
			}
		case contract.LayerCard:
			radFrac := l.Radius
			if radFrac <= 0 {
				radFrac = 0.018
			}
			pngBytes, err = renderCardPNG(bw, bh, int(math.Round(radFrac*float64(canvasW))), fill, opacity, l.Border)
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
	rgba, err := renderIconRGBA(icon, hex, size)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, rgba); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// renderIconRGBA is renderIconPNG without the encode step, so callers can
// composite the icon onto tiles.
func renderIconRGBA(icon, hex string, size int) (*image.RGBA, error) {
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
	return rgba, nil
}

// renderChipPNG composes a feature-tile: a rounded square in the accent
// color at low opacity with the icon centered on top — the icon-chip look
// used across modern landing pages.
func renderChipPNG(icon, hex string, size int) ([]byte, error) {
	rgba, err := renderChipRGBA(icon, hex, size)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, rgba); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func renderChipRGBA(icon, hex string, size int) (*image.RGBA, error) {
	tile, err := renderCardRGBA(size, size, int(math.Round(float64(size)*0.24)), hex, 0.16, false)
	if err != nil {
		return nil, err
	}
	inner := int(math.Round(float64(size) * 0.58))
	glyph, err := renderIconRGBA(icon, hex, inner)
	if err != nil {
		return nil, err
	}
	off := (size - inner) / 2
	draw.Draw(tile, image.Rect(off, off, off+inner, off+inner), glyph, image.Point{}, draw.Over)
	return tile, nil
}

// renderCardPNG draws a rounded rectangle with baked-in alpha (the card
// fill's coverage × opacity) and returns PNG bytes. Coverage comes from the
// rounded-rect signed distance field, giving exact anti-aliasing without a
// supersample pass. With border, a ~2px ring in the same hue at boosted
// opacity outlines the shape.
func renderCardPNG(w, h, r int, hex string, opacity float64, border bool) ([]byte, error) {
	rgba, err := renderCardRGBA(w, h, r, hex, opacity, border)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, rgba); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func renderCardRGBA(w, h, r int, hex string, opacity float64, border bool) (*image.RGBA, error) {
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
	ringAlpha := math.Min(1, opacity+0.4)
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
			alpha := cov * opacity
			if border {
				// ~2px ring hugging the shape edge, AA'd by the distance
				// falloff on both sides of the band.
				ring := math.Min(math.Max(1-math.Abs(d+1), 0), 1)
				if ring > 0 {
					alpha = math.Max(alpha, ring*ringAlpha)
				}
			}
			off := rgba.PixOffset(x, y)
			// image.RGBA stores premultiplied color — scale the channels by
			// the same coverage the alpha gets.
			rgba.Pix[off] = uint8(math.Round(float64(col.r) * alpha))
			rgba.Pix[off+1] = uint8(math.Round(float64(col.g) * alpha))
			rgba.Pix[off+2] = uint8(math.Round(float64(col.b) * alpha))
			rgba.Pix[off+3] = uint8(math.Round(alpha * 255))
		}
	}
	return rgba, nil
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
