package sessions

import (
	"context"
	"errors"
	"testing"
)

type fakeAddonStore struct {
	records map[string]bool
	enabled []string
}

func (f *fakeAddonStore) HasCampaignAddonRecord(_ context.Context, id, slug string) (bool, error) {
	if slug != SessionsAddonSlug {
		return false, errors.New("wrong addon")
	}
	return f.records[id], nil
}

func (f *fakeAddonStore) EnableForCampaignBySlug(_ context.Context, id, slug, userID string) error {
	if slug != SessionsAddonSlug || userID != "" {
		return errors.New("unexpected enable")
	}
	f.enabled = append(f.enabled, id)
	return nil
}

func TestReconcileAddonEnablement(t *testing.T) {
	tests := []struct {
		name    string
		using   []string
		records map[string]bool
		listErr error
		want    []string
		wantErr bool
	}{
		{"never decided: switched on", []string{"c1", "c2"}, nil, nil, []string{"c1", "c2"}, false},
		{"an owner's choice is kept", []string{"on", "off", "new"}, map[string]bool{"on": true, "off": true}, nil, []string{"new"}, false},
		{"blank ids skipped", []string{""}, nil, nil, nil, false},
		{"list fails", nil, nil, errors.New("db down"), nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockSessionRepo{listCampaignIDsUsingGameNightsFn: func(context.Context) ([]string, error) { return tt.using, tt.listErr }}
			store := &fakeAddonStore{records: tt.records}
			n, err := ReconcileAddonEnablement(context.Background(), NewSessionService(repo, nil, nil), store)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if n != len(tt.want) || len(store.enabled) != len(tt.want) {
				t.Fatalf("enabled %v (n=%d), want %v", store.enabled, n, tt.want)
			}
			for i := range tt.want {
				if store.enabled[i] != tt.want[i] {
					t.Errorf("enabled %v, want %v", store.enabled, tt.want)
				}
			}
		})
	}
}
