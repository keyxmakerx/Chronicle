package entities

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// SlugExists counts the page's own slug, so these tests make it answer true
// for the stored slug like the real repository does.
func TestUpdate_RenameKeepsSlugWhenItStillFits(t *testing.T) {
	tests := []struct {
		name     string
		stored   string
		newName  string
		wantSlug string
	}{
		{"case-only rename", "zorn", "ZORN", "zorn"},
		{"punctuation-only rename", "zorn", "Zorn!", "zorn"},
		{"suffixed slug kept for its base name", "zorn-2", "Zorn", "zorn-2"},
		{"real rename regenerates", "zorn", "Blorp", "blorp"},
	}
	for _, tt := range tests {
		t.Run("entity "+tt.name, func(t *testing.T) {
			repo := &mockEntityRepo{
				findByIDFn: func(_ context.Context, id string) (*Entity, error) {
					return &Entity{ID: id, CampaignID: "camp-1", Name: "Zorn", Slug: tt.stored}, nil
				},
				slugExistsFn: func(_ context.Context, _, slug string) (bool, error) { return slug == tt.stored, nil },
			}
			got, err := newTestService(repo, &mockEntityTypeRepo{}).Update(context.Background(), "e1", UpdateEntityInput{Name: patch.Of(tt.newName)})
			if err != nil {
				t.Fatal(err)
			}
			if got.Slug != tt.wantSlug {
				t.Fatalf("slug = %q, want %q", got.Slug, tt.wantSlug)
			}
		})
		t.Run("type "+tt.name, func(t *testing.T) {
			var wrote *EntityType
			types := &mockEntityTypeRepo{
				findByIDFn: func(_ context.Context, _ int) (*EntityType, error) {
					return &EntityType{ID: 1, CampaignID: "camp-1", Name: "Zorn", NamePlural: "Zorns", Icon: "fa-map", Color: "#112233", Slug: tt.stored}, nil
				},
				slugExistsFn: func(_ context.Context, _, slug string) (bool, error) { return slug == tt.stored, nil },
				updateFn:     func(_ context.Context, et *EntityType) error { wrote = et; return nil },
			}
			if _, err := newTestService(&mockEntityRepo{}, types).UpdateEntityType(context.Background(), 1, UpdateEntityTypeInput{Name: patch.Of(tt.newName)}); err != nil {
				t.Fatal(err)
			}
			if wrote.Slug != tt.wantSlug {
				t.Fatalf("slug = %q, want %q", wrote.Slug, tt.wantSlug)
			}
		})
	}
}
