package sessions

// A game night the app marks Played is a record, not a question: its page
// offers no answer buttons, and the bell neither counts its invitation nor
// draws it as unread. The invitation stays listed, and the stored row is not
// changed, so the ask comes back if the night is moved to a later date.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestSessionDetailPage_APlayedNightOffersNoAnswerButtons(t *testing.T) {
	date := "2026-11-13"
	tests := []struct {
		name   string
		status string
		played bool
		want   bool
	}{
		{"a night still ahead", StatusPlanned, false, true},
		{"a night marked Played for this viewer", StatusPlanned, true, false},
		{"a completed night", StatusCompleted, false, false},
		{"a cancelled night", StatusCancelled, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &campaigns.CampaignContext{
				Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Camp"}, IsMember: true, MemberRole: campaigns.RolePlayer,
			}
			s := &Session{
				ID: "s1", CampaignID: "camp-1", Name: "The Dark Lighthouse", Status: tt.status, ScheduledDate: &date,
				Attendees: []Attendee{{UserID: "u1", DisplayName: "Tobin", Status: RSVPInvited}, {UserID: "u2", DisplayName: "Ilse", Status: RSVPAccepted}},
			}
			var sb strings.Builder
			if err := SessionDetailPage(cc, s, "csrf", false, false, "u1", RecapView{Played: tt.played}, "", NightDateNote{}).Render(context.Background(), &sb); err != nil {
				t.Fatalf("render: %v", err)
			}
			out := sb.String()
			if got := hasRSVPButtons(out); got != tt.want {
				t.Errorf("answer buttons shown = %v, want %v", got, tt.want)
			}
			// Who was asked and what they said stays on the page either way.
			if !strings.Contains(out, "Tobin") || !strings.Contains(out, "Ilse") {
				t.Error("the list of who was asked is gone")
			}
		})
	}
}

// bellFixture is a member with three unread notifications: an ask about a
// night played days ago, an ask about a night still ahead, and a piece of
// news; plus one ask they already read.
func bellFixture(t *testing.T) (*Handler, []Notification) {
	t.Helper()
	day := func(offset int) *string {
		d := time.Now().UTC().AddDate(0, 0, offset).Format("2006-01-02")
		return &d
	}
	nights := map[string]*Session{
		"played": {ID: "played", CampaignID: "c1", Status: StatusPlanned, ScheduledDate: day(-4)},
		"ahead":  {ID: "ahead", CampaignID: "c1", Status: StatusPlanned, ScheduledDate: day(4)},
	}
	mk := func(id, ntype, link string, read bool) Notification {
		n := note(ntype, read, link)
		n.ID, n.UserID = id, "u1"
		cid := "c1"
		n.CampaignID = &cid
		return n
	}
	notes := []Notification{
		mk("n-played", NotifSessionScheduled, sessionLink("c1", "played"), false),
		mk("n-ahead", NotifSessionScheduled, sessionLink("c1", "ahead"), false),
		mk("n-news", NotifProposalConfirmed, sessionLink("c1", "played"), false),
		mk("n-old", NotifSessionScheduled, sessionLink("c1", "played"), true),
	}
	repo := &mockSessionRepo{
		findByIDFn:          func(_ context.Context, id string) (*Session, error) { return nights[id], nil },
		listNotificationsFn: func(context.Context, string, int) ([]Notification, error) { return notes, nil },
		countUnreadNotificationsFn: func(context.Context, string) (int, error) {
			n := 0
			for _, x := range notes {
				if x.ReadAt == nil {
					n++
				}
			}
			return n, nil
		},
		listUnreadOfTypeFn: func(_ context.Context, _ string, ntype string, _ int) ([]Notification, error) {
			var out []Notification
			for _, x := range notes {
				if x.ReadAt == nil && x.Type == ntype {
					out = append(out, x)
				}
			}
			return out, nil
		},
	}
	return NewHandler(NewSessionService(repo, nil, nil)), notes
}

func bellRequest(h *Handler, handle func(*echo.Context) error, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, path, nil), rec)
	c.Set("auth_user_id", "u1")
	_ = handle(c)
	return rec
}

func TestBell_LeavesAPlayedNightsAskOutOfItsCount(t *testing.T) {
	h, _ := bellFixture(t)
	rec := bellRequest(h, h.NotificationBadgeAPI, "/notifications/badge")
	if !strings.Contains(rec.Body.String(), `data-count="2"`) {
		t.Errorf("badge = %q, want a count of 2: the ask still ahead and the news", rec.Body.String())
	}
}

func TestBell_ShowsNoBadgeWhenOnlyAPlayedNightsAskIsUnread(t *testing.T) {
	h, notes := bellFixture(t)
	now := time.Now().UTC()
	notes[1].ReadAt, notes[2].ReadAt = &now, &now
	rec := bellRequest(h, h.NotificationBadgeAPI, "/notifications/badge")
	if rec.Body.Len() != 0 {
		t.Errorf("badge = %q, want none", rec.Body.String())
	}
}

func TestBell_KeepsAPlayedNightsAskInItsListDrawnAsRead(t *testing.T) {
	h, _ := bellFixture(t)
	rec := bellRequest(h, h.ListNotificationsAPI, "/notifications")
	var rows []notifDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	read := map[string]bool{}
	for _, r := range rows {
		read[r.ID] = r.Read
	}
	want := map[string]bool{"n-played": true, "n-ahead": false, "n-news": false, "n-old": true}
	if len(rows) != len(want) {
		t.Fatalf("%d rows listed, want all %d: nothing leaves the list", len(rows), len(want))
	}
	for id, w := range want {
		if read[id] != w {
			t.Errorf("%s is drawn as read = %v, want %v", id, read[id], w)
		}
	}
}

func TestBanner_DoesNotAskAboutAPlayedNight(t *testing.T) {
	h, notes := bellFixture(t)
	rec := bellRequest(h, h.CalloutAPI, "/notifications/call-to-action")
	body := rec.Body.String()
	if !strings.Contains(body, sessionLink("c1", "ahead")) || strings.Contains(body, sessionLink("c1", "played")) {
		t.Errorf("banner = %q, want it to open the night still ahead and nothing else", body)
	}
	now := time.Now().UTC()
	notes[1].ReadAt = &now
	if rec := bellRequest(h, h.CalloutAPI, "/notifications/call-to-action"); strings.Contains(rec.Body.String(), "/sessions/") {
		t.Errorf("with only a played night unanswered the banner still asks: %q", rec.Body.String())
	}
}

// The day a night is judged by is the reader's own, resolved as the night's
// page resolves it: the zone they set on that campaign's availability page,
// then their account's, then UTC.
func TestViewerDays_FollowsTheReadersZoneCampaignByCampaign(t *testing.T) {
	noon := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	repo := &mockSessionRepo{
		listCampaignAvailabilityFn: func(_ context.Context, campaignID string) ([]AvailabilityBlock, error) {
			if campaignID == "auckland-table" {
				return []AvailabilityBlock{{UserID: "u1", TZ: "Pacific/Auckland"}, {UserID: "someone-else", TZ: "Pacific/Honolulu"}}, nil
			}
			return nil, nil
		},
	}
	h := NewHandler(NewSessionService(repo, nil, nil))
	h.SetUserDirectory(&stubUserDir{tz: "Pacific/Honolulu"})

	days := h.viewerDays(context.Background(), "u1", noon)
	if got := days("auckland-table"); got != "2026-11-19" {
		t.Errorf("in the campaign where they set Auckland it is %s, want 2026-11-19", got)
	}
	if got := days("other-table"); got != "2026-11-18" {
		t.Errorf("elsewhere their account zone (Honolulu) applies: got %s, want 2026-11-18", got)
	}

	// 05:00 UTC is still the day before in Honolulu, and already the 18th in UTC.
	early := time.Date(2026, 11, 18, 5, 0, 0, 0, time.UTC)
	if got := h.viewerDays(context.Background(), "u1", early)("other-table"); got != "2026-11-17" {
		t.Errorf("Honolulu at 05:00 UTC is %s, want 2026-11-17", got)
	}
	bare := NewHandler(NewSessionService(repo, nil, nil))
	if got := bare.viewerDays(context.Background(), "u1", early)("other-table"); got != "2026-11-18" {
		t.Errorf("with no zone set anywhere UTC applies: got %s, want 2026-11-18", got)
	}
}
