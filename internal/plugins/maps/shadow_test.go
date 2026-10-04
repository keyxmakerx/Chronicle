package maps

import (
	"net/http"
	"net/http/httptest"

	"github.com/labstack/echo/v4"

	"context"
	"encoding/json"
	"errors"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func pts(s string) json.RawMessage { return json.RawMessage(s) }

func TestShadowAreaContains(t *testing.T) {
	area, ok := shadowAreaFromDrawing(Drawing{DrawingType: "shadow", Points: pts(`[{"x":60,"y":70},{"x":20,"y":30}]`)})
	if !ok {
		t.Fatal("expected a valid area from corners given in reverse order")
	}
	cases := []struct {
		name string
		x, y float64
		want bool
	}{
		{"centre", 40, 50, true},
		{"left edge", 20, 50, true},
		{"right edge", 60, 50, true},
		{"top edge", 40, 30, true},
		{"bottom edge", 40, 70, true},
		{"corner a", 20, 30, true},
		{"corner b", 60, 70, true},
		{"just outside left", 19.99, 50, false},
		{"just outside right", 60.01, 50, false},
		{"just above", 40, 29.99, false},
		{"just below", 40, 70.01, false},
		{"far away", 90, 90, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := area.Contains(tc.x, tc.y); got != tc.want {
				t.Errorf("Contains(%v,%v) = %v, want %v", tc.x, tc.y, got, tc.want)
			}
		})
	}
}

func TestShadowAreaFromDrawing_Rejects(t *testing.T) {
	cases := []struct {
		name string
		d    Drawing
	}{
		{"not a shadow", Drawing{DrawingType: "rectangle", Points: pts(`[{"x":1,"y":1},{"x":2,"y":2}]`)}},
		{"one point", Drawing{DrawingType: "shadow", Points: pts(`[{"x":1,"y":1}]`)}},
		{"three points", Drawing{DrawingType: "shadow", Points: pts(`[{"x":1,"y":1},{"x":2,"y":2},{"x":3,"y":3}]`)}},
		{"garbage", Drawing{DrawingType: "shadow", Points: pts(`"nope"`)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := shadowAreaFromDrawing(tc.d); ok {
				t.Error("expected no area")
			}
		})
	}
}

func TestDrawingUnderShadow(t *testing.T) {
	areas := []ShadowArea{{MinX: 10, MinY: 10, MaxX: 30, MaxY: 30}, {MinX: 50, MinY: 50, MaxX: 70, MaxY: 70}}
	cases := []struct {
		name string
		d    Drawing
		want bool
	}{
		{"wholly inside one", Drawing{DrawingType: "freehand", Points: pts(`[{"x":12,"y":12},{"x":28,"y":28}]`)}, true},
		{"each point in a different shadow", Drawing{DrawingType: "polygon", Points: pts(`[{"x":12,"y":12},{"x":60,"y":60}]`)}, true},
		{"one point outside", Drawing{DrawingType: "freehand", Points: pts(`[{"x":12,"y":12},{"x":40,"y":40}]`)}, false},
		{"on the edge counts as inside", Drawing{DrawingType: "text", Points: pts(`[{"x":10,"y":30}]`)}, true},
		{"shadow itself is never hidden", Drawing{DrawingType: "shadow", Points: pts(`[{"x":12,"y":12},{"x":28,"y":28}]`)}, false},
		{"no points: nothing to hide", Drawing{DrawingType: "freehand", Points: pts(`[]`)}, false},
		{"unparseable fails closed", Drawing{DrawingType: "freehand", Points: pts(`{`)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := drawingUnderShadow(areas, tc.d); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
	if drawingUnderShadow(nil, Drawing{DrawingType: "freehand", Points: pts(`{`)}) {
		t.Error("with no shadows nothing is hidden, even an unparseable drawing")
	}
}

func TestShadowHidingApplies(t *testing.T) {
	cases := []struct {
		role int
		want bool
	}{
		{permissions.RoleNone, true},
		{permissions.RolePlayer, true},
		// A scribe is a diligent player: hidden like one.
		{permissions.RoleScribe, true},
		{permissions.RoleOwner, false},
	}
	for _, tc := range cases {
		if got := shadowHidingApplies(tc.role); got != tc.want {
			t.Errorf("role %d: got %v, want %v", tc.role, got, tc.want)
		}
	}
}

func TestNormalizeShadowAlpha(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{0, 0.5}, {0.5, 0.5}, {0.85, 0.85}, {0.7, 0.5}, {1, 0.5}, {-3, 0.5},
	}
	for _, tc := range cases {
		if got := normalizeShadowAlpha(tc.in); got != tc.want {
			t.Errorf("normalizeShadowAlpha(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// shadowRepo serves a fixed set of drawings and shadows.
type shadowRepo struct {
	idorRepo
	drawings []Drawing
	shadows  []Drawing
	created  *Drawing
	listErr  error
}

func (r *shadowRepo) ListDrawings(context.Context, string, int, string) ([]Drawing, error) {
	return r.drawings, nil
}
func (r *shadowRepo) ListShadows(context.Context, string) ([]Drawing, error) {
	return r.shadows, r.listErr
}
func (r *shadowRepo) CreateDrawing(_ context.Context, d *Drawing) error { r.created = d; return nil }

var testShadow = Drawing{ID: "sh", MapID: "map-1", DrawingType: "shadow", Points: pts(`[{"x":10,"y":10},{"x":30,"y":30}]`)}

func TestDrawingService_ListDrawings_ShadowHiding(t *testing.T) {
	inside := Drawing{ID: "in", DrawingType: "freehand", Points: pts(`[{"x":15,"y":15},{"x":25,"y":25}]`)}
	outside := Drawing{ID: "out", DrawingType: "freehand", Points: pts(`[{"x":60,"y":60},{"x":70,"y":70}]`)}
	repo := &shadowRepo{drawings: []Drawing{testShadow, inside, outside}, shadows: []Drawing{testShadow}}
	svc := NewDrawingService(repo)

	ids := func(ds []Drawing) []string {
		var out []string
		for _, d := range ds {
			out = append(out, d.ID)
		}
		return out
	}

	player, err := svc.ListDrawings(context.Background(), "map-1", permissions.RolePlayer, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(player); len(got) != 2 || got[0] != "sh" || got[1] != "out" {
		t.Errorf("player sees %v, want the shadow and the outside drawing only", got)
	}

	scribe, _ := svc.ListDrawings(context.Background(), "map-1", permissions.RoleScribe, "u2")
	if got := ids(scribe); len(got) != 2 || got[0] != "sh" || got[1] != "out" {
		t.Errorf("scribe sees %v, want what a player sees", got)
	}

	// The owner, and a co-DM whose grant promotes the visibility role to owner.
	dm, _ := svc.ListDrawings(context.Background(), "map-1", permissions.RoleOwner, "u3")
	if len(dm) != 3 {
		t.Errorf("owner/co-DM sees %d drawings, want all 3", len(dm))
	}

	repo.listErr = errors.New("db down")
	if _, err := svc.ListDrawings(context.Background(), "map-1", permissions.RolePlayer, "u1"); err == nil {
		t.Error("a failed shadow lookup must fail the player's list, not leak it")
	}
}

func TestDrawingService_IsDrawingShadowed(t *testing.T) {
	repo := &shadowRepo{shadows: []Drawing{testShadow}}
	svc := NewDrawingService(repo)
	inside := &Drawing{MapID: "map-1", DrawingType: "text", Points: pts(`[{"x":20,"y":20}]`)}
	for _, tc := range []struct {
		role int
		want bool
	}{{permissions.RolePlayer, true}, {permissions.RoleScribe, true}, {permissions.RoleOwner, false}} {
		got, err := svc.IsDrawingShadowed(context.Background(), inside, tc.role)
		if err != nil || got != tc.want {
			t.Errorf("role %d: got %v, %v; want %v", tc.role, got, err, tc.want)
		}
	}
}

func TestDrawingService_CreateShadow_Validation(t *testing.T) {
	cases := []struct {
		name      string
		points    string
		alpha     float64
		wantErr   bool
		wantAlpha float64
	}{
		{"default strength", `[{"x":1,"y":1},{"x":9,"y":9}]`, 0, false, 0.5},
		{"almost nothing kept", `[{"x":1,"y":1},{"x":9,"y":9}]`, 0.85, false, 0.85},
		{"odd strength becomes a hint", `[{"x":1,"y":1},{"x":9,"y":9}]`, 0.3, false, 0.5},
		{"one point refused", `[{"x":1,"y":1}]`, 0.5, true, 0},
		{"three points refused", `[{"x":1,"y":1},{"x":2,"y":2},{"x":3,"y":3}]`, 0.5, true, 0},
		{"not coordinates refused", `[1,2]`, 0.5, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &shadowRepo{}
			svc := NewDrawingService(repo)
			d, err := svc.CreateDrawing(context.Background(), CreateDrawingInput{
				MapID: "map-1", DrawingType: "shadow", Points: pts(tc.points),
				FillAlpha: tc.alpha, CallerRole: permissions.RoleOwner, CallerIsDM: true, CreatedBy: "u",
			})
			if tc.wantErr {
				if err == nil || repo.created != nil {
					t.Fatalf("expected refusal, got err=%v created=%v", err, repo.created)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if d.FillAlpha != tc.wantAlpha {
				t.Errorf("fill_alpha = %v, want %v", d.FillAlpha, tc.wantAlpha)
			}
		})
	}
}

type fakeShadowLookup struct {
	areas []ShadowArea
	err   error
}

func (f fakeShadowLookup) ShadowAreas(context.Context, string) ([]ShadowArea, error) {
	return f.areas, f.err
}

func TestMapService_ListMarkers_ShadowHiding(t *testing.T) {
	all := []Marker{
		{ID: "under", MapID: "map-1", X: 20, Y: 20},
		{ID: "edge", MapID: "map-1", X: 30, Y: 10},
		{ID: "clear", MapID: "map-1", X: 80, Y: 80},
	}
	repo := &mockMapRepo{listMarkersFn: func(context.Context, string, int) ([]Marker, error) {
		return append([]Marker(nil), all...), nil
	}}
	area := ShadowArea{MinX: 10, MinY: 10, MaxX: 30, MaxY: 30}
	build := func(l ShadowLookup) *mapService {
		s := NewMapService(repo).(*mapService)
		s.SetShadowLookup(l)
		return s
	}

	player, err := build(fakeShadowLookup{areas: []ShadowArea{area}}).ListMarkers(context.Background(), "camp-1", "map-1", permissions.RolePlayer, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(player) != 1 || player[0].ID != "clear" {
		t.Errorf("player sees %v, want only the pin outside the shadow", player)
	}

	scribe, _ := build(fakeShadowLookup{areas: []ShadowArea{area}}).ListMarkers(context.Background(), "camp-1", "map-1", permissions.RoleScribe, "u2")
	if len(scribe) != 1 || scribe[0].ID != "clear" {
		t.Errorf("scribe sees %v, want what a player sees", scribe)
	}

	owner, _ := build(fakeShadowLookup{areas: []ShadowArea{area}}).ListMarkers(context.Background(), "camp-1", "map-1", permissions.RoleOwner, "u2")
	if len(owner) != 3 {
		t.Errorf("owner/co-DM sees %d pins, want all 3", len(owner))
	}

	if _, err := build(fakeShadowLookup{err: errors.New("db down")}).ListMarkers(context.Background(), "camp-1", "map-1", permissions.RolePlayer, "u1"); err == nil {
		t.Error("a failed shadow lookup must fail the player's list, not leak it")
	}

	hidden, err := build(fakeShadowLookup{areas: []ShadowArea{area}}).IsMarkerShadowed(context.Background(), &all[0], permissions.RolePlayer)
	if err != nil || !hidden {
		t.Errorf("IsMarkerShadowed = %v, %v; want true", hidden, err)
	}
}

// The shadow tool and its two strengths are offered to DMs who can draw, never
// to scribes or players; the shadow module loads for everyone because players
// need it to see the shadow.
func TestMapEditorBody_ShadowToolOnlyForDMs(t *testing.T) {
	render := func(isScribe, isDM bool) string {
		cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", Name: "Test"}}
		data := MapViewData{
			CampaignID: "c1",
			Map:        &Map{ID: "m1", Name: "Test Map", ImageWidth: 1000, ImageHeight: 800},
			IsScribe:   isScribe, IsOwner: isDM, IsDM: isDM,
		}
		var sb strings.Builder
		if err := MapEditorBody(cc, data, "flex-1", "").Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		return sb.String() + viewerScript(t)
	}
	dm := render(true, true)
	for _, want := range []string{`data-tool="shadow"`, `data-shadow-strength="0.5"`, `data-shadow-strength="0.85"`, "Hide under shadow", `data-can-shadow="true"`} {
		if !strings.Contains(dm, want) {
			t.Errorf("DM markup missing %q", want)
		}
	}
	for name, markup := range map[string]string{"scribe": render(true, false), "player": render(false, false)} {
		for _, banned := range []string{`class="mp-btn" data-tool="shadow"`, `<button type="button" class="mp-wd" data-shadow-strength=`, `data-can-shadow="true"`} {
			if strings.Contains(markup, banned) {
				t.Errorf("%s markup must not contain %q", name, banned)
			}
		}
	}
	for _, isDM := range []bool{true, false} {
		if !strings.Contains(render(true, isDM), "map_shadow.js") {
			t.Errorf("shadow script missing for dm=%v", isDM)
		}
	}
}

// A scribe may draw everything else, but creating, changing or deleting a
// shadow is refused at the service whatever the handler passes.
func TestDrawingService_ShadowWritesAreDMOnly(t *testing.T) {
	shadowPts := pts(`[{"x":1,"y":1},{"x":9,"y":9}]`)
	for _, tc := range []struct {
		name    string
		isDM    bool
		wantErr bool
	}{{"scribe", false, true}, {"owner or co-DM", true, false}} {
		t.Run("create/"+tc.name, func(t *testing.T) {
			repo := &shadowRepo{}
			_, err := NewDrawingService(repo).CreateDrawing(context.Background(), CreateDrawingInput{
				MapID: "map-1", DrawingType: "shadow", Points: shadowPts, CallerRole: permissions.RoleScribe, CallerIsDM: tc.isDM,
			})
			if (err != nil) != tc.wantErr || (tc.wantErr && repo.created != nil) {
				t.Errorf("err=%v created=%v, wantErr=%v", err, repo.created, tc.wantErr)
			}
		})
		t.Run("update/"+tc.name, func(t *testing.T) {
			sr := &shadowGetRepo{drawing: Drawing{ID: "sh", MapID: "map-1", DrawingType: "shadow", Points: shadowPts}}
			err := NewDrawingService(sr).UpdateDrawing(context.Background(), "sh", "map-1", permissions.RoleScribe, tc.isDM,
				UpdateDrawingInput{FillAlpha: patch.Of(0.85)})
			if (err != nil) != tc.wantErr {
				t.Errorf("err=%v, wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr && sr.mutated {
				t.Error("a refused update must not write")
			}
		})
		t.Run("delete/"+tc.name, func(t *testing.T) {
			sr := &shadowGetRepo{drawing: Drawing{ID: "sh", MapID: "map-1", DrawingType: "shadow", Points: shadowPts}}
			err := NewDrawingService(sr).DeleteDrawing(context.Background(), "sh", "map-1", nil, "", permissions.RoleScribe, tc.isDM)
			if (err != nil) != tc.wantErr {
				t.Errorf("err=%v, wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr && sr.mutated {
				t.Error("a refused delete must not write")
			}
		})
	}
	// Everything else stays open to a scribe.
	repo := &shadowRepo{}
	if _, err := NewDrawingService(repo).CreateDrawing(context.Background(), CreateDrawingInput{
		MapID: "map-1", DrawingType: "freehand", Points: pts(`[{"x":1,"y":1}]`), CallerRole: permissions.RoleScribe,
	}); err != nil {
		t.Errorf("a scribe must still draw ordinary shapes: %v", err)
	}
}

// guardShadowRepo / handler test: a by-id read must not reveal what the list
// withholds.
func TestDrawingHandler_GetDrawing_ShadowedIs404ForPlayers(t *testing.T) {
	inside := Drawing{ID: "d-1", MapID: "map-1", DrawingType: "freehand", Points: pts(`[{"x":15,"y":15},{"x":25,"y":25}]`)}
	sr := &shadowGetRepo{shadows: []Drawing{testShadow}, drawing: inside}
	svc := NewDrawingService(sr)
	mapSvc := guardMapSvc{}
	h := NewDrawingHandler(mapSvc, svc)

	for _, tc := range []struct {
		role      campaigns.Role
		dmGranted bool
		wantCode  int
	}{
		{campaigns.RolePlayer, false, http.StatusNotFound},
		{campaigns.RoleScribe, false, http.StatusNotFound},
		{campaigns.RoleScribe, true, http.StatusOK}, // a co-DM grant is DM-equivalent
		{campaigns.RoleOwner, false, http.StatusOK},
	} {
		e := echo.New()
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
		c.SetParamNames("id", "mid", "did")
		c.SetParamValues("camp-1", "map-1", "d-1")
		c.Set("campaign_context", dmWriteCampaignCtx(tc.role, tc.dmGranted))
		err := h.GetDrawing(c)
		got := http.StatusOK
		if err != nil {
			var ae *apperror.AppError
			if !errors.As(err, &ae) {
				t.Fatalf("role %d: unexpected error %v", tc.role, err)
			}
			got = ae.Code
		}
		if got != tc.wantCode {
			t.Errorf("role %d: status %d, want %d", tc.role, got, tc.wantCode)
		}
	}
}

type shadowGetRepo struct {
	idorRepo
	shadows []Drawing
	drawing Drawing
}

func (r *shadowGetRepo) ListShadows(context.Context, string) ([]Drawing, error) {
	return r.shadows, nil
}
func (r *shadowGetRepo) GetDrawing(context.Context, string) (*Drawing, error) {
	d := r.drawing
	return &d, nil
}
