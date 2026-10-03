// era_secret_test.go: an era hidden until it begins, and a Director's era
// note, must never reach a player in any response: the calendar's JSON, its
// page, the Calendars page, or any event read. Each test goes
// through the real router and the real service (over the in-memory repo
// fakes), so it checks what production traffic actually gets.
package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

const (
	eraCampaignID = "camp-era-secret"
	eraCalendarID = "cal-era-secret"

	// Distinctive strings that may appear only in a Director's response.
	secretEraName  = "Age of Ash"
	secretEraDesc  = "Ash covers the northern sky"
	secretEraColor = "#3a3a41"
	secretEraNote  = "Only happens if the party fails"
	visibleNote    = "The council is already split"
	secretEvent    = "The sky goes dark"
)

func strp(s string) *string { return &s }

// eraFixture is a 12-month calendar on year 1022 with three eras: two
// players may see (the second carrying a Director's note and an end that
// abuts the secret one), and one hidden until year 1040.
func eraFixture() (Calendar, []Era, []Event) {
	cal := Calendar{ID: eraCalendarID, CampaignID: eraCampaignID, Name: "Wyrmstone Reckoning",
		Mode: ModeFantasy, HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, Visibility: "everyone",
		CurrentYear: 1022, CurrentMonth: 8, CurrentDay: 9, EraLook: DefaultEraLook()}
	end2Y, end2M, end2D := 1039, 12, 30
	eras := []Era{
		{ID: 1, CalendarID: eraCalendarID, Name: "Age of Dragons", StartYear: 1, StartMonth: 1, StartDay: 1,
			EndYear: intp(1022), EndMonth: intp(6), EndDay: intp(17), Color: "#6e1a2a", Color2: strp("#d6893a"), Style: EraStyleGas},
		{ID: 2, CalendarID: eraCalendarID, Name: "Age of Humanity", StartYear: 1022, StartMonth: 6, StartDay: 18,
			EndYear: &end2Y, EndMonth: &end2M, EndDay: &end2D, Color: "#1d3f6e", Style: EraStyleGas, DMNote: strp(visibleNote)},
		{ID: 3, CalendarID: eraCalendarID, Name: secretEraName, StartYear: 1040, StartMonth: 1, StartDay: 1,
			Description: strp(secretEraDesc), Color: secretEraColor, Color2: strp("#c46a3a"), Style: EraStyleInk,
			DMNote: strp(secretEraNote), HiddenUntilBegins: true},
	}
	// The secret event is announced ahead, so only the era rule can hide it.
	ahead := AnnouncedAhead
	events := []Event{
		{ID: "ev-visible", CalendarID: eraCalendarID, Name: "Founding feast", Year: 1022, Month: 7, Day: 19, Visibility: "everyone"},
		{ID: "ev-secret", CalendarID: eraCalendarID, Name: secretEvent, Year: 1040, Month: 1, Day: 1, Visibility: "everyone", Announced: &ahead},
	}
	return cal, eras, events
}

func intp(i int) *int { return &i }

func eraMonths() []Month {
	ms := make([]Month, 12)
	for i := range ms {
		ms[i] = Month{ID: i + 1, CalendarID: eraCalendarID, Name: "M" + string(rune('A'+i)), Days: 30, SortOrder: i}
	}
	return ms
}

// newEraRouter wires the real service over fakes holding cal/eras/events.
func newEraRouter(cal Calendar, eras []Era, events []Event, public bool) *echo.Echo {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id != cal.ID {
				return nil, apperror.NewNotFound("calendar not found")
			}
			c := cal
			return &c, nil
		},
		listByCampaignFn: func(context.Context, string) ([]Calendar, error) { return []Calendar{cal}, nil },
		getMonthsFn:      func(context.Context, string) ([]Month, error) { return eraMonths(), nil },
		getErasFn: func(context.Context, string) ([]Era, error) {
			out := make([]Era, len(eras))
			copy(out, eras)
			return out, nil
		},
	}
	eventRepo := &fakeEventRepo{
		listForMonthFn: func(_ context.Context, _ string, year, month, _ int) ([]Event, error) {
			var out []Event
			for _, e := range events {
				if e.Year == year && e.Month == month {
					out = append(out, e)
				}
			}
			return out, nil
		},
		getEventFn: func(_ context.Context, id string) (*Event, error) {
			for _, e := range events {
				if e.ID == id {
					ev := e
					return &ev, nil
				}
			}
			return nil, nil
		},
		listUpcomingFn: func(context.Context, string, int, int, int, int, int) ([]Event, error) {
			return append([]Event(nil), events...), nil
		},
		listAllFn: func(context.Context, string) ([]Event, error) { return append([]Event(nil), events...), nil },
		searchFn: func(context.Context, string, string, int) ([]Event, error) {
			return append([]Event(nil), events...), nil
		},
	}
	e := echo.New()
	e.Use(emw.Recover())
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.JSON(ae.Code, map[string]string{"error": ae.Type, "message": ae.Message})
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	svc := NewCalendarService(calRepo, eventRepo, &fakeEventKindRepo{}, &fakeWeatherRepo{})
	roles := map[string]campaigns.Role{
		"u-player": campaigns.RolePlayer, "u-scribe": campaigns.RoleScribe, "u-owner": campaigns.RoleOwner,
	}
	RegisterRoutes(e, NewHandler(svc), guardCampaignSvc{roles: roles, public: public}, guardAuthSvc{}, guardAddonSvc{enabled: true})
	return e
}

// secretStrings are what only a Director may receive.
var secretStrings = []string{secretEraName, secretEraDesc, secretEraColor, secretEraNote, visibleNote, secretEvent}

func assertNoSecrets(t *testing.T, what, body string) {
	t.Helper()
	for _, s := range secretStrings {
		if strings.Contains(body, s) {
			t.Errorf("%s leaks %q:\n%s", what, s, body)
		}
	}
}

func TestEraSecret_NeverReachesPlayers(t *testing.T) {
	cal, eras, events := eraFixture()
	base := "/campaigns/" + eraCampaignID + "/calendars"
	reads := []struct {
		name, path string
	}{
		{"calendar JSON", base + "/" + eraCalendarID},
		{"calendar page", base + "/" + eraCalendarID + "/view"},
		{"Calendars page", base},
		{"month of the secret era", base + "/" + eraCalendarID + "/events?year=1040&month=1"},
		{"upcoming events", base + "/upcoming"},
		{"events of the era before the secret one", base + "/" + eraCalendarID + "/eras/2/events"},
	}
	for _, viewer := range []string{"u-player", "u-scribe"} {
		e := newEraRouter(cal, eras, events, false)
		for _, r := range reads {
			t.Run(viewer+"/"+r.name, func(t *testing.T) {
				rec := doRequest(e, http.MethodGet, r.path, viewer)
				if rec.Code != http.StatusOK {
					t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
				}
				assertNoSecrets(t, r.path, rec.Body.String())
			})
		}
		t.Run(viewer+"/the secret era's events", func(t *testing.T) {
			rec := doRequest(e, http.MethodGet, base+"/"+eraCalendarID+"/eras/3/events", viewer)
			if rec.Code != http.StatusNotFound {
				t.Errorf("got %d, want 404: %s", rec.Code, rec.Body.String())
			}
			assertNoSecrets(t, "secret era events", rec.Body.String())
		})
		t.Run(viewer+"/the secret event by id", func(t *testing.T) {
			rec := doRequest(e, http.MethodGet, base+"/"+eraCalendarID+"/events/ev-secret", viewer)
			if rec.Code != http.StatusNotFound {
				t.Errorf("got %d, want 404: %s", rec.Code, rec.Body.String())
			}
			assertNoSecrets(t, "event by id", rec.Body.String())
		})
	}

	t.Run("anonymous visitor on a public campaign", func(t *testing.T) {
		e := newEraRouter(cal, eras, events, true)
		for _, r := range reads {
			rec := doRequest(e, http.MethodGet, r.path, "")
			if rec.Code == http.StatusOK {
				assertNoSecrets(t, "anonymous "+r.path, rec.Body.String())
			}
		}
	})
}

func TestEraSecret_PlayerStillSeesTheKnownEras(t *testing.T) {
	cal, eras, events := eraFixture()
	e := newEraRouter(cal, eras, events, false)
	rec := doRequest(e, http.MethodGet, "/campaigns/"+eraCampaignID+"/calendars/"+eraCalendarID, "u-player")
	body := rec.Body.String()
	for _, want := range []string{"Age of Dragons", "Age of Humanity", "#d6893a"} {
		if !strings.Contains(body, want) {
			t.Errorf("player's calendar should name %q:\n%s", want, body)
		}
	}
	// The era before the secret one carries on: its end is cleared rather
	// than stopping at a date that hints at what follows.
	if strings.Contains(body, `"end_year":1039`) {
		t.Errorf("the era abutting the secret one should read as ongoing:\n%s", body)
	}
	rec = doRequest(e, http.MethodGet, "/campaigns/"+eraCampaignID+"/calendars/"+eraCalendarID+"/events?year=1022&month=7", "u-player")
	if !strings.Contains(rec.Body.String(), "Founding feast") {
		t.Errorf("an event outside the secret era must still show:\n%s", rec.Body.String())
	}
}

func TestEraSecret_DirectorSeesEverything(t *testing.T) {
	cal, eras, events := eraFixture()
	e := newEraRouter(cal, eras, events, false)
	rec := doRequest(e, http.MethodGet, "/campaigns/"+eraCampaignID+"/calendars/"+eraCalendarID, "u-owner")
	for _, want := range []string{secretEraName, secretEraNote, visibleNote, `"end_year":1039`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("Owner's calendar should carry %q", want)
		}
	}
	rec = doRequest(e, http.MethodGet, "/campaigns/"+eraCampaignID+"/calendars/"+eraCalendarID+"/eras/3/events", "u-owner")
	if !strings.Contains(rec.Body.String(), secretEvent) {
		t.Errorf("Owner should see the secret era's events: %s", rec.Body.String())
	}
	rec = doRequest(e, http.MethodGet, "/campaigns/"+eraCampaignID+"/calendars/"+eraCalendarID+"/events/ev-secret", "u-owner")
	if rec.Code != http.StatusOK {
		t.Errorf("Owner reading the secret era's event: got %d", rec.Code)
	}
}

func TestEraSecret_RevealedOnceItBegins(t *testing.T) {
	cal, eras, events := eraFixture()
	cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay = 1040, 1, 1
	e := newEraRouter(cal, eras, events, false)
	rec := doRequest(e, http.MethodGet, "/campaigns/"+eraCampaignID+"/calendars/"+eraCalendarID, "u-player")
	body := rec.Body.String()
	if !strings.Contains(body, secretEraName) {
		t.Errorf("an era that has begun should show to players:\n%s", body)
	}
	// Its note stays the Director's.
	if strings.Contains(body, secretEraNote) || strings.Contains(body, visibleNote) {
		t.Errorf("a Director's note must never reach a player:\n%s", body)
	}
	rec = doRequest(e, http.MethodGet, "/campaigns/"+eraCampaignID+"/calendars/"+eraCalendarID+"/events/ev-secret", "u-player")
	if rec.Code != http.StatusOK {
		t.Errorf("the era's event should show once it has begun: got %d", rec.Code)
	}
}

func TestErasForPlayer(t *testing.T) {
	cal, eras, _ := eraFixture()
	cal.Months = eraMonths()
	tests := []struct {
		name      string
		eras      []Era
		wantNames []string
		wantEnded map[string]bool
	}{
		{"secret era dropped, abutting end cleared", eras, []string{"Age of Dragons", "Age of Humanity"},
			map[string]bool{"Age of Dragons": true, "Age of Humanity": false}},
		{"not flagged: shown with its end", func() []Era {
			out := append([]Era(nil), eras...)
			out[2].HiddenUntilBegins = false
			return out
		}(), []string{"Age of Dragons", "Age of Humanity", secretEraName},
			map[string]bool{"Age of Humanity": true}},
		{"a gap before the secret era keeps the earlier end", func() []Era {
			out := append([]Era(nil), eras...)
			out[1].EndYear = intp(1030)
			return out
		}(), []string{"Age of Dragons", "Age of Humanity"}, map[string]bool{"Age of Humanity": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cal
			c.Eras = append([]Era(nil), tt.eras...)
			got := erasForPlayer(&c)
			var names []string
			for _, e := range got {
				names = append(names, e.Name)
				if e.DMNote != nil {
					t.Errorf("%s kept its note", e.Name)
				}
				if want, ok := tt.wantEnded[e.Name]; ok && (e.EndYear != nil) != want {
					t.Errorf("%s ended = %v, want %v", e.Name, e.EndYear != nil, want)
				}
			}
			if strings.Join(names, ",") != strings.Join(tt.wantNames, ",") {
				t.Errorf("names = %v, want %v", names, tt.wantNames)
			}
		})
	}
}

// fakeEntityGate allows exactly the ids in allowed.
type fakeEntityGate struct{ allowed map[string]bool }

func (g fakeEntityGate) FilterViewableEntityIDs(_ context.Context, _ string, ids []string, _ int, _ string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		if g.allowed[id] {
			out[id] = true
		}
	}
	return out, nil
}

func TestRedactEraLore(t *testing.T) {
	mk := func() []Era {
		return []Era{
			{ID: 1, LoreEntityID: strp("ent-open"), LoreEntityName: "Open page"},
			{ID: 2, LoreEntityID: strp("ent-hidden"), LoreEntityName: "Hidden page"},
		}
	}
	tests := []struct {
		name     string
		gate     EntityVisibilityGate
		role     int
		wantKept []bool
	}{
		{"player: only the page they may see", fakeEntityGate{allowed: map[string]bool{"ent-open": true}}, 1, []bool{true, false}},
		{"no gate wired: fail closed", nil, 1, []bool{false, false}},
		{"owner: untouched", nil, 3, []bool{true, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &calendarService{entityGate: tt.gate}
			eras := mk()
			if err := s.redactEraLore(context.Background(), "c", eras, permissions.RequestViewer(tt.role, "u1")); err != nil {
				t.Fatal(err)
			}
			for i, keep := range tt.wantKept {
				if (eras[i].LoreEntityID != nil) != keep || (eras[i].LoreEntityName != "") != keep {
					t.Errorf("era %d lore kept = %v, want %v", i+1, eras[i].LoreEntityID != nil, keep)
				}
			}
		})
	}
}

func TestListEraEvents_RangeAndCount(t *testing.T) {
	cal, eras, _ := eraFixture()
	events := []Event{
		{ID: "a", CalendarID: eraCalendarID, Name: "Hatching", Year: 3, Month: 1, Day: 1, Visibility: "everyone"},
		{ID: "b", CalendarID: eraCalendarID, Name: "Last flight", Year: 1022, Month: 6, Day: 17, Visibility: "everyone"},
		{ID: "c", CalendarID: eraCalendarID, Name: "First council", Year: 1022, Month: 6, Day: 18, Visibility: "everyone"},
		{ID: "d", CalendarID: eraCalendarID, Name: "Council of the few", Year: 1022, Month: 7, Day: 2, Visibility: "dm_only"},
	}
	e := newEraRouter(cal, eras, events, false)
	tests := []struct {
		name, viewer, era string
		want              []string
	}{
		{"first era stops at its end", "u-player", "1", []string{"Hatching", "Last flight"}},
		{"second era for a player", "u-player", "2", []string{"First council"}},
		{"second era for the Owner", "u-owner", "2", []string{"First council", "Council of the few"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(e, http.MethodGet, "/campaigns/"+eraCampaignID+"/calendars/"+eraCalendarID+"/eras/"+tt.era+"/events", tt.viewer)
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
			}
			var body struct {
				Data  []Event `json:"data"`
				Total int     `json:"total"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, ev := range body.Data {
				got = append(got, ev.Name)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") || body.Total != len(tt.want) {
				t.Errorf("got %v (total %d), want %v", got, body.Total, tt.want)
			}
		})
	}
}
