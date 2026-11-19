package media

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"
)

// A picture inside both side limits can still declare far more pixels than
// the server should decode; it is refused from its header alone.
func TestSanitizeImage_RefusesTooManyPixels(t *testing.T) {
	// One grey row per pixel keeps the encoded file tiny, as an attacker's
	// flat-colour picture would be.
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 9000, 9000))); err != nil {
		t.Fatal(err)
	}
	if _, _, err := sanitizeImage(buf.Bytes(), "image/png"); err == nil || !strings.Contains(err.Error(), "megapixels") {
		t.Fatalf("81 megapixel picture: err = %v, want a megapixel refusal", err)
	}
}

func TestCheckImageSize(t *testing.T) {
	cases := []struct {
		name   string
		w, h   int
		refuse bool
	}{
		{"a large battle map", 8000, 8000, false},
		{"a long strip inside the pixel cap", 10000, 6000, false},
		{"both sides at the limit", 10000, 10000, true},
		{"one side over the limit", 10001, 10, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkImageSize(image.Config{Width: tc.w, Height: tc.h})
			if (err != nil) != tc.refuse {
				t.Fatalf("checkImageSize(%dx%d) = %v, refuse = %v", tc.w, tc.h, err, tc.refuse)
			}
		})
	}
}
