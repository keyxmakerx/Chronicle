// token_position_partial_update_test.go pins the partial-update contract on
// the token move endpoints: a body naming one axis moves the token along that
// axis and leaves the other where it was, rather than snapping it to 0.
package maps

import (
	"context"
	"encoding/json"
	"testing"
)

type tokenMoveRecorder struct {
	mockDrawingRepo
	x, y  float64
	wrote bool
}

func (r *tokenMoveRecorder) UpdateTokenPosition(_ context.Context, _ string, x, y float64) error {
	r.x, r.y, r.wrote = x, y, true
	return nil
}

func TestUpdateTokenPosition_PartialBody(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantX, wantY float64
		wantErr      bool
	}{
		{"both axes", `{"x":10,"y":20}`, 10, 20, false},
		{"x only keeps y", `{"x":10}`, 10, 40, false},
		{"y only keeps x", `{"y":20}`, 30, 20, false},
		{"explicit zero is a value", `{"x":0,"y":0}`, 0, 0, false},
		{"null keeps the stored axis", `{"x":null,"y":5}`, 30, 5, false},
		{"empty body moves nothing", `{}`, 30, 40, false},
		{"out of range is refused before any write", `{"x":101}`, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &tokenMoveRecorder{}
			repo.getTokenFn = func(_ context.Context, id string) (*Token, error) {
				return &Token{ID: id, MapID: "map-1", X: 30, Y: 40}, nil
			}
			var in UpdateTokenPositionInput
			if err := json.Unmarshal([]byte(tt.body), &in); err != nil {
				t.Fatalf("bad test body: %v", err)
			}
			err := NewDrawingService(repo).UpdateTokenPosition(context.Background(), "tok-1", "map-1", true, in)
			if tt.wantErr {
				if err == nil || repo.wrote {
					t.Fatalf("want a refusal with no write, got err=%v wrote=%v", err, repo.wrote)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.x != tt.wantX || repo.y != tt.wantY {
				t.Errorf("wrote (%v,%v), want (%v,%v)", repo.x, repo.y, tt.wantX, tt.wantY)
			}
		})
	}
}
