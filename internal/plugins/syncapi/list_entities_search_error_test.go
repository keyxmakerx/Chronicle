package syncapi

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// stubSearchErr answers every search with a fixed error.
type stubSearchErr struct {
	stubEntityServiceForGM
	err error
}

func (s *stubSearchErr) Search(_ context.Context, _, _ string, _ int, _ int, _ string, _ entities.ListOptions) ([]entities.Entity, int, error) {
	return nil, 0, s.err
}

// A rejected search query is the caller's mistake and keeps its 4xx; any
// other failure still reads as a 500 without leaking its cause.
func TestListEntities_SearchErrorStatus(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"short query keeps its 400", apperror.NewBadRequest("search query must be at least 2 characters"), http.StatusBadRequest},
		{"database fault stays 500", errors.New("dial tcp: connection refused"), http.StatusInternalServerError},
		{"internal app error stays 500", apperror.NewInternal(errors.New("boom")), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewAPIHandler(nil, &stubSearchErr{err: tc.err}, &stubCampaignSvcForGM{role: campaigns.RoleOwner}, nil)
			c, _ := gmContext(http.MethodGet, "/api/v1/campaigns/camp-1/entities?q=a", "", false)

			err := h.ListEntities(c)
			var appErr *apperror.AppError
			if !errors.As(err, &appErr) || appErr.Code != tc.wantCode {
				t.Fatalf("got %v, want an AppError with code %d", err, tc.wantCode)
			}
			if tc.wantCode == http.StatusInternalServerError && appErr.Message == "dial tcp: connection refused" {
				t.Errorf("internal cause leaked to the caller: %q", appErr.Message)
			}
		})
	}
}
