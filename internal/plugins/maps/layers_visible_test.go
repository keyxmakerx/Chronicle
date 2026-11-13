package maps

import "testing"

func TestVisibleLayers(t *testing.T) {
	layers := []Layer{{ID: "a", LayerType: "background"}, {ID: "b", LayerType: "gm"}, {ID: "c", LayerType: "token"}}
	tests := []struct {
		name string
		role int
		want []string
	}{
		{"player", 1, []string{"a", "c"}},
		{"scribe", 2, []string{"a", "c"}},
		{"owner", 3, []string{"a", "b", "c"}},
		{"signed out", 0, []string{"a", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VisibleLayers(layers, tt.role)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d layers, want %d", len(got), len(tt.want))
			}
			for i, l := range got {
				if l.ID != tt.want[i] {
					t.Errorf("layer %d = %s, want %s", i, l.ID, tt.want[i])
				}
			}
		})
	}
}
