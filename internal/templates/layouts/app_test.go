package layouts

import (
	"context"
	"strings"
	"testing"
)

// ctxWithTopbarStyle builds a context carrying the given topbar style, mirroring
// what the LayoutInjector does at request time.
func ctxWithTopbarStyle(s *TopbarStyleData) context.Context {
	return SetTopbarStyle(context.Background(), s)
}

// TestTopbarInlineStyle pins the contract that the helper emits the correct
// background CSS for each mode — in particular that gradient mode produces a
// linear-gradient and image mode produces a background-image url(), the two
// modes that previously rendered nothing on the topbar.
func TestTopbarInlineStyle(t *testing.T) {
	tests := []struct {
		name      string
		style     *TopbarStyleData
		want      string // required substring; ignored when wantEmpty is true
		wantEmpty bool
	}{
		{name: "nil style falls back to default", style: nil, wantEmpty: true},
		{name: "empty mode falls back to default", style: &TopbarStyleData{}, wantEmpty: true},
		{
			name:  "solid emits background-color",
			style: &TopbarStyleData{Mode: "solid", Color: "#6366f1"},
			want:  "background-color: #6366f1;",
		},
		{
			name:  "gradient emits linear-gradient with mapped direction",
			style: &TopbarStyleData{Mode: "gradient", GradientFrom: "#6366f1", GradientTo: "#ec4899", GradientDir: "to-br"},
			want:  "background: linear-gradient(to bottom right, #6366f1, #ec4899);",
		},
		{
			name:  "gradient defaults direction to right",
			style: &TopbarStyleData{Mode: "gradient", GradientFrom: "#111111", GradientTo: "#222222"},
			want:  "linear-gradient(to right, #111111, #222222)",
		},
		{
			name:  "image emits background-image url",
			style: &TopbarStyleData{Mode: "image", ImagePath: "bg.png"},
			want:  "background-image: url('/media/bg.png');",
		},
		{
			name:      "gradient missing a color falls back to default",
			style:     &TopbarStyleData{Mode: "gradient", GradientFrom: "#111111"},
			wantEmpty: true,
		},
		{
			name:      "image missing path falls back to default",
			style:     &TopbarStyleData{Mode: "image"},
			wantEmpty: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := topbarInlineStyle(ctxWithTopbarStyle(tt.style))
			if tt.wantEmpty {
				if got != "" {
					t.Fatalf("expected empty style, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("style %q does not contain %q", got, tt.want)
			}
		})
	}
}

// TestTopbarStyleIsImage verifies the scrim-gating predicate.
func TestTopbarStyleIsImage(t *testing.T) {
	if !topbarStyleIsImage(ctxWithTopbarStyle(&TopbarStyleData{Mode: "image", ImagePath: "x.png"})) {
		t.Fatal("image mode with a path should be reported as image")
	}
	if topbarStyleIsImage(ctxWithTopbarStyle(&TopbarStyleData{Mode: "image"})) {
		t.Fatal("image mode without a path should not be reported as image")
	}
	if topbarStyleIsImage(ctxWithTopbarStyle(&TopbarStyleData{Mode: "gradient", GradientFrom: "#111", GradientTo: "#222"})) {
		t.Fatal("gradient mode should not be reported as image")
	}
	if topbarStyleIsImage(ctxWithTopbarStyle(nil)) {
		t.Fatal("nil style should not be reported as image")
	}
}

// TestTopbarHasCustomStyle verifies the layer-gating predicate tracks
// topbarInlineStyle exactly.
func TestTopbarHasCustomStyle(t *testing.T) {
	if topbarHasCustomStyle(ctxWithTopbarStyle(nil)) {
		t.Fatal("nil style should report no custom style")
	}
	if topbarHasCustomStyle(ctxWithTopbarStyle(&TopbarStyleData{})) {
		t.Fatal("empty mode should report no custom style")
	}
	if !topbarHasCustomStyle(ctxWithTopbarStyle(&TopbarStyleData{Mode: "solid", Color: "#ffffff"})) {
		t.Fatal("solid with a color should report a custom style")
	}
}
