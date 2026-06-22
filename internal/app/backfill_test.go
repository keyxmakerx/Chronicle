package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fakeBackfillAddons records the slug it was queried with and returns canned IDs.
type fakeBackfillAddons struct {
	ids     []string
	err     error
	gotSlug string
}

func (f *fakeBackfillAddons) ListCampaignsUsingAddon(_ context.Context, slug string) ([]string, error) {
	f.gotSlug = slug
	return f.ids, f.err
}

// fakeBackfillEntities records which campaigns it ensured and can fail per-campaign.
type fakeBackfillEntities struct {
	called []string
	failOn map[string]error
}

func (f *fakeBackfillEntities) EnsurePlayerCharacterType(_ context.Context, campaignID string) error {
	f.called = append(f.called, campaignID)
	if err, ok := f.failOn[campaignID]; ok {
		return err
	}
	return nil
}

func TestBackfillPlayerCharacterTypes(t *testing.T) {
	tests := []struct {
		name          string
		ids           []string
		listErr       error
		failOn        map[string]error
		wantProcessed int
		wantErr       bool
		wantEnsured   []string // campaigns EnsurePlayerCharacterType was invoked for
	}{
		{
			name:          "all campaigns ensured",
			ids:           []string{"c1", "c2", "c3"},
			wantProcessed: 3,
			wantEnsured:   []string{"c1", "c2", "c3"},
		},
		{
			name:          "no enabled campaigns is a clean no-op",
			ids:           nil,
			wantProcessed: 0,
			wantEnsured:   nil,
		},
		{
			name:          "list error aborts before ensuring anything",
			listErr:       errors.New("db down"),
			wantProcessed: 0,
			wantErr:       true,
			wantEnsured:   nil,
		},
		{
			name:          "a per-campaign failure is skipped, the rest continue",
			ids:           []string{"c1", "bad", "c3"},
			failOn:        map[string]error{"bad": errors.New("boom")},
			wantProcessed: 2,
			wantEnsured:   []string{"c1", "bad", "c3"}, // attempted for all, counted for 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addonSvc := &fakeBackfillAddons{ids: tt.ids, err: tt.listErr}
			entitySvc := &fakeBackfillEntities{failOn: tt.failOn}

			got, err := backfillPlayerCharacterTypes(context.Background(), addonSvc, entitySvc)

			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if got != tt.wantProcessed {
				t.Errorf("processed = %d, want %d", got, tt.wantProcessed)
			}
			if !tt.wantErr && addonSvc.gotSlug != entities.AddonPlayerCharacterClaiming {
				t.Errorf("queried slug = %q, want %q", addonSvc.gotSlug, entities.AddonPlayerCharacterClaiming)
			}
			if !equalStrings(entitySvc.called, tt.wantEnsured) {
				t.Errorf("ensured campaigns = %v, want %v", entitySvc.called, tt.wantEnsured)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
