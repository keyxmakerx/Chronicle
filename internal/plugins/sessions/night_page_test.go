package sessions

// The calendar's night page reads a night's recap and linked pages from
// NightPageAPI. Its linked pages go through the same visibility filter as
// the Sessions page (ADR-055 rule 3): a page the viewer can't open is
// absent, not named.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func nightPageFor(t *testing.T, role campaigns.Role) nightPage {
	t.Helper()
	recap, recapHTML := "The vault opened.", "<p>The vault opened.</p>"
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			return &Session{ID: id, CampaignID: "camp-1", Name: "Session One", Status: StatusPlanned, Recap: &recap, RecapHTML: &recapHTML}, nil
		},
		listSessionEntitiesFn: func(_ context.Context, _ string) ([]SessionEntity, error) {
			return []SessionEntity{
				{EntityID: "ent-secret", EntityName: "Secret Villain", EntitySlug: "secret-villain", Role: EntityRoleEncountered},
				{EntityID: "ent-public", EntityName: "Town Square", EntitySlug: "town-square", Role: EntityRoleMentioned},
			}, nil
		},
	}
	h := NewHandler(NewSessionService(repo, nil, &stubEntityVisibility{hidden: map[string]bool{"ent-secret": true}}))

	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/sessions/s1/page", nil), rec)
	c.SetParamNames("id", "sid")
	c.SetParamValues("camp-1", "s1")
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role})
	if err := h.NightPageAPI(c); err != nil {
		t.Fatalf("NightPageAPI: %v", err)
	}
	var out nightPage
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return out
}

func TestNightPageAPI(t *testing.T) {
	tests := []struct {
		name  string
		role  campaigns.Role
		links []string
	}{
		{"a player sees only pages they can open", campaigns.RolePlayer, []string{"town-square"}},
		{"a scribe sees every linked page", campaigns.RoleScribe, []string{"secret-villain", "town-square"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := nightPageFor(t, tt.role)
			if p.RecapHTML != "<p>The vault opened.</p>" || p.Recap != "The vault opened." {
				t.Errorf("recap = %q / %q", p.Recap, p.RecapHTML)
			}
			var got []string
			for _, l := range p.Links {
				got = append(got, l.Slug)
			}
			if len(got) != len(tt.links) {
				t.Fatalf("links = %v, want %v", got, tt.links)
			}
			for i := range got {
				if got[i] != tt.links[i] {
					t.Fatalf("links = %v, want %v", got, tt.links)
				}
			}
		})
	}
}

// A night's start time changed in the calendar carries the zone it is in;
// a value that is not a real zone is refused, and an absent one keeps the
// stored zone.
func TestUpdateSession_ScheduledTZ(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
		wantTZ  string
	}{
		{"a real zone is stored", `{"scheduled_time":"20:00","scheduled_tz":"America/Chicago"}`, false, "America/Chicago"},
		{"absent keeps the stored zone", `{"name":"Renamed"}`, false, "Europe/London"},
		{"null clears it", `{"scheduled_tz":null}`, false, ""},
		{"not a zone is refused", `{"scheduled_tz":"Mars/Olympus"}`, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req updateSessionRequest
			if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
				t.Fatalf("decode: %v", err)
			}
			var written *Session
			repo := &mockSessionRepo{
				findByIDFn: func(_ context.Context, _ string) (*Session, error) {
					s := storedSession()
					tz := "Europe/London"
					s.ScheduledTZ = &tz
					return s, nil
				},
				updateFn: func(_ context.Context, s *Session) error { written = s; return nil },
			}
			_, err := newTestSessionService(repo).UpdateSession(context.Background(), "sess-1", req.toInput())
			if tt.wantErr {
				if err == nil {
					t.Fatal("want an error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateSession: %v", err)
			}
			if got := strPtrVal(written.ScheduledTZ); got != tt.wantTZ {
				t.Errorf("ScheduledTZ = %q, want %q", got, tt.wantTZ)
			}
		})
	}
}
