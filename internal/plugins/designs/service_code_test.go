package designs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func encode(v any) string {
	raw, _ := json.Marshal(v)
	return CodePrefix + base64.RawURLEncoding.EncodeToString(raw)
}

func TestCodeRoundTrip(t *testing.T) {
	s, repo, _ := newWorld()
	ctx := context.Background()
	d := mustSave(t, s, "Silver Ward", "silver")
	c := code(t, s, d.ID)
	if !strings.HasPrefix(c, CodePrefix) {
		t.Fatalf("code = %q", c)
	}

	// Bob owns a draw steel campaign that lacks silver: same system, one warning.
	p, err := s.PreviewCode(ctx, "B1", "bob", " \n"+c[:20]+"\n "+c[20:]+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Silver Ward" || p.MakerName != "Alice" || p.Kind != KindRoomTheme || !p.SameSystem || len(p.Lacks) != 1 {
		t.Errorf("preview = %+v", p)
	}
	taken, err := s.TakeCode(ctx, "B1", "bob", c)
	if err != nil {
		t.Fatal(err)
	}
	if taken.ID == d.ID || taken.CampaignID != "B1" || taken.Origin != OriginCode || taken.MakerName != "Alice" || taken.MakerUserID != "" || taken.SystemID != "drawsteel" {
		t.Errorf("taken = %+v", taken)
	}
	if got := len(repo.designs); got != 2 {
		t.Errorf("rows = %d, want 2", got)
	}

	// A code from another game system previews with the warning and still imports.
	p, err = s.PreviewCode(ctx, "C1", "cara", c)
	if err != nil || p.SameSystem {
		t.Fatalf("other system preview = %+v, %v", p, err)
	}
	other, err := s.TakeCode(ctx, "C1", "cara", c)
	if err != nil || other.SystemID != "dnd5e" {
		t.Fatalf("other system take = %+v, %v", other, err)
	}
}

func TestBadCodesAreRefused(t *testing.T) {
	s, repo, _ := newWorld()
	good := map[string]any{"kind": KindRoomTheme, "system_id": "drawsteel", "name": "Ward", "maker_name": "Zed", "data": map[string]any{"part": "oak"}}
	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for a, b := range good {
			m[a] = b
		}
		m[k] = v
		return m
	}
	tests := []struct {
		name string
		code string
		msg  string
	}{
		{"empty", "", "not a Chronicle design code"},
		{"wrong prefix", "chronicle-design:v2:abc", "not a Chronicle design code"},
		{"not base64", CodePrefix + "!!!!", "damaged"},
		{"base64 of non-JSON", CodePrefix + base64.RawURLEncoding.EncodeToString([]byte("hello")), "damaged"},
		{"oversize", CodePrefix + strings.Repeat("A", MaxCodeBytes*2), "too large"},
		{"decoded oversize", CodePrefix + base64.RawURLEncoding.EncodeToString([]byte(`{"pad":"`+strings.Repeat("a", MaxCodeBytes)+`"}`)), "too large"},
		{"unknown kind", encode(with("kind", "poster")), "not a kind"},
		{"unknown part", encode(with("data", map[string]any{"part": "lava"})), "unknown part"},
		{"no name", encode(with("name", " ")), "required"},
		{"name too long", encode(with("name", strings.Repeat("n", 61))), "at most 60"},
		{"maker too long", encode(with("maker_name", strings.Repeat("m", 101))), "at most 100"},
		{"no data", encode(with("data", nil)), "missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, call := range []func() error{
				func() error { _, err := s.PreviewCode(context.Background(), "B1", "bob", tt.code); return err },
				func() error { _, err := s.TakeCode(context.Background(), "B1", "bob", tt.code); return err },
			} {
				err := call()
				wantCode(t, err, 400)
				if !strings.Contains(err.Error(), tt.msg) {
					t.Errorf("error %q does not mention %q", err, tt.msg)
				}
			}
			if len(repo.designs) != 0 {
				t.Errorf("a refused code stored %d rows", len(repo.designs))
			}
		})
	}
	// Control characters in a sender's text are dropped, not stored.
	if d, err := s.TakeCode(context.Background(), "B1", "bob", encode(with("name", "Wa\x00rd\n"))); err != nil || d.Name != "Ward" {
		t.Errorf("name = %v, %v", d, err)
	}
}

func TestCopiesAreIndependent(t *testing.T) {
	s, _, _ := newWorld()
	ctx := context.Background()
	d := mustSave(t, s, "Ward", "oak")
	taken, err := s.TakeCode(ctx, "B1", "bob", code(t, s, d.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, "A1", "alice", d.ID, UpdateDesignInput{Name: patchOf("Renamed")}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(ctx, "B1", "bob", taken.ID); got.Name != "Ward" {
		t.Errorf("editing the original changed the copy to %q", got.Name)
	}
	if err := s.Delete(ctx, "A1", "alice", d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "B1", "bob", taken.ID); err != nil {
		t.Errorf("deleting the original removed the copy: %v", err)
	}
}
