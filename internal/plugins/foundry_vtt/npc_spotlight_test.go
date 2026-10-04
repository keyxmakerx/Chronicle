package foundry_vtt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type fakeNPCResolver struct{ npcs map[string]bool }

func (f fakeNPCResolver) NPCName(_ context.Context, campaignID, entityID string) (string, bool, error) {
	if campaignID != "camp-1" {
		return "", false, nil
	}
	return "Varra", f.npcs[entityID], nil
}

type fakeSpotlightPublisher struct{ sent []string }

func (f *fakeSpotlightPublisher) PublishNPCSpotlight(campaignID, entityID string) {
	f.sent = append(f.sent, campaignID+"/"+entityID)
}

type fakePresence struct {
	seen      bool
	connected bool
}

func (f fakePresence) FoundryPresence(string) (*time.Time, bool) {
	if !f.seen {
		return nil, false
	}
	t := time.Now()
	return &t, f.connected
}

func spotlightContext(method, entityID string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, "/campaigns/camp-1/foundry-vtt/npc-spotlight/"+entityID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "eid")
	c.SetParamValues("camp-1", entityID)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RoleOwner,
		IsMember:   true,
	})
	return c, rec
}

// The button shows only on an NPC page of a campaign that has used
// Foundry; everything else gets an empty fragment.
func TestNPCSpotlightButton(t *testing.T) {
	tests := []struct {
		name     string
		entityID string
		presence fakePresence
		wired    bool
		want     bool
	}{
		{"npc, foundry seen", "npc-1", fakePresence{seen: true}, true, true},
		{"npc, foundry never seen", "npc-1", fakePresence{}, true, false},
		{"not an npc", "loc-1", fakePresence{seen: true, connected: true}, true, false},
		{"not wired", "npc-1", fakePresence{seen: true, connected: true}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(nil)
			h.SetPresenceLookup(tt.presence)
			if tt.wired {
				h.SetNPCSpotlight(fakeNPCResolver{npcs: map[string]bool{"npc-1": true}}, &fakeSpotlightPublisher{})
			}
			c, rec := spotlightContext(http.MethodGet, tt.entityID)
			if err := h.NPCSpotlightButtonHandler(c); err != nil {
				t.Fatalf("handler error: %v", err)
			}
			got := strings.Contains(rec.Body.String(), "Show in Foundry")
			if got != tt.want {
				t.Errorf("button shown = %v, want %v (body %q)", got, tt.want, rec.Body.String())
			}
		})
	}
}

// Pressing it sends one GM-only spotlight when Foundry is connected,
// sends nothing otherwise, and refuses entities that aren't NPCs.
func TestNPCSpotlightAPI(t *testing.T) {
	tests := []struct {
		name       string
		entityID   string
		presence   fakePresence
		wantErr    bool
		wantSent   int
		wantStatus string
	}{
		{"connected", "npc-1", fakePresence{seen: true, connected: true}, false, 1, "Sent to Foundry."},
		{"seen but offline", "npc-1", fakePresence{seen: true}, false, 0, "isn&#39;t connected"},
		{"not an npc", "loc-1", fakePresence{seen: true, connected: true}, true, 0, ""},
		{"never seen", "npc-1", fakePresence{}, true, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub := &fakeSpotlightPublisher{}
			h := NewHandler(nil)
			h.SetPresenceLookup(tt.presence)
			h.SetNPCSpotlight(fakeNPCResolver{npcs: map[string]bool{"npc-1": true}}, pub)
			c, rec := spotlightContext(http.MethodPost, tt.entityID)
			err := h.NPCSpotlightAPI(c)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if len(pub.sent) != tt.wantSent {
				t.Errorf("sent %v, want %d", pub.sent, tt.wantSent)
			}
			if tt.wantSent == 1 && pub.sent[0] != "camp-1/npc-1" {
				t.Errorf("sent %q, want camp-1/npc-1", pub.sent[0])
			}
			if tt.wantStatus != "" && !strings.Contains(rec.Body.String(), tt.wantStatus) {
				t.Errorf("body %q missing %q", rec.Body.String(), tt.wantStatus)
			}
		})
	}
}

// Both routes sit behind the owner-or-DM-access gate.
func TestNPCSpotlightRoutes_DMTeamOnly(t *testing.T) {
	src, err := readSource("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, want := range []string{
		`cg.GET("/foundry-vtt/npc-spotlight/:eid", h.NPCSpotlightButtonHandler, dmTeam)`,
		`cg.POST("/foundry-vtt/npc-spotlight/:eid", h.NPCSpotlightAPI, dmTeam)`,
		`return cc.CanAuthorDmOnly()`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("routes.go missing %s", want)
		}
	}
}
