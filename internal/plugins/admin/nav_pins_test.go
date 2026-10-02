package admin

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// nav_pins_test.go pins an admin's own sidebar pins: only listed pages can be
// pinned (never Home), repeats collapse, the list is capped, unknown stored
// links are ignored on read, and only the caller's own row is written.

var testPinnable = []string{"/admin/users", "/admin/campaigns", "/admin/storage", "/admin/smtp", "/admin/backup", "/admin/systems", "/admin/database"}

func testAllowed() map[string]bool {
	m := map[string]bool{}
	for _, h := range testPinnable {
		m[h] = true
	}
	return m
}

func TestNormalizeAdminNavPins(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		want    []string
		wantErr bool
	}{
		{name: "keeps order", in: []string{"/admin/storage", "/admin/users"}, want: []string{"/admin/storage", "/admin/users"}},
		{name: "empty clears", in: nil, want: []string{}},
		{name: "duplicates collapse to the first", in: []string{"/admin/users", "/admin/storage", "/admin/users"}, want: []string{"/admin/users", "/admin/storage"}},
		{name: "unknown link refused", in: []string{"/admin/users", "/admin/nope"}, wantErr: true},
		{name: "home refused", in: []string{"/admin"}, wantErr: true},
		{name: "off-site link refused", in: []string{"https://example.com/admin/users"}, wantErr: true},
		{name: "exactly the cap is fine", in: testPinnable[:MaxAdminNavPins], want: testPinnable[:MaxAdminNavPins]},
		{name: "over the cap refused", in: testPinnable[:MaxAdminNavPins+1], wantErr: true},
		{name: "repeats do not count toward the cap", in: append(append([]string{}, testPinnable[:MaxAdminNavPins]...), "/admin/users"), want: testPinnable[:MaxAdminNavPins]},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeAdminNavPins(tc.in, testAllowed())
			if tc.wantErr {
				var ae *apperror.AppError
				if err == nil || !errors.As(err, &ae) || ae.Code != 400 {
					t.Fatalf("want a 400 error, got %v", err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestDropUnknownAdminNavPins(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"renamed page dropped", []string{"/admin/users", "/admin/old-name", "/admin/storage"}, []string{"/admin/users", "/admin/storage"}},
		{"home dropped", []string{"/admin"}, []string{}},
		{"repeats dropped", []string{"/admin/users", "/admin/users"}, []string{"/admin/users"}},
		{"nil", nil, []string{}},
		{"capped", append(append([]string{}, testPinnable...), "/admin/users"), testPinnable[:MaxAdminNavPins]},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := dropUnknownAdminNavPins(tc.in, testAllowed()); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// fakePinRepo records what is stored per user.
type fakePinRepo struct {
	store  map[string][]string
	getErr error
	sets   int
}

func (f *fakePinRepo) GetAdminNavPins(_ context.Context, userID string) ([]string, error) {
	return f.store[userID], f.getErr
}

func (f *fakePinRepo) SetAdminNavPins(_ context.Context, userID string, pins []string) error {
	f.sets++
	if f.store == nil {
		f.store = map[string][]string{}
	}
	f.store[userID] = pins
	return nil
}

func TestAdminNavPinService(t *testing.T) {
	ctx := context.Background()

	t.Run("update stores for the caller only", func(t *testing.T) {
		repo := &fakePinRepo{store: map[string][]string{"other": {"/admin/users"}}}
		svc := NewAdminNavPinService(repo, testPinnable)
		got, err := svc.UpdatePins(ctx, "me", []string{"/admin/storage", "/admin/storage"})
		if err != nil || !reflect.DeepEqual(got, []string{"/admin/storage"}) {
			t.Fatalf("UpdatePins = %v, %v", got, err)
		}
		if !reflect.DeepEqual(repo.store["other"], []string{"/admin/users"}) {
			t.Errorf("another admin's pins changed: %v", repo.store["other"])
		}
		if !reflect.DeepEqual(repo.store["me"], []string{"/admin/storage"}) {
			t.Errorf("caller's pins = %v", repo.store["me"])
		}
	})

	t.Run("invalid list writes nothing", func(t *testing.T) {
		repo := &fakePinRepo{}
		svc := NewAdminNavPinService(repo, testPinnable)
		if _, err := svc.UpdatePins(ctx, "me", []string{"/admin"}); err == nil {
			t.Fatal("Home must be refused")
		}
		if repo.sets != 0 {
			t.Errorf("a refused list wrote %d times", repo.sets)
		}
	})

	t.Run("no user is refused", func(t *testing.T) {
		svc := NewAdminNavPinService(&fakePinRepo{}, testPinnable)
		if _, err := svc.UpdatePins(ctx, "", nil); err == nil {
			t.Fatal("an empty user ID must be refused")
		}
	})

	t.Run("read drops unknown stored links", func(t *testing.T) {
		repo := &fakePinRepo{store: map[string][]string{"me": {"/admin/gone", "/admin/smtp"}}}
		svc := NewAdminNavPinService(repo, testPinnable)
		got, err := svc.Pins(ctx, "me")
		if err != nil || !reflect.DeepEqual(got, []string{"/admin/smtp"}) {
			t.Fatalf("Pins = %v, %v", got, err)
		}
	})

	t.Run("read failure is an error, not an empty list", func(t *testing.T) {
		svc := NewAdminNavPinService(&fakePinRepo{getErr: errors.New("db down")}, testPinnable)
		if _, err := svc.Pins(ctx, "me"); err == nil {
			t.Fatal("a failed read must be reported")
		}
	})
}
