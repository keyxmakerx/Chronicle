package calendar

import "testing"

func TestMoonLitPath(t *testing.T) {
	tests := []struct {
		name  string
		phase float64
		want  string // "" or "non-empty"
	}{
		{"new moon: no sliver", 0, ""},
		{"just past new: still too thin to draw", 0.002, ""},
		{"first quarter: a real path", 0.25, "non-empty"},
		{"full moon: a real path", 0.5, "non-empty"},
		{"last quarter: a real path", 0.75, "non-empty"},
		{"just before wrapping to new: still too thin to draw", 0.998, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MoonLitPath(tt.phase, 6)
			if tt.want == "" && got != "" {
				t.Errorf("MoonLitPath(%v) = %q, want empty", tt.phase, got)
			}
			if tt.want == "non-empty" && got == "" {
				t.Errorf("MoonLitPath(%v) = empty, want a path", tt.phase)
			}
		})
	}
}

// TestMoonLitPath_FullMoonIsSymmetric pins the full-moon (p=0.5) path shape:
// cos(2*pi*0.5) = -1, so rx should equal the full radius (a full disc lit on
// one side, k<0 with waxing=false selects the same limb/term the mockup's
// litPath does for a full moon).
func TestMoonLitPath_FullMoonIsSymmetric(t *testing.T) {
	got := MoonLitPath(0.5, 6)
	want := "M0,-6 A6,6 0 0 0 0,6 A6,6 0 0 0 0,-6 Z"
	if got != want {
		t.Errorf("MoonLitPath(0.5, 6) = %q, want %q", got, want)
	}
}
