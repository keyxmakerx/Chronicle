package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

var sinceNow = time.Date(2026, time.November, 14, 12, 0, 0, 0, time.UTC) // a Saturday

func at(daysAgo int) time.Time { return sinceNow.AddDate(0, 0, -daysAgo) }

// fakeSincePages answers as the entity visibility predicate would: a viewer
// below the DM team never gets the page flagged hidden, in the count or the
// names.
type fakeSincePages struct {
	pages []entities.PageSinceRef
	// hidden marks page ids only the DM team may open.
	hidden map[string]bool
	err    error

	gotRole  int
	gotSince time.Time
}

func (f *fakeSincePages) Since(_ context.Context, _ string, since time.Time, role int, _ string, limit int) (entities.PagesSince, error) {
	f.gotRole, f.gotSince = role, since
	if f.err != nil {
		return entities.PagesSince{}, f.err
	}
	var out entities.PagesSince
	for _, p := range f.pages {
		if f.hidden[p.ID] && role < int(campaigns.RoleOwner) {
			continue
		}
		out.Count++
		if len(out.Newest) < limit {
			out.Newest = append(out.Newest, p)
		}
	}
	return out, nil
}

type fakeSinceNights struct {
	recaps, nights, polls []sessions.SinceItem
	recapErr              error
	gotViewer             sessions.SinceViewer
	read                  bool
}

func (f *fakeSinceNights) RecapsSince(_ context.Context, _ string, _ time.Time, v sessions.SinceViewer) ([]sessions.SinceItem, error) {
	f.read, f.gotViewer = true, v
	return f.recaps, f.recapErr
}
func (f *fakeSinceNights) NightsSince(context.Context, string, time.Time, string) ([]sessions.SinceItem, error) {
	f.read = true
	return f.nights, nil
}
func (f *fakeSinceNights) PollsSince(context.Context, string, time.Time) ([]sessions.SinceItem, error) {
	f.read = true
	return f.polls, nil
}

type fakeSinceQuests struct {
	changes *quests.QuestChanges
	got     quests.Viewer
}

func (f *fakeSinceQuests) ChangesSince(_ context.Context, _ string, v quests.Viewer, _ time.Time) (*quests.QuestChanges, error) {
	f.got = v
	return f.changes, nil
}

type fakeMarkers struct {
	seen   campaigns.HomeSeen
	member bool
	marked []time.Time
}

func (f *fakeMarkers) Get(context.Context, string, string) (campaigns.HomeSeen, bool, error) {
	return f.seen, f.member, nil
}
func (f *fakeMarkers) Mark(_ context.Context, _, _ string, at time.Time) (bool, error) {
	if !f.member {
		return false, nil
	}
	f.marked = append(f.marked, at)
	return true, nil
}

func nightsOn(context.Context, string) bool { return true }

func TestSinceBuild(t *testing.T) {
	req := sinceRequest{CampaignID: "c1", Role: int(campaigns.RolePlayer), From: at(10), Now: sinceNow}
	pages := &fakeSincePages{pages: []entities.PageSinceRef{
		{ID: "p1", Name: "Harrowgate", CreatedAt: at(0)},
		{ID: "p2", Name: "The Iron Pact", CreatedAt: at(1)},
		{ID: "p3", Name: "Brindle", CreatedAt: at(2)},
		{ID: "p4", Name: "Kessrin Gate", CreatedAt: at(3)},
		{ID: "p5", Name: "The Sunken Abbey", CreatedAt: at(4)},
	}}

	t.Run("names three then and N more, newest group first", func(t *testing.T) {
		nights := &fakeSinceNights{recaps: []sessions.SinceItem{{ID: "s1", Name: "The Drowned Archive", At: at(1)}}}
		rows, failed := sinceBuild(context.Background(), sinceSources{Pages: pages, Nights: nights, NightsOn: nightsOn}, req)
		if failed || len(rows) != 2 {
			t.Fatalf("rows=%d failed=%v", len(rows), failed)
		}
		if rows[0].Bold != "5 new pages" || rows[0].Sub != "Harrowgate, The Iron Pact, Brindle and 2 more" || rows[0].Day != "Sat" {
			t.Errorf("pages row = %+v", rows[0])
		}
		if rows[1].Bold != "Recap posted" || rows[1].Sub != "The Drowned Archive" || rows[1].Href != "/campaigns/c1/sessions/s1" || rows[1].Day != "Fri" {
			t.Errorf("recap row = %+v", rows[1])
		}
	})

	t.Run("one page links to the page", func(t *testing.T) {
		one := &fakeSincePages{pages: pages.pages[:1]}
		rows, _ := sinceBuild(context.Background(), sinceSources{Pages: one}, req)
		if len(rows) != 1 || rows[0].Bold != "1 new page" || rows[0].Href != "/campaigns/c1/entities/p1" {
			t.Errorf("rows = %+v", rows)
		}
	})

	t.Run("a failing source leaves out its own row only", func(t *testing.T) {
		nights := &fakeSinceNights{recapErr: errors.New("db down"), polls: []sessions.SinceItem{{ID: "q1", Name: "Pick a Thursday", At: at(2)}}}
		rows, failed := sinceBuild(context.Background(), sinceSources{Pages: pages, Nights: nights, NightsOn: nightsOn}, req)
		if !failed || len(rows) != 2 {
			t.Fatalf("rows=%d failed=%v", len(rows), failed)
		}
		for _, r := range rows {
			if strings.Contains(r.Bold, "Recap") {
				t.Errorf("failed recap source still drew a row: %+v", r)
			}
		}
	})

	t.Run("at most five rows", func(t *testing.T) {
		nights := &fakeSinceNights{
			recaps: []sessions.SinceItem{{ID: "s1", Name: "A", At: at(1)}},
			nights: []sessions.SinceItem{{ID: "s2", Name: "B", Date: "2026-11-20", At: at(2)}},
			polls:  []sessions.SinceItem{{ID: "s3", Name: "C", At: at(3)}},
		}
		qs := &fakeSinceQuests{changes: &quests.QuestChanges{
			New:      []quests.QuestChange{{EntityID: "q1", Title: "Find the bell", Saved: at(4)}},
			Finished: []quests.QuestChange{{EntityID: "q2", Title: "Escort", Saved: at(5)}},
			TypeSlug: "quest",
		}}
		rows, _ := sinceBuild(context.Background(), sinceSources{Pages: pages, Nights: nights, Quests: qs, NightsOn: nightsOn}, req)
		if len(rows) != 5 {
			t.Fatalf("rows = %d, want 5", len(rows))
		}
		for _, r := range rows {
			if r.Bold == "Quest finished" {
				t.Error("the oldest group should have been the one cut")
			}
		}
		if rows[0].Bold != "5 new pages" {
			t.Errorf("newest group first, got %q", rows[0].Bold)
		}
	})

	t.Run("nights word the date, game nights off reads no night source", func(t *testing.T) {
		nights := &fakeSinceNights{nights: []sessions.SinceItem{{ID: "s2", Name: "Nine Dark Nights", Date: "2026-11-20", At: at(1)}}}
		rows, _ := sinceBuild(context.Background(), sinceSources{Nights: nights, NightsOn: nightsOn}, req)
		if len(rows) != 1 || rows[0].Bold != "New game night set" || rows[0].Sub != "Nine Dark Nights, 20 November" {
			t.Errorf("rows = %+v", rows)
		}
		off := &fakeSinceNights{nights: nights.nights}
		rows, _ = sinceBuild(context.Background(), sinceSources{Nights: off, NightsOn: func(context.Context, string) bool { return false }}, req)
		if len(rows) != 0 || off.read {
			t.Errorf("game nights off: rows=%d read=%v", len(rows), off.read)
		}
	})
}

func TestSinceEffectiveFrom(t *testing.T) {
	tests := []struct {
		name string
		seen time.Time
		want time.Time
	}{
		{"recent marker stands", at(2), at(2)},
		{"capped at thirty days", at(90), sinceNow.Add(-sinceLookBack)},
		{"never in the future", sinceNow.Add(time.Hour), sinceNow},
	}
	for _, tt := range tests {
		if got := sinceEffectiveFrom(tt.seen, sinceNow); !got.Equal(tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestSinceCaughtUp(t *testing.T) {
	tests := []struct {
		name string
		from time.Time
		want string
	}{
		{"today", sinceNow.Add(-time.Hour), "You're all caught up"},
		{"this week", at(4), "You're all caught up since Tuesday"},
		{"older than a week", at(10), "You're all caught up since 4 November"},
	}
	for _, tt := range tests {
		if got := sinceCaughtUp(tt.from, sinceNow, nil); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func sinceCtx(t *testing.T, method, path string, cc *campaigns.CampaignContext, user string, cookies ...*http.Cookie) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("campaign_context", cc)
	if user != "" {
		c.Set("auth_user_id", user)
	}
	return c, rec
}

func newSinceHandler(m *fakeMarkers, src sinceSources) *sinceHandler {
	return &sinceHandler{src: src, markers: m, now: func() time.Time { return sinceNow }}
}

// A guest, an anonymous visitor and a non-member always get an empty body,
// and none of the sources is read for them.
func TestSinceHandler_GuestsGetNothing(t *testing.T) {
	pages := &fakeSincePages{pages: []entities.PageSinceRef{{ID: "p1", Name: "Harrowgate", CreatedAt: at(1)}}}
	nights := &fakeSinceNights{recaps: []sessions.SinceItem{{ID: "s1", Name: "Secret Night", At: at(1)}}}
	tests := []struct {
		name   string
		cc     *campaigns.CampaignContext
		userID string
	}{
		{"guest of a public campaign", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", IsPublic: true}}, ""},
		{"signed in but not a member", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", IsPublic: true}}, "stranger"},
		{"anonymous with member flag", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, IsMember: true, MemberRole: campaigns.RolePlayer}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newSinceHandler(&fakeMarkers{member: true}, sinceSources{Pages: pages, Nights: nights, NightsOn: nightsOn})
			c, rec := sinceCtx(t, http.MethodGet, "/campaigns/c1/since-last-here", tt.cc, tt.userID)
			if err := h.Show(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
				t.Errorf("status %d body %q, want empty 200", rec.Code, rec.Body.String())
			}
			if nights.read {
				t.Error("a source was read for a non-member")
			}
		})
	}
}

// A player whose campaign holds a hidden page and a hidden quest made after
// their marker sees no trace of either: not the name, not in the count.
func TestSinceHandler_HiddenContentLeavesNoTrace(t *testing.T) {
	pages := &fakeSincePages{
		pages: []entities.PageSinceRef{
			{ID: "pub", Name: "Harrowgate", CreatedAt: at(1)},
			{ID: "sec", Name: "Cult Vault Beneath Harrowgate", CreatedAt: at(0)},
		},
		hidden: map[string]bool{"sec": true},
	}
	member := func(role campaigns.Role, dm bool) *campaigns.CampaignContext {
		return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, IsMember: true, MemberRole: role, IsDmGranted: dm}
	}
	run := func(cc *campaigns.CampaignContext, cookies ...*http.Cookie) string {
		h := newSinceHandler(&fakeMarkers{member: true, seen: campaigns.HomeSeen{SeenAt: ptrTime(at(5))}}, sinceSources{Pages: pages})
		c, rec := sinceCtx(t, http.MethodGet, "/campaigns/c1/since-last-here", cc, "me", cookies...)
		if err := h.Show(c); err != nil {
			t.Fatal(err)
		}
		return rec.Body.String()
	}

	for name, body := range map[string]string{
		"player": run(member(campaigns.RolePlayer, false)),
		"scribe": run(member(campaigns.RoleScribe, false)),
		// The owner "viewing as a player" is read as a player.
		"owner as player": run(member(campaigns.RoleOwner, false), &http.Cookie{Name: "chronicle_view_as_player", Value: "1"}),
	} {
		if strings.Contains(body, "Cult Vault") || strings.Contains(body, "2 new pages") || !strings.Contains(body, "1 new page") {
			t.Errorf("%s leaks or miscounts:\n%s", name, body)
		}
	}
	if body := run(member(campaigns.RoleOwner, false)); !strings.Contains(body, "2 new pages") || !strings.Contains(body, "Cult Vault") {
		t.Errorf("the owner should see both pages:\n%s", body)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

// The viewer the sources get is the real one, and "view as player" strips the
// DM standing from every source.
func TestSinceHandler_ViewerPassedToSources(t *testing.T) {
	owner := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, IsMember: true, MemberRole: campaigns.RoleOwner}
	for _, asPlayer := range []bool{false, true} {
		pages, nights, qs := &fakeSincePages{}, &fakeSinceNights{}, &fakeSinceQuests{changes: &quests.QuestChanges{}}
		h := newSinceHandler(&fakeMarkers{member: true}, sinceSources{Pages: pages, Nights: nights, Quests: qs, NightsOn: nightsOn})
		var cookies []*http.Cookie
		if asPlayer {
			cookies = append(cookies, &http.Cookie{Name: "chronicle_view_as_player", Value: "1"})
		}
		c, _ := sinceCtx(t, http.MethodGet, "/campaigns/c1/since-last-here", owner, "me", cookies...)
		if err := h.Show(c); err != nil {
			t.Fatal(err)
		}
		wantDM := !asPlayer
		if nights.gotViewer.DMTeam != wantDM || qs.got.IsDM != wantDM || (pages.gotRole == int(campaigns.RoleOwner)) != wantDM {
			t.Errorf("asPlayer=%v: recaps DMTeam=%v quests IsDM=%v pages role=%d", asPlayer, nights.gotViewer.DMTeam, qs.got.IsDM, pages.gotRole)
		}
	}
}

// Loading the home never moves the marker, and a member with no marker reads
// from the day they joined, capped at thirty days.
func TestSinceHandler_LoadDoesNotMoveMarker(t *testing.T) {
	m := &fakeMarkers{member: true, seen: campaigns.HomeSeen{JoinedAt: at(100)}}
	pages := &fakeSincePages{}
	h := newSinceHandler(m, sinceSources{Pages: pages})
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, IsMember: true, MemberRole: campaigns.RolePlayer}
	c, rec := sinceCtx(t, http.MethodGet, "/campaigns/c1/since-last-here", cc, "me")
	if err := h.Show(c); err != nil {
		t.Fatal(err)
	}
	if len(m.marked) != 0 {
		t.Error("loading the block moved the marker")
	}
	if !pages.gotSince.Equal(sinceNow.Add(-sinceLookBack)) {
		t.Errorf("since = %v, want the thirty-day cap", pages.gotSince)
	}
	if !strings.Contains(rec.Body.String(), "all caught up") || strings.Contains(rec.Body.String(), "Mark as seen") {
		t.Errorf("nothing new should read caught up with no button:\n%s", rec.Body.String())
	}
}

// If the only source failed there is nothing true to say, so the block goes.
func TestSinceHandler_FailedReadIsNotCaughtUp(t *testing.T) {
	h := newSinceHandler(&fakeMarkers{member: true}, sinceSources{Pages: &fakeSincePages{err: errors.New("boom")}})
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, IsMember: true, MemberRole: campaigns.RolePlayer}
	c, rec := sinceCtx(t, http.MethodGet, "/campaigns/c1/since-last-here", cc, "me")
	if err := h.Show(c); err != nil {
		t.Fatal(err)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body.String())
	}
}

// Rendered text from a name is escaped, and the button posts to the member's
// own campaign with the moment the list was drawn.
func TestSinceHandler_RendersEscapedRowsAndButton(t *testing.T) {
	pages := &fakeSincePages{pages: []entities.PageSinceRef{{ID: "p1", Name: `<script>alert(1)</script>`, CreatedAt: at(1)}}}
	h := newSinceHandler(&fakeMarkers{member: true, seen: campaigns.HomeSeen{SeenAt: ptrTime(at(3))}}, sinceSources{Pages: pages})
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, IsMember: true, MemberRole: campaigns.RolePlayer}
	c, rec := sinceCtx(t, http.MethodGet, "/campaigns/c1/since-last-here", cc, "me")
	if err := h.Show(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{"Since you were last here", "Mark as seen", `hx-post="/campaigns/c1/since-last-here/seen"`, "1 new page"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<script>alert") {
		t.Errorf("name was not escaped:\n%s", body)
	}
}

func TestSinceHandler_SeenMovesMarkerForMembersOnly(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, IsMember: true, MemberRole: campaigns.RolePlayer}
	t.Run("member", func(t *testing.T) {
		m := &fakeMarkers{member: true}
		h := newSinceHandler(m, sinceSources{})
		upto := at(0).Add(-time.Minute)
		c, rec := sinceCtx(t, http.MethodPost, "/campaigns/c1/since-last-here/seen?upto="+url.QueryEscape(strconv.FormatInt(upto.Unix(), 10)), cc, "me")
		if err := h.Seen(c); err != nil {
			t.Fatal(err)
		}
		if len(m.marked) != 1 || !m.marked[0].Equal(upto.Truncate(time.Second)) {
			t.Errorf("marked = %v, want %v", m.marked, upto)
		}
		if !strings.Contains(rec.Body.String(), "all caught up") {
			t.Errorf("body = %q", rec.Body.String())
		}
	})
	t.Run("a future upto is clamped to now", func(t *testing.T) {
		m := &fakeMarkers{member: true}
		h := newSinceHandler(m, sinceSources{})
		c, _ := sinceCtx(t, http.MethodPost, "/campaigns/c1/since-last-here/seen?upto="+strconv.FormatInt(sinceNow.Add(48*time.Hour).Unix(), 10), cc, "me")
		if err := h.Seen(c); err != nil {
			t.Fatal(err)
		}
		if len(m.marked) != 1 || !m.marked[0].Equal(sinceNow) {
			t.Errorf("marked = %v, want now", m.marked)
		}
	})
	for name, tc := range map[string]struct {
		cc   *campaigns.CampaignContext
		user string
	}{
		"guest":         {&campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", IsPublic: true}}, ""},
		"stranger":      {&campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", IsPublic: true}}, "stranger"},
		"row not there": {cc, "gone"},
	} {
		t.Run(name, func(t *testing.T) {
			m := &fakeMarkers{member: name != "row not there"}
			m.member = false
			h := newSinceHandler(m, sinceSources{})
			c, _ := sinceCtx(t, http.MethodPost, "/campaigns/c1/since-last-here/seen", tc.cc, tc.user)
			if err := h.Seen(c); err == nil {
				t.Error("a non-member must be refused")
			}
			if len(m.marked) != 0 {
				t.Error("a non-member moved a marker")
			}
		})
	}
}

// The seen POST sits behind the app's global CSRF check like every other
// state-changing route: without the token it never reaches the handler.
func TestSinceSeenRequiresCSRF(t *testing.T) {
	m := &fakeMarkers{member: true}
	h := newSinceHandler(m, sinceSources{})
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, IsMember: true, MemberRole: campaigns.RolePlayer}
	reached := false
	next := func(c *echo.Context) error {
		reached = true
		c.Set("campaign_context", cc)
		c.Set("auth_user_id", "me")
		return h.Seen(c)
	}
	c, _ := sinceCtx(t, http.MethodPost, "/campaigns/c1/since-last-here/seen", cc, "me")
	err := middleware.CSRF()(next)(c)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != http.StatusForbidden {
		t.Errorf("err = %v, want a 403", err)
	}
	if reached || len(m.marked) != 0 {
		t.Error("the handler ran, or the marker moved, without a CSRF token")
	}
}

func TestSinceNightsOn(t *testing.T) {
	both := fakeComingUpAddons{calendar.PluginSlug: true, sessions.SessionsAddonSlug: true}
	if !sinceNightsOn(both, nil)(context.Background(), "c1") {
		t.Error("both addons on should read nights")
	}
	if sinceNightsOn(fakeComingUpAddons{calendar.PluginSlug: true}, nil)(context.Background(), "c1") {
		t.Error("game nights off must not read nights")
	}
	if sinceNightsOn(both, func(slug string) bool { return slug != sessions.SessionsAddonSlug })(context.Background(), "c1") {
		t.Error("an unhealthy plugin must not read nights")
	}
}
