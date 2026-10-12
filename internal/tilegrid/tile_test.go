package tilegrid

import (
	"context"
	"strings"
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		in   string
		w, h int
		ok   bool
	}{
		{"1x1", 1, 1, true},
		{"2x2", 2, 2, true},
		{"4x6", 4, 6, true},
		{"", 0, 0, false},
		{"2", 0, 0, false},
		{"2x", 0, 0, false},
		{"x2", 0, 0, false},
		{"0x1", 0, 0, false},
		{"5x1", 0, 0, false},
		{"1x7", 0, 0, false},
		{"2X2", 0, 0, false},
		{"02x2", 0, 0, false},
		{" 2x2", 0, 0, false},
		{"2x2x2", 0, 0, false},
		{"-1x2", 0, 0, false},
		{"axb", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			w, h, ok := ParseSize(tt.in)
			if ok != tt.ok || w != tt.w || h != tt.h {
				t.Errorf("ParseSize(%q) = %d,%d,%v; want %d,%d,%v", tt.in, w, h, ok, tt.w, tt.h, tt.ok)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name    string
		stored  string
		allowed []string
		want    string
		ok      bool
	}{
		{"stored size still allowed", "2x2", []string{"1x1", "2x2"}, "2x2", true},
		{"stored size no longer allowed falls back to first", "3x3", []string{"1x2", "2x2"}, "1x2", true},
		{"empty stored size falls back to first", "", []string{"2x1"}, "2x1", true},
		{"type allows nothing", "2x2", nil, "", false},
		{"only malformed sizes declared", "2x2", []string{"big", "9x9"}, "", false},
		{"malformed entries are skipped for the fallback", "3x3", []string{"big", "1x1"}, "1x1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, w, h, ok := Resolve(tt.stored, tt.allowed)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("Resolve(%q, %v) = %q,%v; want %q,%v", tt.stored, tt.allowed, got, ok, tt.want, tt.ok)
			}
			if ok {
				pw, ph, _ := ParseSize(got)
				if w != pw || h != ph {
					t.Errorf("w,h = %d,%d; want %d,%d", w, h, pw, ph)
				}
			}
		})
	}
}

func TestValidateSection(t *testing.T) {
	sizes := func(typ string, _ map[string]any) []string {
		if typ == "steps" {
			return []string{"1x1", "2x2"}
		}
		return nil
	}
	tests := []struct {
		name       string
		hasColumns bool
		tiles      []Tile
		sizesFor   SizesFunc
		wantErr    string
	}{
		{"empty section is fine", false, nil, sizes, ""},
		{"valid tiles", false, []Tile{{ID: "a", Type: "steps", Size: "2x2"}, {ID: "b", Type: "steps", Size: "1x1"}}, sizes, ""},
		{"columns and tiles", true, []Tile{{ID: "a", Type: "steps", Size: "1x1"}}, sizes, "both columns and tiles"},
		{"type with no tile sizes", false, []Tile{{ID: "a", Type: "divider", Size: "1x1"}}, sizes, "can't be shown as a tile"},
		{"size not in the type's list", false, []Tile{{ID: "a", Type: "steps", Size: "2x1"}}, sizes, "Allowed sizes: 1x1, 2x2"},
		{"malformed size", false, []Tile{{ID: "a", Type: "steps", Size: "wide"}}, sizes, "invalid size"},
		{"empty size", false, []Tile{{ID: "a", Type: "steps"}}, sizes, "invalid size"},
		{"missing id", false, []Tile{{Type: "steps", Size: "1x1"}}, sizes, "missing its ID"},
		{"duplicate id", false, []Tile{{ID: "a", Type: "steps", Size: "1x1"}, {ID: "a", Type: "steps", Size: "1x1"}}, sizes, "same block twice"},
		{"no lookup skips type and size checks", false, []Tile{{ID: "a", Type: "anything", Size: "2x2"}}, nil, ""},
		{"no lookup still checks the size shape", false, []Tile{{ID: "a", Type: "anything", Size: "9x9"}}, nil, "invalid size"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSection(1, tt.hasColumns, tt.tiles, map[string]bool{}, tt.sizesFor)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v; want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSectionTooMany(t *testing.T) {
	tiles := make([]Tile, MaxTilesPerSection+1)
	err := ValidateSection(1, false, tiles, map[string]bool{}, nil)
	if err == nil || !strings.Contains(err.Error(), "too many tiles") {
		t.Fatalf("error = %v; want too many tiles", err)
	}
}

func TestValidateKind(t *testing.T) {
	for _, k := range []string{"", KindTiles} {
		if err := ValidateKind(1, k); err != nil {
			t.Errorf("kind %q rejected: %v", k, err)
		}
	}
	if err := ValidateKind(1, "masonry"); err == nil {
		t.Error("unknown kind accepted")
	}
}

func TestCellCarriesSizeAttributes(t *testing.T) {
	var sb strings.Builder
	if err := Cell("t1", 2, 3).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	got := sb.String()
	for _, want := range []string{`class="tile"`, `data-widget="tile"`, `data-tile-id="t1"`, `data-tile-w="2"`, `data-tile-h="3"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}
