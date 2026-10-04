package systemstate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeAccess models the entities visibility gate: entity -> owning campaign,
// plus a minimum visibility role needed to view it.
type fakeAccess struct {
	campaign map[string]string
	minRole  map[string]int
}

func (f fakeAccess) ResolveViewableEntity(_ context.Context, entityID string, role int, _ string) (string, bool, error) {
	c, ok := f.campaign[entityID]
	if !ok {
		return "", false, apperror.NewNotFound("entity not found")
	}
	return c, role >= f.minRole[entityID], nil
}

func newTestHandler() *Handler {
	svc, _, _ := newTestService()
	return NewHandler(svc, fakeAccess{
		campaign: map[string]string{"npc-1": "camp-1", "hidden-npc": "camp-1", "other-npc": "camp-2"},
		minRole:  map[string]int{"hidden-npc": int(campaigns.RoleOwner)},
	})
}

func callState(h *Handler, method, entityID string, cc *campaigns.CampaignContext, body string) (*httptest.ResponseRecorder, error) {
	e := echo.New()
	req := httptest.NewRequest(method, "/campaigns/camp-1/entities/"+entityID+"/system-state/drawsteel/negotiation", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "eid", "system", "key")
	c.SetParamValues("camp-1", entityID, "drawsteel", "negotiation")
	c.Set("campaign_context", cc)
	var err error
	if method == http.MethodPut {
		err = h.Put(c)
	} else {
		err = h.Get(c)
	}
	return rec, err
}

func ctxFor(role campaigns.Role, dmGrant bool) *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1"},
		MemberRole:  role,
		IsMember:    true,
		IsDmGranted: dmGrant,
	}
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return m
}

// The gm half reaches only the owner and DM-access members; players and
// scribes without DM access get the public half and no gm member at all.
func TestHandler_RoleSplit(t *testing.T) {
	tests := []struct {
		name   string
		cc     *campaigns.CampaignContext
		wantGM bool
	}{
		{"player", ctxFor(campaigns.RolePlayer, false), false},
		{"scribe without dm access", ctxFor(campaigns.RoleScribe, false), false},
		{"player with dm access", ctxFor(campaigns.RolePlayer, true), true},
		{"scribe with dm access", ctxFor(campaigns.RoleScribe, true), true},
		{"owner", ctxFor(campaigns.RoleOwner, false), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler()
			// Seed through an owner PUT.
			if _, err := callState(h, http.MethodPut, "npc-1", ctxFor(campaigns.RoleOwner, false),
				`{"gm":{"secret":"s"},"public":{"round":2}}`); err != nil {
				t.Fatal(err)
			}
			rec, err := callState(h, http.MethodGet, "npc-1", tt.cc, "")
			if err != nil {
				t.Fatal(err)
			}
			m := decode(t, rec)
			_, hasGM := m["gm"]
			if hasGM != tt.wantGM {
				t.Fatalf("gm present = %v, want %v (body %s)", hasGM, tt.wantGM, rec.Body.String())
			}
			if !tt.wantGM && strings.Contains(rec.Body.String(), "secret") {
				t.Errorf("gm content leaked: %s", rec.Body.String())
			}
			if string(m["public"]) != `{"round":2}` {
				t.Errorf("public = %s", m["public"])
			}
			if string(m["isGm"]) != map[bool]string{true: "true", false: "false"}[tt.wantGM] {
				t.Errorf("isGm = %s", m["isGm"])
			}
			if string(m["systemId"]) != `"drawsteel"` || string(m["key"]) != `"negotiation"` {
				t.Errorf("ids = %s %s", m["systemId"], m["key"])
			}
		})
	}
}

// A PUT without DM access is refused and writes nothing, even if the route
// gate were bypassed.
func TestHandler_PutRequiresDMTeam(t *testing.T) {
	for name, cc := range map[string]*campaigns.CampaignContext{
		"player": ctxFor(campaigns.RolePlayer, false),
		"scribe": ctxFor(campaigns.RoleScribe, false),
	} {
		t.Run(name, func(t *testing.T) {
			h := newTestHandler()
			_, err := callState(h, http.MethodPut, "npc-1", cc, `{"public":{"a":1}}`)
			if errCode(err) != http.StatusForbidden {
				t.Fatalf("code = %d (err %v), want 403", errCode(err), err)
			}
			rec, gerr := callState(h, http.MethodGet, "npc-1", ctxFor(campaigns.RoleOwner, false), "")
			if gerr != nil {
				t.Fatal(gerr)
			}
			if string(decode(t, rec)["public"]) != `{}` {
				t.Errorf("rejected PUT stored data: %s", rec.Body.String())
			}
		})
	}
}

// An entity id from another campaign, and a page the viewer cannot see, are
// both a plain 404 on read and on write.
func TestHandler_IDORAndVisibility(t *testing.T) {
	tests := []struct {
		name   string
		method string
		entity string
		cc     *campaigns.CampaignContext
	}{
		{"get other campaign entity", http.MethodGet, "other-npc", ctxFor(campaigns.RoleOwner, false)},
		{"put other campaign entity", http.MethodPut, "other-npc", ctxFor(campaigns.RoleOwner, false)},
		{"get unknown entity", http.MethodGet, "ghost", ctxFor(campaigns.RoleOwner, false)},
		{"player get hidden page", http.MethodGet, "hidden-npc", ctxFor(campaigns.RolePlayer, false)},
		{"scribe get hidden page", http.MethodGet, "hidden-npc", ctxFor(campaigns.RoleScribe, false)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := callState(newTestHandler(), tt.method, tt.entity, tt.cc, `{"public":{"a":1}}`)
			if errCode(err) != http.StatusNotFound {
				t.Fatalf("code = %d (err %v), want 404", errCode(err), err)
			}
		})
	}
}

func TestHandler_PutBody(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{"public only", `{"public":{"a":1}}`, 0},
		{"gm only", `{"gm":{"a":1}}`, 0},
		{"empty object body changes nothing", `{}`, 0},
		{"null half is rejected, not read as absent", `{"public":null}`, http.StatusUnprocessableEntity},
		{"array half", `{"gm":[]}`, http.StatusUnprocessableEntity},
		{"unknown member", `{"publc":{"a":1}}`, http.StatusUnprocessableEntity},
		{"body is an array", `[]`, http.StatusBadRequest},
		{"body is not json", `nope`, http.StatusBadRequest},
		{"body too large", `{"public":{"k":"` + strings.Repeat("x", 3*MaxHalfBytes) + `"}}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := callState(newTestHandler(), http.MethodPut, "npc-1", ctxFor(campaigns.RoleOwner, false), tt.body)
			if got := errCode(err); got != tt.wantCode {
				t.Fatalf("code = %d (err %v), want %d", got, err, tt.wantCode)
			}
			if tt.wantCode == 0 && rec.Code != http.StatusOK {
				t.Errorf("status = %d", rec.Code)
			}
		})
	}
}

// Routes are the only place the write gate lives; pin it like the NPC
// spotlight routes are pinned.
func TestSystemStateRoutes_WriteIsDMTeamOnly(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, want := range []string{
		`cg.GET("/entities/:eid/system-state/:system/:key", h.Get)`,
		`cg.PUT("/entities/:eid/system-state/:system/:key", h.Put, dmTeam)`,
		`return cc.CanAuthorDmOnly()`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("routes.go missing %s", want)
		}
	}
}
