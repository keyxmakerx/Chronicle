package maps

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// fogWriteRepo serves one drawing and one token on map-1 and records whether a
// write reached it.
type fogWriteRepo struct {
	idorRepo
	d      Drawing
	tok    Token
	tokens []Token
}

func (r *fogWriteRepo) GetDrawing(context.Context, string) (*Drawing, error) {
	c := r.d
	return &c, nil
}
func (r *fogWriteRepo) GetToken(context.Context, string) (*Token, error) {
	c := r.tok
	return &c, nil
}
func (r *fogWriteRepo) ListTokens(context.Context, string, int) ([]Token, error) {
	return append([]Token(nil), r.tokens...), nil
}
func (r *fogWriteRepo) ListDrawings(context.Context, string, int, string) ([]Drawing, error) {
	return []Drawing{r.d}, nil
}

// positionRecorder records the map and audience flag of token position events.
type positionRecorder struct {
	NoopMapEventPublisher
	mapID string
}

func (p *positionRecorder) PublishTokenPositionEvent(_, mapID, _ string, _, _ float64, _ bool) {
	p.mapID = mapID
}

func fogWriteFixture() (*fogWriteRepo, DrawingService) {
	dx, dy, lx, ly := fogPositions()
	repo := &fogWriteRepo{
		d:   Drawing{ID: "d", MapID: "map-1", DrawingType: "freehand", Points: []byte(`[{"x":` + fl(dx) + `,"y":` + fl(dy) + `}]`)},
		tok: Token{ID: "t", MapID: "map-1", X: dx, Y: dy},
		tokens: []Token{
			{ID: "dark", MapID: "map-1", X: dx, Y: dy},
			{ID: "lit", MapID: "map-1", X: lx, Y: ly},
		},
	}
	svc := NewDrawingService(repo)
	svc.SetHexFogLookup(fakeFogLookup{mask: fogMaskFixture()})
	return repo, svc
}

// A scribe writing to a drawing or token in unexplored land gets the answer a
// missing id gets, and nothing is written; a DM is not limited.
func TestFogHiddenWrites_DrawingsAndTokens(t *testing.T) {
	tests := []struct {
		name  string
		isDM  bool
		write func(DrawingService) error
	}{
		{"update drawing", false, func(s DrawingService) error {
			return s.UpdateDrawing(context.Background(), "d", "map-1", permissions.RoleScribe, false, UpdateDrawingInput{})
		}},
		{"delete drawing", false, func(s DrawingService) error {
			return s.DeleteDrawing(context.Background(), "d", "map-1", nil, "u", permissions.RoleScribe, false)
		}},
		{"update token", false, func(s DrawingService) error {
			return s.UpdateToken(context.Background(), "t", "map-1", false, UpdateTokenInput{})
		}},
		{"move token", false, func(s DrawingService) error {
			return s.UpdateTokenPosition(context.Background(), "t", "map-1", false, UpdateTokenPositionInput{X: patch.Of(float64(1)), Y: patch.Of(float64(1))})
		}},
		{"DM updates drawing", true, func(s DrawingService) error {
			return s.UpdateDrawing(context.Background(), "d", "map-1", permissions.RoleOwner, true, UpdateDrawingInput{})
		}},
		{"DM moves token", true, func(s DrawingService) error {
			return s.UpdateTokenPosition(context.Background(), "t", "map-1", true, UpdateTokenPositionInput{X: patch.Of(float64(1)), Y: patch.Of(float64(1))})
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, svc := fogWriteFixture()
			err := tc.write(svc)
			if tc.isDM {
				if err != nil || !repo.mutated {
					t.Fatalf("DM write: err=%v mutated=%v, want it applied", err, repo.mutated)
				}
				return
			}
			assertAppError(t, err, http.StatusNotFound)
			if repo.mutated {
				t.Error("a refused write reached the repository")
			}
		})
	}
}

func TestListTokens_FogHiding(t *testing.T) {
	for _, tc := range []struct {
		name string
		role int
		want int
	}{
		{"public", permissions.RoleNone, 1},
		{"player", permissions.RolePlayer, 1},
		{"scribe", permissions.RoleScribe, 1},
		{"owner / co-DM", permissions.RoleOwner, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, svc := fogWriteFixture()
			got, err := svc.ListTokens(context.Background(), "map-1", tc.role)
			if err != nil || len(got) != tc.want {
				t.Fatalf("tokens = %v (err %v), want %d", got, err, tc.want)
			}
			if tc.want == 1 && got[0].ID != "lit" {
				t.Errorf("kept %q, want the token on explored land", got[0].ID)
			}
		})
	}
	t.Run("a failed fog lookup fails the list", func(t *testing.T) {
		repo, _ := fogWriteFixture()
		svc := NewDrawingService(repo)
		svc.SetHexFogLookup(fakeFogLookup{err: errors.New("down")})
		if _, err := svc.ListTokens(context.Background(), "map-1", permissions.RolePlayer); err == nil {
			t.Error("must fail closed")
		}
	})
}

func TestIsTokenHidden(t *testing.T) {
	dx, dy, lx, ly := fogPositions()
	_, svc := fogWriteFixture()
	for _, tc := range []struct {
		name string
		tok  Token
		role int
		want bool
	}{
		{"player, token in the dark", Token{MapID: "map-1", X: dx, Y: dy}, permissions.RolePlayer, true},
		{"scribe, token in the dark", Token{MapID: "map-1", X: dx, Y: dy}, permissions.RoleScribe, true},
		{"owner, token in the dark", Token{MapID: "map-1", X: dx, Y: dy}, permissions.RoleOwner, false},
		{"player, token on explored land", Token{MapID: "map-1", X: lx, Y: ly}, permissions.RolePlayer, false},
		{"player, hidden token on explored land", Token{MapID: "map-1", X: lx, Y: ly, IsHidden: true}, permissions.RolePlayer, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tok := tc.tok
			got, err := svc.IsTokenHidden(context.Background(), &tok, tc.role)
			if err != nil || got != tc.want {
				t.Errorf("got %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

// The drag event names its map so the publisher can test the new position
// against that map's fog.
func TestUpdateTokenPosition_EventNamesTheMap(t *testing.T) {
	repo, svc := fogWriteFixture()
	repo.tok.X, repo.tok.Y = 0, 0
	rec := &positionRecorder{}
	svc.SetEventPublisher(rec)
	if err := svc.UpdateTokenPosition(context.Background(), "t", "map-1", true, UpdateTokenPositionInput{X: patch.Of(float64(5)), Y: patch.Of(float64(5))}); err != nil {
		t.Fatal(err)
	}
	if rec.mapID != "map-1" {
		t.Errorf("event map = %q, want map-1", rec.mapID)
	}
}

func TestFogWithholdsMedia(t *testing.T) {
	img := "media-1"
	for _, tc := range []struct {
		name   string
		d      Drawing
		lookup fakeFogLookup
		media  string
		want   bool
		err    bool
	}{
		{"picture reaching into the dark", Drawing{ID: "p", DrawingType: DrawingTypeImage, ImageID: &img, Points: []byte(`[{"x":12,"y":6.5},{"x":40,"y":40}]`)}, fakeFogLookup{mask: fogMaskFixture()}, img, true, false},
		{"picture on explored land", Drawing{ID: "p", DrawingType: DrawingTypeImage, ImageID: &img, Points: []byte(`[{"x":12,"y":6.5},{"x":14,"y":8.5}]`)}, fakeFogLookup{mask: fogMaskFixture()}, img, false, false},
		{"another file", Drawing{ID: "p", DrawingType: DrawingTypeImage, ImageID: &img, Points: []byte(`[{"x":12,"y":6.5},{"x":40,"y":40}]`)}, fakeFogLookup{mask: fogMaskFixture()}, "media-2", false, false},
		{"no fog", Drawing{ID: "p", DrawingType: DrawingTypeImage, ImageID: &img, Points: []byte(`[{"x":12,"y":6.5},{"x":40,"y":40}]`)}, fakeFogLookup{}, img, false, false},
		{"lookup fails", Drawing{ID: "p", DrawingType: DrawingTypeImage, ImageID: &img}, fakeFogLookup{err: errors.New("down")}, img, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewDrawingService(&fogWriteRepo{d: tc.d})
			svc.SetHexFogLookup(tc.lookup)
			got, err := svc.FogWithholdsMedia(context.Background(), "map-1", tc.media)
			if got != tc.want || (err != nil) != tc.err {
				t.Errorf("got %v, %v; want %v, err=%v", got, err, tc.want, tc.err)
			}
		})
	}
}

// A picture write can change which files the fog withholds, so it drops the
// media guard's cached answers like a shadow write does.
func TestPictureWritesNotifyTheMediaGuard(t *testing.T) {
	for _, tc := range []struct {
		kind string
		want bool
	}{{DrawingTypeImage, true}, {DrawingTypeShadow, true}, {"freehand", false}} {
		t.Run(tc.kind, func(t *testing.T) {
			repo := &fogWriteRepo{d: Drawing{ID: "d", MapID: "map-1", DrawingType: tc.kind}}
			svc := NewDrawingService(repo).(*drawingService)
			called := false
			svc.SetShadowChangeHook(func(string) { called = true })
			if err := svc.DeleteDrawing(context.Background(), "d", "map-1", nil, "u", permissions.RoleOwner, true); err != nil {
				t.Fatal(err)
			}
			if called != tc.want {
				t.Errorf("hook called = %v, want %v", called, tc.want)
			}
		})
	}
}

// A scribe editing or deleting a pin in unexplored land gets the answer a
// missing pin gets; a DM is not limited.
func TestFogHiddenWrites_Markers(t *testing.T) {
	dx, dy, _, _ := fogPositions()
	creator := "u"
	stored := func() *Marker {
		return &Marker{ID: "k", MapID: "map-1", Name: "Lair", X: dx, Y: dy, Visibility: "everyone", CreatedBy: &creator}
	}
	for _, tc := range []struct {
		name  string
		isDM  bool
		write func(MapService) error
	}{
		{"scribe updates", false, func(s MapService) error {
			return s.UpdateMarker(context.Background(), "k", UpdateMarkerInput{}, false)
		}},
		{"scribe deletes their own", false, func(s MapService) error {
			return s.DeleteMarker(context.Background(), "k", nil, false, "u", permissions.RoleScribe)
		}},
		{"DM updates", true, func(s MapService) error {
			return s.UpdateMarker(context.Background(), "k", UpdateMarkerInput{}, true)
		}},
		{"DM deletes", true, func(s MapService) error {
			return s.DeleteMarker(context.Background(), "k", nil, true, "o", permissions.RoleOwner)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wrote := false
			repo := &mockMapRepo{
				getMarkerFn:    func(context.Context, string) (*Marker, error) { return stored(), nil },
				updateMarkerFn: func(context.Context, *Marker) error { wrote = true; return nil },
				deleteMarkerFn: func(context.Context, string) error { wrote = true; return nil },
			}
			s := NewMapService(repo)
			s.SetHexFogLookup(fakeFogLookup{mask: fogMaskFixture()})
			err := tc.write(s)
			if tc.isDM {
				if err != nil || !wrote {
					t.Fatalf("DM write: err=%v wrote=%v", err, wrote)
				}
				return
			}
			assertAppError(t, err, http.StatusNotFound)
			if wrote {
				t.Error("a refused write reached the repository")
			}
		})
	}
}
