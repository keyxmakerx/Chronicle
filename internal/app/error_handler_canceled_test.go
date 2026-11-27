package app

// A request the browser abandoned is not a server error: it must not reach
// the error ring the diagnostics read (and so not update health either),
// while a real failure still does.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/observability"
)

func TestErrorHandler_AbandonedRequestIsNotAServerError(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		cancel  bool
		counted bool
	}{
		{"a database read cut off", fmt.Errorf("search encounters: %w", context.Canceled), false, false},
		{"wrapped in an internal AppError", apperror.NewInternal(fmt.Errorf("querying campaign by id: %w", context.Canceled)), false, false},
		{"any error once the client has gone", errors.New("write: broken pipe"), true, false},
		{"a real failure", errors.New("boom"), false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/encounter-run", nil).WithContext(ctx)
			c := e.NewContext(req, httptest.NewRecorder())
			before := observability.Recent(1).Total
			(&App{}).errorHandler(c, tc.err)
			if got := observability.Recent(1).Total - before; (got == 1) != tc.counted {
				t.Errorf("recorded %d server errors, want counted=%v", got, tc.counted)
			}
		})
	}
}
