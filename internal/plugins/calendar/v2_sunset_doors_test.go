// v2_sunset_doors_test.go — C-CALV4-V2SUNSET R2-4, §3's door sweep ([VS-2]
// SIGNED) and §13's completeness gate, shipped as a test rather than as a grep
// pasted into a PR body.
//
// WHY THE GREP IS A TEST. §13 requires the completeness grep's output in the PR
// and says "anything else is a door you missed". A grep in a PR body is checked
// once, by whoever wrote it; the twentieth door added six months from now is
// caught by nothing. This file runs the same grep over the same tree with the
// same permitted-hit list, so a new /calendar/v2 link fails CI with the reason
// attached.
//
// THE PERMITTED LIST IS THE RULING, not a convenience. Every entry below is a
// class [VS-2] SIGNED named, and each carries WHY it is permitted. Adding an
// entry to it is amending a signed block and needs the same care.
package calendar

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- the individual doors ---------------------------------------------------

// TestSunsetDoor_RsvpEventLinkIsTheBench is the EGRESS door that reaches an
// inbox ([VS-2] SIGNED, rsvp_handler.go). It is the strongest argument for
// [VS-3]'s zero-removals ruling: mail already sent carries the old URL, and
// that URL must keep resolving.
func TestSunsetDoor_RsvpEventLinkIsTheBench(t *testing.T) {
	got := rsvpEventLink("camp-1", &Event{ID: "e-1", CalendarID: "cal-1", Name: "The Muster"})
	if want := "/campaigns/camp-1/apps/calendar"; got != want {
		t.Errorf("rsvpEventLink = %q, want %q — this URL goes into NOTIFICATION EMAIL and leaves "+
			"the application entirely", got, want)
	}
	if strings.Contains(got, "/calendar/v2") {
		t.Error("R2-4 must MINT no further /calendar/v2 URLs into email; the already-sent ones " +
			"keep working because no route is removed, and C-CALV4-SHELL-REMOVAL is what finally " +
			"breaks them (accepted cost)")
	}
}

// TestSunsetDoor_EntityHelpersRenamedAndRePointed covers both halves of
// entity_calendar_block.go: the "Open full calendar" header link and the
// linked-event row, whose date cursor [VS-13] drops.
func TestSunsetDoor_EntityHelpersRenamedAndRePointed(t *testing.T) {
	if got := openCalendarHref("camp-1", "cal-1"); got != "/campaigns/camp-1/apps/calendar" {
		t.Errorf("openCalendarHref = %q, want the Bench", got)
	}
	// The bound-calendar id is deliberately ignored — the Bench cannot honour it
	// ([VS-12] measured). Bound and unbound must therefore agree, or the door
	// would be claiming a selection it does not make.
	if openCalendarHref("camp-1", "cal-1") != openCalendarHref("camp-1", "") {
		t.Error("[VS-12]: openCalendarHref must not pretend to select a calendar the Bench " +
			"never reads")
	}
	evt := Event{Year: 1523, Month: 2, Day: 7}
	got := entityEventHref("camp-1", evt)
	if got != "/campaigns/camp-1/apps/calendar" {
		t.Errorf("entityEventHref = %q, want the Bench", got)
	}
	// [VS-13]: the ?year=&month=&day= cursor is DROPPED, and the drop is
	// asserted rather than left to be discovered. It rides with
	// C-CALV4-BENCH-CALID.
	for _, param := range []string{"year=", "month=", "day="} {
		if strings.Contains(got, param) {
			t.Errorf("entityEventHref carries %q — the Bench parses `y`/`m` (a MONTH cursor in "+
				"one calendar's own month list) and no day at all, so a cursor here would be a "+
				"date the target cannot honour", param)
		}
	}
}

// TestSunsetDoor_CalendarSettingsBackArrow renders the settings page and pins
// its one door ([VS-2] SIGNED, calendar_settings.templ).
func TestSunsetDoor_CalendarSettingsBackArrow(t *testing.T) {
	cc := &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Imix"}, MemberRole: campaigns.RoleOwner,
	}
	cal := &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Harptos", Mode: ModeFantasy}
	var buf bytes.Buffer
	if err := CalendarSettingsPage(cc, cal, "tok").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render settings page: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "/campaigns/camp-1/apps/calendar") {
		t.Error("the settings page's back arrow does not lead to the Bench")
	}
	if strings.Contains(html, "/calendar/v2") {
		t.Error("the settings page still leaves for the V2 shell")
	}
}

// TestSunsetDoor_WidgetPopupLink pins the ONE door that lives in a JS file
// ([VS-2] SIGNED, static/js/calendar_widget.js). Asserted against the source
// because the popup is built by string concatenation at runtime and there is no
// Go render to drive.
func TestSunsetDoor_WidgetPopupLink(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("static", "js", "calendar_widget.js"))
	if err != nil {
		t.Fatalf("reading calendar_widget.js: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "'/apps/calendar\"") && !strings.Contains(body, "/apps/calendar") {
		t.Error("the dashboard widget's 'Edit in Calendar' popup no longer links to the Bench")
	}
	if strings.Contains(body, "/calendar/v2") {
		t.Error("the dashboard widget still links to the V2 shell")
	}
}

// --- §13's completeness gate ------------------------------------------------

// sunsetPermittedV2Hit reports whether a file may still mention /calendar/v2,
// and returns the reason. The list is [VS-2] SIGNED's permitted classes, as
// amended — and nothing else.
func sunsetPermittedV2Hit(rel string) (string, bool) {
	switch {
	case strings.HasSuffix(rel, "_test.go"):
		return "test files assert the shell's routes still exist", true
	case strings.HasSuffix(rel, "_templ.go"):
		return "generated from a .templ that is itself judged", true

	// The routes themselves. R2-4 removes NOTHING ([VS-1] SIGNED BY THE
	// OPERATOR): the shell stays reachable by URL, just not by click.
	case strings.HasSuffix(rel, "calendar/routes.go"):
		return "the route registrations and their comments", true
	case strings.HasSuffix(rel, "calendar/handler_v2.go"):
		return "ShowV2's own doc comments and v2CalendarRedirect's, which names the week/day gap", true

	// The settings surface that merely SHARES the prefix (§7, [VS-7] SIGNED).
	// It is not a view, it does not move, and it SURVIVES the shell.
	case strings.Contains(rel, "subresource_v2"):
		return "the sub-resource settings namespace — survives the shell (§7)", true

	// The frozen shell's own interior.
	case strings.Contains(rel, "calendar_v2"):
		return "the V2 shell itself and its helpers — frozen, not opened", true
	case strings.HasSuffix(rel, "js/event_grid.js"):
		return "the shell's own grid script", true

	// THE V1 EMBED'S CHIP — the one door deliberately LEFT on V2 ([VS-13]).
	case strings.HasSuffix(rel, "calendar/calendar.templ"):
		return "dayCellEventHref, the V1 embed's chip — EXCEPTION, retires with the embed", true

	// RETAINED-BUT-UNROUTED DEAD CODE. Four references no user can reach.
	// FORBIDDEN to edit in this slice ([VS-18]); booked as C-CAL-DASH-DEADCODE.
	case strings.HasSuffix(rel, "calendar/app_dashboard.templ"):
		return "CalendarAppDashboardPage — unrouted dead code, booked as C-CAL-DASH-DEADCODE", true

	// The demo tree is not the product.
	case strings.Contains(rel, "templates/demo"):
		return "the demo index", true

	// handler.go and builder_handler.go were permitted while the sweep landed
	// ahead of the redirects. STAGE 4 RE-POINTED ALL SIX REDIRECT TARGETS, so
	// the exemption is GONE and this list is one class shorter than it was a
	// commit ago. A guard that only ever grows is a guard on its way to being
	// decorative.
	}
	return "", false
}

// TestSunset_NoLiveDoorRemains is §13's grep, executed.
//
// It walks internal/ for the string "calendar/v2" and fails on any hit outside
// the permitted classes. A comment that merely NARRATES the sweep is allowed
// (the sweep's own commentary says "/calendar/v2" a dozen times); what is
// forbidden is a hit that could be a URL a user follows, so lines whose hit sits
// inside a Go or JS line comment are not counted.
func TestSunset_NoLiveDoorRemains(t *testing.T) {
	root := filepath.Join("..", "..", "..", "internal")
	var offenders []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".templ" && ext != ".js" {
			return nil
		}
		rel := filepath.ToSlash(path)
		if _, ok := sunsetPermittedV2Hit(rel); ok {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for i, line := range strings.Split(string(src), "\n") {
			idx := strings.Index(line, "calendar/v2")
			if idx < 0 {
				continue
			}
			// A hit inside a line comment is narration, not a door.
			if c := strings.Index(line, "//"); c >= 0 && c < idx {
				continue
			}
			offenders = append(offenders, rel+":"+itoa(i+1)+": "+strings.TrimSpace(line))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal/: %v", err)
	}
	if len(offenders) > 0 {
		t.Errorf("[VS-2] SIGNED: %d live door(s) to the V2 shell remain, and every one of them is "+
			"a place the operator's \"legacy calendar still shows up\" is still true:\n\t%s\n\n"+
			"If a new one is legitimate, it belongs in sunsetPermittedV2Hit with its reason — "+
			"amending a signed block, not silencing a test.",
			len(offenders), strings.Join(offenders, "\n\t"))
	}
}
