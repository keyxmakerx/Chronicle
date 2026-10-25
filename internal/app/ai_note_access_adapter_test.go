package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type noteWritersStub struct {
	campaigns.CampaignService
	members  map[string]campaigns.CampaignMember
	granted  map[string]bool
	grantErr error
}

func (s noteWritersStub) GetMember(_ context.Context, _, userID string) (*campaigns.CampaignMember, error) {
	m, ok := s.members[userID]
	if !ok {
		return nil, errors.New("not a member")
	}
	return &m, nil
}
func (s noteWritersStub) IsUserDmGranted(_ context.Context, _, userID string) (bool, error) {
	return s.granted[userID], s.grantErr
}

func TestAINoteAccess_NoteWriter(t *testing.T) {
	members := map[string]campaigns.CampaignMember{
		"mira": {UserID: "mira", DisplayName: "Mira", Role: campaigns.RolePlayer},
		"sam":  {UserID: "sam", DisplayName: "Sam", Role: campaigns.RoleScribe},
		"tam":  {UserID: "tam", DisplayName: "Tam", Role: campaigns.RolePlayer},
		"both": {UserID: "both", DisplayName: "Bo", Role: campaigns.RoleScribe},
	}
	granted := map[string]bool{"mira": true, "both": true}
	tests := []struct {
		name, user string
		grantErr   error
		want       records.NoteWriter
	}{
		{"co-DM by grant", "mira", nil, records.NoteWriter{Name: "Mira", Kind: records.WriterCoDM}},
		{"Scribe", "sam", nil, records.NoteWriter{Name: "Sam", Kind: records.WriterScribe}},
		{"player", "tam", nil, records.NoteWriter{Name: "Tam", Kind: records.WriterPlayer}},
		{"a granted Scribe is a co-DM", "both", nil, records.NoteWriter{Name: "Bo", Kind: records.WriterCoDM}},
		{"left the campaign", "gone", nil, records.NoteWriter{Kind: records.WriterPlayer}},
		{"grant unreadable falls back to the role", "mira", errors.New("db down"), records.NoteWriter{Name: "Mira", Kind: records.WriterPlayer}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := aiNoteAccess{campaigns: noteWritersStub{members: members, granted: granted, grantErr: tc.grantErr}}
			got, err := a.NoteWriter(context.Background(), "c", tc.user)
			if err != nil || got != tc.want {
				t.Errorf("got %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}
