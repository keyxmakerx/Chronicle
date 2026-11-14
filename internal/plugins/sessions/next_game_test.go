package sessions

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func nextGameHTML(t *testing.T, v nextGameView) string {
	t.Helper()
	var b bytes.Buffer
	if err := nextGameBlock(v).Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func TestBuildNextGame(t *testing.T) {
	night := func(date string, hour float64, clock string) couldPlayNightIn {
		return couldPlayNightIn{Date: date, Hour: hour, couldPlayNight: couldPlayNight{
			Title: "The Drowned Archive", Clock: clock, Answer: "You're going", Tone: "go"}}
	}
	tests := []struct {
		name                       string
		nights                     []couldPlayNightIn
		wantSet                    bool
		wantCount, wantUnit, wantT string
		wantWhen                   string
	}{
		{"three days out", []couldPlayNightIn{night("2026-11-19", 19, "7 pm")}, true, "5", "days", "", "Thu 19 Nov, 7 pm"},
		{"tomorrow is one day", []couldPlayNightIn{night("2026-11-15", 19, "7 pm")}, true, "1", "day", "", "Sun 15 Nov, 7 pm"},
		{"tonight after five", []couldPlayNightIn{night("2026-11-14", 19, "7 pm")}, true, "", "", "Tonight, 7 pm", "Sat 14 Nov, 7 pm"},
		{"today before evening", []couldPlayNightIn{night("2026-11-14", 14, "2 pm")}, true, "", "", "Today, 2 pm", "Sat 14 Nov, 2 pm"},
		{"today with no time", []couldPlayNightIn{night("2026-11-14", -1, "")}, true, "", "", "Today", "Sat 14 Nov"},
		{"the first night wins", []couldPlayNightIn{night("2026-11-16", 19, "7 pm"), night("2026-11-20", 19, "7 pm")}, true, "2", "days", "", "Mon 16 Nov, 7 pm"},
		{"nothing set", nil, false, "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := buildNextGame(tt.nights, "2026-11-14", false, "/campaigns/c1/game-nights")
			if v.Set != tt.wantSet || v.Count != tt.wantCount || v.Unit != tt.wantUnit || v.Today != tt.wantT || v.When != tt.wantWhen {
				t.Errorf("got %+v", v)
			}
		})
	}
}

func TestNextGameRender(t *testing.T) {
	n := couldPlayNightIn{Date: "2026-11-19", Hour: 19, couldPlayNight: couldPlayNight{
		Title: "The Drowned Archive", Clock: "7 pm", Answer: "You're going", Tone: "go"}}
	t.Run("member sees title, answer and the link", func(t *testing.T) {
		html := nextGameHTML(t, buildNextGame([]couldPlayNightIn{n}, "2026-11-14", false, "/campaigns/c1/game-nights"))
		for _, want := range []string{"Next game", "Open Game nights", "The Drowned Archive", "Thu 19 Nov, 7 pm", "re going", ">5<", "days"} {
			if !strings.Contains(html, want) {
				t.Errorf("block missing %q:\n%s", want, html)
			}
		}
	})
	t.Run("a night the member owes reads Answer", func(t *testing.T) {
		o := n
		o.Answer, o.Owes, o.Href = "", true, "/campaigns/c1/game-nights?session=s1&date=2026-11-19"
		html := nextGameHTML(t, buildNextGame([]couldPlayNightIn{o}, "2026-11-14", false, "/campaigns/c1/game-nights"))
		if !strings.Contains(html, ">Answer<") || strings.Contains(html, "re going") {
			t.Errorf("owed answer not shown:\n%s", html)
		}
	})
	t.Run("nothing set", func(t *testing.T) {
		html := nextGameHTML(t, buildNextGame(nil, "2026-11-14", false, "/campaigns/c1/game-nights"))
		if !strings.Contains(html, "No game night set yet") {
			t.Errorf("empty line missing:\n%s", html)
		}
	})
}

// A guest is told when and nothing else: no title, no answer, no link into
// the table's own page, even though the nights they were built from had all.
func TestNextGameGuestLeaksNothing(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	nights := []GameNight{{
		SessionID: "secret-session-id", Name: "The Drowned Archive", Date: "2026-11-19", Time: "19:00", TZ: "UTC",
		Roster: []NightAnswer{{UserID: "me", Name: "Mira Voss", Answer: NightYes}}, Tally: NightTally{Going: 4, Maybe: 1},
	}}
	placed := couldPlayNightsFrom(nights, "/campaigns/c1", "me", time.UTC, true, now, "2026-11-14", "2027-01-12")
	html := nextGameHTML(t, buildNextGame(placed, "2026-11-14", true, "/campaigns/c1/game-nights"))
	for _, not := range []string{"Drowned Archive", "Mira", "secret-session-id", "game-nights", "Open Game nights", "Answer", "going", "Maybe", "4 of"} {
		if strings.Contains(html, not) {
			t.Errorf("guest block holds %q:\n%s", not, html)
		}
	}
	for _, want := range []string{"Next game", "Thu 19 Nov, 7 pm", "days"} {
		if !strings.Contains(html, want) {
			t.Errorf("guest block missing %q:\n%s", want, html)
		}
	}
}

// The route is reachable on any public campaign, so a guest gets the block
// only where the owner placed it on the Guests home.
func TestNextGameBlockGuestNeedsPlacement(t *testing.T) {
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/c1/next-game", nil), rec)
	c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", IsPublic: true}})
	// A zero Handler has no nights service: the gate must answer first.
	if err := (&Handler{}).NextGameBlock(c); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Errorf("an unplaced block must answer empty, got %d %q", rec.Code, rec.Body.String())
	}
}
