package sessions

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

// couldPlayHTML renders the block as a reader would receive it.
func couldPlayHTML(t *testing.T, v couldPlayView) string {
	t.Helper()
	var b bytes.Buffer
	if err := whenWeCouldPlayBlock(v).Render(context.Background(), &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func TestAvailabilityStep(t *testing.T) {
	tests := []struct {
		name        string
		free, total int
		want        int
	}{
		{"all of five is most", 5, 5, stepMost},
		{"four of five is most", 4, 5, stepMost},
		{"exactly three quarters is most", 3, 4, stepMost},
		{"three of five is half", 3, 5, stepHalf},
		{"exactly half is half", 2, 4, stepHalf},
		{"under half is nothing", 2, 5, stepNone},
		{"none is nothing", 0, 6, stepNone},
		{"empty table is nothing", 0, 0, stepNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := availabilityStep(tt.free, tt.total); got != tt.want {
				t.Errorf("availabilityStep(%d,%d) = %d, want %d", tt.free, tt.total, got, tt.want)
			}
		})
	}
}

func TestStepPhrase(t *testing.T) {
	steps := func(from, to, step int) [couldPlayAxisHours]int {
		var s [couldPlayAxisHours]int
		for h := from; h < to; h++ {
			s[h-couldPlayAxisStart] = step
		}
		return s
	}
	tests := []struct {
		name  string
		steps [couldPlayAxisHours]int
		want  string
	}{
		{"evening, most", steps(19, 22, stepMost), "Evening, about 7 to 10"},
		{"afternoon, most", steps(13, 17, stepMost), "Afternoon, about 1 to 5"},
		{"half says might work", steps(13, 17, stepHalf), "Afternoon might work"},
		{"late evening to midnight", steps(21, 24, stepMost), "Late evening, about 9 to 12"},
		{"most wins over a longer half", func() [couldPlayAxisHours]int {
			s := steps(12, 17, stepHalf)
			e := steps(19, 21, stepMost)
			for i := range s {
				if e[i] != 0 {
					s[i] = e[i]
				}
			}
			return s
		}(), "Evening, about 7 to 9"},
		{"nothing", [couldPlayAxisHours]int{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stepPhrase(tt.steps); got != tt.want {
				t.Errorf("stepPhrase = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShadeGradientBlends(t *testing.T) {
	var steps [couldPlayAxisHours]int
	for h := 7; h < 10; h++ {
		steps[h] = stepMost
	}
	g := shadeGradient(steps)
	alpha := regexp.MustCompile(`var\(--color-text-muted\) (\d+)%`)
	var as []int
	for _, m := range alpha.FindAllStringSubmatch(g, -1) {
		var a int
		for _, c := range m[1] {
			a = a*10 + int(c-'0')
		}
		as = append(as, a)
	}
	if len(as) != couldPlayAxisHours {
		t.Fatalf("want one stop per hour, got %d in %s", len(as), g)
	}
	// A hard edge would jump the full darkest strength between neighbours.
	for i := 1; i < len(as); i++ {
		d := as[i] - as[i-1]
		if d < 0 {
			d = -d
		}
		if d > couldPlayMaxAlpha*3/4 {
			t.Errorf("hour %d jumps %d points, the bands do not blend: %v", i, d, as)
		}
	}
	// The strength must fade in on the hour before the run and out after it.
	if as[6] == 0 || as[10] == 0 || as[6] >= as[7] {
		t.Errorf("edges should be soft, got %v", as)
	}
	if !strings.HasPrefix(g, "linear-gradient(to right, ") {
		t.Errorf("not a left-to-right gradient: %s", g)
	}
}

// day builds an overlay day whose hours from..to have free members.
func day(date string, wd time.Weekday, from, to, free int) OverlayDay {
	m := run(from, to, free)
	return hoursFree(date, int(wd), m)
}

func TestBuildCouldPlay(t *testing.T) {
	const today = "2026-11-14"
	thu := day("2026-11-19", time.Thursday, 19, 22, 5) // most of 5
	sat := day("2026-11-21", time.Saturday, 13, 17, 3) // half of 5
	sun := day("2026-11-22", time.Sunday, 19, 22, 2)   // under half
	blip := day("2026-11-20", time.Friday, 19, 20, 5)  // one hour only
	week := func(total int, days ...OverlayDay) []WeekOverlay {
		return []WeekOverlay{{TotalMembers: total, Days: days}}
	}
	night := couldPlayNightIn{Date: "2026-11-26", Weekday: time.Thursday, Hour: 19, couldPlayNight: couldPlayNight{
		Title: "Session 14", Clock: "7 pm", Left: 58, Width: 33, Answer: "You're going", Tone: "go"}}

	t.Run("rows only for nights, polls and shading", func(t *testing.T) {
		v := buildCouldPlay(couldPlayInput{CampaignID: "c1", Today: today, Last: "2026-12-18",
			Weeks: week(5, thu, sat, sun, blip), Nights: []couldPlayNightIn{night},
			PollDates: map[string]bool{"2026-11-28": true}})
		var dates []string
		for _, r := range v.Rows {
			dates = append(dates, r.Date)
		}
		if got, want := strings.Join(dates, ","), "2026-11-19,2026-11-21,2026-11-26,2026-11-28"; got != want {
			t.Errorf("rows = %s, want %s", got, want)
		}
		if p := v.Rows[0].Phrase; p != "Evening, about 7 to 10" {
			t.Errorf("thursday phrase = %q", p)
		}
		if p := v.Rows[1].Phrase; p != "Afternoon might work" {
			t.Errorf("saturday phrase = %q", p)
		}
		if p := v.Rows[3].Phrase; p != "In a poll" || v.Rows[3].Shade != "" {
			t.Errorf("poll-only row = %q shade=%q", p, v.Rows[3].Shade)
		}
		if !v.Rows[3].Extra || v.Rows[2].Extra || v.ExtraN != 1 {
			t.Errorf("three rows show, the rest fold: %+v extra=%d", v.Rows, v.ExtraN)
		}
	})
	t.Run("a poll date that is also shaded joins the phrase", func(t *testing.T) {
		v := buildCouldPlay(couldPlayInput{CampaignID: "c1", Today: today, Last: "2026-12-18",
			Weeks: week(5, thu), PollDates: map[string]bool{"2026-11-19": true}})
		if got := v.Rows[0].Phrase; got != "Evening, about 7 to 10 · in a poll" {
			t.Errorf("phrase = %q", got)
		}
	})
	t.Run("a table of three or fewer gets no shading or phrase", func(t *testing.T) {
		v := buildCouldPlay(couldPlayInput{CampaignID: "c1", Today: today, Last: "2026-12-18",
			Weeks: week(3, thu, sat), Nights: []couldPlayNightIn{night},
			PollDates: map[string]bool{"2026-11-28": true}})
		for _, r := range v.Rows {
			if r.Shade != "" || (r.Phrase != "" && r.Phrase != "In a poll") {
				t.Errorf("small table leaked availability on %s: %+v", r.Date, r)
			}
		}
		if len(v.Rows) != 2 {
			t.Errorf("only the night and the poll date remain, got %d rows", len(v.Rows))
		}
	})
	t.Run("days before today are left out", func(t *testing.T) {
		v := buildCouldPlay(couldPlayInput{CampaignID: "c1", Today: "2026-11-20", Last: "2026-12-18", Weeks: week(5, thu)})
		if len(v.Rows) != 0 {
			t.Errorf("got %+v", v.Rows)
		}
	})
	t.Run("empty says so honestly", func(t *testing.T) {
		html := couldPlayHTML(t, buildCouldPlay(couldPlayInput{CampaignID: "c1", Today: today, Last: "2026-12-18"}))
		if !strings.Contains(html, "Nothing planned yet, and no times stand out.") {
			t.Errorf("empty line missing:\n%s", html)
		}
	})
}

func TestCouldPlayGuest(t *testing.T) {
	night := func(date string, wd time.Weekday, hour float64, clock string) couldPlayNightIn {
		return couldPlayNightIn{Date: date, Weekday: wd, Hour: hour, couldPlayNight: couldPlayNight{Clock: clock, Left: 58, Width: 33}}
	}
	nights := []couldPlayNightIn{
		night("2026-11-19", time.Thursday, 19, "7 pm"),
		night("2026-11-26", time.Thursday, 19, "7 pm"),
		night("2026-11-22", time.Sunday, 14, "2 pm"),
	}
	in := couldPlayInput{CampaignID: "c1", Guest: true, Today: "2026-11-14", Last: "2026-12-18", Nights: nights,
		// Everything a member would see: a guest must get none of it.
		Weeks:     []WeekOverlay{{TotalMembers: 5, Days: []OverlayDay{day("2026-11-20", time.Friday, 19, 22, 5)}}},
		PollDates: map[string]bool{"2026-11-21": true}}
	v := buildCouldPlay(in)
	html := couldPlayHTML(t, v)
	if v.Usually != "Usually plays Thursday evenings" || !strings.Contains(html, "Usually plays Thursday evenings") {
		t.Errorf("usually line = %q", v.Usually)
	}
	if len(v.Rows) != 3 {
		t.Errorf("a guest gets the set nights only, got %d rows", len(v.Rows))
	}
	for _, r := range v.Rows {
		if r.Shade != "" || r.Phrase != "" {
			t.Errorf("guest row carries availability or poll words: %+v", r)
		}
	}
	for _, not := range []string{"in a poll", "In a poll", "might work", "about", "Darker", "linear-gradient", "Answer", "going"} {
		if strings.Contains(html, not) {
			t.Errorf("guest block holds %q:\n%s", not, html)
		}
	}
	if !strings.Contains(html, "7 pm") {
		t.Errorf("guest should see the time:\n%s", html)
	}
	one := buildCouldPlay(couldPlayInput{CampaignID: "c1", Guest: true, Today: "2026-11-14", Last: "2026-12-18", Nights: nights[:1]})
	if one.Usually != "" {
		t.Errorf("one night is no habit, got %q", one.Usually)
	}
}

func TestCouldPlayNightsFrom(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, loc)
	roster := func(answer string, recheck bool) []NightAnswer {
		return []NightAnswer{{UserID: "me", Name: "Mira Voss", Answer: answer, Recheck: recheck}, {UserID: "u2", Name: "Dax Orrin", Answer: NightYes}}
	}
	mk := func(date, tm string, r []NightAnswer) GameNight {
		return GameNight{SessionID: "s1", Name: "The Ember Coast", Date: date, Time: tm, TZ: "UTC", Roster: r, Tally: NightTally{Going: 4, Maybe: 1}}
	}
	nights := []GameNight{
		mk("2026-11-19", "19:00", roster(NightYes, false)),
		mk("2026-11-20", "19:30", roster(NightNoAnswer, false)),
		mk("2026-11-21", "20:00", roster(NightMaybe, false)),
		mk("2026-11-22", "20:00", roster(NightNo, false)),
		mk("2026-11-23", "20:00", roster(NightYes, true)),
		mk("2026-12-30", "20:00", roster(NightYes, false)),
	}
	got := couldPlayNightsFrom(nights, "/campaigns/c1", "me", loc, false, now, "2026-11-14", "2026-12-18")
	if len(got) != 5 {
		t.Fatalf("want 5 nights in the window, got %d", len(got))
	}
	want := []struct {
		answer string
		owes   bool
		clock  string
	}{
		{"You're going", false, "7 pm"}, {"", true, "7:30 pm"}, {"Maybe", false, "8 pm"},
		{"Can't go", false, "8 pm"}, {"", true, "8 pm"},
	}
	for i, w := range want {
		if got[i].Answer != w.answer || got[i].Owes != w.owes || got[i].Clock != w.clock {
			t.Errorf("night %d = %+v, want %+v", i, got[i], w)
		}
		if got[i].Title != "The Ember Coast" {
			t.Errorf("member sees the title, got %q", got[i].Title)
		}
	}
	guest := couldPlayNightsFrom(nights, "/campaigns/c1", "", loc, true, now, "2026-11-14", "2026-12-18")
	for _, n := range guest {
		if n.Title != "" || n.Answer != "" || n.Owes || n.Href != "" {
			t.Errorf("guest night leaks: %+v", n)
		}
	}
	t.Run("a night in another zone lands on the viewer's day", func(t *testing.T) {
		ny := time.FixedZone("EST", -5*3600)
		n := GameNight{SessionID: "s", Date: "2026-11-20", Time: "02:00", TZ: "UTC", Roster: roster(NightYes, false)}
		out := couldPlayNightsFrom([]GameNight{n}, "/c", "me", ny, false, now, "2026-11-14", "2026-12-18")
		if len(out) != 1 || out[0].Date != "2026-11-19" || out[0].Clock != "9 pm" {
			t.Errorf("got %+v", out)
		}
	})
}

// Whatever a member's block says, it must not hold names, ids, counts or the
// tallies of a poll or a night.
func TestCouldPlayMemberBlockLeaksNothing(t *testing.T) {
	n := couldPlayNightIn{Date: "2026-11-19", Weekday: time.Thursday, Hour: 19, couldPlayNight: couldPlayNight{
		Title: "The Ember Coast", Clock: "7 pm", Left: 58, Width: 33, Owes: true, Href: "/campaigns/c1/game-nights?session=s1&date=2026-11-19"}}
	days := []OverlayDay{
		day("2026-11-19", time.Thursday, 19, 22, 5), day("2026-11-21", time.Saturday, 13, 17, 3),
		day("2026-11-22", time.Sunday, 18, 21, 4), day("2026-11-24", time.Tuesday, 18, 21, 4),
	}
	v := buildCouldPlay(couldPlayInput{CampaignID: "c1", Today: "2026-11-14", Last: "2026-12-18",
		Weeks: []WeekOverlay{{TotalMembers: 5, Days: days}}, Nights: []couldPlayNightIn{n},
		PollDates: map[string]bool{"2026-11-24": true}})
	html := couldPlayHTML(t, v)
	for _, bad := range []*regexp.Regexp{
		regexp.MustCompile(`\d+ of \d+`),
		regexp.MustCompile(`(?i)\b\d+ (free|going|coming|yes|maybe|no|answers?|members?|players?)\b`),
		regexp.MustCompile(`(?i)\beveryone\b`),
		regexp.MustCompile(`(?i)user-?id|avatar|<img`),
	} {
		if m := bad.FindString(html); m != "" {
			t.Errorf("member block holds %q", m)
		}
	}
	for _, want := range []string{"When we could play", "Open Game nights", "Show 1 more day", "Darker means more of the table is usually free", "Answer", "noon"} {
		if !strings.Contains(html, want) {
			t.Errorf("block missing %q", want)
		}
	}
}

func TestUsuallyLine(t *testing.T) {
	n := func(wd time.Weekday, hour float64) couldPlayNightIn { return couldPlayNightIn{Weekday: wd, Hour: hour} }
	tests := []struct {
		name   string
		nights []couldPlayNightIn
		want   string
	}{
		{"none", nil, ""},
		{"one", []couldPlayNightIn{n(time.Thursday, 19)}, ""},
		{"thursday evenings", []couldPlayNightIn{n(time.Thursday, 19), n(time.Thursday, 20), n(time.Sunday, 14)}, "Usually plays Thursday evenings"},
		{"sunday afternoons", []couldPlayNightIn{n(time.Sunday, 14), n(time.Sunday, 13)}, "Usually plays Sunday afternoons"},
		{"late evening reads as evening", []couldPlayNightIn{n(time.Friday, 22.5), n(time.Friday, 22.5)}, "Usually plays Friday evenings"},
		{"no times, just the day", []couldPlayNightIn{n(time.Friday, -1), n(time.Friday, -1)}, "Usually plays Fridays"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usuallyLine(tt.nights); got != tt.want {
				t.Errorf("usuallyLine = %q, want %q", got, tt.want)
			}
		})
	}
}

// When we play says how much of the table is free in words, never "N of M".
func TestWhenWePlayWords(t *testing.T) {
	var b bytes.Buffer
	times := []playTime{
		{Weekday: time.Thursday, Start: 19, End: 22, Free: 4, Total: 5},
		{Weekday: time.Sunday, Start: 14, End: 18, Free: 3, Total: 5},
	}
	if err := whenWePlayBlock("c1", times, "UTC").Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	for _, want := range []string{"Most of the table", "About half the table"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q:\n%s", want, html)
		}
	}
	if m := regexp.MustCompile(`\d+ of \d+`).FindString(html); m != "" {
		t.Errorf("a count leaked: %q", m)
	}
	if times[0].Percent() != 100 || times[1].Percent() != 50 {
		t.Errorf("bar should be two steps, got %d and %d", times[0].Percent(), times[1].Percent())
	}
}

func TestHomeHasBlock(t *testing.T) {
	col := &campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{{Columns: []campaigns.DashboardColumn{{Blocks: []campaigns.DashboardBlock{{Type: "party"}, {Type: campaigns.BlockWhenWeCouldPlay}}}}}}}
	tile := &campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{{Kind: "tiles", Tiles: []tilegrid.Tile{{Type: campaigns.BlockWhenWeCouldPlay}}}}}
	none := &campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{{Columns: []campaigns.DashboardColumn{{Blocks: []campaigns.DashboardBlock{{Type: "party"}}}}}}}
	tests := []struct {
		name string
		l    *campaigns.DashboardLayout
		want bool
	}{
		{"in a column", col, true},
		{"as a tile", tile, true},
		{"not placed", none, false},
		{"nil layout", nil, false},
		{"built-in guest home", campaigns.DefaultGuestDashboardLayout(), false},
	}
	for _, tt := range tests {
		if got := homeHasBlock(tt.l, campaigns.BlockWhenWeCouldPlay); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
