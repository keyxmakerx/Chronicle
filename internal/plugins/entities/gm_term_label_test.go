package entities

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// A campaign whose system names its game master reads that word, and one
// with no system reads the default.
func TestVisibilityCornerLabel_GMTerm(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"no system", context.Background(), "GM team only"},
		{"draw steel", layouts.SetGMTerm(context.Background(), "Director"), "Director team only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := visibilityCornerLabel(tt.ctx, VisStateDMOnly); got != tt.want {
				t.Fatalf("label = %q, want %q", got, tt.want)
			}
		})
	}
}
