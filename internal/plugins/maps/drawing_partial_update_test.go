// drawing_partial_update_test.go pins the token, drawing and layer half of
// the absent-means-preserve contract (.ai/conventions.md): every field of
// UpdateTokenInput, UpdateDrawingInput and UpdateLayerInput must use
// patch.Field[T] rather than a plain *T, since a plain pointer bound from
// JSON can't distinguish "key omitted" from "key sent null". A drag PUT
// carrying only {x, y} must not zero IsHidden/IsLocked/HP bars/aura-light-
// vision, and a SortOrder-only layer PUT must not reset visibility/lock.
package maps

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// mockDrawingRepo is a configurable DrawingRepository for these regression
// tests. Only the Get*/Update* methods under test are wired per-case;
// everything else is a harmless no-op so the type satisfies the interface.
type mockDrawingRepo struct {
	getDrawingFn    func(ctx context.Context, id string) (*Drawing, error)
	updateDrawingFn func(ctx context.Context, d *Drawing) error
	createTokenFn   func(ctx context.Context, t *Token) error
	getTokenFn      func(ctx context.Context, id string) (*Token, error)
	updateTokenFn   func(ctx context.Context, t *Token) error
	getLayerFn      func(ctx context.Context, id string) (*Layer, error)
	updateLayerFn   func(ctx context.Context, l *Layer) error
}

func (r *mockDrawingRepo) CreateDrawing(context.Context, *Drawing) error { return nil }
func (r *mockDrawingRepo) GetDrawing(ctx context.Context, id string) (*Drawing, error) {
	return r.getDrawingFn(ctx, id)
}
func (r *mockDrawingRepo) UpdateDrawing(ctx context.Context, d *Drawing) error {
	return r.updateDrawingFn(ctx, d)
}
func (r *mockDrawingRepo) DeleteDrawing(context.Context, string) error            { return nil }
func (r *mockDrawingRepo) ListShadows(context.Context, string) ([]Drawing, error) { return nil, nil }
func (r *mockDrawingRepo) ListDrawings(context.Context, string, int, string) ([]Drawing, error) {
	return nil, nil
}

func (r *mockDrawingRepo) CreateToken(ctx context.Context, t *Token) error {
	if r.createTokenFn != nil {
		return r.createTokenFn(ctx, t)
	}
	return nil
}
func (r *mockDrawingRepo) GetToken(ctx context.Context, id string) (*Token, error) {
	return r.getTokenFn(ctx, id)
}
func (r *mockDrawingRepo) UpdateToken(ctx context.Context, t *Token) error {
	return r.updateTokenFn(ctx, t)
}
func (r *mockDrawingRepo) UpdateTokenPosition(context.Context, string, float64, float64) error {
	return nil
}
func (r *mockDrawingRepo) DeleteToken(context.Context, string) error { return nil }
func (r *mockDrawingRepo) ListTokens(context.Context, string, int) ([]Token, error) {
	return nil, nil
}

func (r *mockDrawingRepo) CreateLayer(context.Context, *Layer) error { return nil }
func (r *mockDrawingRepo) GetLayer(ctx context.Context, id string) (*Layer, error) {
	return r.getLayerFn(ctx, id)
}
func (r *mockDrawingRepo) UpdateLayer(ctx context.Context, l *Layer) error {
	return r.updateLayerFn(ctx, l)
}
func (r *mockDrawingRepo) DeleteLayer(context.Context, string) error           { return nil }
func (r *mockDrawingRepo) ListLayers(context.Context, string) ([]Layer, error) { return nil, nil }

func (r *mockDrawingRepo) CreateFog(context.Context, *FogRegion) error        { return nil }
func (r *mockDrawingRepo) GetFog(context.Context, string) (*FogRegion, error) { return nil, nil }
func (r *mockDrawingRepo) DeleteFog(context.Context, string) error            { return nil }
func (r *mockDrawingRepo) ListFog(context.Context, string) ([]FogRegion, error) {
	return nil, nil
}
func (r *mockDrawingRepo) ResetFog(context.Context, string) error { return nil }

func strPtrDS(s string) *string     { return &s }
func floatPtrDS(f float64) *float64 { return &f }
func intPtrDS(i int) *int           { return &i }

func assertStrPtrDS(t *testing.T, field string, got, want *string) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s = %q, want nil (preserved as absent)", field, *got)
	case want != nil && got == nil:
		t.Errorf("%s = nil, want %q (preserved)", field, *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("%s = %q, want %q (preserved)", field, *got, *want)
	}
}

func assertFloatPtrDS(t *testing.T, field string, got, want *float64) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s = %v, want nil (preserved as absent)", field, *got)
	case want != nil && got == nil:
		t.Errorf("%s = nil, want %v (preserved)", field, *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("%s = %v, want %v (preserved)", field, *got, *want)
	}
}

func assertIntPtrDS(t *testing.T, field string, got, want *int) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s = %v, want nil (preserved as absent)", field, *got)
	case want != nil && got == nil:
		t.Errorf("%s = nil, want %v (preserved)", field, *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("%s = %v, want %v (preserved)", field, *got, *want)
	}
}

// --- Token ---

// storedToken is the fully-configured row every token case starts from.
func storedToken() *Token {
	return &Token{
		ID:             "tok-1",
		MapID:          "map-1",
		Name:           "Ambush Wolf",
		ImagePath:      strPtrDS("wolf.png"),
		X:              10,
		Y:              20,
		Width:          2,
		Height:         2,
		Rotation:       15,
		Scale:          1.5,
		IsHidden:       true,
		IsLocked:       true,
		Bar1Value:      intPtrDS(12),
		Bar1Max:        intPtrDS(20),
		Bar2Value:      intPtrDS(3),
		Bar2Max:        intPtrDS(5),
		AuraRadius:     floatPtrDS(15),
		AuraColor:      strPtrDS("#ff0000"),
		LightRadius:    floatPtrDS(20),
		LightDimRadius: floatPtrDS(30),
		LightColor:     strPtrDS("#ffffff"),
		VisionEnabled:  true,
		VisionRange:    floatPtrDS(60),
		Elevation:      5,
		StatusEffects:  json.RawMessage(`{"poisoned":true}`),
		Flags:          json.RawMessage(`{"foo":"bar"}`),
		FoundryID:      strPtrDS("foundry-tok-1"),
	}
}

func runTokenUpdate(t *testing.T, input UpdateTokenInput) *Token {
	t.Helper()
	var written *Token
	repo := &mockDrawingRepo{
		getTokenFn:    func(_ context.Context, _ string) (*Token, error) { return storedToken(), nil },
		updateTokenFn: func(_ context.Context, tok *Token) error { written = tok; return nil },
	}
	if err := NewDrawingService(repo).UpdateToken(context.Background(), "tok-1", "map-1", true, input); err != nil {
		t.Fatalf("UpdateToken: %v", err)
	}
	if written == nil {
		t.Fatal("nothing was written")
	}
	return written
}

// THE headline regression: a drag body carrying only {x, y} must move the
// token and change nothing else.
func TestTokenDrag_MovesTheTokenAndNothingElse(t *testing.T) {
	got := runTokenUpdate(t, UpdateTokenInput{X: patch.Of(77.5), Y: patch.Of(12.25)})
	want := storedToken()

	if got.X != 77.5 || got.Y != 12.25 {
		t.Errorf("position = (%v,%v), want (77.5,12.25)", got.X, got.Y)
	}
	if !got.IsHidden {
		t.Error("IsHidden flipped to false: a hidden token must stay hidden across a position-only update")
	}
	if !got.IsLocked {
		t.Error("IsLocked flipped to false")
	}
	if !got.VisionEnabled {
		t.Error("VisionEnabled flipped to false")
	}
	if got.Name != want.Name {
		t.Errorf("Name = %q, want preserved", got.Name)
	}
	if got.Elevation != want.Elevation {
		t.Errorf("Elevation = %d, want %d (preserved)", got.Elevation, want.Elevation)
	}
	if got.Width != want.Width || got.Height != want.Height || got.Rotation != want.Rotation || got.Scale != want.Scale {
		t.Errorf("size/rotation/scale changed: got (%v,%v,%v,%v), want (%v,%v,%v,%v)",
			got.Width, got.Height, got.Rotation, got.Scale, want.Width, want.Height, want.Rotation, want.Scale)
	}
	assertIntPtrDS(t, "Bar1Value", got.Bar1Value, want.Bar1Value)
	assertIntPtrDS(t, "Bar1Max", got.Bar1Max, want.Bar1Max)
	assertIntPtrDS(t, "Bar2Value", got.Bar2Value, want.Bar2Value)
	assertIntPtrDS(t, "Bar2Max", got.Bar2Max, want.Bar2Max)
	assertFloatPtrDS(t, "AuraRadius", got.AuraRadius, want.AuraRadius)
	assertStrPtrDS(t, "AuraColor", got.AuraColor, want.AuraColor)
	assertFloatPtrDS(t, "LightRadius", got.LightRadius, want.LightRadius)
	assertFloatPtrDS(t, "LightDimRadius", got.LightDimRadius, want.LightDimRadius)
	assertStrPtrDS(t, "LightColor", got.LightColor, want.LightColor)
	assertFloatPtrDS(t, "VisionRange", got.VisionRange, want.VisionRange)
	assertStrPtrDS(t, "ImagePath", got.ImagePath, want.ImagePath)
	if string(got.StatusEffects) != string(want.StatusEffects) {
		t.Errorf("StatusEffects = %s, want %s (preserved)", got.StatusEffects, want.StatusEffects)
	}
	if string(got.Flags) != string(want.Flags) {
		t.Errorf("Flags = %s, want %s (preserved)", got.Flags, want.Flags)
	}
}

// The three directions on a token's pointer-shaped fields: absent preserves,
// present replaces, explicit null clears.
func TestToken_AuraRadiusAndImagePath_ThreeDirections(t *testing.T) {
	cases := []struct {
		name          string
		input         UpdateTokenInput
		wantAura      *float64
		wantImagePath *string
	}{
		{"absent preserves both", UpdateTokenInput{Elevation: patch.Of(9)}, floatPtrDS(15), strPtrDS("wolf.png")},
		{"present replaces both", UpdateTokenInput{AuraRadius: patch.Of(40.0), ImagePath: patch.Of("dire-wolf.png")}, floatPtrDS(40), strPtrDS("dire-wolf.png")},
		{"explicit null clears both", UpdateTokenInput{AuraRadius: patch.Null[float64](), ImagePath: patch.Null[string]()}, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runTokenUpdate(t, tc.input)
			assertFloatPtrDS(t, "AuraRadius", got.AuraRadius, tc.wantAura)
			assertStrPtrDS(t, "ImagePath", got.ImagePath, tc.wantImagePath)
		})
	}
}

// --- Drawing ---

func storedDrawing() *Drawing {
	return &Drawing{
		ID:          "d-1",
		MapID:       "map-1",
		DrawingType: "freehand",
		Points:      json.RawMessage(`[{"x":1,"y":1}]`),
		StrokeColor: "#111111",
		StrokeWidth: 3,
		FillColor:   strPtrDS("#222222"),
		FillAlpha:   0.5,
		TextContent: strPtrDS("keep me"),
		FontSize:    intPtrDS(14),
		Rotation:    30,
		Visibility:  "dm_only",
		FoundryID:   strPtrDS("foundry-draw-1"),
	}
}

func runDrawingUpdate(t *testing.T, input UpdateDrawingInput) *Drawing {
	t.Helper()
	var written *Drawing
	repo := &mockDrawingRepo{
		getDrawingFn:    func(_ context.Context, _ string) (*Drawing, error) { return storedDrawing(), nil },
		updateDrawingFn: func(_ context.Context, d *Drawing) error { written = d; return nil },
	}
	if err := NewDrawingService(repo).UpdateDrawing(context.Background(), "d-1", "map-1", "", permissions.RoleOwner, true, input); err != nil {
		t.Fatalf("UpdateDrawing: %v", err)
	}
	if written == nil {
		t.Fatal("nothing was written")
	}
	return written
}

// A reshape body carrying only {points} must move the stroke and change
// nothing else — not its fill, its text, its font size, or its rotation.
func TestDrawingReshape_MovesPointsAndNothingElse(t *testing.T) {
	newPoints := json.RawMessage(`[{"x":9,"y":9}]`)
	got := runDrawingUpdate(t, UpdateDrawingInput{Points: patch.Of(newPoints)})
	want := storedDrawing()

	if string(got.Points) != string(newPoints) {
		t.Errorf("Points = %s, want %s", got.Points, newPoints)
	}
	assertStrPtrDS(t, "FillColor", got.FillColor, want.FillColor)
	assertStrPtrDS(t, "TextContent", got.TextContent, want.TextContent)
	assertIntPtrDS(t, "FontSize", got.FontSize, want.FontSize)
	if got.FillAlpha != want.FillAlpha {
		t.Errorf("FillAlpha = %v, want %v (preserved)", got.FillAlpha, want.FillAlpha)
	}
	if got.Rotation != want.Rotation {
		t.Errorf("Rotation = %v, want %v (preserved)", got.Rotation, want.Rotation)
	}
	if got.Visibility != want.Visibility {
		t.Errorf("Visibility = %q, want %q (preserved)", got.Visibility, want.Visibility)
	}
}

// The three directions on a drawing's pointer-shaped fields.
func TestDrawing_FillColorAndTextContent_ThreeDirections(t *testing.T) {
	cases := []struct {
		name            string
		input           UpdateDrawingInput
		wantFillColor   *string
		wantTextContent *string
	}{
		{"absent preserves both", UpdateDrawingInput{Rotation: patch.Of(45.0)}, strPtrDS("#222222"), strPtrDS("keep me")},
		{"present replaces both", UpdateDrawingInput{FillColor: patch.Of("#abcdef"), TextContent: patch.Of("new text")}, strPtrDS("#abcdef"), strPtrDS("new text")},
		{"explicit null clears both", UpdateDrawingInput{FillColor: patch.Null[string](), TextContent: patch.Null[string]()}, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runDrawingUpdate(t, tc.input)
			assertStrPtrDS(t, "FillColor", got.FillColor, tc.wantFillColor)
			assertStrPtrDS(t, "TextContent", got.TextContent, tc.wantTextContent)
		})
	}
}

// --- Layer ---

func storedLayer() *Layer {
	return &Layer{
		ID:        "l-1",
		MapID:     "map-1",
		Name:      "GM Layer",
		LayerType: "gm",
		SortOrder: 2,
		IsVisible: true,
		Opacity:   0.8,
		IsLocked:  true,
	}
}

func runLayerUpdate(t *testing.T, input UpdateLayerInput) *Layer {
	t.Helper()
	var written *Layer
	repo := &mockDrawingRepo{
		getLayerFn:    func(_ context.Context, _ string) (*Layer, error) { return storedLayer(), nil },
		updateLayerFn: func(_ context.Context, l *Layer) error { written = l; return nil },
	}
	if err := NewDrawingService(repo).UpdateLayer(context.Background(), "l-1", "map-1", input); err != nil {
		t.Fatalf("UpdateLayer: %v", err)
	}
	if written == nil {
		t.Fatal("nothing was written")
	}
	return written
}

// A drag-to-reorder body carrying only {sort_order} must reorder the layer
// and change nothing else — not its visibility, not its lock state.
func TestLayerReorder_ChangesSortOrderAndNothingElse(t *testing.T) {
	got := runLayerUpdate(t, UpdateLayerInput{SortOrder: patch.Of(5)})
	want := storedLayer()

	if got.SortOrder != 5 {
		t.Errorf("SortOrder = %d, want 5", got.SortOrder)
	}
	if !got.IsVisible {
		t.Error("IsVisible flipped to false: a reorder must not hide the layer")
	}
	if !got.IsLocked {
		t.Error("IsLocked flipped to false: a reorder must not unlock the layer")
	}
	if got.Opacity != want.Opacity {
		t.Errorf("Opacity = %v, want %v (preserved)", got.Opacity, want.Opacity)
	}
	if got.Name != want.Name {
		t.Errorf("Name = %q, want preserved", got.Name)
	}
}
