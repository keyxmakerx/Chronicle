// update_entity_type_partial_test.go pins the partial-update contract on
// PUT /api/v1/campaigns/:id/entity-types/:typeID, the public route a Foundry
// module or script uses: a body naming only some keys must leave the rest of
// the type alone.
package syncapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// storedTypeRepo serves one stored entity type and records what is written.
type storedTypeRepo struct {
	entities.EntityTypeRepository
	stored  entities.EntityType
	written *entities.EntityType
}

func (r *storedTypeRepo) FindByID(context.Context, int) (*entities.EntityType, error) {
	cp := r.stored
	return &cp, nil
}

func (r *storedTypeRepo) SlugExists(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *storedTypeRepo) Update(_ context.Context, et *entities.EntityType) error {
	r.written = et
	return nil
}

func TestUpdateEntityType_PartialBody(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantName   string
		wantPlural string
		wantIcon   string
		wantColor  string
	}{
		{"rename only keeps plural, icon and color", `{"name":"Site"}`, "Site", "Places", "fa-map", "#112233"},
		{"color only", `{"color":"#ffffff"}`, "Location", "Places", "fa-map", "#ffffff"},
		{"icon only", `{"icon":"fa-dragon"}`, "Location", "Places", "fa-dragon", "#112233"},
		{"plural only", `{"name_plural":"Locales"}`, "Location", "Locales", "fa-map", "#112233"},
		{"full body still replaces", `{"name":"Site","name_plural":"Sites","icon":"fa-dragon","color":"#000000"}`, "Site", "Sites", "fa-dragon", "#000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &storedTypeRepo{stored: entities.EntityType{
				ID: 3, CampaignID: "camp-1", Name: "Location", NamePlural: "Places",
				Icon: "fa-map", Color: "#112233", Slug: "location",
			}}
			h := NewAPIHandler(nil, entities.NewEntityService(nil, repo, nil), stubCampaignSvcOwner{}, nil)

			req := httptest.NewRequest(http.MethodPut, "/api/v1/campaigns/camp-1/entity-types/3", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.SetParamNames("id", "typeID")
			c.SetParamValues("camp-1", "3")
			c.Set(apiKeyContextKey, &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "user-1", IsActive: true})

			if err := h.UpdateEntityType(c); err != nil {
				t.Fatalf("UpdateEntityType: %v", err)
			}
			w := repo.written
			if w == nil {
				t.Fatal("nothing was written")
			}
			if w.Name != tt.wantName || w.NamePlural != tt.wantPlural || w.Icon != tt.wantIcon || w.Color != tt.wantColor {
				t.Errorf("wrote %q/%q/%q/%q, want %q/%q/%q/%q",
					w.Name, w.NamePlural, w.Icon, w.Color,
					tt.wantName, tt.wantPlural, tt.wantIcon, tt.wantColor)
			}
		})
	}
}

// Entity types are an Owner-only surface on the web; the API must not be a
// way around that for a Scribe or a player.
func TestEntityTypeWrites_RequireOwner(t *testing.T) {
	for _, role := range []campaigns.Role{campaigns.RolePlayer, campaigns.RoleScribe} {
		for _, door := range []string{"create", "update"} {
			t.Run(door, func(t *testing.T) {
				repo := &storedTypeRepo{stored: entities.EntityType{ID: 3, CampaignID: "camp-1", Name: "Location", Slug: "location"}}
				h := NewAPIHandler(nil, entities.NewEntityService(nil, repo, nil), stubCampaignSvcRole{role: role}, nil)
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"Site"}`))
				req.Header.Set("Content-Type", "application/json")
				c := echo.New().NewContext(req, httptest.NewRecorder())
				c.SetParamNames("id", "typeID")
				c.SetParamValues("camp-1", "3")
				c.Set(apiKeyContextKey, &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "user-1", IsActive: true})

				var err error
				if door == "create" {
					err = h.CreateEntityType(c)
				} else {
					err = h.UpdateEntityType(c)
				}
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != http.StatusForbidden {
					t.Fatalf("role %v: err = %v, want 403", role, err)
				}
				if repo.written != nil {
					t.Fatal("a forbidden call still wrote")
				}
			})
		}
	}
}

// stubCampaignSvcRole answers resolveRole's membership lookup with a fixed role.
type stubCampaignSvcRole struct {
	campaigns.CampaignService
	role campaigns.Role
}

func (s stubCampaignSvcRole) GetMember(_ context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
	return &campaigns.CampaignMember{CampaignID: campaignID, UserID: userID, Role: s.role}, nil
}
