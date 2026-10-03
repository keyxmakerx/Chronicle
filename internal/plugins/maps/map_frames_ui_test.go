package maps

import (
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func renderEditorFor(t *testing.T, role campaigns.Role, m *Map, frame string) string {
	t.Helper()
	cc := &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "c1", Name: "Test"},
		MemberRole: role,
	}
	data := MapViewData{
		CampaignID: "c1",
		Map:        m,
		IsScribe:   role >= campaigns.RoleScribe,
		IsOwner:    role >= campaigns.RoleOwner,
	}
	data.Display = ResolveDisplay(m, frame)
	return render(t, mapShowContent(cc, data))
}

func baseMap() *Map {
	return &Map{ID: "m1", CampaignID: "c1", Name: "Vellmoor Isle", ImageWidth: 1000, ImageHeight: 800}
}

// The page wears the campaign's frame, a map's own pick wins, and every frame's
// ornaments are present so the sheet can switch between them live.
func TestMapPage_FrameFollowsCampaignThenMap(t *testing.T) {
	out := renderEditorFor(t, campaigns.RolePlayer, baseMap(), "gilded")
	if !strings.Contains(out, `id="mp-frame" data-frame="gilded"`) {
		t.Errorf("page should wear the campaign frame (gilded); got:\n%s", firstMatch(out, `<div class="mp-frame"[^>]*>`))
	}
	m := baseMap()
	m.Display = &DisplaySettings{Frame: &FrameDisplay{Style: "old", Tint: ptrB(false)}}
	out = renderEditorFor(t, campaigns.RolePlayer, m, "gilded")
	if !strings.Contains(out, `data-frame="old"`) || !strings.Contains(out, `data-tint="0"`) {
		t.Errorf("map override (old, tint off) not applied: %s", firstMatch(out, `<div class="mp-frame"[^>]*>`))
	}
	for _, o := range []string{`data-o="atlas"`, `data-o="arcane"`, `data-o="old"`, `data-o="futuristic"`} {
		if !strings.Contains(out, o) {
			t.Errorf("missing ornament block %s", o)
		}
	}
	if !strings.Contains(out, "mp-hudpos") || !strings.Contains(out, "mp-seal") || !strings.Contains(out, "mp-runes") {
		t.Error("the futuristic readout, wax seal and runes must all be rendered")
	}
	if !strings.Contains(out, "/static/css/map_frames.css") {
		t.Error("the frame stylesheet must be linked")
	}
}

func firstMatch(s, re string) string {
	return regexp.MustCompile(re).FindString(s)
}

// The settings sheet and its gear are owners' tools: a scribe edits pins and
// drawings but cannot save map settings (the PUT is Owner-only), so offering
// the sheet would only end in a refusal.
func TestMapPage_SettingsSheetIsOwnersOnly(t *testing.T) {
	cases := []struct {
		role      campaigns.Role
		wantSheet bool
	}{
		{campaigns.RoleOwner, true},
		{campaigns.RoleScribe, false},
		{campaigns.RolePlayer, false},
	}
	for _, tc := range cases {
		out := renderEditorFor(t, tc.role, baseMap(), "")
		has := strings.Contains(out, `id="mp-settings-btn"`) && strings.Contains(out, `id="mp-sheet"`)
		if has != tc.wantSheet {
			t.Errorf("role %d: sheet present = %v, want %v", tc.role, has, tc.wantSheet)
		}
		if strings.Contains(out, `id="map-settings-modal"`) {
			t.Errorf("role %d: the old settings modal must be gone", tc.role)
		}
	}
}

// Everything the old modal held is still in the sheet.
func TestMapSettingsSheet_KeepsTheOldModalsFields(t *testing.T) {
	out := renderEditorFor(t, campaigns.RoleOwner, baseMap(), "")
	for _, id := range []string{
		`id="ms-name"`, `id="ms-description"`, `id="ms-image-id"`, `id="ms-image-file"`,
		`id="ms-pick-existing"`, `data-widget="media-picker"`, `id="ms-background-color"`,
		`id="ms-bg-reset"`, `id="ms-delete"`, `id="ms-save"`,
	} {
		if !strings.Contains(out, id) {
			t.Errorf("sheet lost %s", id)
		}
	}
	for _, label := range []string{"Sea and background colour", "Tint the map to match the frame", "Kinds of pin", "Who can draw", "Opening the map", "Use the current view"} {
		if !strings.Contains(out, label) {
			t.Errorf("sheet missing %q", label)
		}
	}
}

// "Who can draw" decides whether the drawing tools are offered at all.
func TestMapPage_DrawToolsFollowTheGate(t *testing.T) {
	ownersOnly := baseMap()
	ownersOnly.Display = &DisplaySettings{Draw: &DrawDisplay{Who: DrawWhoOwners}}
	cases := []struct {
		name     string
		role     campaigns.Role
		m        *Map
		wantDraw bool
	}{
		{"scribe on a default map draws", campaigns.RoleScribe, baseMap(), true},
		{"scribe on an owners-only map does not", campaigns.RoleScribe, ownersOnly, false},
		{"owner on an owners-only map draws", campaigns.RoleOwner, ownersOnly, true},
		{"player never draws", campaigns.RolePlayer, baseMap(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := renderEditorFor(t, tc.role, tc.m, "")
			if got := strings.Contains(out, `class="mp-btn" data-tool="draw"`); got != tc.wantDraw {
				t.Errorf("draw button present = %v, want %v", got, tc.wantDraw)
			}
			if got := strings.Contains(out, `data-can-draw="true"`); got != tc.wantDraw {
				t.Errorf("data-can-draw true = %v, want %v", got, tc.wantDraw)
			}
			// Pins stay available to scribes whatever the draw gate says.
			if tc.role >= campaigns.RoleScribe && !strings.Contains(out, `class="mp-btn" data-tool="pin"`) {
				t.Error("scribes must keep the pin tool")
			}
		})
	}
}

// The page hands the script the resolved settings, not raw storage, so the
// defaults live in one place.
func TestMapPage_ConfigCarriesResolvedDisplay(t *testing.T) {
	m := baseMap()
	m.Display = &DisplaySettings{
		Pins:  &PinDisplay{Style: PinStyleSeal, Labels: PinLabelsAlways},
		Kinds: map[string]KindDisplay{"danger": {Label: "Monsters"}},
		Grid:  &GridDisplay{Type: GridHex},
	}
	out := renderEditorFor(t, campaigns.RoleOwner, m, "arcane")
	raw := regexp.MustCompile(`data-display="([^"]*)"`).FindStringSubmatch(out)
	if raw == nil {
		t.Fatal("no data-display attribute")
	}
	var got ResolvedDisplay
	if err := json.Unmarshal([]byte(html.UnescapeString(raw[1])), &got); err != nil {
		t.Fatalf("data-display is not JSON: %v", err)
	}
	if got.Frame != "arcane" || got.PinStyle != "seal" || got.PinLabels != "always" || got.PinSize != "m" || got.GridType != "hex" {
		t.Errorf("resolved display wrong: %+v", got)
	}
	var danger KindDisplay
	for _, k := range got.Kinds {
		if k.ID == "danger" {
			danger = k
		}
	}
	if danger.Label != "Monsters" || danger.Color != "#dc2626" {
		t.Errorf("danger kind = %+v", danger)
	}
}

// The maps list shows the campaign's frame in small form on every card.
func TestMapList_CardsWearTheCampaignFrame(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", Name: "Test"}, MemberRole: campaigns.RolePlayer}
	data := MapListData{CampaignID: "c1", CampaignFrame: "futuristic", Maps: []Map{*baseMap(), {ID: "m2", Name: "Second"}}}
	out := render(t, mapListContent(cc, data))
	if n := strings.Count(out, `class="mp-frame mp-mini mb-3" data-frame="futuristic"`); n != 2 {
		t.Errorf("want 2 framed cards, got %d", n)
	}
	if !strings.Contains(out, "/static/css/map_frames.css") {
		t.Error("list must link the frame stylesheet")
	}
}

// The Customize tab body: six frames, the current one marked, and a form per
// other frame that PUTs to the maps route.
func TestMapFrameSection(t *testing.T) {
	out := render(t, mapFrameSection("c1", "old", true))
	if n := strings.Count(out, `name="frame"`); n != 6 {
		t.Errorf("want 6 frame forms, got %d", n)
	}
	if strings.Count(out, "In use") != 1 || strings.Count(out, "Use this") != 5 {
		t.Errorf("want one 'In use' and five 'Use this': in-use=%d use=%d", strings.Count(out, "In use"), strings.Count(out, "Use this"))
	}
	if !strings.Contains(out, `hx-put="/campaigns/c1/maps/frame-style"`) {
		t.Error("forms must PUT to the frame-style route")
	}
	if !strings.Contains(out, "Saved") {
		t.Error("a saved render must say so")
	}
	if strings.Contains(render(t, mapFrameSection("c1", "old", false)), "Saved") {
		t.Error("a plain render must not claim Saved")
	}
}

func frameRequest(role campaigns.Role, body, contentType string, htmx bool) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/campaigns/camp-1/maps/frame-style", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	c.Set("campaign_context", dmWriteCampaignCtx(role, false))
	return c, rec
}

func TestSetCampaignFrameAPI(t *testing.T) {
	cases := []struct {
		name        string
		body, ctype string
		htmx        bool
		wantStored  string
		wantErr     bool
		wantStatus  int
	}{
		{"form post from the Customize page", "frame=arcane", "application/x-www-form-urlencoded", true, "arcane", false, http.StatusOK},
		{"json body", `{"frame":"modern"}`, "application/json", false, "modern", false, http.StatusNoContent},
		{"unknown frame is refused", "frame=steampunk", "application/x-www-form-urlencoded", true, "", true, 0},
		{"missing frame is refused", "", "application/x-www-form-urlencoded", true, "", true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stored := ""
			repo := &mockMapRepo{setCampaignFrame: func(_ context.Context, campaignID, frame string) error {
				if campaignID != "camp-1" {
					t.Errorf("stored against %q, want the URL's campaign", campaignID)
				}
				stored = frame
				return nil
			}}
			h := NewHandler(NewMapService(repo))
			c, rec := frameRequest(campaigns.RoleOwner, tc.body, tc.ctype, tc.htmx)
			err := h.SetCampaignFrameAPI(c)
			if tc.wantErr {
				if err == nil || stored != "" {
					t.Fatalf("want a refusal with nothing stored, got err=%v stored=%q", err, stored)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if stored != tc.wantStored || rec.Code != tc.wantStatus {
				t.Errorf("stored=%q status=%d, want %q %d", stored, rec.Code, tc.wantStored, tc.wantStatus)
			}
			if tc.htmx && !strings.Contains(rec.Body.String(), `id="map-frame-section"`) {
				t.Error("an HTMX save must answer with the refreshed section")
			}
		})
	}
}

// The Customize hook: a tab keyed "maps" whose body reads the current frame.
func TestCustomizeTabFactory(t *testing.T) {
	repo := &mockMapRepo{campaignFrame: "gilded"}
	h := NewHandler(NewMapService(repo))
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	tab := h.CustomizeTabFactory()(cc)
	if tab.ID != "maps" || tab.Label == "" || tab.Content == nil {
		t.Fatalf("tab = %+v", tab)
	}
	out := render(t, tab.Content)
	if !strings.Contains(out, "Map frame") {
		t.Error("tab body missing its heading")
	}
	// The stored frame is the one marked "In use".
	if !regexp.MustCompile(`(?s)data-frame="gilded".*?In use`).MatchString(out) {
		t.Error("the campaign's stored frame (gilded) should be marked In use")
	}
}
