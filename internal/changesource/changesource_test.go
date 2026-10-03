package changesource

import (
	"context"
	"testing"
)

func TestWithFrom(t *testing.T) {
	tests := []struct {
		name   string
		ctx    context.Context
		want   Source
		wantOK bool
	}{
		{"absent", context.Background(), Source{}, false},
		{"web", With(context.Background(), Source{Kind: KindWeb, UserID: "u1"}), Source{Kind: KindWeb, UserID: "u1"}, true},
		{"shop with label", With(context.Background(), Source{Kind: KindShop, Label: "Smithy", UserID: "u2"}), Source{Kind: KindShop, Label: "Smithy", UserID: "u2"}, true},
		{"innermost wins", With(With(context.Background(), Source{Kind: KindWeb}), Source{Kind: KindStash}), Source{Kind: KindStash}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := From(tt.ctx)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("From() = %+v, %v; want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
