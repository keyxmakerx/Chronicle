package entities

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The Add Adventures button answers 201 the first time and 200 after, and the
// second press creates nothing.
func TestEnsureAdventuresAPI_IsIdempotent(t *testing.T) {
	var stored []EntityType
	typeRepo := &mockEntityTypeRepo{
		listByPresetCategoryFn: func(context.Context, string, string) ([]EntityType, error) { return stored, nil },
		createFn: func(_ context.Context, et *EntityType) error {
			et.ID = 77
			stored = append(stored, *et)
			return nil
		},
	}
	h := &Handler{service: NewEntityService(&mockEntityRepo{}, typeRepo, &mockPermissionRepo{})}

	for i, want := range []int{http.StatusCreated, http.StatusOK} {
		c, rec := advCtx(campaigns.RoleOwner, false, http.MethodPost, "/campaigns/c1/entity-types/adventures")
		c.SetParamNames("id")
		c.SetParamValues("c1")
		if err := h.EnsureAdventuresAPI(c); err != nil {
			t.Fatalf("press %d: %v", i+1, err)
		}
		if rec.Code != want {
			t.Errorf("press %d: status %d, want %d", i+1, rec.Code, want)
		}
		var body struct {
			ID      int  `json:"id"`
			Created bool `json:"created"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.ID != 77 || body.Created != (i == 0) {
			t.Errorf("press %d: body %+v", i+1, body)
		}
	}
	if len(stored) != 1 {
		t.Errorf("created %d categories, want 1", len(stored))
	}
}
