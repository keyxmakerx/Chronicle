package campaigns

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// leaveSpyRepo records every write the service makes, so a test can say
// "this and nothing else".
type leaveSpyRepo struct {
	*mockCampaignRepo
	writes []string
}

func (r *leaveSpyRepo) RemoveMember(ctx context.Context, campaignID, userID string) error {
	r.writes = append(r.writes, "RemoveMember:"+userID)
	return r.mockCampaignRepo.RemoveMember(ctx, campaignID, userID)
}
func (r *leaveSpyRepo) SetJoinCode(ctx context.Context, campaignID, code string) error {
	r.writes = append(r.writes, "SetJoinCode")
	return r.mockCampaignRepo.SetJoinCode(ctx, campaignID, code)
}
func (r *leaveSpyRepo) ClearJoinCode(ctx context.Context, campaignID string) error {
	r.writes = append(r.writes, "ClearJoinCode")
	return r.mockCampaignRepo.ClearJoinCode(ctx, campaignID)
}
func (r *leaveSpyRepo) UpdateSettings(ctx context.Context, campaignID, settingsJSON string) error {
	r.writes = append(r.writes, "UpdateSettings:"+settingsJSON)
	return r.mockCampaignRepo.UpdateSettings(ctx, campaignID, settingsJSON)
}

type fakeReleaser struct {
	names    []string
	err      error
	released []string
}

func (f *fakeReleaser) ReleaseCharacters(_ context.Context, campaignID, userID string) ([]string, error) {
	f.released = append(f.released, campaignID+"/"+userID)
	return f.names, f.err
}
func (f *fakeReleaser) ClaimedCharacters(context.Context, string, []string) (map[string][]string, error) {
	return map[string][]string{"camp-1": f.names}, nil
}

type bellNote struct {
	to                           []string
	message, detail, action, url string
}

type fakeLeaveBell struct{ sent []bellNote }

func (f *fakeLeaveBell) MemberLeft(_ context.Context, _ string, to []string, message, detail, action, link string) error {
	f.sent = append(f.sent, bellNote{to, message, detail, action, link})
	return nil
}

func TestLeaveCampaign(t *testing.T) {
	const code = "joincode123"
	members := []CampaignMember{
		{UserID: "owner", Role: RoleOwner, DisplayName: "Mira"},
		{UserID: "other", Role: RolePlayer, DisplayName: "Sela"},
		{UserID: "tam", Role: RolePlayer, DisplayName: "Tam"},
	}
	tests := []struct {
		name       string
		userID     string
		role       Role
		known      bool
		chars      []string
		relErr     error
		wantCode   int
		wantNote   *bellNote
		wantFreed  bool
		wantNoNote bool
	}{
		{name: "owner is refused", userID: "owner", role: RoleOwner, known: true, wantCode: 400, wantNoNote: true},
		{name: "non-member is refused", userID: "stranger", known: false, wantCode: 404, wantNoNote: true},
		{
			name: "player with one character", userID: "tam", role: RolePlayer, known: true,
			chars: []string{"Brannoc"}, wantFreed: true,
			wantNote: &bellNote{to: []string{"owner"}, message: "Tam left Therin",
				detail: "Brannoc is free to pick. Tam's posts and notes stay.", action: "Give Brannoc to someone",
				url: "/campaigns/camp-1/characters?open=players"},
		},
		{
			name: "player with several characters", userID: "tam", role: RolePlayer, known: true,
			chars: []string{"Brannoc", "Sela", "Vex"}, wantFreed: true,
			wantNote: &bellNote{to: []string{"owner"}, message: "Tam left Therin",
				detail: "Brannoc, Sela and Vex are free to pick. Tam's posts and notes stay.", action: "Give Brannoc, Sela and Vex to someone",
				url: "/campaigns/camp-1/characters?open=players"},
		},
		{
			name: "scribe with no character", userID: "tam", role: RoleScribe, known: true, wantFreed: true,
			wantNote: &bellNote{to: []string{"owner"}, message: "Tam left Therin",
				detail: "Tam's posts and notes stay.", url: "/campaigns/camp-1/characters?open=players"},
		},
		{
			name: "freeing fails after leaving", userID: "tam", role: RolePlayer, known: true,
			chars: []string{"Brannoc"}, relErr: errors.New("db down"), wantFreed: true,
			wantNote: &bellNote{to: []string{"owner"}, message: "Tam left Therin",
				detail: "Tam's posts and notes stay.", url: "/campaigns/camp-1/characters?open=players"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := &mockCampaignRepo{
				findMemberFn: func(_ context.Context, _, uid string) (*CampaignMember, error) {
					if !tc.known || uid != tc.userID {
						return nil, errors.New("not found")
					}
					for _, m := range members {
						if m.UserID == uid {
							m.Role = tc.role
							return &m, nil
						}
					}
					return &CampaignMember{UserID: uid, Role: tc.role}, nil
				},
				findByIDFn: func(_ context.Context, id string) (*Campaign, error) {
					j := code
					return &Campaign{ID: id, Name: "Therin", Settings: "{}", SidebarConfig: "{}", JoinCode: &j}, nil
				},
				listMembersFn: func(context.Context, string) ([]CampaignMember, error) { return members, nil },
			}
			if !tc.known {
				base.findMemberFn = func(context.Context, string, string) (*CampaignMember, error) {
					return nil, notFoundErr()
				}
			}
			repo := &leaveSpyRepo{mockCampaignRepo: base}
			svc := NewCampaignService(repo, &mockUserFinder{}, nil, nil, "http://localhost")
			rel := &fakeReleaser{names: tc.chars, err: tc.relErr}
			bell := &fakeLeaveBell{}
			ConfigureLeaving(svc, rel, bell)

			err := svc.LeaveCampaign(context.Background(), "camp-1", tc.userID)
			if tc.wantCode != 0 {
				assertAppError(t, err, tc.wantCode)
				if len(repo.writes) != 0 || len(rel.released) != 0 || len(bell.sent) != 0 {
					t.Fatalf("a refused leave changed things: writes=%v released=%v sent=%v", repo.writes, rel.released, bell.sent)
				}
				return
			}
			if err != nil {
				t.Fatalf("LeaveCampaign: %v", err)
			}

			// The only thing written is the membership row going: the person
			// is not marked removed (no settings write), the join code is not
			// replaced, and nothing they made is touched.
			if want := []string{"RemoveMember:" + tc.userID}; !reflect.DeepEqual(repo.writes, want) {
				t.Errorf("repo writes = %v, want exactly %v", repo.writes, want)
			}
			if tc.wantFreed && len(rel.released) != 1 {
				t.Errorf("characters released %d times, want once", len(rel.released))
			}
			if len(bell.sent) != 1 {
				t.Fatalf("owner notes sent = %d, want exactly 1", len(bell.sent))
			}
			if got := bell.sent[0]; !reflect.DeepEqual(got, *tc.wantNote) {
				t.Errorf("note = %+v, want %+v", got, *tc.wantNote)
			}
		})
	}
}

func notFoundErr() error {
	_, err := (&mockCampaignRepo{}).FindMember(context.Background(), "", "")
	if err != nil {
		return err
	}
	return errors.New("not found")
}

// A leaver holding a co-DM grant loses it, as a removed member does, but is
// not remembered as removed.
func TestLeaveCampaignDropsGrantWithoutRemovedMark(t *testing.T) {
	var saved string
	repo := &mockCampaignRepo{
		findMemberFn: func(context.Context, string, string) (*CampaignMember, error) {
			return &CampaignMember{UserID: "tam", Role: RoleScribe, DisplayName: "Tam"}, nil
		},
		findByIDFn: func(_ context.Context, id string) (*Campaign, error) {
			return &Campaign{ID: id, Name: "Therin", Settings: `{"dm_grant_ids":["tam","x"]}`, SidebarConfig: "{}"}, nil
		},
		updateSettingsFn: func(_ context.Context, _, s string) error { saved = s; return nil },
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	if err := svc.LeaveCampaign(context.Background(), "camp-1", "tam"); err != nil {
		t.Fatalf("LeaveCampaign: %v", err)
	}
	if strings.Contains(saved, `"tam"`) {
		t.Errorf("grant still holds tam: %s", saved)
	}
	if strings.Contains(saved, "removed_member") {
		t.Errorf("leaver was marked removed: %s", saved)
	}
}

// Leaving runs the same hooks removal does, so every credential the person
// held in the campaign ends at once.
func TestLeaveCampaignRunsRemovalHooks(t *testing.T) {
	repo := &mockCampaignRepo{
		findMemberFn: func(context.Context, string, string) (*CampaignMember, error) {
			return &CampaignMember{UserID: "tam", Role: RolePlayer}, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	var hooked []string
	OnMemberRemoved(svc, func(_ context.Context, campaignID, userID string) { hooked = append(hooked, campaignID+"/"+userID) })
	if err := svc.LeaveCampaign(context.Background(), "camp-1", "tam"); err != nil {
		t.Fatalf("LeaveCampaign: %v", err)
	}
	if !reflect.DeepEqual(hooked, []string{"camp-1/tam"}) {
		t.Errorf("removal hooks = %v", hooked)
	}
}

func TestLeaveCharacterLine(t *testing.T) {
	tests := []struct {
		names []string
		want  string
	}{
		{nil, ""},
		{[]string{"Brannoc"}, "Brannoc stays with the campaign. The DM can give them to someone else."},
		{[]string{"Brannoc", "Sela"}, "Brannoc and Sela stay with the campaign. The DM can give them to someone else."},
	}
	for _, tc := range tests {
		if got := leaveCharacterLine(tc.names); got != tc.want {
			t.Errorf("leaveCharacterLine(%v) = %q, want %q", tc.names, got, tc.want)
		}
	}
}
