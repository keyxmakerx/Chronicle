package entities

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// memPins is an in-memory PagePinRepository: shared pins in pin order, and
// each member's starred pages newest first.
type memPins struct {
	shared []PinnedPage
	own    map[string][]PinnedPage
	failed bool
}

func (m *memPins) Shared(context.Context, string) ([]PinnedPage, error) {
	if m.failed {
		return nil, errors.New("no table")
	}
	return append([]PinnedPage(nil), m.shared...), nil
}
func (m *memPins) IsShared(_ context.Context, _, id string) (bool, error) {
	for _, p := range m.shared {
		if p.ID == id {
			return true, nil
		}
	}
	return false, nil
}
func (m *memPins) CountShared(context.Context, string) (int, error) { return len(m.shared), nil }
func (m *memPins) Share(_ context.Context, _, id, _ string) error {
	m.shared = append(m.shared, PinnedPage{ID: id, Name: id})
	return nil
}
func (m *memPins) Unshare(_ context.Context, _, id string) error {
	kept := m.shared[:0]
	for _, p := range m.shared {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	m.shared = kept
	return nil
}
func (m *memPins) Own(_ context.Context, _, userID string) ([]PinnedPage, error) {
	return append([]PinnedPage(nil), m.own[userID]...), nil
}
func (m *memPins) RemoveOwn(_ context.Context, _, id, userID string) error {
	kept := m.own[userID][:0]
	for _, p := range m.own[userID] {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	m.own[userID] = kept
	return nil
}

// hiddenFrom answers FilterViewableEntityIDs: every page is viewable except
// the ones hidden from a viewer below the owner's role.
type hiddenFrom map[string]bool

func (h hiddenFrom) FilterViewableEntityIDs(_ context.Context, _ string, ids []string, role int, _ string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		if !h[id] || role >= int(campaigns.RoleOwner) {
			out[id] = true
		}
	}
	return out, nil
}

func pages(ids ...string) []PinnedPage {
	out := make([]PinnedPage, len(ids))
	for i, id := range ids {
		out[i] = PinnedPage{ID: id, Name: "Page " + id, TypeName: "Place"}
	}
	return out
}

func ids(ps []PinnedPage) string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = p.ID
	}
	return strings.Join(s, ",")
}

func TestPagePinService_ForViewer(t *testing.T) {
	many := make([]string, maxOwnPinsShown+3)
	for i := range many {
		many[i] = fmt.Sprintf("o%d", i)
	}
	tests := []struct {
		name        string
		repo        *memPins
		hidden      hiddenFrom
		available   bool
		viewer      PinViewer
		wantShared  string
		wantOwn     string
		wantMoreOwn int
	}{
		{
			name:       "the Director's pins sit above the viewer's own",
			repo:       &memPins{shared: pages("s1", "s2"), own: map[string][]PinnedPage{"u1": pages("o1")}},
			available:  true,
			viewer:     PinViewer{UserID: "u1", Role: int(campaigns.RolePlayer)},
			wantShared: "s1,s2", wantOwn: "o1",
		},
		{
			name:       "pages a player can't open are left out of both lists",
			repo:       &memPins{shared: pages("s1", "secret"), own: map[string][]PinnedPage{"u1": pages("o1", "hidden")}},
			hidden:     hiddenFrom{"secret": true, "hidden": true},
			available:  true,
			viewer:     PinViewer{UserID: "u1", Role: int(campaigns.RolePlayer)},
			wantShared: "s1", wantOwn: "o1",
		},
		{
			name:       "the owner sees the same pages unfiltered",
			repo:       &memPins{shared: pages("s1", "secret")},
			hidden:     hiddenFrom{"secret": true},
			available:  true,
			viewer:     PinViewer{UserID: "owner", Role: int(campaigns.RoleOwner), Director: true},
			wantShared: "s1,secret",
		},
		{
			name:       "a page pinned for everyone isn't listed again under the viewer's own",
			repo:       &memPins{shared: pages("s1"), own: map[string][]PinnedPage{"u1": pages("s1", "o1")}},
			available:  true,
			viewer:     PinViewer{UserID: "u1", Role: int(campaigns.RolePlayer)},
			wantShared: "s1", wantOwn: "o1",
		},
		{
			name:       "a visitor who isn't signed in gets only the Director's pins",
			repo:       &memPins{shared: pages("s1"), own: map[string][]PinnedPage{"": pages("o1")}},
			available:  true,
			viewer:     PinViewer{},
			wantShared: "s1",
		},
		{
			name:        "the viewer's own pins past the first ones are counted, not drawn",
			repo:        &memPins{own: map[string][]PinnedPage{"u1": pages(many...)}},
			available:   true,
			viewer:      PinViewer{UserID: "u1", Role: int(campaigns.RolePlayer)},
			wantOwn:     strings.Join(many[:maxOwnPinsShown], ","),
			wantMoreOwn: 3,
		},
		{
			name:      "without the pins table, members still get their own",
			repo:      &memPins{shared: pages("s1"), own: map[string][]PinnedPage{"u1": pages("o1")}},
			available: false,
			viewer:    PinViewer{UserID: "u1", Role: int(campaigns.RolePlayer)},
			wantOwn:   "o1",
		},
		{
			name:      "an unreadable pins table drops only the Director's pins",
			repo:      &memPins{failed: true, own: map[string][]PinnedPage{"u1": pages("o1")}},
			available: true,
			viewer:    PinViewer{UserID: "u1", Role: int(campaigns.RolePlayer)},
			wantOwn:   "o1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.repo.own == nil {
				tt.repo.own = map[string][]PinnedPage{}
			}
			svc := NewPagePinService(tt.repo, tt.hidden, tt.available)
			got, err := svc.ForViewer(context.Background(), tt.viewer)
			if err != nil {
				t.Fatalf("ForViewer: %v", err)
			}
			if ids(got.Shared) != tt.wantShared {
				t.Errorf("shared = %q, want %q", ids(got.Shared), tt.wantShared)
			}
			if ids(got.Own) != tt.wantOwn {
				t.Errorf("own = %q, want %q", ids(got.Own), tt.wantOwn)
			}
			if got.MoreOwn != tt.wantMoreOwn {
				t.Errorf("more = %d, want %d", got.MoreOwn, tt.wantMoreOwn)
			}
		})
	}
}

func TestPagePinService_SetShared(t *testing.T) {
	full := make([]string, MaxSharedPins)
	for i := range full {
		full[i] = fmt.Sprintf("s%d", i)
	}
	director := PinViewer{UserID: "dm", Role: int(campaigns.RoleOwner), Director: true}
	coDM := PinViewer{UserID: "co", Role: int(campaigns.RolePlayer), Director: true}
	tests := []struct {
		name       string
		shared     []string
		hidden     hiddenFrom
		available  bool
		viewer     PinViewer
		page       string
		on         bool
		wantCode   int
		wantShared string
	}{
		{"the Director pins a page", nil, nil, true, director, "p1", true, 0, "p1"},
		{"pinning it again changes nothing", []string{"p1"}, nil, true, director, "p1", true, 0, "p1"},
		{"the Director unpins it", []string{"p1", "p2"}, nil, true, director, "p1", false, 0, "p2"},
		{"a player may not pin for everyone", nil, nil, true, PinViewer{UserID: "u1", Role: int(campaigns.RolePlayer)}, "p1", true, http.StatusForbidden, ""},
		{"a page the pinner can't open is not found", nil, hiddenFrom{"p1": true}, true, coDM, "p1", true, http.StatusNotFound, ""},
		{"no more than the cap", full, nil, true, director, "extra", true, http.StatusBadRequest, strings.Join(full, ",")},
		{"a page already pinned at the cap can still be pinned again", full, nil, true, director, "s0", true, 0, strings.Join(full, ",")},
		{"without the pins table nothing is stored", nil, nil, false, director, "p1", true, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &memPins{shared: pages(tt.shared...), own: map[string][]PinnedPage{}}
			err := NewPagePinService(repo, tt.hidden, tt.available).SetShared(context.Background(), tt.viewer, tt.page, tt.on)
			code := 0
			var ae *apperror.AppError
			if errors.As(err, &ae) {
				code = ae.Code
			} else if err != nil {
				t.Fatalf("SetShared: %v", err)
			}
			if code != tt.wantCode {
				t.Fatalf("code = %d, want %d (err %v)", code, tt.wantCode, err)
			}
			if got := ids(repo.shared); got != tt.wantShared {
				t.Errorf("shared = %q, want %q", got, tt.wantShared)
			}
		})
	}
}

func TestPagePinService_RemoveOwn(t *testing.T) {
	repo := &memPins{own: map[string][]PinnedPage{"u1": pages("o1", "o2"), "u2": pages("o1")}}
	svc := NewPagePinService(repo, nil, true)
	if err := svc.RemoveOwn(context.Background(), PinViewer{UserID: "u1"}, "o1"); err != nil {
		t.Fatalf("RemoveOwn: %v", err)
	}
	if got := ids(repo.own["u1"]); got != "o2" {
		t.Errorf("u1 own = %q, want o2", got)
	}
	if got := ids(repo.own["u2"]); got != "o1" {
		t.Errorf("another member's pin changed: %q", got)
	}
	var ae *apperror.AppError
	if err := svc.RemoveOwn(context.Background(), PinViewer{}, "o1"); !errors.As(err, &ae) || ae.Code != http.StatusForbidden {
		t.Errorf("a visitor removing a pin: %v, want forbidden", err)
	}
}

// renderPins draws the widget the way the handler does and returns its HTML.
func renderPins(t *testing.T, p PinnedPages, director bool) string {
	t.Helper()
	var buf bytes.Buffer
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}}
	if err := PinnedPagesWidget(cc, p, director).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestPinnedPagesWidget(t *testing.T) {
	both := PinnedPages{Shared: pages("s1"), Own: pages("o1"), SharedAvailable: true}
	tests := []struct {
		name     string
		pages    PinnedPages
		director bool
		want     []string
		notWant  []string
	}{
		{
			name:     "an empty widget tells the Director how to fill it",
			pages:    PinnedPages{SharedAvailable: true},
			director: true,
			want:     []string{"Nothing pinned yet", "pin beside a page"},
		},
		{
			name:    "an empty widget shows players nothing",
			pages:   PinnedPages{SharedAvailable: true},
			notWant: []string{"Nothing pinned yet", "<h2", "<li"},
		},
		{
			name:    "a player sees the Director's pins, then their own, and can unpin only their own",
			pages:   both,
			want:    []string{"From the GM", "pinned for all", "Your pins", "Page o1", "/pinned-pages/mine/o1"},
			notWant: []string{"/pinned-pages/everyone/s1"},
		},
		{
			name:     "the Director can unpin a page for everyone",
			pages:    both,
			director: true,
			want:     []string{"/pinned-pages/everyone/s1", `aria-label="Unpin Page s1 for everyone"`},
		},
		{
			name:    "pins past the first ones are counted",
			pages:   PinnedPages{Own: pages("o1"), MoreOwn: 4},
			want:    []string{"and 4 more under Favorites"},
			notWant: []string{"From the GM"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html := renderPins(t, tt.pages, tt.director)
			if !strings.Contains(html, `id="pinned-pages"`) {
				t.Errorf("the widget lost the id its × buttons swap: %s", html)
			}
			for _, w := range tt.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q in %s", w, html)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(html, w) {
					t.Errorf("unexpected %q in %s", w, html)
				}
			}
		})
	}
}

func TestPagePinHandler_Widget(t *testing.T) {
	repo := &memPins{shared: pages("s1", "secret"), own: map[string][]PinnedPage{"owner": pages("o1")}}
	h := NewPagePinHandler(NewPagePinService(repo, hiddenFrom{"secret": true}, true))
	tests := []struct {
		name     string
		asPlayer bool
		want     []string
		notWant  []string
	}{
		{"the owner sees every pin and can unpin for everyone", false, []string{"Page secret", "Page o1", "/pinned-pages/everyone/s1"}, nil},
		{"looking as a player hides what players can't open, and the Director's ×", true, []string{"Page s1", "Page o1"}, []string{"Page secret", "/pinned-pages/everyone/"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/pinned-pages", nil)
			if tt.asPlayer {
				req.AddCookie(&http.Cookie{Name: "chronicle_view_as_player", Value: "1"})
			}
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues("camp-1")
			c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner})
			c.Set("auth_user_id", "owner")
			if err := h.Widget(c); err != nil {
				t.Fatalf("Widget: %v", err)
			}
			body := rec.Body.String()
			for _, w := range tt.want {
				if !strings.Contains(body, w) {
					t.Errorf("missing %q in %s", w, body)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(body, w) {
					t.Errorf("unexpected %q in %s", w, body)
				}
			}
		})
	}
}
