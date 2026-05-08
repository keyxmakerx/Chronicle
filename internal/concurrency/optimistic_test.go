package concurrency

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestCheck(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	older := now.Add(-time.Minute)
	newer := now.Add(time.Minute)

	tests := []struct {
		name     string
		current  time.Time
		expected *time.Time
		wantErr  bool
	}{
		{"nil expected accepts any current", now, nil, false},
		{"equal timestamps accept", now, &now, false},
		{"current older than expected accepts (clock skew safe)", older, &now, false},
		{"current newer than expected rejects", newer, &now, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Check(tt.current, tt.expected, "thing")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected conflict error, got nil")
				}
				var appErr *apperror.AppError
				if !errors.As(err, &appErr) {
					t.Fatalf("expected AppError, got %T: %v", err, err)
				}
				if appErr.Code != http.StatusConflict {
					t.Errorf("expected 409, got %d", appErr.Code)
				}
				if appErr.Type != "conflict" {
					t.Errorf("expected type 'conflict', got %q", appErr.Type)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
