package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// fakeLookCampaigns moves campaigns whose look is unset; adopted records it.
type fakeLookCampaigns struct {
	fakeExtrasCampaigns
	look    map[string]string // campaign id -> stored page look
	failing map[string]bool
	calls   int
}

func (f *fakeLookCampaigns) AdoptMaterialPageLook(_ context.Context, id string) (bool, error) {
	f.calls++
	if f.failing[id] {
		return false, errors.New("db down")
	}
	if f.look[id] != "" {
		return false, nil
	}
	f.look[id] = "annals"
	return true, nil
}

func TestMaterialPageLookOnce(t *testing.T) {
	many := make([]string, 0, 230)
	for i := 0; i < 230; i++ {
		many = append(many, fmt.Sprintf("c%d", i))
	}
	tests := []struct {
		name        string
		ids         []string
		look        map[string]string
		failing     map[string]bool
		ran         bool
		wantChanged int
		wantErr     bool
		wantMarked  bool
	}{
		{name: "moves the unset and keeps the chosen", ids: []string{"a", "b"}, look: map[string]string{"b": "annals"}, wantChanged: 1, wantMarked: true},
		{name: "every page of campaigns", ids: many, look: map[string]string{}, wantChanged: 230, wantMarked: true},
		{name: "already ran", ids: []string{"a"}, look: map[string]string{}, ran: true},
		{name: "a failure is reported and not retried", ids: []string{"a", "b"}, look: map[string]string{}, failing: map[string]bool{"a": true}, wantChanged: 1, wantErr: true, wantMarked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeExtrasSettings{vals: map[string]string{}}
			if tt.ran {
				st.vals[materialPageLookKey] = "1"
			}
			camps := &fakeLookCampaigns{fakeExtrasCampaigns: fakeExtrasCampaigns{ids: tt.ids}, look: tt.look, failing: tt.failing}
			n, err := materialPageLookOnce(context.Background(), st, camps)
			if (err != nil) != tt.wantErr || n != tt.wantChanged {
				t.Fatalf("got %d, %v; want %d, err %v", n, err, tt.wantChanged, tt.wantErr)
			}
			if tt.ran && camps.calls != 0 {
				t.Errorf("ran again: %d calls", camps.calls)
			}
			if got := st.vals[materialPageLookKey] == "1"; got != tt.wantMarked && !tt.ran {
				t.Errorf("marked = %v, want %v", got, tt.wantMarked)
			}
		})
	}
}
