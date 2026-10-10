package layouts

import (
	"context"
	"testing"
)

func TestAvatarURL(t *testing.T) {
	signed := SetMediaURLFunc(context.Background(), func(id string) string { return "/media/" + id + "?sig=x" })
	signed = SetMediaThumbFunc(signed, func(id, size string) string {
		return "/media/" + id + "/thumb/" + size + "?sig=x"
	})

	tests := []struct {
		name   string
		ctx    context.Context
		stored string
		want   string
	}{
		{"empty stays empty", context.Background(), "", ""},
		{"media id, unsigned fallback", context.Background(), "b7c17bb1-6563-462c-8b49-5b2e8bd57108", "/media/b7c17bb1-6563-462c-8b49-5b2e8bd57108/thumb/300"},
		{"media id, signed", signed, "b7c17bb1-6563-462c-8b49-5b2e8bd57108", "/media/b7c17bb1-6563-462c-8b49-5b2e8bd57108/thumb/300?sig=x"},
		{"legacy path passes through", signed, "/uploads/avatars/a.png", "/uploads/avatars/a.png"},
		{"absolute url passes through", signed, "https://cdn.example/a.png", "https://cdn.example/a.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AvatarURL(tt.ctx, tt.stored); got != tt.want {
				t.Fatalf("AvatarURL(%q) = %q, want %q", tt.stored, got, tt.want)
			}
		})
	}
}
