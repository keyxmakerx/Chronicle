// marker_partial_update_test.go pins the marker half of the
// absent-means-preserve contract (.ai/conventions.md):
//
//   - a marker edit or drag PUT that omits pin_category or visibility_rules
//     must preserve them, not erase them (visibility_rules is access-control
//     data);
//   - an edit that omits foundry_id must preserve the Foundry pairing key,
//     not null it (a nulled key resurfaces as a duplicate marker on the next
//     sync);
//   - a write refused by permission (e.g. a Scribe editing the Owner's
//     per-player rules) must drop to absent, not to null.
//
// The web form never sends foundry_id, and syncapi clears it only via an
// explicit null — absent-preserve does not block either path.
package maps

import (
	"context"
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
	if err := newTestMapService(repo).UpdateMarker(context.Background(), "mk-1", input, true); err != nil {
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
		{"present replaces", UpdateMarkerInput{PinCategory: patch.Of("danger"), VisibilityRules: patch.Of(`{"denied_users":["u-2"]}`)}, strPtrMK("danger"), strPtrMK(`{"denied_users":["u-2"]}`)},
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
	if err := newTestMapService(repo).UpdateMarker(context.Background(), "mk-1", UpdateMarkerInput{X: patch.Of(150.0)}, true); err == nil {
		t.Error("an out-of-range x must still be rejected")
	}
}

// The client half: a drag must send only the position, and the edit form
// must send an explicit null for a field the operator emptied, and send the
// entity link only when it changed: a link to a page the viewer can't see
// arrives blank, and echoing that blank back would unlink it.
func TestMarkerClients_SendOnlyWhatTheyMean(t *testing.T) {
	text := templSource(t) + viewerScript(t)
	if !strings.Contains(text, "body: { x: mk.x, y: mk.y },") {
		t.Error("the drag-end PUT no longer sends exactly {x, y}; echoing other fields is the pattern the ruling replaced, and it never covered pin_category / visibility_rules / foundry_id anyway")
	}
	for _, want := range []string{
		"body.description = fd.get('description') || null;",
		"entityInput.dataset.original = entityInput.value;",
		"if (!markerID || entityVal !== entityOriginal) {",
		"body.entity_id = entityVal || null;",
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

// pin_category is a closed set. Only a value the caller SENT is checked, so a
// legacy stored value never blocks an unrelated edit, and null still clears.
func TestMarker_PinCategoryValidation(t *testing.T) {
	cases := []struct {
		name    string
		input   UpdateMarkerInput
		wantErr bool
	}{
		{"location", UpdateMarkerInput{PinCategory: patch.Of("location")}, false},
		{"danger", UpdateMarkerInput{PinCategory: patch.Of("danger")}, false},
		{"treasure", UpdateMarkerInput{PinCategory: patch.Of("treasure")}, false},
		{"quest", UpdateMarkerInput{PinCategory: patch.Of("quest")}, false},
		{"note", UpdateMarkerInput{PinCategory: patch.Of("note")}, false},
		{"explicit null clears", UpdateMarkerInput{PinCategory: patch.Null[string]()}, false},
		{"absent keeps a legacy stored value", UpdateMarkerInput{Name: patch.Of("renamed")}, false},
		{"unknown value", UpdateMarkerInput{PinCategory: patch.Of("landmark")}, true},
		{"empty string", UpdateMarkerInput{PinCategory: patch.Of("")}, true},
		{"wrong case", UpdateMarkerInput{PinCategory: patch.Of("Danger")}, true},
	}
	for _, tc := range cases {
		t.Run("update/"+tc.name, func(t *testing.T) {
			repo := &mockMapRepo{
				getMarkerFn:    func(_ context.Context, _ string) (*Marker, error) { return storedMarker(), nil },
				updateMarkerFn: func(_ context.Context, _ *Marker) error { return nil },
			}
			err := newTestMapService(repo).UpdateMarker(context.Background(), "mk-1", tc.input, true)
			if tc.wantErr {
				assertAppError(t, err, 422)
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	createCases := []struct {
		name    string
		cat     *string
		wantErr bool
	}{
		{"nil is fine", nil, false},
		{"valid", strPtrMK("quest"), false},
		{"unknown", strPtrMK("secret"), true},
	}
	for _, tc := range createCases {
		t.Run("create/"+tc.name, func(t *testing.T) {
			repo := &mockMapRepo{createMarkerFn: func(_ context.Context, _ *Marker) error { return nil }}
			_, err := newTestMapService(repo).CreateMarker(context.Background(), CreateMarkerInput{
				MapID: "map-1", Name: "Pin", X: 1, Y: 1, PinCategory: tc.cat,
			})
			if tc.wantErr {
				assertAppError(t, err, 422)
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
