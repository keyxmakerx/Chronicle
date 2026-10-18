package syncapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// createFailRepo fails the insert with a canned error; the lookups find nothing.
type createFailRepo struct {
	SyncMappingRepository
	createErr error
}

func (r createFailRepo) FindByChronicle(context.Context, string, string, string, string) (*SyncMapping, error) {
	return nil, nil
}
func (r createFailRepo) Create(context.Context, *SyncMapping) error { return r.createErr }

const validMappingBody = `{"chronicle_type":"entity","chronicle_id":"e1","external_system":"foundry","external_id":"x1"}`

// A duplicate-key insert must surface as 409 "already exists" (the Foundry
// module matches on that wording), and nothing internal may reach the client.
func TestCreateMapping_ErrorStatuses(t *testing.T) {
	tests := []struct {
		name     string
		repoErr  error
		wantCode int
		wantMsg  string
		leakNot  string
	}{
		{"duplicate key is a conflict", apperror.NewInternal(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'k' for key 'uq_sync_mapping'"}), http.StatusConflict, "already exists", "uq_sync_mapping"},
		{"other insert failure stays internal", apperror.NewInternal(errors.New("dial tcp 10.0.0.5: refused")), http.StatusInternalServerError, "", "10.0.0.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewSyncMappingService(createFailRepo{createErr: tt.repoErr})
			h := NewSyncHandler(svc)
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(validMappingBody))
			req.Header.Set("Content-Type", "application/json")
			c := echo.New().NewContext(req, httptest.NewRecorder())
			c.SetParamNames("id")
			c.SetParamValues("camp-1")

			err := h.CreateMapping(c)
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != tt.wantCode {
				t.Fatalf("err = %v, want app error %d", err, tt.wantCode)
			}
			if !strings.Contains(ae.Message, tt.wantMsg) || strings.Contains(ae.Message, tt.leakNot) {
				t.Fatalf("message = %q", ae.Message)
			}
		})
	}
}
