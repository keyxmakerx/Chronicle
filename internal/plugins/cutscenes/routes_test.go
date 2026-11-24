package cutscenes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	emw "github.com/labstack/echo/v5/middleware"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The routes through the real registration and middleware, with the real
// service over the in-memory repository, so what is checked is what a browser
// gets: the gates, and what each answer holds.

const sessionCookie = "chronicle_session"

type guardAuth struct{ auth.AuthService }

func (guardAuth) ValidateSession(_ context.Context, token string) (*auth.Session, error) {
	if token == "" {
		return nil, apperror.NewUnauthorized("no token")
	}
	return &auth.Session{UserID: token}, nil
}

type guardAddons struct {
	addons.AddonService
	on bool
}

func (g guardAddons) IsEnabledForCampaign(context.Context, string, string) (bool, error) {
	return g.on, nil
}

// guardCampaigns knows camp-1 (dir owner, codm a granted player, ana and bo
// players, scribe a scribe) and camp-2 (other, owner).
type guardCampaigns struct{ campaigns.CampaignService }

func (guardCampaigns) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	return &campaigns.Campaign{ID: id, Settings: `{"dm_grant_ids":["codm"]}`}, nil
}

func (guardCampaigns) GetMember(_ context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
	roles := map[string]campaigns.Role{"dir": campaigns.RoleOwner, "codm": campaigns.RolePlayer, "ana": campaigns.RolePlayer, "bo": campaigns.RolePlayer, "scribe": campaigns.RoleScribe}
	if campaignID == "camp-2" {
		roles = map[string]campaigns.Role{"other": campaigns.RoleOwner}
	}
	role, ok := roles[userID]
	if !ok {
		return nil, apperror.NewNotFound("not a member")
	}
	return &campaigns.CampaignMember{UserID: userID, Role: role}, nil
}

type routeRig struct {
	*rig
	e       *echo.Echo
	scene   *Scene
	showing *Showing
}

func newRouteRig(t *testing.T, addonOn bool) *routeRig {
	t.Helper()
	r := newRig()
	e := echo.New()
	e.Use(emw.Recover())
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		if c.Response().(*echo.Response).Committed {
			return
		}
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.JSON(ae.Code, map[string]string{"error": ae.Type, "message": ae.Message})
			return
		}
		_ = c.NoContent(echo.StatusCode(err))
	}
	RegisterRoutes(e, NewHandler(r.svc), guardCampaigns{}, guardAuth{}, guardAddons{on: addonOn})
	sc := r.scene(t, "SECRET-SCENE-NAME")
	sh, err := r.svc.Show(ctx, "camp-1", sc.ID, vDir, ShowInput{To: []string{"ana"}})
	if err != nil {
		t.Fatal(err)
	}
	return &routeRig{rig: r, e: e, scene: sc, showing: sh}
}

func (r *routeRig) do(method, path, user string, form url.Values, hdr ...string) *httptest.ResponseRecorder {
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	if user != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: user})
	}
	rec := httptest.NewRecorder()
	r.e.ServeHTTP(rec, req)
	return rec
}

type route struct {
	method, path string
	form         url.Values
}

// directorRoutes are every Director route, with a body that would succeed.
func (r *routeRig) directorRoutes() []route {
	base := "/campaigns/camp-1/cutscenes"
	s := base + "/" + r.scene.ID
	good := url.Values{"name": {"Renamed"}, "bg_kind": {"black"}}
	return []route{
		{"GET", base, nil},
		{"GET", base + "/new", nil},
		{"POST", base, url.Values{"name": {"Another"}}},
		{"POST", base + "/preview", good},
		{"GET", base + "/showings", nil},
		{"POST", base + "/showings/" + r.showing.ID + "/stop", url.Values{}},
		{"GET", s, nil},
		{"POST", s, good},
		{"GET", s + "/show", nil},
		{"POST", s + "/show", url.Values{"to": {"bo"}}},
		{"POST", s + "/delete", url.Values{}},
	}
}

// A player or scribe is told every Director route is not there, nothing of
// the scene is in the answer, and nothing changed.
func TestDirectorRoutesAre404ForPlayersAndScribes(t *testing.T) {
	r := newRouteRig(t, true)
	for _, user := range []string{"ana", "bo", "scribe"} {
		for _, rt := range r.directorRoutes() {
			// A malformed form must not draw the editor for a player either.
			form := rt.form
			if rt.method == "POST" && form != nil {
				form = url.Values{"name": {"x"}, "bg_seconds": {"not-a-number"}, "to": {"bo"}}
			}
			rec := r.do(rt.method, rt.path, user, form, "HX-Request", "true")
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s %s: %d, want 404", user, rt.method, rt.path, rec.Code)
			}
			if b := rec.Body.String(); strings.Contains(b, "SECRET-SCENE-NAME") || strings.Contains(b, "csw-") {
				t.Errorf("%s %s %s: the answer holds the scene or page markup", user, rt.method, rt.path)
			}
		}
	}
	if len(r.pub.shown) != 1 || len(r.pub.stops) != 0 {
		t.Errorf("a player's request went out live: %d shown, %d stopped", len(r.pub.shown), len(r.pub.stops))
	}
	if _, err := r.svc.Get(ctx, "camp-1", r.scene.ID, vDir); err != nil {
		t.Errorf("a player's delete went through: %v", err)
	}
}

func TestDirectorRoutesOpenForDirectors(t *testing.T) {
	for _, user := range []string{"dir", "codm"} {
		r := newRouteRig(t, true)
		for _, rt := range r.directorRoutes() {
			rec := r.do(rt.method, rt.path, user, rt.form, "HX-Request", "true")
			if rec.Code >= 400 {
				t.Errorf("%s %s %s: %d %s", user, rt.method, rt.path, rec.Code, rec.Body.String())
			}
		}
	}
}

// Signed out, not a member, or the add-on off: the site-wide gates answer
// before any handler, on every route.
func TestGatesBeforeHandlers(t *testing.T) {
	r := newRouteRig(t, true)
	off := newRouteRig(t, false)
	all := append(r.directorRoutes(),
		route{"GET", "/campaigns/camp-1/cutscenes/mine", nil},
		route{"GET", "/campaigns/camp-1/cutscenes/showings/" + r.showing.ID, nil},
		route{"POST", "/campaigns/camp-1/cutscenes/showings/" + r.showing.ID + "/watched", url.Values{}},
	)
	for _, rt := range all {
		if rec := r.do(rt.method, rt.path, "", rt.form); rec.Code != http.StatusSeeOther && rec.Code != http.StatusUnauthorized {
			t.Errorf("signed out %s %s: %d", rt.method, rt.path, rec.Code)
		}
		if rec := r.do(rt.method, rt.path, "other", rt.form); rec.Code != http.StatusForbidden {
			t.Errorf("non-member %s %s: %d, want 403", rt.method, rt.path, rec.Code)
		}
		offPath := strings.Replace(rt.path, r.scene.ID, off.scene.ID, 1)
		offPath = strings.Replace(offPath, r.showing.ID, off.showing.ID, 1)
		// A page is sent back to the campaign; a fragment or JSON caller is told 404.
		if rec := off.do(rt.method, offPath, "dir", rt.form); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/camp-1" {
			t.Errorf("add-on off %s %s: %d", rt.method, offPath, rec.Code)
		}
		if rec := off.do(rt.method, offPath, "dir", rt.form, "Accept", "application/json"); rec.Code != http.StatusNotFound {
			t.Errorf("add-on off (JSON) %s %s: %d", rt.method, offPath, rec.Code)
		}
	}
	if len(off.pub.shown) != 1 {
		t.Errorf("a show went out with the add-on off")
	}
}

func TestPlayerRoutes(t *testing.T) {
	r := newRouteRig(t, true)
	base := "/campaigns/camp-1/cutscenes"
	sh := base + "/showings/" + r.showing.ID

	rec := r.do("GET", base+"/mine", "ana", nil, "Accept", "application/json")
	var mine struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &mine); err != nil || rec.Code != 200 || mine.Total != 1 || mine.Data[0]["showingId"] != r.showing.ID || mine.Data[0]["name"] != "SECRET-SCENE-NAME" {
		t.Fatalf("ana's list: %d %s", rec.Code, rec.Body.String())
	}
	for _, k := range []string{"showingId", "name", "shownAt", "rewatchUntil", "watched"} {
		if _, ok := mine.Data[0][k]; !ok {
			t.Errorf("mine item lacks %s", k)
		}
	}
	if rec := r.do("GET", base+"/mine", "bo", nil); !strings.Contains(rec.Body.String(), `"total":0`) {
		t.Errorf("bo's list: %s", rec.Body.String())
	}

	rec = r.do("GET", sh, "ana", nil)
	var read map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &read)
	if rec.Code != 200 || read["showingId"] != r.showing.ID || read["scene"] == nil || read["rewatchUntil"] == nil {
		t.Errorf("ana's read: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do("GET", sh, "bo", nil); rec.Code != 404 || strings.Contains(rec.Body.String(), "SECRET") {
		t.Errorf("bo's read: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do("GET", base+"/showings/missing", "bo", nil); rec.Code != 404 {
		t.Errorf("a missing id: %d", rec.Code)
	}
	if rec := r.do("POST", sh+"/watched", "bo", url.Values{}); rec.Code != 404 {
		t.Errorf("bo marked it watched: %d", rec.Code)
	}
	if rec := r.do("POST", sh+"/watched", "ana", url.Values{}); rec.Code != 204 {
		t.Errorf("ana marked it watched: %d", rec.Code)
	}
	if r.repo.showings[r.showing.ID].Viewers[0].WatchedAt == nil {
		t.Error("not marked")
	}
	// A Director's id under another campaign's address never reaches it.
	if rec := r.do("GET", "/campaigns/camp-2/cutscenes/showings/"+r.showing.ID, "other", nil); rec.Code != 404 {
		t.Errorf("camp-2 read camp-1's showing: %d", rec.Code)
	}
}

// What a Director's requests answer: the Show sheet lists people and where
// they are, a refused Show redraws the sheet with the reason, and Preview
// answers the checked scene.
func TestDirectorAnswers(t *testing.T) {
	r := newRouteRig(t, true)
	s := "/campaigns/camp-1/cutscenes/" + r.scene.ID

	sheet := r.do("GET", s+"/show", "dir", nil, "HX-Request", "true").Body.String()
	for _, want := range []string{"Ana", "In Chronicle", "In Foundry", "Both", "Away", "Everyone here", "Tonight (12 hours)", `value="12h" checked`, "(you)"} {
		if !strings.Contains(sheet, want) {
			t.Errorf("sheet lacks %q", want)
		}
	}
	if strings.Contains(sheet, "<script") {
		t.Error("the sheet carries a script tag; controls in a swapped fragment use inline handlers")
	}

	rec := r.do("POST", s+"/show", "dir", url.Values{"rewatch": {"1h"}}, "HX-Request", "true")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "pick at least one person") || rec.Header().Get("HX-Retarget") != "#"+sheetID(r.scene.ID) {
		t.Errorf("refused show: %d %q %s", rec.Code, rec.Header().Get("HX-Retarget"), rec.Body.String())
	}
	rec = r.do("POST", s+"/show", "dir", url.Values{"to": {"ana", "bo"}}, "HX-Request", "true")
	if b := rec.Body.String(); rec.Code != 200 || !strings.Contains(b, "Showing to Ana and Bo") || !strings.Contains(b, "Stop") || !strings.Contains(b, `hx-swap-oob="true"`) {
		t.Errorf("show: %d %s", rec.Code, b)
	}

	rec = r.do("POST", "/campaigns/camp-1/cutscenes/preview", "dir", url.Values{"name": {"P"}, "ov_kind": {"eye"}, "ov_at": {"500"}, "ov_color": {"#FF0000"}}, "Accept", "application/json")
	var sc Scene
	if err := json.Unmarshal(rec.Body.Bytes(), &sc); err != nil || rec.Code != 200 || sc.Overlays[0].At != MaxAt || sc.Overlays[0].Params.Color != "#ff0000" {
		t.Errorf("preview: %d %s", rec.Code, rec.Body.String())
	}
	rec = r.do("POST", "/campaigns/camp-1/cutscenes/preview", "dir", url.Values{"name": {"P"}, "ov_kind": {"dragon"}}, "Accept", "application/json")
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "dragon") {
		t.Errorf("bad preview: %d %s", rec.Code, rec.Body.String())
	}

	// A refused save redraws the form with the reason and keeps what was typed.
	rec = r.do("POST", s, "dir", url.Values{"name": {"Kept words"}, "ov_kind": {"eye"}, "ov_color": {"red"}}, "HX-Request", "true")
	if b := rec.Body.String(); rec.Code != 200 || !strings.Contains(b, "#rrggbb") || !strings.Contains(b, "Kept words") {
		t.Errorf("refused save: %d %s", rec.Code, b)
	}
	// Stop from "Shown lately" answers that list's row, kept whole.
	rec = r.do("POST", "/campaigns/camp-1/cutscenes/showings/"+r.showing.ID+"/stop?from=recent", "dir", nil, "HX-Request", "true")
	if b := rec.Body.String(); rec.Code != 200 || !strings.Contains(b, recentRowID(r.showing.ID)) || !strings.Contains(b, "SECRET-SCENE-NAME") || !strings.Contains(b, "stopped") {
		t.Errorf("stop from recent: %d %s", rec.Code, b)
	}
	rec = r.do("POST", "/campaigns/camp-1/cutscenes/showings/"+r.showing.ID+"/stop", "dir", nil, "Accept", "application/json")
	if b := rec.Body.String(); rec.Code != 200 || !strings.Contains(b, `"stopped":true`) {
		t.Errorf("stop as JSON: %d %s", rec.Code, b)
	}

	editor := r.do("GET", s, "dir", nil).Body.String()
	for _, want := range []string{"Background", "On top", "Words", "Ending", "Preview", "Show to…", "data-engine-src", "cutscene_engine.js",
		`name="bg_look"`, `name="ov_look"`, `name="ov_pupil"`, ">The look's own<", ">Round<", ">Dragon slit<", "data-csw-ov-pupil", "The page drifts off", "Heavy eyelids", "Ink in water", "Sparks gather", "Lids part", "Burning seam", "data-csw-preview-box", "Preview on", ">Computer<", ">Tablet<", ">Phone<"} {
		if !strings.Contains(editor, want) {
			t.Errorf("editor lacks %q", want)
		}
	}

	// The preview carries the looks picked, and drops one the kind lacks.
	rec = r.do("POST", "/campaigns/camp-1/cutscenes/preview", "dir", url.Values{"name": {"P"}, "bg_kind": {"fall-asleep"}, "bg_look": {"ink"},
		"ov_kind": {"eye", "embers"}, "ov_look": {"almond", "rift"}}, "Accept", "application/json")
	sc = Scene{}
	if err := json.Unmarshal(rec.Body.Bytes(), &sc); err != nil || sc.Background.Params.Look != "ink" || sc.Overlays[0].Params.Look != "almond" || sc.Overlays[1].Params.Look != "" {
		t.Errorf("preview looks: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"look":"ink"`) {
		t.Errorf("the look is not sent as params.look: %s", rec.Body.String())
	}
}
