package foundry_vtt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type fakeCreatureResolver struct{ creatures map[string]bool }

func (f fakeCreatureResolver) IsCreature(_ context.Context, campaignID, entityID string) (bool, error) {
	return campaignID == "camp-1" && f.creatures[entityID], nil
}

type fakeCreatureAddPublisher struct{ sent []string }

func (f *fakeCreatureAddPublisher) PublishCreatureAdd(campaignID, entityID string) {
	f.sent = append(f.sent, campaignID+"/"+entityID)
}

func creatureAddContext(method, entityID string) (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, "/campaigns/camp-1/foundry-vtt/creature-add/"+entityID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "eid", Value: entityID}})
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RoleOwner,
		IsMember:   true,
	})
	return c, rec
}

// The button shows only on a creature page of a campaign that has used
// Foundry; everything else gets an empty fragment.
func TestCreatureAddButton(t *testing.T) {
	tests := []struct {
		name     string
		entityID string
		presence fakePresence
		wired    bool
		want     bool
	}{
		{"creature, foundry seen", "cr-1", fakePresence{seen: true}, true, true},
		{"creature, foundry never seen", "cr-1", fakePresence{}, true, false},
		{"not a creature", "hero-1", fakePresence{seen: true, connected: true}, true, false},
		{"not wired", "cr-1", fakePresence{seen: true, connected: true}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(nil)
			h.SetPresenceLookup(tt.presence)
			if tt.wired {
				h.SetCreatureAdd(fakeCreatureResolver{creatures: map[string]bool{"cr-1": true}}, &fakeCreatureAddPublisher{})
			}
			c, rec := creatureAddContext(http.MethodGet, tt.entityID)
			if err := h.CreatureAddButtonHandler(c); err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if got := strings.Contains(rec.Body.String(), "Add to Foundry"); got != tt.want {
				t.Errorf("button shown = %v, want %v (body %q)", got, tt.want, rec.Body.String())
			}
		})
	}
}

// Pressing it records one request whether or not Foundry is connected (a
// closed world reads it from the change feed), says which, and refuses
// pages that aren't creatures.
func TestCreatureAddAPI(t *testing.T) {
	tests := []struct {
		name       string
		entityID   string
		presence   fakePresence
		wantErr    bool
		wantSent   int
		wantStatus string
	}{
		{"connected", "cr-1", fakePresence{seen: true, connected: true}, false, 1, "Chronicle: Creatures folder"},
		{"seen but closed", "cr-1", fakePresence{seen: true}, false, 1, "next time the world is open"},
		{"not a creature", "hero-1", fakePresence{seen: true, connected: true}, true, 0, ""},
		{"never seen", "cr-1", fakePresence{}, true, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub := &fakeCreatureAddPublisher{}
			h := NewHandler(nil)
			h.SetPresenceLookup(tt.presence)
			h.SetCreatureAdd(fakeCreatureResolver{creatures: map[string]bool{"cr-1": true}}, pub)
			c, rec := creatureAddContext(http.MethodPost, tt.entityID)
			err := h.CreatureAddAPI(c)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if len(pub.sent) != tt.wantSent {
				t.Errorf("sent %v, want %d", pub.sent, tt.wantSent)
			}
			if tt.wantStatus != "" && !strings.Contains(rec.Body.String(), tt.wantStatus) {
				t.Errorf("body %q missing %q", rec.Body.String(), tt.wantStatus)
			}
		})
	}
}
