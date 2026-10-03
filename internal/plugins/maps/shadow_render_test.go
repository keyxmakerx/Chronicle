package maps

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// checkerboard is a harsh test picture: black and white squares, so any
// surviving detail shows up as a large difference.
func checkerboard(w, h, cell int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(0)
			if (x/cell+y/cell)%2 == 0 {
				v = 255
			}
			img.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	return img
}

// stats returns the mean brightness, the mean absolute difference from ref, and
// the standard deviation of brightness over the rectangle.
func stats(img, ref image.Image, r image.Rectangle) (mean, diff, std float64) {
	n := 0.0
	var sum, sumSq, d float64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, _, _, _ := img.At(x, y).RGBA()
			rr, _, _, _ := ref.At(x, y).RGBA()
			v, rv := float64(cr>>8), float64(rr>>8)
			sum += v
			sumSq += v * v
			d += math.Abs(v - rv)
			n++
		}
	}
	mean = sum / n
	return mean, d / n, math.Sqrt(sumSq/n - mean*mean)
}

func TestRenderPlayerImage_SmudgesShadowsOnly(t *testing.T) {
	src := checkerboard(200, 200, 4)
	// The deep middle of the 25-75% box, clear of any feathered edge.
	inside := image.Rect(80, 80, 120, 120)
	// A far corner, well beyond the feather.
	outside := image.Rect(0, 0, 30, 30)

	cases := []struct {
		name     string
		strength float64
		maxMean  float64 // brightness ceiling inside
	}{
		{"a hint is blurred and dimmed", ShadowAlphaHint, 80},
		{"almost nothing is nearly black", ShadowAlphaHidden, 15},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := renderPlayerImage(src, []ShadowArea{{MinX: 25, MinY: 25, MaxX: 75, MaxY: 75, Strength: tc.strength}}, 4096)

			mean, diff, std := stats(out, src, inside)
			_, _, srcStd := stats(src, src, inside)
			if diff < 40 {
				t.Errorf("shadowed region differs by %.1f from the original, want a strong change", diff)
			}
			if mean > tc.maxMean {
				t.Errorf("shadowed region brightness %.1f, want <= %.1f", mean, tc.maxMean)
			}
			if std > srcStd/4 {
				t.Errorf("shadowed region keeps detail: std %.1f against %.1f in the original", std, srcStd)
			}

			if _, diff, _ := stats(out, src, outside); diff != 0 {
				t.Errorf("unshadowed region changed by %.2f, want it untouched", diff)
			}
		})
	}
}

// Where two shadows overlap, the stronger one wins whichever was drawn first.
func TestRenderPlayerImage_StrongerShadowWinsOverlap(t *testing.T) {
	src := checkerboard(200, 200, 4)
	hint := ShadowArea{MinX: 10, MinY: 10, MaxX: 90, MaxY: 90, Strength: ShadowAlphaHint}
	hidden := ShadowArea{MinX: 30, MinY: 30, MaxX: 70, MaxY: 70, Strength: ShadowAlphaHidden}
	centre := image.Rect(90, 90, 110, 110)
	for name, areas := range map[string][]ShadowArea{"hint first": {hint, hidden}, "hidden first": {hidden, hint}} {
		mean, _, _ := stats(renderPlayerImage(src, areas, 4096), src, centre)
		if mean > 15 {
			t.Errorf("%s: overlap brightness %.1f, want the near-black of the stronger shadow", name, mean)
		}
	}
}

func TestRenderPlayerImage_CapsLongSideAndFlattensAlpha(t *testing.T) {
	cases := []struct {
		name         string
		w, h         int
		maxSide      int
		wantW, wantH int
	}{
		{"wide picture is scaled by its long side", 5000, 100, 4096, 4096, 82},
		{"tall picture is scaled by its long side", 100, 8192, 4096, 50, 4096},
		{"small picture is left alone", 300, 200, 4096, 300, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := renderPlayerImage(image.NewNRGBA(image.Rect(0, 0, tc.w, tc.h)), nil, tc.maxSide)
			if got := out.Bounds(); got.Dx() != tc.wantW || got.Dy() != tc.wantH {
				t.Errorf("size %dx%d, want %dx%d", got.Dx(), got.Dy(), tc.wantW, tc.wantH)
			}
			// Fully transparent input must not leave colour behind a zero alpha.
			if px := out.RGBAAt(0, 0); px != (color.RGBA{0, 0, 0, 255}) {
				t.Errorf("transparent pixel became %v, want opaque black", px)
			}
		})
	}
}

// A shadow on a scaled-down copy is placed by percentage, so it covers the same
// part of the picture as on the original.
func TestRenderPlayerImage_ShadowIsPercentOfScaledCopy(t *testing.T) {
	src := checkerboard(1000, 1000, 10)
	out := renderPlayerImage(src, []ShadowArea{{MinX: 0, MinY: 0, MaxX: 50, MaxY: 100, Strength: ShadowAlphaHidden}}, 500)
	if out.Bounds().Dx() != 500 {
		t.Fatalf("width %d, want 500", out.Bounds().Dx())
	}
	left, _, _ := stats(out, out, image.Rect(50, 50, 200, 450))
	right, _, _ := stats(out, out, image.Rect(300, 50, 450, 450))
	if left > 15 || right < 80 {
		t.Errorf("left half brightness %.1f (want near black), right half %.1f (want the original's mid-grey)", left, right)
	}
}

func TestPlayerImageKey(t *testing.T) {
	a := ShadowArea{MinX: 10, MinY: 10, MaxX: 30, MaxY: 30, Strength: ShadowAlphaHint}
	b := ShadowArea{MinX: 50, MinY: 40, MaxX: 70, MaxY: 60, Strength: ShadowAlphaHidden}
	base := playerImageKey("media-1", "map-1", []ShadowArea{a, b})

	moved := b
	moved.MaxX = 71
	stronger := a
	stronger.Strength = ShadowAlphaHidden
	noise := a
	noise.MinX += 0.0001

	cases := []struct {
		name  string
		key   string
		equal bool
	}{
		{"same shadows", playerImageKey("media-1", "map-1", []ShadowArea{a, b}), true},
		{"order does not matter", playerImageKey("media-1", "map-1", []ShadowArea{b, a}), true},
		{"float noise does not matter", playerImageKey("media-1", "map-1", []ShadowArea{noise, b}), true},
		{"a box moved", playerImageKey("media-1", "map-1", []ShadowArea{a, moved}), false},
		{"a strength changed", playerImageKey("media-1", "map-1", []ShadowArea{stronger, b}), false},
		{"a shadow added", playerImageKey("media-1", "map-1", []ShadowArea{a, b, b}), false},
		{"a shadow removed", playerImageKey("media-1", "map-1", []ShadowArea{a}), false},
		{"no shadows", playerImageKey("media-1", "map-1", nil), false},
		{"another picture", playerImageKey("media-2", "map-1", []ShadowArea{a, b}), false},
		{"another map", playerImageKey("media-1", "map-2", []ShadowArea{a, b}), false},
	}
	for _, tc := range cases {
		if (tc.key == base) != tc.equal {
			t.Errorf("%s: key equal=%v, want %v", tc.name, tc.key == base, tc.equal)
		}
	}
}
