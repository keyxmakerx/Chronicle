package sessions

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// hoursFree builds a day's 24 cells with the given free counts from hour
// start to end (exclusive), zero elsewhere.
func hoursFree(date string, weekday int, free map[int]int) OverlayDay {
	d := OverlayDay{Date: date, Weekday: weekday, Hours: make([]OverlayHour, 24)}
	for h, n := range free {
		d.Hours[h].Free = n
	}
	return d
}

func run(from, to, n int) map[int]int {
	m := map[int]int{}
	for h := from; h < to; h++ {
		m[h] = n
	}
	return m
}

func TestPlayTimesFrom(t *testing.T) {
	// 2026-11-09 is a Monday.
	week := func(total int, days ...OverlayDay) WeekOverlay {
		return WeekOverlay{TotalMembers: total, Days: days}
	}
	thu := hoursFree("2026-11-12", int(time.Thursday), run(19, 22, 5))
	sun := hoursFree("2026-11-15", int(time.Sunday), run(14, 18, 4))
	tue := hoursFree("2026-11-10", int(time.Tuesday), run(20, 22, 3))
	oneHour := hoursFree("2026-11-11", int(time.Wednesday), run(19, 20, 5))
	minority := hoursFree("2026-11-13", int(time.Friday), run(19, 22, 2))

	t.Run("best windows first, only a majority, never a single hour", func(t *testing.T) {
		got := playTimesFrom([]WeekOverlay{week(5, tue, oneHour, thu, minority, sun)}, "2026-11-09")
		var days []string
		for _, p := range got {
			days = append(days, p.Day())
		}
		if want := "Thursdays,Sundays,Tuesdays"; strings.Join(days, ",") != want {
			t.Errorf("days = %v, want %s", days, want)
		}
		if got[0].Span() != "7 to 10 pm" || got[0].Free != 5 || got[0].Total != 5 {
			t.Errorf("first = %+v (%s)", got[0], got[0].Span())
		}
	})
	t.Run("days before today are skipped", func(t *testing.T) {
		got := playTimesFrom([]WeekOverlay{week(5, tue, thu)}, "2026-11-11")
		if len(got) != 1 || got[0].Weekday != time.Thursday {
			t.Errorf("got %+v, want Thursday only", got)
		}
	})
	t.Run("a weekday keeps its first appearance", func(t *testing.T) {
		next := hoursFree("2026-11-19", int(time.Thursday), run(18, 23, 5))
		got := playTimesFrom([]WeekOverlay{week(5, thu), week(5, next)}, "2026-11-09")
		if len(got) != 1 || got[0].Start != 19 {
			t.Errorf("got %+v, want this week's Thursday", got)
		}
	})
	t.Run("the window's free count is its weakest hour", func(t *testing.T) {
		m := run(19, 22, 5)
		m[20] = 3
		got := playTimesFrom([]WeekOverlay{week(5, hoursFree("2026-11-12", int(time.Thursday), m))}, "2026-11-09")
		if len(got) != 1 || got[0].Free != 3 {
			t.Errorf("got %+v, want 3 of 5", got)
		}
	})
	t.Run("a table of three or fewer has no times to show", func(t *testing.T) {
		for _, n := range []int{1, 2, 3} {
			if got := playTimesFrom([]WeekOverlay{week(n, thu)}, "2026-11-09"); len(got) != 0 {
				t.Errorf("table of %d: got %+v", n, got)
			}
		}
	})
	t.Run("at most three", func(t *testing.T) {
		days := []OverlayDay{thu, sun, tue, hoursFree("2026-11-11", int(time.Wednesday), run(19, 22, 4))}
		if got := playTimesFrom([]WeekOverlay{week(5, days...)}, "2026-11-09"); len(got) != playTimesShown {
			t.Errorf("len = %d", len(got))
		}
	})
}

func TestPlayTimeSpan(t *testing.T) {
	tests := []struct {
		start, end int
		want       string
	}{
		{19, 22, "7 to 10 pm"},
		{14, 18, "2 to 6 pm"},
		{9, 11, "9 to 11 am"},
		{10, 14, "10 am to 2 pm"},
		{11, 12, "11 am to noon"},
		{12, 15, "noon to 3 pm"},
		{20, 24, "8 pm to midnight"},
		{0, 2, "midnight to 2 am"},
	}
	for _, tt := range tests {
		if got := (playTime{Start: tt.start, End: tt.end}).Span(); got != tt.want {
			t.Errorf("Span(%d,%d) = %q, want %q", tt.start, tt.end, got, tt.want)
		}
	}
}

func TestRecapExcerpt(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"first paragraphs while they fit", "<p>The party bargained.</p><p>Kettle lifted a ledger.</p>", "The party bargained.\n\nKettle lifted a ledger."},
		{"headings and tags dropped", "<h2>Session 13</h2><p>The <strong>tide-witch</strong> &amp; Oona.</p>", "The tide-witch & Oona."},
		{"empty", "<p> </p><h3>Only a heading</h3>", ""},
		{"scripts are not markup in the excerpt", "<p>Hello <script>alert(1)</script>there</p>", "Hello alert(1) there"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := recapExcerpt(tt.in); got != tt.want {
				t.Errorf("recapExcerpt = %q, want %q", got, tt.want)
			}
		})
	}
	long := "<p>" + strings.Repeat("word ", 200) + "</p><p>second</p>"
	got := recapExcerpt(long)
	if !strings.HasSuffix(got, "…") || len([]rune(got)) > recapExcerptLen+1 || strings.Contains(got, "second") {
		t.Errorf("a long first paragraph should be cut at a word, got %d runes: %q", len([]rune(got)), got)
	}
	two := "<p>" + strings.Repeat("a ", 100) + "</p><p>" + strings.Repeat("b ", 150) + "</p>"
	if got := recapExcerpt(two); strings.Contains(got, "b") {
		t.Errorf("a second paragraph that does not fit is left out whole, got %q", got)
	}
}

func lastTimeHandler(recapHTML *string) *Handler {
	date := "2000-01-01"
	night := Session{ID: "s1", CampaignID: "camp-1", Name: "Session 13", ScheduledDate: &date, Status: StatusCompleted}
	repo := &mockSessionRepo{
		listByCampaignFn: func(context.Context, string) ([]Session, error) { return []Session{night}, nil },
		findByIDFn: func(_ context.Context, id string) (*Session, error) {
			s := night
			s.RecapHTML = recapHTML
			return &s, nil
		},
	}
	h := NewHandler(NewSessionService(repo, nil, &stubEntityVisibility{}))
	h.SetMentionHider(recapSeesOnly{})
	return h
}

// Last time shows the latest night's recap as its viewer may read it: a
// player, and the Director previewing as one, never read GM-only text or the
// name of a page they can't open; the Director not previewing reads it all.
func TestLastTimeBlock(t *testing.T) {
	recap := `<p>We rested. <span data-secret="true">The duke followed us.</span></p>` +
		`<p>We met <a data-mention-id="` + recapHiddenPage + `">The vampire duke</a>.</p>`
	call := func(h *Handler, role campaigns.Role, preview bool) string {
		c, rec := recapCtx(role, false, http.MethodGet, "")
		if preview {
			c.Request().AddCookie(&http.Cookie{Name: "chronicle_view_as_player", Value: "1"})
		}
		if err := h.LastTimeBlock(c); err != nil {
			t.Fatalf("LastTimeBlock: %v", err)
		}
		return rec.Body.String()
	}
	h := lastTimeHandler(&recap)

	player := call(h, campaigns.RolePlayer, false)
	for _, want := range []string{"Session 13", "We rested.", "Read the whole recap", "/campaigns/camp-1/sessions/s1"} {
		if !strings.Contains(player, want) {
			t.Errorf("player's block missing %q:\n%s", want, player)
		}
	}
	for _, not := range []string{"followed us", "vampire duke"} {
		if strings.Contains(player, not) {
			t.Errorf("player's block holds %q:\n%s", not, player)
		}
	}
	if previewing := call(h, campaigns.RoleOwner, true); strings.Contains(previewing, "followed us") || strings.Contains(previewing, "vampire duke") {
		t.Errorf("the Director previewing as a player must read what a player reads:\n%s", previewing)
	}
	if own := call(h, campaigns.RoleOwner, false); !strings.Contains(own, "followed us") {
		t.Errorf("the Director not previewing reads the whole recap:\n%s", own)
	}

	empty := ""
	if body := call(lastTimeHandler(&empty), campaigns.RolePlayer, false); body != "" {
		t.Errorf("no recap should remove the block, got:\n%s", body)
	}
	if body := call(lastTimeHandler(nil), campaigns.RolePlayer, false); body != "" {
		t.Errorf("no recap should remove the block, got:\n%s", body)
	}
}
