package entities

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeSidebarPins is an in-memory SidebarPinRepository.
type fakeSidebarPins struct {
	at   map[string]int
	next int
}

func (f *fakeSidebarPins) ListByCampaign(context.Context, string) (map[string]int, error) {
	out := make(map[string]int, len(f.at))
	for k, v := range f.at {
		out[k] = v
	}
	return out, nil
}
func (f *fakeSidebarPins) Pin(_ context.Context, _, id, _ string) error {
	if _, ok := f.at[id]; !ok {
		f.at[id] = f.next
		f.next++
	}
	return nil
}
func (f *fakeSidebarPins) Unpin(_ context.Context, _, id string) error {
	delete(f.at, id)
	return nil
}

func TestFolderService_SetSidebarPin(t *testing.T) {
	tests := []struct {
		name     string
		director bool
		id       string
		pinned   bool
		want     string
		wantErr  int
	}{
		{"a Director pins a page", true, "zeus", true, "town,zeus", 0},
		{"pinning again keeps its place", true, "town", true, "town", 0},
		{"a Director unpins", true, "town", false, "", 0},
		{"below the Director team is refused", false, "zeus", true, "town", http.StatusForbidden},
		{"a page of another campaign is not found", true, "abroad", true, "town", http.StatusNotFound},
		{"a page that does not exist is not found", true, "nope", true, "town", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := &fakeNodes{parents: map[string]*string{}, nodes: map[string]*SidebarNode{}}
			ents := &fakeFolderEntities{parent: map[string]*string{}, nodeOf: map[string]*string{}, ents: map[string]*Entity{
				"zeus": {ID: "zeus", CampaignID: "c1"}, "town": {ID: "town", CampaignID: "c1"}, "abroad": {ID: "abroad", CampaignID: "c2"},
			}}
			pins := &fakeSidebarPins{at: map[string]int{"town": 0}, next: 1}
			svc := NewFolderService(nodes, ents, &fakeFolderTypes{}, &fakeOrders{set: map[string]string{}}, pins)
			err := svc.SetSidebarPin(context.Background(), "c1", tt.id, "u1", tt.director, tt.pinned)
			if tt.wantErr != 0 {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != tt.wantErr {
					t.Fatalf("err = %v, want code %d", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			got, _ := svc.SidebarPins(context.Background(), "c1")
			order := make([]string, len(got))
			for id, p := range got {
				order[p-minPin(got)] = id
			}
			if s := strings.Join(order, ","); s != tt.want {
				t.Errorf("pins = %s, want %s", s, tt.want)
			}
		})
	}
}

func minPin(m map[string]int) int {
	lo := 0
	first := true
	for _, v := range m {
		if first || v < lo {
			lo, first = v, false
		}
	}
	return lo
}

// Who may pin a sidebar page is decided on the server: the owner and a
// member given DM access may; a Scribe, a player and a guest are refused.
func TestSetSidebarPinAPI_Roles(t *testing.T) {
	cases := []struct {
		name string
		cc   *campaigns.CampaignContext
		code int
	}{
		{"owner", &campaigns.CampaignContext{MemberRole: campaigns.RoleOwner, IsMember: true}, http.StatusOK},
		{"co-Director (DM access)", &campaigns.CampaignContext{MemberRole: campaigns.RoleScribe, IsMember: true, IsDmGranted: true}, http.StatusOK},
		{"Scribe", &campaigns.CampaignContext{MemberRole: campaigns.RoleScribe, IsMember: true}, http.StatusForbidden},
		{"player", &campaigns.CampaignContext{MemberRole: campaigns.RolePlayer, IsMember: true}, http.StatusForbidden},
		{"guest", &campaigns.CampaignContext{MemberRole: campaigns.RoleNone}, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ents := &fakeFolderEntities{parent: map[string]*string{}, nodeOf: map[string]*string{}, ents: map[string]*Entity{"zeus": {ID: "zeus", CampaignID: "c1"}}}
			pins := &fakeSidebarPins{at: map[string]int{}}
			h := &Handler{folders: NewFolderService(&fakeNodes{parents: map[string]*string{}, nodes: map[string]*SidebarNode{}}, ents, &fakeFolderTypes{}, &fakeOrders{set: map[string]string{}}, pins)}
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"entity_id":"zeus","pinned":true}`))
			req.Header.Set("Content-Type", "application/json")
			c := e.NewContext(req, httptest.NewRecorder())
			tc.cc.Campaign = &campaigns.Campaign{ID: "c1"}
			c.Set("campaign_context", tc.cc)
			err := h.SetSidebarPinAPI(c)
			code := http.StatusOK
			if err != nil {
				var ae *apperror.AppError
				if !errors.As(err, &ae) {
					t.Fatalf("unexpected error %v", err)
				}
				code = ae.Code
			}
			if code != tc.code {
				t.Fatalf("code = %d, want %d", code, tc.code)
			}
			if _, ok := pins.at["zeus"]; ok != (tc.code == http.StatusOK) {
				t.Errorf("stored pin = %v, want %v", ok, tc.code == http.StatusOK)
			}
		})
	}
}

// TestServedPinRanks pins that a reader sees pin positions with no gaps, so a
// pinned page hidden from them leaves no trace in the numbers.
func TestServedPinRanks(t *testing.T) {
	pins := map[string]int{"hidden": 0, "secret": 1, "a": 2, "b": 5}
	tests := []struct {
		name   string
		served []Entity
		want   map[string]int
	}{
		{"hidden pins leave no gap", []Entity{{ID: "b"}, {ID: "x"}, {ID: "a"}}, map[string]int{"a": 0, "b": 1}},
		{"Director sees every pin", []Entity{{ID: "a"}, {ID: "hidden"}, {ID: "secret"}, {ID: "b"}}, map[string]int{"hidden": 0, "secret": 1, "a": 2, "b": 3}},
		{"no pinned page served", []Entity{{ID: "x"}}, map[string]int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := servedPinRanks(pins, tt.served)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}
