package entities

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// A creature page's token picture reaches the hover card as a link, only when
// it is a media id, and never as a raw id among the page's facts.
func TestPreviewAPI_TokenPicture(t *testing.T) {
	const id = "0f8b6c1e-2d3a-4b5c-8d9e-0a1b2c3d4e5f"
	img := "2026/03/" + id + ".png"
	tests := []struct {
		name      string
		token     any
		gmOnly    bool
		role      campaigns.Role
		wantToken string
	}{
		{"media id", id, false, campaigns.RolePlayer, "/media/" + id},
		{"empty", "", false, campaigns.RolePlayer, ""},
		{"not an id", "https://evil.example/x.png", false, campaigns.RolePlayer, ""},
		{"braced id", "{" + id + "}", false, campaigns.RolePlayer, ""},
		{"not a string", 42, false, campaigns.RolePlayer, ""},
		{"GM-only, player", id, true, campaigns.RolePlayer, ""},
		{"GM-only, owner", id, true, campaigns.RoleOwner, "/media/" + id},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			et := &EntityType{ID: 3, CampaignID: "c1", Slug: "creature", Name: "Creature", Enabled: true, Fields: []FieldDefinition{
				{Key: TokenFieldKey, Label: "Token picture", Type: "text", GMOnly: tt.gmOnly},
				{Key: "level", Label: "Level", Type: "number"},
			}}
			e := &Entity{ID: "goblin", CampaignID: "c1", EntityTypeID: 3, Name: "Goblin", Visibility: VisibilityDefault,
				ImagePath: &img, FieldsData: map[string]any{TokenFieldKey: tt.token, "level": 1}}
			typeRepo := &mockEntityTypeRepo{findByIDFn: func(context.Context, int) (*EntityType, error) { cp := *et; return &cp, nil }}
			entityRepo := &mockEntityRepo{findByIDFn: func(_ context.Context, eid string) (*Entity, error) {
				if eid != e.ID {
					return nil, apperror.NewNotFound("entity not found")
				}
				cp := *e
				return &cp, nil
			}}
			h := &Handler{service: NewEntityService(entityRepo, typeRepo, &mockPermissionRepo{})}
			c, rec := advCtx(tt.role, false, http.MethodGet, "/x")
			c.SetParamNames("id", "eid")
			c.SetParamValues("c1", "goblin")
			if err := h.PreviewAPI(c); err != nil {
				t.Fatalf("PreviewAPI err = %v", err)
			}
			var got struct {
				ImagePath  string              `json:"image_path"`
				TokenPath  string              `json:"token_path"`
				Attributes []map[string]string `json:"attributes"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.TokenPath != tt.wantToken {
				t.Errorf("token_path = %q, want %q", got.TokenPath, tt.wantToken)
			}
			if got.ImagePath != "/media/"+id {
				t.Errorf("image_path = %q", got.ImagePath)
			}
			for _, a := range got.Attributes {
				if a["label"] == "Token picture" {
					t.Errorf("the token's media id is listed as a fact: %v", a)
				}
			}
		})
	}
}
