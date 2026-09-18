package vworker

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/vworker/contract"
)

func colorAt(t *testing.T, img image.Image, x, y int) [4]uint8 {
	t.Helper()
	r, g, b, a := img.At(x, y).RGBA()
	return [4]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func maxDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// TestEmbeddedIconsComplete cross-checks every name in contract.ValidIcons
// against the embedded SVG set — a name accepted by validation but missing a
// body would fail only at render time.
func TestEmbeddedIconsComplete(t *testing.T) {
	for name := range contract.ValidIcons {
		t.Run(name, func(t *testing.T) {
			b, err := renderIconPNG(name, "FACC15", 96)
			if err != nil {
				t.Fatalf("render %q: %v", name, err)
			}
			img, err := png.Decode(bytes.NewReader(b))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if img.Bounds().Dx() != 96 || img.Bounds().Dy() != 96 {
				t.Fatalf("size = %dx%d, want 96x96", img.Bounds().Dx(), img.Bounds().Dy())
			}
		})
	}
}

func TestRenderIconPNG(t *testing.T) {
	t.Run("stroke colored and background transparent", func(t *testing.T) {
		img, err := png.Decode(bytes.NewReader(mustIcon(t, "check", "FF0000", 64)))
		if err != nil {
			t.Fatal(err)
		}
		corner := colorAt(t, img, 1, 1)
		if corner[3] != 0 {
			t.Fatalf("corner alpha = %d, want 0 (transparent)", corner[3])
		}
		// The stroke must paint red pixels somewhere in the canvas.
		found := false
		for y := 0; y < 64 && !found; y++ {
			for x := 0; x < 64 && !found; x++ {
				c := colorAt(t, img, x, y)
				if c[3] > 200 && c[0] > 180 && c[1] < 100 {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("no red stroke pixel found anywhere in the canvas")
		}
	})

	t.Run("unknown icon rejected", func(t *testing.T) {
		if _, err := renderIconPNG("rocket", "FFFFFF", 64); err == nil {
			t.Fatal("unknown icon should error")
		}
	})
}

func TestRenderCardPNG(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(mustCard(t, 300, 120, 16, "1E293B", 0.45)))
	if err != nil {
		t.Fatal(err)
	}
	corner := colorAt(t, img, 0, 0)
	if corner[3] != 0 {
		t.Fatalf("outside-corner alpha = %d, want 0", corner[3])
	}
	center := colorAt(t, img, 150, 60)
	if center[3] == 0 {
		t.Fatal("center alpha = 0, want the card fill")
	}
	// colorAt reads premultiplied channels — #1E293B at opacity 0.45.
	if maxDiff(int(center[0]), 14) > 2 || maxDiff(int(center[1]), 18) > 2 || maxDiff(int(center[2]), 26) > 2 {
		t.Fatalf("center rgb = %v, want premultiplied ~#1E293B", center[:3])
	}
	wantA := uint8(math.Round(0.45 * 255))
	if maxDiff(int(center[3]), int(wantA)) > 2 {
		t.Fatalf("center alpha = %d, want ~%d (opacity 0.45)", center[3], wantA)
	}
	// Just inside a rounded corner must be filled; just outside must not.
	inner := colorAt(t, img, 17, 17)
	if inner[3] == 0 {
		t.Fatal("pixel inside the rounded corner should be filled")
	}
	outer := colorAt(t, img, 2, 2)
	if outer[3] != 0 {
		t.Fatalf("pixel outside the rounded corner should be transparent, alpha=%d", outer[3])
	}
}

func TestHexRGB(t *testing.T) {
	c, err := hexRGB("#F08223")
	if err != nil {
		t.Fatal(err)
	}
	if c.r != 0xF0 || c.g != 0x82 || c.b != 0x23 {
		t.Fatalf("got %v", c)
	}
	if _, err := hexRGB("#XYZ"); err == nil {
		t.Fatal("bad length should error")
	}
	if _, err := hexRGB("#GGGGGG"); err == nil {
		t.Fatal("bad digits should error")
	}
}

// TestWriteIconSamples dumps sample renders for visual inspection when
// GOWORKER_ICON_SAMPLES points at a directory — skipped in normal runs.
func TestWriteIconSamples(t *testing.T) {
	dir := os.Getenv("GOWORKER_ICON_SAMPLES")
	if dir == "" {
		t.Skip("set GOWORKER_ICON_SAMPLES to dump sample renders")
	}
	for _, c := range [][2]string{
		{"check", "22C55E"}, {"zap", "FACC15"}, {"users", "38BDF8"},
		{"trending-up", "F87171"}, {"git-branch", "A78BFA"},
	} {
		b, err := renderIconPNG(c[0], c[1], 128)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, c[0]+".png"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	card, err := renderCardPNG(300, 120, 10, "1E293B", 0.5, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "card.png"), card, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustIcon(t *testing.T, name, hex string, size int) []byte {
	t.Helper()
	b, err := renderIconPNG(name, hex, size)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustCard(t *testing.T, w, h, r int, hex string, opacity float64) []byte {
	t.Helper()
	b, err := renderCardPNG(w, h, r, hex, opacity, false)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
