package sessions

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestAgoText(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	// The same cases as test/js/last_seen.test.mjs (agoText), so the line
	// reads alike on the server and after the Director changes it.
	tests := []struct {
		date, want string
	}{
		{"2026-10-18", "coming up"},
		{"2026-10-17", "today"},
		{"2026-10-16", "yesterday"},
		{"2026-10-10", "7 days ago"},
		{"2026-10-03", "2 weeks ago"},
		{"2026-09-12", "5 weeks ago"},
		{"2026-08-22", "8 weeks ago"},
		{"2026-08-15", "2 months ago"},
		{"2025-10-17", "12 months ago"},
		{"2023-10-17", "3 years ago"},
		{"", ""},
		{"nonsense", ""},
	}
	for _, tt := range tests {
		t.Run(tt.date, func(t *testing.T) {
			if got := AgoText(tt.date, now); got != tt.want {
				t.Errorf("AgoText(%q) = %q, want %q", tt.date, got, tt.want)
			}
		})
	}
}

func TestBuildLastSeenView(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		in   *Sightings
		want LastSeenView
	}{
		{"never seen", &Sightings{Seen: []Sighting{}}, LastSeenView{What: "Not seen yet"}},
		{"nil", nil, LastSeenView{What: "Not seen yet"}},
		{
			"newest dated night leads",
			&Sightings{Seen: []Sighting{{SessionName: "The Drowned Bell", Date: "2026-09-12"}, {SessionName: "Old", Date: "2026-01-01"}}},
			LastSeenView{Seen: 2, What: "The Drowned Bell", When: "5 weeks ago"},
		},
		{
			"a series-wide link names no night",
			&Sightings{Seen: []Sighting{{SessionName: "Weekly game", Source: SightingSourceLink}}},
			LastSeenView{Seen: 1, What: "during Weekly game, night not set"},
		},
		{
			"suggestions are counted for review",
			&Sightings{Seen: []Sighting{}, Suggestions: []Sighting{{SessionName: "A"}, {SessionName: "B"}}},
			LastSeenView{What: "Not seen yet", Review: 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildLastSeenView(tt.in, now); got != tt.want {
				t.Errorf("view = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestShowLastSeen(t *testing.T) {
	tests := []struct {
		name string
		v    LastSeenView
		dm   bool
		want bool
	}{
		{"player, never seen: no block at all", LastSeenView{What: "Not seen yet"}, false, false},
		{"player, never seen, DM-only suggestions exist: still nothing", LastSeenView{What: "Not seen yet", Review: 0}, false, false},
		{"player, seen", LastSeenView{Seen: 1}, false, true},
		{"DM, never seen: the line is where they add one", LastSeenView{What: "Not seen yet"}, true, true},
		{"DM, seen", LastSeenView{Seen: 3}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShowLastSeen(tt.v, tt.dm); got != tt.want {
				t.Errorf("ShowLastSeen = %v, want %v", got, tt.want)
			}
		})
	}
}

func renderLastSeen(t *testing.T, v LastSeenView, dm bool) string {
	t.Helper()
	var buf bytes.Buffer
	if err := BlockLastSeen("c1", "e1", "Brother Ansel", "u1", v, dm).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// The Director's controls are not in a player's markup at all, so they leave
// no gap and nothing for a script or the page source to reveal.
func TestBlockLastSeen_DirectorControlsOnlyForDirector(t *testing.T) {
	v := LastSeenView{Seen: 2, What: "The Drowned Bell", When: "5 weeks ago", Review: 3}
	dmOnly := []string{"data-dm-only", "Add a sighting", "data-ls-sugtog", "3 to review", "ls-addf", "ls-dmtag"}

	dm := renderLastSeen(t, v, true)
	for _, want := range append([]string{"The Drowned Bell", "5 weeks ago", "2 nights"}, dmOnly...) {
		if !strings.Contains(dm, want) {
			t.Errorf("DM markup lacks %q", want)
		}
	}
	player := renderLastSeen(t, v, false)
	for _, want := range []string{"The Drowned Bell", "5 weeks ago", "2 nights", `data-dm="0"`, "Close"} {
		if !strings.Contains(player, want) {
			t.Errorf("player markup lacks %q", want)
		}
	}
	for _, bad := range dmOnly {
		if strings.Contains(player, bad) {
			t.Errorf("player markup contains %q", bad)
		}
	}
	// Inline handlers only: a templ script helper would not run after a swap.
	if strings.Contains(player, "<script") || !strings.Contains(player, "ChronicleLastSeen.toggle") {
		t.Errorf("handlers must be inline IIFEs: %s", player)
	}
}

func TestBlockLastSeen_EscapesNames(t *testing.T) {
	v := LastSeenView{Seen: 1, What: `<img src=x onerror=alert(1)>`}
	out := renderLastSeen(t, v, false)
	if strings.Contains(out, "<img src=x") {
		t.Errorf("a session name reached the markup unescaped: %s", out)
	}
}

// sightingsGET drives the real handler (route logic aside) for one member
// role over a page with stored answers, links and a "mentioned" suggestion.
func sightingsGET(t *testing.T, role campaigns.Role, path string, handle func(*Handler, echo.Context) error) (int, string) {
	t.Helper()
	repo := &fakeSightingRepo{
		rows: []SightingRow{
			{SessionID: "weekly", SessionName: "Weekly", OccurrenceDate: "2026-10-10", State: SightingSeen, Source: SightingSourceManual, AddedBy: "dm1"},
			{SessionID: "weekly", SessionName: "Weekly", OccurrenceDate: "2026-10-03", State: SightingNotSeen, Source: SightingSourceCheckin, AddedBy: "dm1"},
		},
		links: []SightingLink{
			{SessionID: "met", SessionName: "Met", Role: EntityRoleKey, ScheduledDate: "2026-08-15"},
			{SessionID: "named", SessionName: "Named", Role: EntityRoleMentioned, ScheduledDate: "2026-09-20"},
		},
	}
	d := "2026-10-03"
	weekly := RecurrenceWeekly
	sessions := &mockSessionRepo{listByCampaignFn: func(context.Context, string) ([]Session, error) {
		return []Session{{ID: "weekly", Name: "Weekly", Status: StatusPlanned, ScheduledDate: &d, IsRecurring: true, RecurrenceType: &weekly}}, nil
	}}
	h := NewHandler(NewSessionService(&mockSessionRepo{}, nil, &stubEntityVisibility{}))
	h.SetSightingService(NewSightingService(repo, sessions, &mockEntityChecker{}, &stubEntityVisibility{}))

	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, path, nil), rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role})
	if err := handle(h, c); err != nil {
		t.Fatalf("handler: %v", err)
	}
	return rec.Code, rec.Body.String()
}

// A player's GET carries no suggestions, no dismissed nights and no who-added-it,
// and the Director's carries all three.
func TestSightingsAPI_PlayerNeverReceivesDirectorData(t *testing.T) {
	get := func(h *Handler, c echo.Context) error { return h.SightingsAPI(c) }
	path := "/campaigns/camp-1/sessions/sightings?entity=e1"

	code, body := sightingsGET(t, campaigns.RolePlayer, path, get)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	for _, key := range []string{"suggestions", "notSeen", "addedBy", "Named", "2026-10-03", "dm1"} {
		if strings.Contains(body, key) {
			t.Errorf("player response contains %q: %s", key, body)
		}
	}
	var got Sightings
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Seen) != 2 || len(got.Suggestions) != 0 || len(got.NotSeen) != 0 {
		t.Errorf("player got %+v, want only the two seen nights", got)
	}

	_, body = sightingsGET(t, campaigns.RoleOwner, path, get)
	for _, key := range []string{`"suggestions"`, `"notSeen"`, `"addedBy":"dm1"`} {
		if !strings.Contains(body, key) {
			t.Errorf("owner response lacks %s: %s", key, body)
		}
	}
}

func TestSightingNightsAPI(t *testing.T) {
	get := func(h *Handler, c echo.Context) error { return h.SightingNightsAPI(c) }
	code, body := sightingsGET(t, campaigns.RolePlayer, "/campaigns/camp-1/sessions/sightings/nights", get)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	var got SightingNights
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Nights) == 0 || got.Nights[0].SessionID != "weekly" || got.Nights[0].Date <= got.Nights[len(got.Nights)-1].Date {
		t.Errorf("nights = %+v, want the weekly series newest first", got.Nights)
	}
	for _, banned := range []string{"addedBy", "suggestions", "notSeen"} {
		if strings.Contains(body, banned) {
			t.Errorf("nights response contains %q", banned)
		}
	}
}

func TestNightsFor(t *testing.T) {
	d := func(s string) *string { return &s }
	weekly := RecurrenceWeekly
	end := "2026-09-20"
	list := []Session{
		{ID: "once", Name: "One-off", Status: StatusCompleted, ScheduledDate: d("2026-09-01")},
		{ID: "future", Name: "Future", Status: StatusPlanned, ScheduledDate: d("2026-12-01")},
		{ID: "cancelled", Name: "Cancelled", Status: StatusCancelled, ScheduledDate: d("2026-08-01")},
		{ID: "undated", Name: "Undated", Status: StatusPlanned},
		{ID: "series", Name: "Weekly", Status: StatusPlanned, ScheduledDate: d("2026-09-06"), IsRecurring: true, RecurrenceType: &weekly, RecurrenceEndDate: &end},
	}
	repo := &mockSessionRepo{listByCampaignFn: func(context.Context, string) ([]Session, error) { return list, nil }}
	svc := NewSightingService(&fakeSightingRepo{}, repo, &mockEntityChecker{}, &stubEntityVisibility{})

	got, err := svc.NightsFor(context.Background(), "c1", "2026-10-17")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, n := range got.Nights {
		keys = append(keys, n.SessionID+"|"+n.Date)
	}
	want := []string{"series|2026-09-20", "series|2026-09-13", "series|2026-09-06", "once|2026-09-01"}
	if !equalStrings(keys, want) {
		t.Errorf("nights = %v, want %v (no future, cancelled or undated sessions; a series stops at its end date)", keys, want)
	}
	if got.Truncated {
		t.Error("truncated with four nights")
	}

	// A long-running series is cut at the bound, newest kept.
	long := []Session{{ID: "s", Name: "Daily-ish", Status: StatusPlanned, ScheduledDate: d("2019-01-07"), IsRecurring: true, RecurrenceType: &weekly}}
	repo.listByCampaignFn = func(context.Context, string) ([]Session, error) { return long, nil }
	got, err = svc.NightsFor(context.Background(), "c1", "2026-10-17")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Nights) != maxSightingNights || !got.Truncated || got.Nights[0].Date != "2026-10-12" {
		t.Errorf("long series: %d nights, truncated=%v, newest %s", len(got.Nights), got.Truncated, got.Nights[0].Date)
	}
}
