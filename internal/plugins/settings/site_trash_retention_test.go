package settings

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// TestSiteTrashRetentionDays pins that only an unset setting means the
// default; a read failure or an unusable value is an error, because the purger
// deletes for good and a guessed 30 could be shorter than the configured 90.
func TestSiteTrashRetentionDays(t *testing.T) {
	tests := []struct {
		name    string
		get     func(context.Context, string) (string, error)
		want    int
		wantErr bool
	}{
		{"unset is the default", func(context.Context, string) (string, error) {
			return "", apperror.NewNotFound("setting not found")
		}, DefaultSiteTrashRetentionDays, false},
		{"configured 90", func(context.Context, string) (string, error) { return "90", nil }, 90, false},
		{"configured 7 with spaces", func(context.Context, string) (string, error) { return " 7 ", nil }, 7, false},
		{"a transient read error is an error", func(context.Context, string) (string, error) {
			return "", errors.New("connection reset")
		}, 0, true},
		{"an internal app error is an error", func(context.Context, string) (string, error) {
			return "", apperror.NewInternal(errors.New("db"))
		}, 0, true},
		{"not a number is an error", func(context.Context, string) (string, error) { return "soon", nil }, 0, true},
		{"not an offered choice is an error", func(context.Context, string) (string, error) { return "3", nil }, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewSettingsService(&mockSettingsRepo{getFn: tc.get})
			got, err := svc.SiteTrashRetentionDays(context.Background())
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("SiteTrashRetentionDays = %d, %v; want %d, err %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
