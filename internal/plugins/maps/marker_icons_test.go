// marker_icons_test.go pins the canonical marker icon vocabulary (the
// source of truth the editor picker, the stored value, and the Foundry sync
// contract all share), the editor's inline pin-create affordances, and the
// marker-icons API.
package maps

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestMarkerIconCatalog_Integrity(t *testing.T) {
	cat := MarkerIconCatalog()
	if len(cat) < 30 {
		t.Fatalf("expected a substantial catalog; got %d", len(cat))
	}
	seen := map[string]bool{}
	for _, ic := range cat {
		if ic.ID == "" || ic.Label == "" || ic.Category == "" {
			t.Errorf("incomplete catalog entry: %+v", ic)
		}
		if !strings.HasPrefix(ic.ID, "fa-") {
			t.Errorf("icon ID %q is not a Font Awesome class", ic.ID)
		}
		if seen[ic.ID] {
			t.Errorf("duplicate icon ID %q", ic.ID)
		}
		seen[ic.ID] = true
	}
	// The returned slice is a copy — mutating it must not corrupt the source.
	cat[0].ID = "mutated"
	if MarkerIconCatalog()[0].ID == "mutated" {
		t.Errorf("MarkerIconCatalog returned the package slice, not a copy")
	}
}

func TestMarkerIcon_Validation(t *testing.T) {
	if !IsValidMarkerIcon("fa-castle") {
		t.Errorf("fa-castle should be valid")
	}
	if IsValidMarkerIcon("fa-not-a-real-icon") {
		t.Errorf("unknown icon should be invalid")
	}
	if !IsValidMarkerIcon(DefaultMarkerIcon) {
		t.Errorf("the default icon must itself be in the catalog")
	}
	if got := NormalizeMarkerIcon("fa-castle"); got != "fa-castle" {
		t.Errorf("NormalizeMarkerIcon kept a valid icon wrong: %q", got)
	}
	if got := NormalizeMarkerIcon("bogus"); got != DefaultMarkerIcon {
		t.Errorf("NormalizeMarkerIcon(bogus) = %q; want default %q", got, DefaultMarkerIcon)
	}
	if got := NormalizeMarkerIcon(""); got != DefaultMarkerIcon {
		t.Errorf("NormalizeMarkerIcon(empty) = %q; want default", got)
	}
}

func TestMarkerIconGroups_CoverAndOrder(t *testing.T) {
	groups := MarkerIconGroups()
	if len(groups) < 5 {
		t.Fatalf("expected several groups; got %d", len(groups))
	}
	// Every catalog icon appears in exactly one group; total matches.
	total := 0
	for _, g := range groups {
		if g.Category == "" || len(g.Icons) == 0 {
			t.Errorf("empty group: %+v", g)
		}
		total += len(g.Icons)
	}
	if total != len(MarkerIconCatalog()) {
		t.Errorf("groups cover %d icons; catalog has %d", total, len(MarkerIconCatalog()))
	}
	// First group is "General" (display-order preserved from the catalog).
	if groups[0].Category != "General" {
		t.Errorf("first group should be General; got %q", groups[0].Category)
	}
}

// Part A — the editor's icon <select> renders from the catalog (every icon ID
// + a couple of group labels present), proving the single source of truth.
func TestMapEditorBody_IconSelectFromCatalog(t *testing.T) {
	html := renderMapEditor(t, true)
	for _, ic := range MarkerIconCatalog() {
		if !strings.Contains(html, `value="`+ic.ID+`"`) {
			t.Errorf("editor icon select missing %q", ic.ID)
		}
	}
	// Group labels (ampersand-free ones, since templ HTML-escapes "&").
	for _, label := range []string{"General", "Fortifications", "Maritime"} {
		if !strings.Contains(html, label) {
			t.Errorf("editor icon select missing group %q", label)
		}
	}
}

// Part B — the editor offers pin creation without leaving the map: the Pin tool
// on the floating rail, and double-click, with Leaflet's own double-click zoom
// disabled so the gesture doesn't zoom under the new pin.
func TestMapEditorBody_InlinePinCreate(t *testing.T) {
	html := renderMapEditor(t, true)
	for _, want := range []string{
		`class="mp-btn" data-tool="pin"`, // the Pin tool on the rail
		"doubleClickZoom: false",         // zoom disabled so dblclick-create is clean
		"map.on('dblclick'",              // the inline-create handler
		"More options",                   // card -> full marker modal, nothing lost
	} {
		if !strings.Contains(html, want) {
			t.Errorf("editor missing inline-pin affordance %q", want)
		}
	}
}

// Players get the map, search, layers and zoom, but none of the authoring
// controls. (Strings in the shared init script appear for everyone, so the
// rail markup is the reliable role signal.)
func TestMapEditorBody_PlayerHasNoToolRail(t *testing.T) {
	html := renderMapEditor(t, false)
	for _, banned := range []string{`class="mp-btn" data-tool="pin"`, `class="mp-btn" data-tool="draw"`, `id="mp-undo"`, `id="mp-settings-btn"`} {
		if strings.Contains(html, banned) {
			t.Errorf("players must not see %s", banned)
		}
	}
	for _, want := range []string{`id="marker-search"`, `id="mp-layers-btn"`, `id="mp-zoom-in"`} {
		if !strings.Contains(html, want) {
			t.Errorf("players should still get %s", want)
		}
	}
}

// The toolbar row above the map is gone: every control floats over the map
// wrapper, and Leaflet's own zoom control (which collided with the old
// top-left draw toolbar) is off.
func TestMapEditorBody_ControlsFloatOverTheMap(t *testing.T) {
	html := renderMapEditor(t, true)
	if strings.Contains(html, "place-marker-btn") || strings.Contains(html, "togglePlaceMode") {
		t.Errorf("the old toolbar row's Place Marker toggle must be gone")
	}
	if !strings.Contains(html, "zoomControl: false") {
		t.Errorf("Leaflet's default zoom control must be disabled")
	}
	wrap := strings.Index(html, `id="map-wrap"`)
	search := strings.Index(html, `id="marker-search"`)
	end := strings.Index(html, `id="mp-hint"`)
	if wrap < 0 || search < wrap || end < search {
		t.Errorf("search, rail and hint must render inside #map-wrap (wrap=%d search=%d hint=%d)", wrap, search, end)
	}
}

// The quick card must send only the fields a person changed, so a rename can
// never clear the kind or flip visibility (the partial-update contract).
func TestMapEditorBody_QuickCardSendsOnlyChanges(t *testing.T) {
	src, err := os.ReadFile("maps.templ")
	if err != nil {
		t.Fatalf("read maps.templ: %v", err)
	}
	text := string(src)
	for _, want := range []string{
		"if (nm !== mk.name) body.name = nm;",
		"if (kind !== origKind) body.pin_category = kind || null;",
		"if (visibility !== (mk.visibility || 'everyone')) body.visibility = visibility;",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("maps.templ quick card no longer has %q", want)
		}
	}
}

// All five kinds are offered in the full modal too, so "More options" loses
// nothing.
func TestMapEditorBody_ModalOffersEveryPinKind(t *testing.T) {
	html := renderMapEditor(t, true)
	for kind := range pinCategories {
		if !strings.Contains(html, `value="`+kind+`"`) {
			t.Errorf("marker modal missing kind %q", kind)
		}
	}
}

// The drawing script loads for players as well: it renders saved drawings for
// them and only builds tools for Scribe+.
func TestMapEditorBody_DrawingScriptForEveryone(t *testing.T) {
	for _, scribe := range []bool{true, false} {
		if !strings.Contains(renderMapEditor(t, scribe), "map_drawing_tools.js") {
			t.Errorf("drawing script missing for scribe=%v", scribe)
		}
	}
}

// Players don't get the inline-create hint (Scribe+ gated). (The
// place-marker-btn id also appears in the shared init script's string body, so
// the toolbar hint — which is markup-only — is the reliable role signal.)
func TestMapEditorBody_PlayerNoCreateHint(t *testing.T) {
	html := renderMapEditor(t, false)
	if strings.Contains(html, "data-map-pin-hint") {
		t.Errorf("players must not see the pin-create hint")
	}
	// The Scribe toolbar buttons are gated markup; the rendered <button
	// id="place-marker-btn"> must be absent for players.
	if strings.Contains(html, `id="place-marker-btn"`) {
		t.Errorf("players must not see the Place Marker button")
	}
}

func TestMarkerIconsAPI_ReturnsCatalog(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/maps/marker-icons", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := &Handler{}
	if err := h.MarkerIconsAPI(c); err != nil {
		t.Fatalf("MarkerIconsAPI: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Default string            `json:"default"`
		Icons   []MarkerIcon      `json:"icons"`
		Groups  []MarkerIconGroup `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Default != DefaultMarkerIcon {
		t.Errorf("default = %q; want %q", body.Default, DefaultMarkerIcon)
	}
	if len(body.Icons) != len(MarkerIconCatalog()) {
		t.Errorf("api returned %d icons; catalog has %d", len(body.Icons), len(MarkerIconCatalog()))
	}
	if len(body.Groups) == 0 {
		t.Errorf("api returned no groups")
	}
}

func renderMapEditor(t *testing.T, scribe bool) string {
	t.Helper()
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", Name: "Test"}}
	data := MapViewData{
		CampaignID: "c1",
		Map:        &Map{ID: "m1", Name: "Test Map", ImageWidth: 1000, ImageHeight: 800},
		IsScribe:   scribe,
	}
	var sb strings.Builder
	if err := MapEditorBody(cc, data, "flex-1", "").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render MapEditorBody: %v", err)
	}
	return sb.String()
}
