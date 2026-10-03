package calendar

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// postStructureForm posts import_json (and a fingerprint) the way the
// editor's htmx form does.
func postStructureForm(t *testing.T, path, userID, importJSON string) *httptest.ResponseRecorder {
	t.Helper()
	e, _ := newAccessTestRouter(false, true, map[string]campaigns.Role{userID: campaigns.RoleOwner})
	return postStructureFormTo(e, path, userID, importJSON)
}

func postStructureFormTo(e http.Handler, path, userID, importJSON string) *httptest.ResponseRecorder {
	form := url.Values{"import_json": {importJSON}, "fingerprint": {"fp"}}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	if userID != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userID})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// TestStructureRoutes_OwnerOnly: the structure page, its preview and its
// save answer only the campaign Owner. A co-Director (Scribe with the
// dm_only grant), a Scribe, a Player and a non-member all get the same 403
// the wizard and PUT /calendars/:calid give them.
func TestStructureRoutes_OwnerOnly(t *testing.T) {
	type caller struct {
		name    string
		role    campaigns.Role
		member  bool
		granted bool
	}
	callers := []caller{
		{"Owner", campaigns.RoleOwner, true, false},
		{"co-Director", campaigns.RoleScribe, true, true},
		{"Scribe", campaigns.RoleScribe, true, false},
		{"Player", campaigns.RolePlayer, true, false},
		{"no access", campaigns.RoleNone, false, false},
	}
	routes := []struct {
		name, method, path string
		ownerWant          int
	}{
		{"structure page", http.MethodGet, "/campaigns/camp-1/calendars/cal-1/structure", http.StatusOK},
		{"structure preview", http.MethodPost, "/campaigns/camp-1/calendars/cal-1/structure/preview", http.StatusOK},
		{"structure save", http.MethodPost, "/campaigns/camp-1/calendars/cal-1/structure", http.StatusNoContent},
	}
	const userID = "u-caller"
	for _, rt := range routes {
		for _, c := range callers {
			t.Run(rt.name+" as "+c.name, func(t *testing.T) {
				roles := map[string]campaigns.Role{}
				if c.member {
					roles[userID] = c.role
				}
				settings := ""
				if c.granted {
					settings = `{"dm_grant_ids":["` + userID + `"]}`
				}
				e, _ := newAccessTestRouterWithSettings(false, true, roles, settings)
				var rec *httptest.ResponseRecorder
				if rt.method == http.MethodGet {
					rec = serve(e, newHTMXRequest(rt.method, rt.path, userID))
				} else {
					rec = postStructureFormTo(e, rt.path, userID, buildImportJSON(t, 3))
				}
				want := http.StatusForbidden
				if c.role >= campaigns.RoleOwner {
					want = rt.ownerWant
				}
				if rec.Code != want {
					t.Errorf("%s %s as %s: got %d, want %d: %s", rt.method, rt.path, c.name, rec.Code, want, rec.Body.String())
				}
			})
		}
	}
}

func TestStructureRoutes_Unauthenticated(t *testing.T) {
	e, _ := newAccessTestRouter(false, true, map[string]campaigns.Role{})
	if rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/cal-1/structure", ""); !isLoginRedirect(rec) {
		t.Errorf("expected a login redirect, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestStructurePage_RendersTheEditor(t *testing.T) {
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner}
	e, _ := newAccessTestRouter(false, true, roles)

	full := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/cal-1/structure", "u-owner")
	if full.Code != http.StatusOK {
		t.Fatalf("got %d: %s", full.Code, full.Body.String())
	}
	page := full.Body.String()
	for _, want := range []string{
		"Edit structure", "Months", "Weekdays", "Leap rule", "Moons", "Seasons",
		`name="import_json"`, `hx-post="/campaigns/camp-1/calendars/cal-1/structure/preview"`,
		`id="calv5-structure-preview"`, "Preview changes",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("structure page missing %q", want)
		}
	}
	if strings.Contains(page, "<h4>Era</h4>") {
		t.Error("the structure page must not offer the wizard's single-era section; eras have their own manager")
	}
	if n := strings.Count(castSwappedRegion(t, page), "<script"); n != 0 {
		t.Errorf("the structure page emits %d <script> tag(s) inside #main-content; htmx strips them on a boosted navigation", n)
	}
}

func TestStructurePreview_ShowsPreviewAndCarriesTheStructure(t *testing.T) {
	rec := postStructureForm(t, "/campaigns/camp-1/calendars/cal-1/structure/preview", "u-owner", buildImportJSON(t, 3))
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, body)
	}
	for _, want := range []string{"Before you save", `name="fingerprint" value="fp"`, `name="import_json"`,
		`hx-post="/campaigns/camp-1/calendars/cal-1/structure"`, "Back to editing", "Save changes"} {
		if !strings.Contains(body, want) {
			t.Errorf("preview missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<script") {
		t.Errorf("the preview arrives by htmx swap and must not carry a script element:\n%s", body)
	}
}

func TestStructurePreview_MalformedStructureShowsAnError(t *testing.T) {
	rec := postStructureForm(t, "/campaigns/camp-1/calendars/cal-1/structure/preview", "u-owner", "{not json")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `class="calv5-err"`) {
		t.Errorf("want the error shown in the preview slot, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestStructureSave_RedirectsToTheCalendar(t *testing.T) {
	rec := postStructureForm(t, "/campaigns/camp-1/calendars/cal-1/structure", "u-owner", buildImportJSON(t, 3))
	if got := rec.Header().Get("HX-Redirect"); got != "/campaigns/camp-1/calendars/cal-1/view" {
		t.Errorf("HX-Redirect = %q, want the calendar's own page (status %d: %s)", got, rec.Code, rec.Body.String())
	}
}

// TestCalendarsListCard_EditStructureIsOwnerOnly: the card menu's "Edit
// structure" link exists only where the menu does, for the Owner.
func TestCalendarsListCard_EditStructureIsOwnerOnly(t *testing.T) {
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner, "u-player": campaigns.RolePlayer}
	e, _ := newAccessTestRouter(false, true, roles)
	owner := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars", "u-owner").Body.String()
	if !strings.Contains(owner, `href="/campaigns/camp-1/calendars/cal-1/structure"`) {
		t.Error("the Owner's card menu should link to Edit structure")
	}
	player := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars", "u-player").Body.String()
	if strings.Contains(player, "/structure") {
		t.Error("a Player must not see the Edit structure link")
	}
}

func TestCalendarEditorStatePrefill(t *testing.T) {
	cal := structureFixture()
	cal.Months[1].IsIntercalary = true
	st := calendarEditorState(cal)
	if len(st.Months) != 3 || st.Months[1].Name != "Beta" || !st.Months[1].Inter || st.Months[1].Leap != 1 {
		t.Errorf("months = %+v", st.Months)
	}
	if !st.LeapOn || st.LeapEvery != 4 {
		t.Errorf("leap = %v every %d, want on every 4", st.LeapOn, st.LeapEvery)
	}
	if len(st.Moons) != 1 || st.Moons[0].ID == nil || *st.Moons[0].ID != 7 {
		t.Errorf("moons = %+v, want id 7 carried", st.Moons)
	}
	if len(st.Seasons) != 1 || st.Seasons[0].ID != 3 || st.Seasons[0].StartKey != st.Months[0].Key || st.Seasons[0].EndKey != st.Months[1].Key {
		t.Errorf("seasons = %+v, want id 3 pointing at Alpha..Beta by key", st.Seasons)
	}
	x := structureEditorXData(st, structurePreviewSlot)
	for _, want := range []string{`"id":7`, `"Beta"`, `"inter":true`, structurePreviewSlot} {
		if !strings.Contains(x, want) {
			t.Errorf("x-data missing %s:\n%s", want, x)
		}
	}
}
