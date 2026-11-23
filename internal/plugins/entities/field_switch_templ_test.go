package entities

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func renderSwitch(t *testing.T, c templ.Component, v *FieldSwitchView) string {
	t.Helper()
	var b bytes.Buffer
	if err := c.Render(withFieldSwitchView(context.Background(), v), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestFieldSwitchTemplates pins where each answer draws: the list's offer
// only while unanswered, the Fields tab always, and the right button for the
// state.
func TestFieldSwitchTemplates(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}}
	et := &EntityType{ID: 4, NamePlural: "Items", Fields: itemFields()}
	view := func(sw *FieldSwitch) *FieldSwitchView { return switchView(et, treasureOffer(), sw) }
	tests := []struct {
		name         string
		v            *FieldSwitchView
		wantOffer    []string // in the list's offer; nil for no offer
		wantSettings []string
	}{
		{"no system offer", nil, nil, nil},
		{"unanswered", view(nil),
			[]string{"Use Draw Steel’s treasure fields for Items?", "/campaigns/c1/entity-types/4/field-switch\"", "/field-switch/dismiss", "Not now", "quiet-word"},
			[]string{"Switch to Draw Steel fields", `name="return_to" value="fields"`, "data-fs-note"}},
		{"Not now", view(&FieldSwitch{State: FieldSwitchDismissed}), nil,
			[]string{"Switch to Draw Steel fields"}},
		{"switched", view(&FieldSwitch{State: FieldSwitchSwitched, OldFields: itemFields()}), nil,
			[]string{"Items use Draw Steel’s treasure fields.", "/field-switch/back", "Switch back"}},
		{"made with them", view(&FieldSwitch{State: FieldSwitchSwitched}), nil,
			[]string{"nothing to switch back to"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			offer := renderSwitch(t, fieldSwitchOffer(cc, et), tc.v)
			if (offer != "") != (tc.wantOffer != nil) {
				t.Fatalf("offer drawn = %v, want %v: %s", offer != "", tc.wantOffer != nil, offer)
			}
			for _, w := range tc.wantOffer {
				if !strings.Contains(offer, w) {
					t.Errorf("offer lacks %q", w)
				}
			}
			settings := renderSwitch(t, fieldSwitchSettings(cc, et), tc.v)
			if (settings != "") != (tc.wantSettings != nil) {
				t.Fatalf("settings drawn = %v, want %v", settings != "", tc.wantSettings != nil)
			}
			for _, w := range tc.wantSettings {
				if !strings.Contains(settings, w) {
					t.Errorf("settings lack %q", w)
				}
			}
			if strings.Contains(settings, "Switch back") && strings.Contains(settings, "nothing to switch back to") {
				t.Error("settings offer to switch back with nothing to go back to")
			}
		})
	}
}

// TestAddFieldSwitchViewOwnerOnly pins that only the owner's page carries the
// offer: a co-DM, scribe or player never sees it.
func TestAddFieldSwitchViewOwnerOnly(t *testing.T) {
	svc, repo := newSwitchService(true)
	h := &Handler{}
	h.SetFieldSwitch(svc)
	for _, tc := range []struct {
		role campaigns.Role
		want bool
	}{
		{campaigns.RolePlayer, false},
		{campaigns.RoleScribe, false},
		{campaigns.RoleOwner, true},
	} {
		e := echo.New()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
		cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: tc.role}
		h.addFieldSwitchView(c, cc, repo.types[4])
		if got := fieldSwitchViewOf(c.Request().Context()) != nil; got != tc.want {
			t.Errorf("role %v carries the offer = %v, want %v", tc.role, got, tc.want)
		}
	}
	var unwired Handler
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	unwired.addFieldSwitchView(c, &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner}, repo.types[4])
	if fieldSwitchViewOf(c.Request().Context()) != nil {
		t.Error("unwired, no page carries the offer")
	}
}
