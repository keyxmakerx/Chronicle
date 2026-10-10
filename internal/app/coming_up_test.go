package app

// coming_up_test.go pins the "Coming up" composer: what each source
// contributes, that a failed source is left out rather than breaking the
// card, and that who is looking reaches the calendar unchanged so its own
// visibility rules (not a copy here) decide which events appear.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

type fakeComingUpCalendar struct {
	cal       *calendar.Calendar
	calErr    error
	events    []calendar.Event // everything stored, GM-only ones included
	eventsErr error
	seen      []permissions.Viewer
}

func (f *fakeComingUpCalendar) GetDefaultCalendarForViewer(_ context.Context, _ string, v permissions.Viewer) (*calendar.Calendar, error) {
	f.seen = append(f.seen, v)
	return f.cal, f.calErr
}

// ListUpcomingEvents stands in for the real service's role filter: a viewer
// below the DM-only role gets only "everyone" events.
func (f *fakeComingUpCalendar) ListUpcomingEvents(_ context.Context, _, _ string, limit int, v permissions.Viewer) ([]calendar.Event, error) {
	f.seen = append(f.seen, v)
	if f.eventsErr != nil {
		return nil, f.eventsErr
	}
	var out []calendar.Event
	for _, e := range f.events {
		if e.Visibility == "dm_only" && !permissions.CanSeeDmOnly(v.Role()) {
			continue
		}
		out = append(out, e)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type fakeComingUpNights struct {
	nights     []sessions.GameNight
	nightsErr  error
	proposals  []sessions.ProposalSummary
	propErr    error
	asked      time.Time
	askErr     error
	mine       *sessions.MyAvailabilityResponse
	nightsSeen int
}

func (f *fakeComingUpNights) ListGameNights(_ context.Context, _, _, _, _ string, _ []sessions.NightMember) ([]sessions.GameNight, error) {
	f.nightsSeen++
	return f.nights, f.nightsErr
}
func (f *fakeComingUpNights) ListProposalSummaries(_ context.Context, _, _ string) ([]sessions.ProposalSummary, error) {
	return f.proposals, f.propErr
}
func (f *fakeComingUpNights) GetMyAvailability(_ context.Context, _, _ string) (*sessions.MyAvailabilityResponse, error) {
	if f.mine == nil {
		return &sessions.MyAvailabilityResponse{}, nil
	}
	return f.mine, nil
}
func (f *fakeComingUpNights) LastConfirmAsk(_ context.Context, _ string) (time.Time, error) {
	return f.asked, f.askErr
}

type fakeComingUpMembers struct{ err error }

func (f fakeComingUpMembers) ListMembers(_ context.Context, _ string) ([]campaigns.CampaignMember, error) {
	return []campaigns.CampaignMember{{UserID: "me", DisplayName: "Me"}, {UserID: "bo", DisplayName: "Bo"}}, f.err
}

// testCal is a three-month fantasy calendar standing on Frost 10, year 1200.
func testCal() *calendar.Calendar {
	return &calendar.Calendar{
		ID: "cal-1", CurrentYear: 1200, CurrentMonth: 2, CurrentDay: 10,
		Months: []calendar.Month{{Name: "Ember", Days: 30}, {Name: "Frostfall", Days: 30}, {Name: "Deepwinter", Days: 30}},
	}
}

func night(sid, date, clock string, mine, other string) sessions.GameNight {
	return sessions.GameNight{
		SessionID: sid, Name: "Session " + sid, Date: date, Time: clock,
		Roster: []sessions.NightAnswer{{UserID: "me", Answer: mine}, {UserID: "bo", Answer: other}},
		Tally: sessions.NightTally{
			Going:    count(mine, other, sessions.NightYes),
			Maybe:    count(mine, other, sessions.NightMaybe),
			Cant:     count(mine, other, sessions.NightNo),
			NoAnswer: count(mine, other, sessions.NightNoAnswer),
		},
	}
}

func count(a, b, want string) int {
	n := 0
	for _, v := range []string{a, b} {
		if v == want {
			n++
		}
	}
	return n
}

func titles(rows []comingUpRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Title
	}
	return out
}

func TestBuildComingUp(t *testing.T) {
	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	member := permissions.RequestViewer(int(campaigns.RolePlayer), "me")
	baseReq := comingUpRequest{CampaignID: "c1", Viewer: member, Member: true, Calendar: true, Nights: true, Now: now}

	gmOnly := calendar.Event{Name: "Secret coup", Year: 1200, Month: 3, Day: 1, Visibility: "dm_only"}
	festival := calendar.Event{Name: "Festival of Lanterns", Year: 1200, Month: 2, Day: 13, Visibility: "everyone", KindName: "Festival"}
	wedding := calendar.Event{Name: "The duke's wedding", Year: 1200, Month: 3, Day: 2, Visibility: "everyone"}

	tests := []struct {
		name       string
		req        func(r comingUpRequest) comingUpRequest
		cal        *fakeComingUpCalendar
		nights     *fakeComingUpNights
		wantNights []string
		wantWait   []string
		wantWorld  []string
		wantAnswer int
		check      func(t *testing.T, v comingUpView)
	}{
		{
			name: "all three sections, nights ordered across sessions",
			cal:  &fakeComingUpCalendar{cal: testCal(), events: []calendar.Event{festival, wedding}},
			nights: &fakeComingUpNights{nights: []sessions.GameNight{
				night("B", "2026-10-24", "19:00", sessions.NightYes, sessions.NightYes),
				night("A", "2026-10-17", "19:30", sessions.NightYes, sessions.NightNo),
			}},
			wantNights: []string{"Session A · 7:30 pm", "Session B · 7 pm"},
			wantWorld:  []string{"Festival of Lanterns", "The duke's wedding"},
			wantAnswer: 4,
			check: func(t *testing.T, v comingUpView) {
				if got := v.Nights[0].Sub; got != "1 of 2 coming · you're going" {
					t.Errorf("night sub = %q", got)
				}
				if v.Nights[0].Action != "Change" || v.Nights[0].Waiting {
					t.Errorf("an answered night offers Change and is not waiting: %+v", v.Nights[0])
				}
				if v.World[0].Sub != "In 3 days" || v.World[0].Chip != "Festival" || v.World[0].TileTop != "Fro" {
					t.Errorf("world row = %+v", v.World[0])
				}
				if v.World[1].Sub != "In 22 days" || v.World[1].Chip != "" {
					t.Errorf("second world row = %+v", v.World[1])
				}
				if v.Nights[0].TileTop != "Sat" || v.Nights[0].TileBig != "17" {
					t.Errorf("tile = %s %s", v.Nights[0].TileTop, v.Nights[0].TileBig)
				}
			},
		},
		{
			name: "own unanswered night lands in waiting on you and offers Answer",
			cal:  &fakeComingUpCalendar{cal: testCal()},
			nights: &fakeComingUpNights{nights: []sessions.GameNight{
				night("A", "2026-10-17", "", sessions.NightNoAnswer, sessions.NightYes),
			}},
			wantNights: []string{"Session A"},
			wantWait:   []string{"Session A"},
			wantAnswer: 4,
			check: func(t *testing.T, v comingUpView) {
				if v.Nights[0].Action != "Answer" || !v.Nights[0].Waiting {
					t.Errorf("unanswered night row = %+v", v.Nights[0])
				}
				if !v.Waiting[0].Primary || !strings.Contains(v.Waiting[0].Href, "session=A") {
					t.Errorf("waiting row = %+v", v.Waiting[0])
				}
			},
		},
		{
			name: "a night moved after the viewer answered is waiting on them",
			cal:  &fakeComingUpCalendar{cal: testCal()},
			nights: &fakeComingUpNights{nights: []sessions.GameNight{func() sessions.GameNight {
				n := night("A", "2026-10-17", "", sessions.NightYes, sessions.NightYes)
				n.Roster[0].Recheck = true
				return n
			}()}},
			wantNights: []string{"Session A"},
			wantWait:   []string{"Session A"},
			wantAnswer: 4,
		},
		{
			name: "past nights and nights the viewer is not rostered on are not asked of them",
			cal:  &fakeComingUpCalendar{cal: testCal()},
			nights: &fakeComingUpNights{nights: []sessions.GameNight{
				func() sessions.GameNight {
					n := night("P", "2026-10-01", "", sessions.NightNoAnswer, sessions.NightYes)
					n.Past = true
					return n
				}(),
				func() sessions.GameNight {
					n := night("R", "2026-10-17", "", sessions.NightNoAnswer, sessions.NightYes)
					n.Roster = n.Roster[1:]
					return n
				}(),
			}},
			wantNights: []string{"Session R"},
			wantAnswer: 4,
		},
		{
			name: "open poll the viewer has not answered; answered and closed polls are left out",
			cal:  &fakeComingUpCalendar{cal: testCal()},
			nights: &fakeComingUpNights{proposals: []sessions.ProposalSummary{
				{Proposal: sessions.SlotProposal{ID: "p1", Title: "Pick a night in November", Status: sessions.ProposalOpen}, ResponderN: 2},
				{Proposal: sessions.SlotProposal{ID: "p2", Title: "Already answered", Status: sessions.ProposalOpen}, MyResponded: true},
				{Proposal: sessions.SlotProposal{ID: "p3", Title: "Closed", Status: sessions.ProposalClosed}},
			}},
			wantWait:   []string{"Pick a night in November"},
			wantAnswer: 4,
			check: func(t *testing.T, v comingUpView) {
				if v.Waiting[0].Href != "/campaigns/c1/proposals/p1" || v.Waiting[0].Sub != "Date poll · 2 answers so far" {
					t.Errorf("poll row = %+v", v.Waiting[0])
				}
			},
		},
		{
			name: "confirm-your-times ask shows only while the viewer's hours predate it",
			cal:  &fakeComingUpCalendar{cal: testCal()},
			nights: &fakeComingUpNights{
				asked: time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC),
				mine:  &sessions.MyAvailabilityResponse{Answered: true, AnsweredAt: "2026-09-01T10:00:00Z"},
			},
			wantWait:   []string{"Confirm your times"},
			wantAnswer: 4,
		},
		{
			name: "confirm ask already confirmed after it was sent",
			cal:  &fakeComingUpCalendar{cal: testCal()},
			nights: &fakeComingUpNights{
				asked: time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC),
				mine:  &sessions.MyAvailabilityResponse{Answered: true, AnsweredAt: "2026-10-09T10:00:00Z"},
			},
			wantAnswer: 4,
		},
		{
			name: "no ask ever sent, nothing to confirm",
			cal:  &fakeComingUpCalendar{cal: testCal()},
			nights: &fakeComingUpNights{
				mine: &sessions.MyAvailabilityResponse{Answered: true, AnsweredAt: "2026-01-01T10:00:00Z"},
			},
			wantAnswer: 4,
		},
		{
			name:       "player does not get GM-only events (calendar's filter, viewer passed through)",
			cal:        &fakeComingUpCalendar{cal: testCal(), events: []calendar.Event{gmOnly, festival}},
			nights:     &fakeComingUpNights{},
			wantWorld:  []string{"Festival of Lanterns"},
			wantAnswer: 4,
			check:      func(t *testing.T, v comingUpView) {},
		},
		{
			name: "owner sees the GM-only event too",
			req: func(r comingUpRequest) comingUpRequest {
				r.Viewer = permissions.RequestViewer(int(campaigns.RoleOwner), "me")
				return r
			},
			cal:        &fakeComingUpCalendar{cal: testCal(), events: []calendar.Event{gmOnly, festival}},
			nights:     &fakeComingUpNights{},
			wantWorld:  []string{"Secret coup", "Festival of Lanterns"},
			wantAnswer: 4,
		},
		{
			name: "calendar addon off leaves out the world section",
			req:  func(r comingUpRequest) comingUpRequest { r.Calendar = false; return r },
			cal:  &fakeComingUpCalendar{cal: testCal(), events: []calendar.Event{festival}},
			nights: &fakeComingUpNights{nights: []sessions.GameNight{
				night("A", "2026-10-17", "", sessions.NightYes, sessions.NightYes),
			}},
			wantNights: []string{"Session A"},
			wantAnswer: 3,
		},
		{
			name:       "game nights addon off leaves out nights and waiting",
			req:        func(r comingUpRequest) comingUpRequest { r.Nights = false; return r },
			cal:        &fakeComingUpCalendar{cal: testCal(), events: []calendar.Event{festival}},
			nights:     &fakeComingUpNights{nights: []sessions.GameNight{night("A", "2026-10-17", "", sessions.NightNoAnswer, sessions.NightYes)}},
			wantWorld:  []string{"Festival of Lanterns"},
			wantAnswer: 1,
			check: func(t *testing.T, v comingUpView) {
				if v.Nights != nil || v.Waiting != nil {
					t.Errorf("nights leaked with the addon off: %+v", v)
				}
			},
		},
		{
			name: "public visitor gets the calendar only, never the table's nights",
			req: func(r comingUpRequest) comingUpRequest {
				r.Member = false
				r.Viewer = permissions.RequestViewer(int(campaigns.RoleNone), "")
				return r
			},
			cal:        &fakeComingUpCalendar{cal: testCal(), events: []calendar.Event{gmOnly, festival}},
			nights:     &fakeComingUpNights{nights: []sessions.GameNight{night("A", "2026-10-17", "", sessions.NightNoAnswer, sessions.NightYes)}},
			wantWorld:  []string{"Festival of Lanterns"},
			wantAnswer: 1,
			check: func(t *testing.T, v comingUpView) {
				if v.CalendarHref != "/campaigns/c1/calendars/cal-1/view" {
					t.Errorf("calendar link = %q", v.CalendarHref)
				}
			},
		},
		{
			name:       "failing calendar is left out, nights still show",
			cal:        &fakeComingUpCalendar{calErr: errors.New("db down")},
			nights:     &fakeComingUpNights{nights: []sessions.GameNight{night("A", "2026-10-17", "", sessions.NightYes, sessions.NightYes)}},
			wantNights: []string{"Session A"},
			wantAnswer: 3,
		},
		{
			name:       "failing events read is left out",
			cal:        &fakeComingUpCalendar{cal: testCal(), eventsErr: errors.New("boom")},
			nights:     &fakeComingUpNights{nights: []sessions.GameNight{night("A", "2026-10-17", "", sessions.NightYes, sessions.NightYes)}},
			wantNights: []string{"Session A"},
			wantAnswer: 3,
		},
		{
			name:       "failing nights read is left out, calendar still shows",
			cal:        &fakeComingUpCalendar{cal: testCal(), events: []calendar.Event{festival}},
			nights:     &fakeComingUpNights{nightsErr: errors.New("boom"), propErr: errors.New("boom"), askErr: errors.New("boom")},
			wantWorld:  []string{"Festival of Lanterns"},
			wantAnswer: 1,
		},
		{
			name:       "no calendar yet is ordinary, not a failure",
			cal:        &fakeComingUpCalendar{calErr: apperror.NewNotFound("none")},
			nights:     &fakeComingUpNights{},
			wantAnswer: 4,
			check: func(t *testing.T, v comingUpView) {
				if !v.Empty() {
					t.Errorf("expected the empty state, got %+v", v)
				}
			},
		},
		{
			name:       "everything failing answers nothing so the block renders nothing",
			cal:        &fakeComingUpCalendar{calErr: errors.New("x")},
			nights:     &fakeComingUpNights{nightsErr: errors.New("x"), propErr: errors.New("x"), askErr: errors.New("x")},
			wantAnswer: 0,
		},
		{
			name: "waiting on you is capped",
			cal:  &fakeComingUpCalendar{cal: testCal()},
			nights: &fakeComingUpNights{
				nights: []sessions.GameNight{
					night("A", "2026-10-17", "", sessions.NightNoAnswer, sessions.NightYes),
					night("B", "2026-10-24", "", sessions.NightNoAnswer, sessions.NightYes),
					night("C", "2026-10-31", "", sessions.NightNoAnswer, sessions.NightYes),
				},
				proposals: []sessions.ProposalSummary{
					{Proposal: sessions.SlotProposal{ID: "p1", Title: "P1", Status: sessions.ProposalOpen}},
					{Proposal: sessions.SlotProposal{ID: "p2", Title: "P2", Status: sessions.ProposalOpen}},
				},
			},
			wantNights: []string{"Session A", "Session B", "Session C"},
			wantWait:   []string{"Session A", "Session B", "Session C", "P1"},
			wantAnswer: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := baseReq
			if tt.req != nil {
				req = tt.req(req)
			}
			v := buildComingUp(context.Background(), comingUpSources{Calendar: tt.cal, Nights: tt.nights, Members: fakeComingUpMembers{}}, req)
			if got := titles(v.Nights); !equalStrings(got, tt.wantNights) {
				t.Errorf("nights = %v, want %v", got, tt.wantNights)
			}
			if got := titles(v.Waiting); !equalStrings(got, tt.wantWait) {
				t.Errorf("waiting = %v, want %v", got, tt.wantWait)
			}
			if got := titles(v.World); !equalStrings(got, tt.wantWorld) {
				t.Errorf("world = %v, want %v", got, tt.wantWorld)
			}
			if v.Answered != tt.wantAnswer {
				t.Errorf("answered = %d, want %d", v.Answered, tt.wantAnswer)
			}
			if v.Month != "October" {
				t.Errorf("month = %q", v.Month)
			}
			if tt.check != nil {
				tt.check(t, v)
			}
		})
	}
}

// The calendar must be asked as the person looking, never as a trusted
// caller, or its own filters would not apply.
func TestBuildComingUp_PassesTheViewerToTheCalendar(t *testing.T) {
	cal := &fakeComingUpCalendar{cal: testCal()}
	viewer := permissions.RequestViewer(int(campaigns.RolePlayer), "me")
	buildComingUp(context.Background(), comingUpSources{Calendar: cal}, comingUpRequest{
		CampaignID: "c1", Viewer: viewer, Member: true, Calendar: true, Now: time.Now(),
	})
	if len(cal.seen) != 2 {
		t.Fatalf("calendar reads = %d, want 2", len(cal.seen))
	}
	for _, v := range cal.seen {
		if v != viewer || v.IsSystem() {
			t.Errorf("calendar was asked as %+v, want the request viewer %+v", v, viewer)
		}
	}
}

func TestBuildComingUp_NightsNeverReadForNonMembers(t *testing.T) {
	nights := &fakeComingUpNights{}
	buildComingUp(context.Background(), comingUpSources{Nights: nights, Members: fakeComingUpMembers{}}, comingUpRequest{
		CampaignID: "c1", Viewer: permissions.RequestViewer(int(campaigns.RoleNone), ""),
		Member: false, Nights: true, Now: time.Now(),
	})
	if nights.nightsSeen != 0 {
		t.Errorf("game nights were read for a non-member")
	}
}

func TestComingUpWording(t *testing.T) {
	t.Run("inDaysLabel", func(t *testing.T) {
		for in, want := range map[int]string{-1: "Today", 0: "Today", 1: "Tomorrow", 2: "In 2 days", 21: "In 21 days"} {
			if got := inDaysLabel(in); got != want {
				t.Errorf("inDaysLabel(%d) = %q, want %q", in, got, want)
			}
		}
	})
	t.Run("nightClock", func(t *testing.T) {
		for in, want := range map[string]string{"19:00": "7 pm", "19:30": "7:30 pm", "": "", "junk": "", "00:05": "12:05 am"} {
			if got := nightClock(in); got != want {
				t.Errorf("nightClock(%q) = %q, want %q", in, got, want)
			}
		}
	})
	t.Run("monthAbbrev", func(t *testing.T) {
		for in, want := range map[string]string{"Frostfall": "Fro", "Ra": "Ra", "": ""} {
			if got := monthAbbrev(in); got != want {
				t.Errorf("monthAbbrev(%q) = %q, want %q", in, got, want)
			}
		}
	})
}

func TestComingUpFragment_Renders(t *testing.T) {
	v := comingUpView{
		Month: "October", CalendarHref: "/campaigns/c1/calendars/cal-1/view", Answered: 1,
		Nights:  []comingUpRow{{TileTop: "Fri", TileBig: "17", Title: "Session 12 · 7 pm", Sub: "4 of 5 coming · you're going", Href: "/campaigns/c1/game-nights?session=s", Action: "Change"}},
		Waiting: []comingUpRow{{TileTop: "Poll", TileIcon: "fa-solid fa-chart-simple", Title: "Pick <b>a night</b>", Sub: "Date poll", Waiting: true, Href: "/campaigns/c1/proposals/p", Action: "Answer", Primary: true}},
		World:   []comingUpRow{{TileTop: "Fro", TileBig: "14", Title: "Festival", Sub: "In 3 days", Chip: "Festival"}},
	}
	got := renderToString(t, comingUpFragment("c1", v))
	for _, want := range []string{
		`Coming up <span>October</span>`, `Open the calendar`, `>Game nights<`, `>Waiting on you<`, `>In the world<`,
		`cu-btn cu-btn-pri`, `cu-dot cu-dot-wait`, `cu-chip">Festival`, `/static/css/coming_up.css`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "<b>a night</b>") {
		t.Errorf("poll title was not escaped: %s", got)
	}

	empty := renderToString(t, comingUpFragment("c1", comingUpView{Month: "October", Answered: 1}))
	if !strings.Contains(empty, "Nothing is coming up yet") || strings.Contains(empty, "cu-sec") {
		t.Errorf("empty state wrong: %s", empty)
	}
}

type fakeComingUpAddons map[string]bool

func (f fakeComingUpAddons) IsEnabledForCampaign(_ context.Context, _, slug string) (bool, error) {
	return f[slug], nil
}

// The handler's only jobs are gating and rendering: with both addons off it
// answers an empty body (so the dashboard drops the placeholder), and it
// reads nothing the switches say is off.
func TestComingUpHandler_AddonGating(t *testing.T) {
	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		addons     fakeComingUpAddons
		unhealthy  string
		wantEmpty  bool
		wantNights bool
		wantWorld  bool
	}{
		{"both off", fakeComingUpAddons{}, "", true, false, false},
		{"calendar only", fakeComingUpAddons{calendar.PluginSlug: true}, "", false, false, true},
		{"sessions without calendar is off", fakeComingUpAddons{sessions.SessionsAddonSlug: true}, "", true, false, false},
		{"both on", fakeComingUpAddons{calendar.PluginSlug: true, sessions.SessionsAddonSlug: true}, "", false, true, true},
		{"calendar plugin unhealthy", fakeComingUpAddons{calendar.PluginSlug: true, sessions.SessionsAddonSlug: true}, calendar.PluginSlug, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cal := &fakeComingUpCalendar{cal: testCal(), events: []calendar.Event{{Name: "Festival", Year: 1200, Month: 2, Day: 13, Visibility: "everyone"}}}
			nights := &fakeComingUpNights{nights: []sessions.GameNight{night("A", "2026-10-17", "", sessions.NightYes, sessions.NightYes)}}
			h := &comingUpHandler{
				src:     comingUpSources{Calendar: cal, Nights: nights, Members: fakeComingUpMembers{}},
				addons:  tt.addons,
				healthy: func(slug string) bool { return slug != tt.unhealthy },
				now:     func() time.Time { return now },
			}
			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/c1/coming-up", nil), rec)
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsMember: true,
			})
			c.Set("auth_user_id", "me")
			if err := h.Show(c); err != nil {
				t.Fatalf("Show: %v", err)
			}
			body := rec.Body.String()
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			if tt.wantEmpty != (body == "") {
				t.Errorf("empty body = %v, want %v: %q", body == "", tt.wantEmpty, body)
			}
			if got := strings.Contains(body, "Session A"); got != tt.wantNights {
				t.Errorf("nights shown = %v, want %v", got, tt.wantNights)
			}
			if got := strings.Contains(body, "Festival"); got != tt.wantWorld {
				t.Errorf("world shown = %v, want %v", got, tt.wantWorld)
			}
		})
	}
}
