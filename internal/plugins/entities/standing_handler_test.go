package entities

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// standingHandlerEnv is the Standing handler over the real entity service (so
// "a page this player may not open" is the code that ships) and the real
// standing service on an in-memory repository holding a Director-only reason.
func standingHandlerEnv(t *testing.T) (*StandingHandler, *fakeStandingRepo) {
	t.Helper()
	repo := seededStanding()
	for i := range repo.rows {
		repo.rows[i].EntityID = "npc-page"
		repo.rows[i].CampaignID = "c1"
	}
	entitySvc := advHandlerEnv().service
	return NewStandingHandler(NewStandingService(repo, entitySvc), entitySvc), repo
}

func standingReq(role campaigns.Role, dmGranted bool, method, page, subject, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: role, IsDmGranted: dmGranted, IsMember: true,
	})
	auth.SetSession(c, &auth.Session{UserID: "u1"})
	c.SetParamNames("id", "eid", "subject")
	c.SetParamValues("c1", page, subject)
	return c, rec
}

func TestStandingGet_WhatEachRoleIsSent(t *testing.T) {
	tests := []struct {
		name       string
		role       campaigns.Role
		dm         bool
		page       string
		subject    string
		switchOff  bool
		wantCode   int
		wantSecret bool
		wantFlag   bool
	}{
		{name: "player: public reasons only", role: campaigns.RolePlayer, page: "npc-page", subject: "party", wantCode: 200},
		{name: "director: sees the secret, flagged", role: campaigns.RoleOwner, page: "npc-page", subject: "party", wantCode: 200, wantSecret: true, wantFlag: true},
		{name: "co-director sees what the owner sees", role: campaigns.RolePlayer, dm: true, page: "npc-page", subject: "party", wantCode: 200, wantSecret: true, wantFlag: true},
		{name: "player: switch off is a 404", role: campaigns.RolePlayer, page: "npc-page", subject: "party", switchOff: true, wantCode: 404},
		{name: "player: a page they cannot open is a 404", role: campaigns.RolePlayer, page: "adv-page", subject: "party", wantCode: 404},
		{name: "player: a page that does not exist is a 404", role: campaigns.RolePlayer, page: "nope", subject: "party", wantCode: 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, repo := standingHandlerEnv(t)
			if tt.switchOff {
				repo.playersSee["npc-page"] = false
			}
			c, rec := standingReq(tt.role, tt.dm, http.MethodGet, tt.page, tt.subject, "")
			err := h.Get(c)
			if tt.wantCode != 200 {
				if err == nil {
					t.Fatalf("status %d, want an error with code %d", rec.Code, tt.wantCode)
				}
				if !notFound(err) {
					t.Fatalf("err = %v, want 404", err)
				}
				if strings.Contains(rec.Body.String(), secret) {
					t.Error("an error response carried the secret")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			body := rec.Body.String()
			if strings.Contains(body, secret) != tt.wantSecret {
				t.Errorf("secret present = %v, want %v: %s", !tt.wantSecret, tt.wantSecret, body)
			}
			if strings.Contains(body, `"dmOnly"`) != tt.wantFlag {
				t.Errorf("dmOnly flag present = %v, want %v: %s", !tt.wantFlag, tt.wantFlag, body)
			}
		})
	}
}

func TestStandingWrites_AreTheDirectors(t *testing.T) {
	post := `{"direction":"up","text":"Paid the debt","when":"session 13","dmOnly":true,"level":3}`
	tests := []struct {
		name     string
		role     campaigns.Role
		dm       bool
		page     string
		do       func(*StandingHandler, echo.Context) error
		method   string
		body     string
		wantCode int
	}{
		{name: "player cannot add", role: campaigns.RolePlayer, page: "npc-page", method: http.MethodPost, body: post, wantCode: 403,
			do: (*StandingHandler).AddReason},
		{name: "scribe cannot add", role: campaigns.RoleScribe, page: "npc-page", method: http.MethodPost, body: post, wantCode: 403,
			do: (*StandingHandler).AddReason},
		{name: "player cannot remove", role: campaigns.RolePlayer, page: "npc-page", method: http.MethodDelete, wantCode: 403,
			do: (*StandingHandler).RemoveReason},
		{name: "player cannot flip the switch", role: campaigns.RolePlayer, page: "npc-page", method: http.MethodPut, body: `{"playersSee":false}`, wantCode: 403,
			do: (*StandingHandler).SetSettings},
		{name: "director adds", role: campaigns.RoleOwner, page: "npc-page", method: http.MethodPost, body: post, wantCode: 201,
			do: (*StandingHandler).AddReason},
		{name: "co-director adds", role: campaigns.RolePlayer, dm: true, page: "npc-page", method: http.MethodPost, body: post, wantCode: 201,
			do: (*StandingHandler).AddReason},
		{name: "a bad body is a 400", role: campaigns.RoleOwner, page: "npc-page", method: http.MethodPost, body: `{`, wantCode: 400,
			do: (*StandingHandler).AddReason},
		{name: "the switch needs its value", role: campaigns.RoleOwner, page: "npc-page", method: http.MethodPut, body: `{}`, wantCode: 400,
			do: (*StandingHandler).SetSettings},
		{name: "director: a page they cannot open is a 404", role: campaigns.RoleOwner, page: "nope", method: http.MethodPost, body: post, wantCode: 404,
			do: (*StandingHandler).AddReason},
		{name: "director flips the switch", role: campaigns.RoleOwner, page: "npc-page", method: http.MethodPut, body: `{"playersSee":false}`, wantCode: 200,
			do: (*StandingHandler).SetSettings},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, repo := standingHandlerEnv(t)
			nReasons := len(repo.reasons)
			c, rec := standingReq(tt.role, tt.dm, tt.method, tt.page, "party", tt.body)
			c.SetParamNames("id", "eid", "subject", "rid")
			c.SetParamValues("c1", tt.page, "party", uuidFor("r1"))
			err := tt.do(h, c)
			code := rec.Code
			if err != nil {
				code = errCode(err)
			}
			if code != tt.wantCode {
				t.Fatalf("status = %d (err %v), want %d", code, err, tt.wantCode)
			}
			if tt.wantCode >= 400 {
				if len(repo.reasons) != nReasons {
					t.Error("a refused write still changed the reasons")
				}
				if v, _ := repo.PlayersSee(c.Request().Context(), "npc-page"); !v {
					t.Error("a refused write still turned the switch off")
				}
			}
		})
	}
}

// errCode is the HTTP status an error would be answered with.
func errCode(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	var he *echo.HTTPError
	if errors.As(err, &he) {
		return he.Code
	}
	return http.StatusInternalServerError
}

// The flow a Director and a player live through: the Director adds a
// DM-only reason, and the player's next read never has it.
func TestStandingFlow_DMOnlyReasonNeverReachesAPlayer(t *testing.T) {
	h, _ := standingHandlerEnv(t)
	c, rec := standingReq(campaigns.RoleOwner, false, http.MethodPost, "npc-page", "party",
		`{"direction":"down","text":"Sold a map of the towers","when":"session 13","dmOnly":true,"level":0}`)
	if err := h.AddReason(c); err != nil {
		t.Fatal(err)
	}
	var added StandingSubjectView
	if err := json.Unmarshal(rec.Body.Bytes(), &added); err != nil {
		t.Fatal(err)
	}
	if added.Word != "Hostile" || len(added.Reasons) == 0 || !added.Reasons[0].DMOnly {
		t.Fatalf("director's answer = %+v", added)
	}

	pc, prec := standingReq(campaigns.RolePlayer, false, http.MethodGet, "npc-page", "party", "")
	if err := h.Get(pc); err != nil {
		t.Fatal(err)
	}
	if body := prec.Body.String(); strings.Contains(body, "Sold a map") || strings.Contains(body, secret) {
		t.Errorf("player's read carries Director-only text: %s", body)
	}
	var seen StandingSubjectView
	_ = json.Unmarshal(prec.Body.Bytes(), &seen)
	if seen.Word != "Hostile" {
		t.Errorf("player's word = %q; the word itself is theirs to see", seen.Word)
	}
}

// directorOnly is the route's early refusal; the service is the one that
// cannot be skipped, but the gate must still turn players away.
func TestStandingDirectorOnlyGate(t *testing.T) {
	tests := []struct {
		name string
		cc   *campaigns.CampaignContext
		want int
	}{
		{"owner passes", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner, IsMember: true}, 200},
		{"granted co-director passes", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsDmGranted: true, IsMember: true}, 200},
		{"scribe is turned away", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleScribe, IsMember: true}, 403},
		{"player is turned away", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsMember: true}, 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/x", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set("campaign_context", tt.cc)
			err := directorOnly()(func(c echo.Context) error { return c.NoContent(200) })(c)
			code := rec.Code
			if err != nil {
				code = errCode(err)
			}
			if code != tt.want {
				t.Fatalf("status = %d (err %v), want %d", code, err, tt.want)
			}
		})
	}
}
