package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// permsEntitySvc returns the entity as stored after the save, so the handler's
// re-read is what supplies updated_at.
type permsEntitySvc struct {
	entities.EntityService
	e *entities.Entity
}

func (s *permsEntitySvc) GetByID(context.Context, string) (*entities.Entity, error) {
	return s.e, nil
}

func (s *permsEntitySvc) SetEntityPermissions(context.Context, string, entities.SetPermissionsInput) error {
	return nil
}

// TestSetEntityPermissions_ReturnsStampedUpdatedAt pins that the response
// keeps its "status":"ok" and adds the version the save stamped.
func TestSetEntityPermissions_ReturnsStampedUpdatedAt(t *testing.T) {
	stamped := time.Date(2026, 3, 4, 5, 6, 7, 123000000, time.UTC)
	campSvc := &stubCampaignSvcForRole{getMemberFn: memberWithRole(campaigns.RoleOwner)}
	h := NewAPIHandler(&stubSyncSvcForRole{}, &permsEntitySvc{e: &entities.Entity{ID: "ent-1", CampaignID: "camp-1", UpdatedAt: stamped}}, campSvc, nil)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/campaigns/camp-1/entities/ent-1/permissions",
		strings.NewReader(`{"visibility":"default","is_private":false}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "entityID", Value: "ent-1"}})
	c.Set(apiKeyContextKey, &APIKey{ID: 1, CampaignID: "camp-1", UserID: "owner-1"})

	if err := h.SetEntityPermissions(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "ok" {
		t.Fatalf("status = %q, want ok", got["status"])
	}
	parsed, err := time.Parse(time.RFC3339Nano, got["updated_at"])
	if err != nil || !parsed.Equal(stamped) {
		t.Fatalf("updated_at = %q (%v), want %v", got["updated_at"], err, stamped)
	}
}
