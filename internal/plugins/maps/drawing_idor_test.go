// drawing_idor_test.go — cross-map write guard for drawings, tokens, and layers
// (audit-R2 Finding 2, IDOR). A child object that belongs to map B must never be
// mutated through a request scoped to map A: the service returns NotFound (not
// Forbidden, so existence isn't leaked) and never reaches the repo write.
package maps

import (
	"context"
	"net/http"
	"testing"
)

// idorRepo is a minimal DrawingRepository that hands back objects pinned to
// "map-B" and records whether any mutation reached the persistence layer.
type idorRepo struct {
	mutated bool
}

func (r *idorRepo) GetDrawing(_ context.Context, id string) (*Drawing, error) {
	return &Drawing{ID: id, MapID: "map-B"}, nil
}
func (r *idorRepo) GetToken(_ context.Context, id string) (*Token, error) {
	return &Token{ID: id, MapID: "map-B"}, nil
}
func (r *idorRepo) GetLayer(_ context.Context, id string) (*Layer, error) {
	return &Layer{ID: id, MapID: "map-B"}, nil
}

func (r *idorRepo) UpdateDrawing(context.Context, *Drawing) error { r.mutated = true; return nil }
func (r *idorRepo) DeleteDrawing(context.Context, string) error   { r.mutated = true; return nil }
func (r *idorRepo) UpdateToken(context.Context, *Token) error     { r.mutated = true; return nil }
func (r *idorRepo) UpdateTokenPosition(context.Context, string, float64, float64) error {
	r.mutated = true
	return nil
}
func (r *idorRepo) DeleteToken(context.Context, string) error { r.mutated = true; return nil }
func (r *idorRepo) UpdateLayer(context.Context, *Layer) error { r.mutated = true; return nil }
func (r *idorRepo) DeleteLayer(context.Context, string) error { r.mutated = true; return nil }

// Unused-by-these-tests methods round out the interface.
func (r *idorRepo) CreateDrawing(context.Context, *Drawing) error                { return nil }
func (r *idorRepo) ListDrawings(context.Context, string, int) ([]Drawing, error) { return nil, nil }
func (r *idorRepo) CreateToken(context.Context, *Token) error                    { return nil }
func (r *idorRepo) ListTokens(context.Context, string, int) ([]Token, error)     { return nil, nil }
func (r *idorRepo) CreateLayer(context.Context, *Layer) error                    { return nil }
func (r *idorRepo) ListLayers(context.Context, string) ([]Layer, error)          { return nil, nil }
func (r *idorRepo) CreateFog(context.Context, *FogRegion) error                  { return nil }
func (r *idorRepo) GetFog(context.Context, string) (*FogRegion, error)           { return &FogRegion{}, nil }
func (r *idorRepo) DeleteFog(context.Context, string) error                      { return nil }
func (r *idorRepo) ListFog(context.Context, string) ([]FogRegion, error)         { return nil, nil }
func (r *idorRepo) ResetFog(context.Context, string) error                       { return nil }

// TestDrawingWrites_CrossMapRejected: every child-object write scoped to the
// wrong map (the object lives in "map-B", the request carries "map-A") must be
// rejected with 404 and must not mutate the repo.
func TestDrawingWrites_CrossMapRejected(t *testing.T) {
	const wrongMap = "map-A" // objects belong to "map-B"

	cases := []struct {
		name string
		call func(svc DrawingService) error
	}{
		{"UpdateDrawing", func(s DrawingService) error {
			return s.UpdateDrawing(context.Background(), "d-1", wrongMap, UpdateDrawingInput{})
		}},
		{"DeleteDrawing", func(s DrawingService) error {
			return s.DeleteDrawing(context.Background(), "d-1", wrongMap, nil)
		}},
		{"UpdateToken", func(s DrawingService) error {
			return s.UpdateToken(context.Background(), "t-1", wrongMap, UpdateTokenInput{})
		}},
		{"UpdateTokenPosition", func(s DrawingService) error {
			return s.UpdateTokenPosition(context.Background(), "t-1", wrongMap, UpdateTokenPositionInput{X: 10, Y: 10})
		}},
		{"DeleteToken", func(s DrawingService) error {
			return s.DeleteToken(context.Background(), "t-1", wrongMap, nil)
		}},
		{"UpdateLayer", func(s DrawingService) error {
			return s.UpdateLayer(context.Background(), "l-1", wrongMap, UpdateLayerInput{})
		}},
		{"DeleteLayer", func(s DrawingService) error {
			return s.DeleteLayer(context.Background(), "l-1", wrongMap, nil)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &idorRepo{}
			svc := NewDrawingService(repo)
			err := tc.call(svc)
			assertAppError(t, err, http.StatusNotFound)
			if repo.mutated {
				t.Errorf("%s: cross-map write must short-circuit before the repo mutation", tc.name)
			}
		})
	}
}

// TestDrawingWrites_SameMapAllowed: the happy path — when the request's mapID
// matches the object's map ("map-B"), the write reaches the repo. This is the
// regression guard so the new IDOR check doesn't block legitimate edits.
func TestDrawingWrites_SameMapAllowed(t *testing.T) {
	const rightMap = "map-B" // objects belong to "map-B"

	cases := []struct {
		name string
		call func(svc DrawingService) error
	}{
		{"UpdateDrawing", func(s DrawingService) error {
			return s.UpdateDrawing(context.Background(), "d-1", rightMap, UpdateDrawingInput{})
		}},
		{"DeleteDrawing", func(s DrawingService) error {
			return s.DeleteDrawing(context.Background(), "d-1", rightMap, nil)
		}},
		{"UpdateToken", func(s DrawingService) error {
			return s.UpdateToken(context.Background(), "t-1", rightMap, UpdateTokenInput{})
		}},
		{"UpdateTokenPosition", func(s DrawingService) error {
			return s.UpdateTokenPosition(context.Background(), "t-1", rightMap, UpdateTokenPositionInput{X: 10, Y: 10})
		}},
		{"DeleteToken", func(s DrawingService) error {
			return s.DeleteToken(context.Background(), "t-1", rightMap, nil)
		}},
		{"UpdateLayer", func(s DrawingService) error {
			return s.UpdateLayer(context.Background(), "l-1", rightMap, UpdateLayerInput{})
		}},
		{"DeleteLayer", func(s DrawingService) error {
			return s.DeleteLayer(context.Background(), "l-1", rightMap, nil)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &idorRepo{}
			svc := NewDrawingService(repo)
			if err := tc.call(svc); err != nil {
				t.Fatalf("%s: same-map write should succeed, got: %v", tc.name, err)
			}
			if !repo.mutated {
				t.Errorf("%s: same-map write should have reached the repo", tc.name)
			}
		})
	}
}
