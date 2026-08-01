package calendar

// schedule_shots_test.go — THE FIDELITY HARNESS (C-CALV4-RSVP-P8 Part B).
//
// The operator's signature on the /schedule mockup carries one condition, in
// their own words: "as long as the result looks the same". That is a claim about
// PIXELS, and it cannot be asserted from Go strings — so this harness renders
// the BUILT surface, at the sealed mockup's own shot keys, into standalone HTML
// that a headless browser can shoot and a person can hold beside the wg-*
// stills.
//
// IT IS OPT-IN and writes nothing during a normal `go test ./...`: the gate is
// SCHEDULE_SHOTS=<dir>. A test that writes files into a repo on every run is a
// test that eventually writes the wrong files.
//
// WHAT IT RENDERS IS THE REAL PRODUCER. scheduleBody is the same component the
// route serves; the fixture is the oracle's, so the numbers in a shot are the
// numbers the oracle already proved. The one thing it does NOT render is the
// app shell (layouts.App needs a request), which is why the still and the shot
// differ above the page head — stated, not hidden.

import (
	"context"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// scheduleShotData assembles a full ScheduleData from the oracle fixture,
// mirroring buildSchedule's assembly without its reads.
func scheduleShotData(isGM bool) ScheduleData {
	in := scheduleOracleInput(isGM)
	in.ViewerID = "u-kael"
	if !isGM {
		in.ViewerID = "u-tam"
	}
	in.OwnLanes = scheduleOracleAvail().Lanes[in.ViewerID]
	in.EventID, in.CalendarID = "ev-41", "cal-1"
	in.Session = &BenchRsvpSession{
		Name: "Session 41", DaysUntil: 4, Anchored: true,
		Instant: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC),
	}

	week := scheduleOracleWeek()
	data := ScheduleData{
		CampaignID: "camp-1", CampaignName: "Embers of the Reach",
		IsGM: isGM, CSRFToken: "csrf",
		WeekStart: week.Format("2006-01-02"),
		WeekLabel: week.Format("Mon 2 Jan 2006"),
		WeekRange: scheduleWeekRange(week),
		PrevHref:  "?week=2026-07-13", NextHref: "?week=2026-07-27",
		Zone: "America/Chicago", ZoneLeaf: "Chicago", ZoneSource: "member",
		ZoneFrame:  "times in Chicago · your zone",
		Band:       "evening",
		BandLabel:  "Evening 16–24",
		BandFrom:   16,
		BandTo:     24,
		Zoom:       "week",
		MotionLine: scheduleMotionLine,
		Proportion: scheduleProportionLine,
	}
	base := scheduleQuery(scheduleInput{Band: "evening", Zoom: "week", Scope: "week"}, data)
	in.Base = base
	data.BandOptions = scheduleBandOptions("camp-1", base, "evening")
	data.ZoomOptions = scheduleZoomOptions("camp-1", base, "week")
	data.Verdict = scheduleBuildVerdict(in)
	data.Matrix = scheduleBuildMatrix(in)
	data.Roster = scheduleBuildRoster(in)
	data.Painter = scheduleBuildPainter(in)
	data.Answer = scheduleBuildAnswer(in)
	return data
}

// scheduleShotPage wraps the rendered surface in a standalone document with both
// stylesheets INLINED, because a file:// page cannot reach /static.
const scheduleShotPage = `<!doctype html>
<html lang="en"{{if .Dark}} class="dark"{{end}}>
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}}</title>
<style>
html,body{margin:0;padding:0;font-family:Inter,"Liberation Sans",system-ui,sans-serif}
body{margin:0}
/* The wrapper carries the surface's OWN tokens, so the harness cannot invent a
   colour the product does not have. Setting them on <body> instead would leave
   the page head inheriting a light-mode ink on the dark shot — a harness
   artefact that would read as a real contrast defect. */
.shotwrap{padding:20px;background:var(--surface-page);color:var(--text-primary);min-height:100vh}
.shotwrap h1{color:var(--text-primary)}
{{.BenchCSS}}
{{.ScheduleCSS}}
</style></head>
<body><div class="cal-bench cal-schedule shotwrap">{{.Body}}</div></body></html>`

// TestScheduleFidelityShots renders the mockup's shot keys. Opt-in.
func TestScheduleFidelityShots(t *testing.T) {
	dir := os.Getenv("SCHEDULE_SHOTS")
	if dir == "" {
		t.Skip("set SCHEDULE_SHOTS=<dir> to write the fidelity harness pages")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	tpl := template.Must(template.New("shot").Parse(scheduleShotPage))
	benchCSS := scheduleCSS(t, "calendar-bench.css")
	schedCSS := scheduleCSS(t, "calendar-schedule.css")

	for _, role := range []struct {
		key  string
		isGM bool
	}{{"gm", true}, {"player", false}} {
		data := scheduleShotData(role.isGM)
		var body strings.Builder
		if err := scheduleBody(data).Render(context.Background(), &body); err != nil {
			t.Fatalf("render %s: %v", role.key, err)
		}
		head := strings.Builder{}
		if err := scheduleHead(data).Render(context.Background(), &head); err != nil {
			t.Fatalf("render head %s: %v", role.key, err)
		}
		full := head.String() + body.String() +
			`<div class="sc-proportion"><b>How this page is proportioned.</b> ` +
			template.HTMLEscapeString(data.Proportion) + `</div>` +
			`<p class="sc-motion">` + template.HTMLEscapeString(data.MotionLine) + `</p>`

		for _, theme := range []struct {
			key  string
			dark bool
		}{{"light", false}, {"dark", true}} {
			name := fmt.Sprintf("schedule-%s-%s.html", role.key, theme.key)
			f, err := os.Create(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("create %s: %v", name, err)
			}
			err = tpl.Execute(f, map[string]any{
				"Title":       "schedule " + role.key + " " + theme.key,
				"Dark":        theme.dark,
				"BenchCSS":    template.CSS(benchCSS),
				"ScheduleCSS": template.CSS(schedCSS),
				"Body":        template.HTML(full),
			})
			_ = f.Close()
			if err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
			t.Logf("wrote %s", name)
		}
	}
}
