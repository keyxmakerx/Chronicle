package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// scopeEntitySvc adds a page in another campaign to the fixture entities.
type scopeEntitySvc struct{ advEntitySvc }

func (scopeEntitySvc) GetByID(ctx context.Context, id string) (*entities.Entity, error) {
	if id == "foreign-page" {
		return &entities.Entity{ID: id, CampaignID: "camp-2", EntityTypeID: 1}, nil
	}
	return advEntitySvc{}.GetByID(ctx, id)
}

type createRelSvc struct {
	relations.RelationService
	created int
}

func (s *createRelSvc) Create(_ context.Context, campaignID, src, tgt, _, _, _ string, _ json.RawMessage, _ ...bool) (*relations.Relation, error) {
	s.created++
	return &relations.Relation{CampaignID: campaignID, SourceEntityID: src, TargetEntityID: tgt}, nil
}

func TestCreateRelation_TargetScopedToCampaignAndVisibility(t *testing.T) {
	tests := []struct {
		name    string
		role    campaigns.Role
		source  string
		target  string
		created int
		want404 bool
	}{
		{"two visible pages", campaigns.RolePlayer, "npc-page", "npc-page", 1, false},
		{"target in another campaign", campaigns.RolePlayer, "npc-page", "foreign-page", 0, true},
		{"target hidden from the caller", campaigns.RolePlayer, "npc-page", "adv-page", 0, true},
		{"target hidden page visible to scribe", campaigns.RoleScribe, "npc-page", "adv-page", 1, false},
		{"unknown target", campaigns.RolePlayer, "npc-page", "nope", 0, true},
		{"source hidden from the caller", campaigns.RolePlayer, "adv-page", "npc-page", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := &createRelSvc{}
			h := NewAPIHandler(nil, scopeEntitySvc{}, &stubCampaignSvcForDmGrant{role: tt.role}, rel)
			body := `{"target_entity_id":"` + tt.target + `","relation_type":"ally"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/x", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			c := echo.New().NewContext(req, httptest.NewRecorder())
			c.SetParamNames("id", "entityID")
			c.SetParamValues("camp-1", tt.source)
			c.Set(apiKeyContextKey, &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "u1", IsActive: true})

			err := h.CreateRelation(c)
			if tt.want404 != isNotFound(err) || (!tt.want404 && err != nil) {
				t.Fatalf("err = %v, want404 = %v", err, tt.want404)
			}
			if rel.created != tt.created {
				t.Errorf("created = %d, want %d", rel.created, tt.created)
			}
		})
	}
}
