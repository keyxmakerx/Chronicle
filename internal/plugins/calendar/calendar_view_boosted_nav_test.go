package calendar

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// calendar_view_boosted_nav_test.go pins that the calendar's own page
// survives a boosted sidebar navigation the same way every other page must
// (internal/plugins/entities/characters_boosted_nav_test.go carries the full
// explanation): every sidebar link is hx-boost with hx-target="#main-content",
// boot.js sets htmx.config.allowScriptTags=false, and under that setting htmx
// REMOVES any <script> tag inside the swapped fragment rather than just
// declining to execute it. calendar_view.js and calendar_editor.js must
// therefore ship from the plugin body-script registry
// (internal/app/routes.go's pluginBodyScripts), never a `<script src>` inside
// the page body itself.

// castSwappedRegion returns the substring htmx would keep on a boosted
// sidebar navigation: the contents of <main id="main-content">. Both bounds
// are checked before use, so a layout rename fails loudly here instead of
// silently reducing the assertion to a scan of the empty string.
func castSwappedRegion(t *testing.T, page string) string {
	t.Helper()
	const marker = `id="main-content"`
	at := strings.Index(page, marker)
	if at < 0 {
		t.Fatalf("no %s in the rendered page — the App layout's swap target was renamed and this "+
			"boosted-navigation assertion just stopped reading anything", marker)
	}
	open := strings.Index(page[at:], ">")
	if open < 0 {
		t.Fatal("unterminated <main> open tag in the rendered page")
	}
	rest := page[at+open+1:]
	end := strings.Index(rest, "</main>")
	if end < 0 {
		t.Fatal("no </main> in the rendered page")
	}
	return rest[:end]
}

// renderCalendarViewPage renders CalendarViewPage with a minimal but valid
// fixture. calendarViewContent dereferences data.Calendar.Name directly (the
// page's second breadcrumb), so, unlike every other CalendarViewData field,
// the fixture cannot leave Calendar nil.
func renderCalendarViewPage(t *testing.T) string {
	t.Helper()
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp1", Name: "Test Campaign"}}
	data := CalendarViewData{
		CampaignID: "camp1",
		CalendarID: "cal1",
		Calendar:   &Calendar{ID: "cal1", CampaignID: "camp1", Name: "Test Calendar"},
		CanEdit:    true,
	}
	var sb strings.Builder
	if err := CalendarViewPage(cc, data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render CalendarViewPage: %v", err)
	}
	return sb.String()
}

// TestCalendarViewPageMountsNoScriptInsideTheSwappedRegion checks the
// swapped region only (inside <main>), not the whole document: the shell's
// own script tags sit outside it and a boosted navigation is meant to keep
// those.
func TestCalendarViewPageMountsNoScriptInsideTheSwappedRegion(t *testing.T) {
	swapped := castSwappedRegion(t, renderCalendarViewPage(t))
	if n := strings.Count(swapped, "<script"); n != 0 {
		t.Errorf("the calendar view page emits %d <script> tag(s) inside #main-content; htmx DELETES "+
			"every one of them on a boosted sidebar navigation (allowScriptTags=false), so "+
			"calendar_view.js/calendar_editor.js would wire up on a direct load and silently not "+
			"through the sidebar. Contribute them to the plugin body-script registry in "+
			"internal/app/routes.go instead.", n)
	}
}

// TestCalendarScriptsShipFromThePluginBodyScriptRegistry pins that
// calendar_view.js, calendar_rule.js, calendar_event_drawer.js,
// calendar_weather_sheet.js, calendar_editor.js and calendar_open.js load via the plugin
// body-script registry, not a `<script src>` inside the page body; without
// this, the previous test would pass trivially by deleting the tag and
// orphaning both scripts. It reads the source directly since
// pluginBodyScripts is a startup-wiring local with no exported value to
// assert against.
func TestCalendarScriptsShipFromThePluginBodyScriptRegistry(t *testing.T) {
	src, err := os.ReadFile("../../app/routes.go")
	if err != nil {
		t.Fatalf("read the plugin body-script registry: %v", err)
	}
	s := string(src)

	const head = "pluginBodyScripts := []string{"
	at := strings.Index(s, head)
	if at < 0 {
		t.Fatalf("no %q in internal/app/routes.go — the registry was renamed and this guard stopped "+
			"reading anything", head)
	}
	rest := s[at+len(head):]
	end := strings.Index(rest, "}")
	if end < 0 {
		t.Fatal("unterminated pluginBodyScripts slice literal in internal/app/routes.go")
	}
	slice := rest[:end]

	for _, script := range []string{"/static/js/widgets/calendar_view.js", "/static/js/widgets/calendar_rule.js", "/static/js/widgets/calendar_event_drawer.js", "/static/js/widgets/calendar_weather_sheet.js", "/static/js/widgets/calendar_editor.js", "/static/js/calendar_open.js"} {
		if !strings.Contains(slice, script) {
			t.Errorf("the plugin body-script registry does not mount %s — the calendar page reached "+
				"through the sidebar would render with the widget stripped, and look identical while "+
				"doing it", script)
		}
	}
}
