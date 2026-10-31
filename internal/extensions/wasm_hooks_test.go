package extensions

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/changesource"
)

func TestAnnounced(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{"no source", context.Background(), true},
		{"web write", changesource.With(context.Background(), changesource.Source{Kind: changesource.KindWeb}), true},
		{"foundry write", changesource.With(context.Background(), changesource.Source{Kind: changesource.KindFoundry}), true},
		{"plugin write", changesource.With(context.Background(), changesource.Source{Kind: changesource.KindExtension}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := announced(tt.ctx); got != tt.want {
				t.Errorf("announced() = %v, want %v", got, tt.want)
			}
		})
	}
}
