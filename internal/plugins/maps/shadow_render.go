package maps

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"sort"

	xdraw "golang.org/x/image/draw"
)

// The player copy of a map picture: the original with every shadowed area
// smudged into the pixels, so there is nothing under a shadow to download.
// Rendering is pure (no I/O) so the rule is testable on small generated images.

const (
	// playerImageMaxSide caps the long side of the player copy. Decoding and
	// blurring a very large picture costs memory and time per shadow change, and
	// players see a screen-sized view anyway; an original larger than this is
	// scaled down, so the player copy can be lower resolution than the original.
	playerImageMaxSide = 4096

	// playerImageRenderVersion is folded into the cache key so a change to the
	// look below regenerates every cached copy.
	playerImageRenderVersion = "v1"

	// How much of the blurred picture survives under each strength.
	hintRetain   = 0.45 // "A hint": a dim, formless blur.
	hiddenRetain = 0.05 // "Almost nothing": close to black.

	playerImageJPEGQuality = 85
)

// playerImageKey identifies one player copy: the same picture, map and shadow
// set always give the same key, and any change to a shadow's box or strength
// gives a different one. Boxes are sorted so the order shadows were drawn in
// does not matter, and rounded so float noise in stored points does not churn
// the cache.
func playerImageKey(mediaID, mapID string, areas []ShadowArea) string {
	sorted := sortedShadowAreas(areas)
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%s|%s", playerImageRenderVersion, mediaID, mapID)
	for _, a := range sorted {
		_, _ = fmt.Fprintf(h, "|%.3f,%.3f,%.3f,%.3f,%.2f", a.MinX, a.MinY, a.MaxX, a.MaxY, a.Strength)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// sortedShadowAreas returns a copy ordered weakest strength first, then by
// position. Rendering uses the same order, so a stronger shadow that overlaps a
// weaker one is applied last and wins.
func sortedShadowAreas(areas []ShadowArea) []ShadowArea {
	out := make([]ShadowArea, len(areas))
	copy(out, areas)
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.Strength != b.Strength:
			return a.Strength < b.Strength
		case a.MinX != b.MinX:
			return a.MinX < b.MinX
		case a.MinY != b.MinY:
			return a.MinY < b.MinY
		case a.MaxX != b.MaxX:
			return a.MaxX < b.MaxX
		}
		return a.MaxY < b.MaxY
	})
	return out
}

// renderPlayerImage returns src, scaled so neither side exceeds maxSide, with
// each shadow area blurred and darkened. The result is opaque: transparent
// source pixels are flattened onto black so no colour data survives behind a
// zero alpha.
func renderPlayerImage(src image.Image, areas []ShadowArea, maxSide int) *image.RGBA {
	sb := src.Bounds()
	w, h := sb.Dx(), sb.Dy()
	if w > maxSide || h > maxSide {
		scale := float64(maxSide) / float64(max(w, h))
		w = max(1, int(math.Round(float64(w)*scale)))
		h = max(1, int(math.Round(float64(h)*scale)))
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if w == sb.Dx() && h == sb.Dy() {
		draw.Draw(dst, dst.Bounds(), src, sb.Min, draw.Over)
	} else {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, sb, xdraw.Over, nil)
	}
	for _, a := range sortedShadowAreas(areas) {
		smudgeArea(dst, a)
	}
	return dst
}

// smudgeArea blurs and darkens the box in place, feathering the edge so the
// shadow does not end on a hard line. The blur is made on a shrunken copy of the
// area (a strong blur for little work) and scaled back up.
func smudgeArea(img *image.RGBA, a ShadowArea) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	long := max(w, h)

	box := image.Rect(
		clampInt(int(math.Floor(a.MinX/100*float64(w))), 0, w),
		clampInt(int(math.Floor(a.MinY/100*float64(h))), 0, h),
		clampInt(int(math.Ceil(a.MaxX/100*float64(w))), 0, w),
		clampInt(int(math.Ceil(a.MaxY/100*float64(h))), 0, h),
	)
	if box.Empty() {
		return
	}
	feather := max(6, long/100)
	region := image.Rect(box.Min.X-feather, box.Min.Y-feather, box.Max.X+feather, box.Max.Y+feather).Intersect(b)

	retain := hintRetain
	if a.Strength >= ShadowAlphaHidden {
		retain = hiddenRetain
	}

	// Aim for a blur reach of about 2.5% of the picture, at least 10 px.
	reach := max(10, long/40)
	shrink := max(1, reach/4)
	sw := max(1, (region.Dx()+shrink-1)/shrink)
	sh := max(1, (region.Dy()+shrink-1)/shrink)
	small := image.NewRGBA(image.Rect(0, 0, sw, sh))
	xdraw.BiLinear.Scale(small, small.Bounds(), img, region, xdraw.Src, nil)
	boxBlur(small, 3, 3)
	smudge := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	xdraw.BiLinear.Scale(smudge, smudge.Bounds(), small, small.Bounds(), xdraw.Src, nil)

	for y := region.Min.Y; y < region.Max.Y; y++ {
		dy := distanceOutside(y, box.Min.Y, box.Max.Y)
		for x := region.Min.X; x < region.Max.X; x++ {
			dist := max(distanceOutside(x, box.Min.X, box.Max.X), dy)
			weight := 1.0
			if dist > 0 {
				s := 1 - float64(dist)/float64(feather)
				if s <= 0 {
					continue
				}
				weight = s * s * (3 - 2*s)
			}
			po := img.PixOffset(x, y)
			ps := smudge.PixOffset(x-region.Min.X, y-region.Min.Y)
			for c := 0; c < 3; c++ {
				orig := float64(img.Pix[po+c])
				shaded := float64(smudge.Pix[ps+c]) * retain
				img.Pix[po+c] = uint8(orig*(1-weight) + shaded*weight + 0.5)
			}
			img.Pix[po+3] = 255
		}
	}
}

// distanceOutside is how far v is beyond [lo, hi), 0 when inside.
func distanceOutside(v, lo, hi int) int {
	switch {
	case v < lo:
		return lo - v
	case v >= hi:
		return v - hi + 1
	}
	return 0
}

// boxBlur blurs img in place with a running-sum window of the given radius,
// repeated `passes` times in each direction (three passes approximate a
// Gaussian). Edge pixels are clamped. Alpha is untouched: the picture is opaque.
func boxBlur(img *image.RGBA, radius, passes int) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	buf := make([]uint8, max(w, h)*3)
	for p := 0; p < passes; p++ {
		for y := 0; y < h; y++ {
			blurLine(img.Pix[y*img.Stride:], 4, w, radius, buf)
		}
		for x := 0; x < w; x++ {
			blurLine(img.Pix[x*4:], img.Stride, h, radius, buf)
		}
	}
}

// blurLine blurs n pixels spaced `step` bytes apart starting at pix[0]; buf
// receives the result before it is written back.
func blurLine(pix []uint8, step, n, radius int, buf []uint8) {
	window := 2*radius + 1
	for c := 0; c < 3; c++ {
		sum := 0
		for i := -radius; i <= radius; i++ {
			sum += int(pix[clampInt(i, 0, n-1)*step+c])
		}
		for i := 0; i < n; i++ {
			buf[i*3+c] = uint8(sum / window)
			sum += int(pix[clampInt(i+radius+1, 0, n-1)*step+c]) - int(pix[clampInt(i-radius, 0, n-1)*step+c])
		}
	}
	for i := 0; i < n; i++ {
		copy(pix[i*step:i*step+3], buf[i*3:i*3+3])
	}
}

// encodePlayerImage is the one place the copy is serialised: always JPEG, so a
// PNG's metadata chunks (and any text hidden in them) are not carried over.
func encodePlayerImage(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: playerImageJPEGQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
