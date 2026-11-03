package settings

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestLongTermTrashMonths(t *testing.T) {
	tests := []struct {
		name string
		get  func(context.Context, string) (string, error)
		want int
	}{
		{"unset is the default", func(context.Context, string) (string, error) {
			return "", apperror.NewNotFound("setting not found")
		}, DefaultLongTermTrashMonths},
		{"off", func(context.Context, string) (string, error) { return "0", nil }, 0},
		{"a year", func(context.Context, string) (string, error) { return " 12 ", nil }, 12},
		{"a read error is the default", func(context.Context, string) (string, error) { return "", errors.New("reset") }, DefaultLongTermTrashMonths},
		{"not an offered choice is the default", func(context.Context, string) (string, error) { return "7", nil }, DefaultLongTermTrashMonths},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewSettingsService(&mockSettingsRepo{getFn: tc.get})
			if got := svc.LongTermTrashMonths(context.Background()); got != tc.want {
				t.Errorf("LongTermTrashMonths = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestUpdateLongTermTrashMonths(t *testing.T) {
	for _, m := range []int{0, 3, 6, 12, 24} {
		var saved string
		svc := NewSettingsService(&mockSettingsRepo{setFn: func(_ context.Context, key, v string) error {
			if key == KeyLongTermTrashMonths {
				saved = v
			}
			return nil
		}})
		if err := svc.UpdateLongTermTrashMonths(context.Background(), m); err != nil {
			t.Fatalf("%d months: %v", m, err)
		}
		if saved == "" {
			t.Errorf("%d months was not saved", m)
		}
	}
	svc := NewSettingsService(&mockSettingsRepo{})
	for _, m := range []int{-1, 1, 5, 36} {
		if err := svc.UpdateLongTermTrashMonths(context.Background(), m); err == nil {
			t.Errorf("%d months must be refused", m)
		}
	}
}
