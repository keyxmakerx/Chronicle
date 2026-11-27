package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

type fakeReturnMembers struct {
	member map[string]bool
	names  map[string]string
}

func (f fakeReturnMembers) GetMember(_ context.Context, cid, uid string) (*campaigns.CampaignMember, error) {
	if f.member[cid+"/"+uid] {
		return &campaigns.CampaignMember{}, nil
	}
	return nil, errors.New("not a member")
}

func (f fakeReturnMembers) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	if n, ok := f.names[id]; ok {
		return &campaigns.Campaign{ID: id, Name: n}, nil
	}
	return nil, errors.New("no campaign")
}

func TestReturnCampaign(t *testing.T) {
	svc := fakeReturnMembers{
		member: map[string]bool{"c2/u1": true},
		names:  map[string]string{"c2": "Saltreach", "c3": "Left Behind"},
	}
	listed := []layouts.NavCampaign{{ID: "c1", Name: "Therin"}}
	tests := []struct {
		name, id, user, want string
	}{
		{"in the header's list", "c1", "u1", "Therin"},
		{"a member, read again", "c2", "u1", "Saltreach"},
		{"no longer a member", "c3", "u1", ""},
		{"no cookie", "", "u1", ""},
		{"signed out", "c2", "", ""},
		{"an oversized value", string(make([]byte, 65)), "u1", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ""
			if rc := returnCampaign(context.Background(), svc, tt.id, tt.user, listed); rc != nil {
				got = rc.Name
			}
			if got != tt.want {
				t.Errorf("returnCampaign = %q, want %q", got, tt.want)
			}
		})
	}
}
