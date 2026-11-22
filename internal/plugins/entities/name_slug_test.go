package entities

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// slugTable answers FindBySlug and ListSuffixedSlugs the way the repository
// does: trashed rows hold their slug but are never returned.
func slugTable(rows []Entity, trashed map[string]bool) *mockEntityRepo {
	return &mockEntityRepo{
		findBySlugFn: func(_ context.Context, c, slug string) (*Entity, error) {
			for i := range rows {
				if rows[i].CampaignID == c && rows[i].Slug == slug && !trashed[rows[i].ID] {
					e := rows[i]
					return &e, nil
				}
			}
			return nil, apperror.NewNotFound("entity not found")
		},
		listSuffixedFn: func(_ context.Context, c, base string) ([]Entity, error) {
			var out []Entity
			for _, r := range rows {
				if r.CampaignID == c && strings.HasPrefix(r.Slug, base+"-") && !trashed[r.ID] {
					out = append(out, r)
				}
			}
			return out, nil
		},
	}
}

func TestGetByNameSlug(t *testing.T) {
	parent := "saltreach"
	rows := []Entity{
		{ID: "old-shop", CampaignID: "c", Name: "Hessa Ord's Workshop", Slug: "hessa-ord-s-workshop"},
		{ID: "shop", CampaignID: "c", Name: "Hessa Ord's Workshop", Slug: "hessa-ord-s-workshop-2", ParentID: &parent},
		{ID: "rope", CampaignID: "c", Name: "Rope", Slug: "rope"},
		{ID: "rope-two", CampaignID: "c", Name: "Rope 2", Slug: "rope-2"},
		{ID: "lamp-two", CampaignID: "c", Name: "Lamp 2", Slug: "lamp-2"},
		{ID: "salve", CampaignID: "c", Name: "Salve", Slug: "salve-0a1b2c3d"},
		{ID: "elsewhere", CampaignID: "other", Name: "Tonic", Slug: "tonic-2"},
	}
	svc := newTestService(slugTable(rows, map[string]bool{"old-shop": true}), &mockEntityTypeRepo{})

	tests := []struct {
		name, want string
	}{
		{"Hessa Ord's Workshop", "shop"}, // plain slug held by a trashed page
		{"Hessa Ord’s Workshop", "shop"}, // a curly apostrophe names it too
		{"Rope", "rope"},
		{"Rope 2", "rope-two"},
		{"Lamp", ""}, // "Lamp 2" holds lamp-2 but is not called Lamp
		{"Salve", "salve"},
		{"Tonic", ""}, // another campaign's page
		{"Nothing Here", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := svc.GetByNameSlug(context.Background(), "c", Slugify(tt.name))
			if tt.want == "" {
				if e != nil || apperror.SafeCode(err) != 404 {
					t.Fatalf("got %+v, %v; want not found", e, err)
				}
				return
			}
			if err != nil || e == nil || e.ID != tt.want {
				t.Fatalf("got %+v, %v; want %s", e, err, tt.want)
			}
		})
	}
}
