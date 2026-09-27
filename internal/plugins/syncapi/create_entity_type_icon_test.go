// create_entity_type_icon_test.go pins that POST .../entity-types accepts the
// style-prefixed icon the Foundry module's import wizard sends and stores the
// bare name, through the real entities service.
package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// captureTypeRepo answers the calls CreateEntityType makes and records the
// row it would insert.
type captureTypeRepo struct {
	entities.EntityTypeRepository
	created *entities.EntityType
}

func (r *captureTypeRepo) SlugExists(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *captureTypeRepo) MaxSortOrder(context.Context, string) (int, error) { return 0, nil }

func (r *captureTypeRepo) Create(_ context.Context, et *entities.EntityType) error {
	et.ID = 7
	r.created = et
	return nil
}

func TestCreateEntityType_IconNormalised(t *testing.T) {
	tests := []struct {
		name, body string
		wantStatus int
		wantIcon   string
	}{
		{"import wizard body", `{"name":"Ships","icon":"fa-solid fa-circle"}`, http.StatusCreated, "fa-circle"},
		{"fas prefix", `{"name":"Ships","icon":"fas fa-ship"}`, http.StatusCreated, "fa-ship"},
		{"bare name", `{"name":"Ships","icon":"fa-ship"}`, http.StatusCreated, "fa-ship"},
		{"no icon", `{"name":"Ships"}`, http.StatusCreated, "fa-circle"},
		{"brand icon refused", `{"name":"Ships","icon":"fa-brands fa-github"}`, http.StatusBadRequest, ""},
		{"extra class refused", `{"name":"Ships","icon":"fa-solid fa-circle extra"}`, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &captureTypeRepo{}
			h := NewAPIHandler(nil, entities.NewEntityService(nil, repo, nil), nil, nil)

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-1/entity-types", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues("camp-1")

			err := h.CreateEntityType(c)
			if tt.wantStatus != http.StatusCreated {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != tt.wantStatus {
					t.Fatalf("CreateEntityType error = %v, want status %d", err, tt.wantStatus)
				}
				if repo.created != nil {
					t.Error("a refused icon must not reach the repository")
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateEntityType: %v", err)
			}
			if repo.created == nil || repo.created.Icon != tt.wantIcon {
				t.Fatalf("stored entity type = %+v, want icon %q", repo.created, tt.wantIcon)
			}
			var resp struct {
				Icon string `json:"icon"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decoding response: %v", err)
			}
			if rec.Code != tt.wantStatus || resp.Icon != tt.wantIcon {
				t.Errorf("response = %d %q, want %d %q", rec.Code, resp.Icon, tt.wantStatus, tt.wantIcon)
			}
		})
	}
}
