// marker_partial_update_test.go — sweep R4, the maps half of the
// absent-means-preserve contract.
//
// Three losses on one struct, all reproduced against the shipped code:
//
//   - the Chronicle web edit form and the drag-end PUT send neither
//     pin_category nor visibility_rules, and UpdateMarker assigned both
//     unguarded, so every edit and every DRAG erased them — one of them
//     access-control data;
//   - the web request struct has no foundry_id member at all, so every web
//     edit NULLed the marker's Foundry pairing key, which resurfaces later
//     as duplicate markers on the next sync;
//   - a Scribe's edit dropped the Owner's per-player rules to nil, because
//     "you may not write this" was implemented as "write nil".
//
// The R3 booking called the last one a fork: adding foundry_id to the web
// struct lets a browser form clear a sync pairing key, while nil-preserving
// in the service would stop syncapi clearing one too. Absent-preserve is
// neither horn — the web form still never sends the key, and syncapi still
// clears it with an explicit null.
package maps

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// storedMarker is the fully-configured row every case starts from.
func storedMarker() *Marker {
	s := func(v string) *string { return &v }
	return &Marker{
		ID:              "mk-1",
		MapID:           "map-1",
		Name:            "The Sunken Door",
		Description:     s("only opens at low tide"),
		X:               10,
		Y:               20,
		Icon:            "fa-map-pin",
		Color:           "#3b82f6",
		PinCategory:     s("secret"),
		EntityID:        s("ent-door"),
		Visibility:      "dm_only",
		VisibilityRules: s(`{"allowed_users":["u-1"]}`),
		FoundryID:       s("foundry-note-42"),
	}
}

func runMarkerUpdate(t *testing.T, input UpdateMarkerInput) *Marker {
	t.Helper()
	var written *Marker
	repo := &mockMapRepo{
		getMarkerFn:    func(_ context.Context, _ string) (*Marker, error) { return storedMarker(), nil },
		updateMarkerFn: func(_ context.Context, mk *Marker) error { written = mk; return nil },
	}
	if err := newTestMapService(repo).UpdateMarker(context.Background(), "mk-1", input); err != nil {
		t.Fatalf("UpdateMarker: %v", err)
	}
	if written == nil {
		t.Fatal("nothing was written")
	}
	return written
}

// THE headline regression: the body a drag now sends must move the marker
// and change nothing else — including the three fields the drag never had a
// copy of.
func TestDrag_MovesTheMarkerAndNothingElse(t *testing.T) {
	got := runMarkerUpdate(t, UpdateMarkerInput{X: patch.Of(77.5), Y: patch.Of(12.25)})
	want := storedMarker()

	if got.X != 77.5 || got.Y != 12.25 {
		t.Errorf("position = (%v,%v), want (77.5,12.25)", got.X, got.Y)
	}
	if got.Name != want.Name {
		t.Errorf("Name = %q, want preserved", got.Name)
	}
	assertMarkerPtr(t, "PinCategory", got.PinCategory, want.PinCategory)
	assertMarkerPtr(t, "VisibilityRules", got.VisibilityRules, want.VisibilityRules)
	assertMarkerPtr(t, "FoundryID", got.FoundryID, want.FoundryID)
	assertMarkerPtr(t, "EntityID", got.EntityID, want.EntityID)
	assertMarkerPtr(t, "Description", got.Description, want.Description)
	if got.Visibility != want.Visibility {
		t.Errorf("Visibility = %q, want preserved", got.Visibility)
	}
	if got.Icon != want.Icon || got.Color != want.Color {
		t.Errorf("icon/color = %q/%q, want preserved", got.Icon, got.Color)
	}
}

// The three directions on the pairing key: the web form (absent) preserves,
// syncapi can still set it, and syncapi can still CLEAR it with a null.
func TestMarker_FoundryID_AbsentPreserves_PresentReplaces_NullClears(t *testing.T) {
	cases := []struct {
		name  string
		input patch.Field[string]
		want  *string
	}{
		{"absent preserves the pairing (the web form never sends it)", patch.Absent[string](), strPtrMK("foundry-note-42")},
		{"present re-pairs", patch.Of("foundry-note-99"), strPtrMK("foundry-note-99")},
		{"explicit null unpairs (syncapi keeps this power)", patch.Null[string](), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runMarkerUpdate(t, UpdateMarkerInput{FoundryID: tc.input})
			assertMarkerPtr(t, "FoundryID", got.FoundryID, tc.want)
		})
	}
}

func TestMarker_PinCategoryAndVisibilityRules_ThreeDirections(t *testing.T) {
	cases := []struct {
		name    string
		input   UpdateMarkerInput
		wantPin *string
		wantVis *string
	}{
		{"absent preserves both", UpdateMarkerInput{Name: patch.Of("renamed")}, strPtrMK("secret"), strPtrMK(`{"allowed_users":["u-1"]}`)},
		{"present replaces", UpdateMarkerInput{PinCategory: patch.Of("landmark"), VisibilityRules: patch.Of(`{"denied_users":["u-2"]}`)}, strPtrMK("landmark"), strPtrMK(`{"denied_users":["u-2"]}`)},
		{"explicit null clears", UpdateMarkerInput{PinCategory: patch.Null[string](), VisibilityRules: patch.Null[string]()}, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runMarkerUpdate(t, tc.input)
			assertMarkerPtr(t, "PinCategory", got.PinCategory, tc.wantPin)
			assertMarkerPtr(t, "VisibilityRules", got.VisibilityRules, tc.wantVis)
		})
	}
}

// The validators must read the MERGED value: an absent name is not an empty
// name, and an absent coordinate is not 0.
func TestMarker_ValidatorsReadTheMergedRow(t *testing.T) {
	if got := runMarkerUpdate(t, UpdateMarkerInput{}); got.Name != "The Sunken Door" {
		t.Errorf("an empty input errored or blanked the name: %q", got.Name)
	}
	// …and a coordinate that IS sent is still range-checked.
	repo := &mockMapRepo{getMarkerFn: func(_ context.Context, _ string) (*Marker, error) { return storedMarker(), nil }}
	if err := newTestMapService(repo).UpdateMarker(context.Background(), "mk-1", UpdateMarkerInput{X: patch.Of(150.0)}); err == nil {
		t.Error("an out-of-range x must still be rejected")
	}
}

// The client half: a drag must send only the position, and the edit form
// must send an explicit null for a field the operator emptied.
func TestMarkerClients_SendOnlyWhatTheyMean(t *testing.T) {
	src, err := os.ReadFile("maps.templ")
	if err != nil {
		t.Fatalf("read maps.templ: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "body: { x: mk.x, y: mk.y },") {
		t.Error("the drag-end PUT no longer sends exactly {x, y}; echoing other fields is the pattern the ruling replaced, and it never covered pin_category / visibility_rules / foundry_id anyway")
	}
	for _, want := range []string{
		"body.description = fd.get('description') || null;",
		"body.entity_id = fd.get('entity_id') || null;",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("maps.templ no longer sends %q — under absent-means-preserve, an emptied field that is merely omitted can never be cleared", want)
		}
	}
	// The browser form must never carry the Foundry pairing key. Comment
	// lines are skipped — this file explains WHY the key is absent, and the
	// explanation must not be what trips the pin.
	for i, line := range strings.Split(text, "\n") {
		code := strings.TrimSpace(line)
		if strings.HasPrefix(code, "//") || strings.HasPrefix(code, "<!--") {
			continue
		}
		if strings.Contains(code, "foundry_id") {
			t.Errorf("maps.templ:%d carries foundry_id in code: %q. A browser form has no business setting or clearing a sync pairing key.", i+1, code)
		}
	}
}

func strPtrMK(s string) *string { return &s }

func assertMarkerPtr(t *testing.T, field string, got, want *string) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s = %q, want nil", field, *got)
	case want != nil && got == nil:
		t.Errorf("%s = nil, want %q", field, *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("%s = %q, want %q", field, *got, *want)
	}
}
