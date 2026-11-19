package campaigns

import (
	"context"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// Peek on the in-memory store, same rule as the SQL: live codes only.
func (m *memGuestCodes) Peek(_ context.Context, hash string, now time.Time) (string, error) {
	g, ok := m.byHash[hash]
	if !ok || g.Status(now) != "open" {
		return "", apperror.NewNotFound("no such code")
	}
	return g.CampaignID, nil
}

type doorCampaigns struct {
	CampaignRepository
	byCode  map[string]*Campaign
	byID    map[string]*Campaign
	members []CampaignMember
}

func (d *doorCampaigns) FindByJoinCode(_ context.Context, code string) (*Campaign, error) {
	if c, ok := d.byCode[code]; ok {
		return c, nil
	}
	return nil, apperror.NewNotFound("no such link")
}

func (d *doorCampaigns) FindByID(_ context.Context, id string) (*Campaign, error) {
	if c, ok := d.byID[id]; ok {
		return c, nil
	}
	return nil, apperror.NewNotFound("no such campaign")
}

func (d *doorCampaigns) ListMembers(context.Context, string) ([]CampaignMember, error) {
	return d.members, nil
}

type doorInvites struct {
	InviteRepository
	byToken map[string]*Invite
}

func (d *doorInvites) GetByToken(_ context.Context, token string) (*Invite, error) {
	if i, ok := d.byToken[token]; ok {
		return i, nil
	}
	return nil, apperror.NewNotFound("no such invite")
}

func TestInvitationSourceLookups(t *testing.T) {
	ctx := context.Background()
	backdrop := "bd-1"
	avatar := "av-1"
	camp := &Campaign{ID: "c1", Name: "The Ashen Reach", BackdropPath: &backdrop}
	archived := &Campaign{ID: "c2", Name: "Old"}
	archivedAt := time.Now().Add(-time.Hour)
	archived.ArchivedAt = &archivedAt
	camps := &doorCampaigns{
		byCode: map[string]*Campaign{"link1": camp, "oldlink": archived},
		byID:   map[string]*Campaign{"c1": camp, "c2": archived},
		members: []CampaignMember{
			{DisplayName: "Olive", Email: "olive@secret.example", AvatarPath: &avatar},
			{DisplayName: "Pat", Email: "pat@secret.example"},
		},
	}
	now := time.Now().UTC()
	accepted := now.Add(-time.Hour)
	invites := &doorInvites{byToken: map[string]*Invite{
		"live":    {CampaignID: "c1", ExpiresAt: now.Add(time.Hour)},
		"used":    {CampaignID: "c1", ExpiresAt: now.Add(time.Hour), AcceptedAt: &accepted},
		"expired": {CampaignID: "c1", ExpiresAt: now.Add(-time.Hour)},
	}}
	codes, repo, _ := newGuestCodeRig()
	live, _ := codes.Create(ctx, "c1", "owner", "", 0)
	usedCode, _ := codes.Create(ctx, "c1", "owner", "", 0)
	repo.byHash[hashGuestCode(usedCode.Code)].UsedAt = &accepted
	expiredCode, _ := codes.Create(ctx, "c1", "owner", "", 0)
	repo.byHash[hashGuestCode(expiredCode.Code)].ExpiresAt = now.Add(-time.Minute)
	archivedCode, _ := codes.Create(ctx, "c2", "owner", "", 0)

	night := &auth.InvitationNight{Date: "2026-12-01", Time: "19:00", TZ: "UTC", At: now.Add(48 * time.Hour)}
	src := NewInvitationSource(camps, invites, codes, func(context.Context, string, time.Time) *auth.InvitationNight { return night })

	tests := []struct {
		name string
		got  *auth.Invitation
		want bool
	}{
		{"link valid", src.ByLink(ctx, "link1"), true},
		{"link wrong", src.ByLink(ctx, "nope"), false},
		{"link revoked (cleared)", src.ByLink(ctx, ""), false},
		{"link of archived campaign", src.ByLink(ctx, "oldlink"), false},
		{"email valid", src.ByToken(ctx, "live"), true},
		{"email wrong", src.ByToken(ctx, "nope"), false},
		{"email used", src.ByToken(ctx, "used"), false},
		{"email expired", src.ByToken(ctx, "expired"), false},
		{"email revoked (deleted)", src.ByToken(ctx, "deleted"), false},
		{"code valid", src.ByGuestCode(ctx, live.Code), true},
		{"code typed loosely", src.ByGuestCode(ctx, " "+live.Code[:4]+live.Code[5:]+" "), true},
		{"code wrong", src.ByGuestCode(ctx, "ZZZZ-ZZZZ"), false},
		{"code too short", src.ByGuestCode(ctx, "ABC"), false},
		{"code used", src.ByGuestCode(ctx, usedCode.Code), false},
		{"code expired", src.ByGuestCode(ctx, expiredCode.Code), false},
		{"code of archived campaign", src.ByGuestCode(ctx, archivedCode.Code), false},
	}
	for _, tc := range tests {
		if (tc.got != nil) != tc.want {
			t.Errorf("%s: got %v, want shown=%v", tc.name, tc.got, tc.want)
		}
	}

	inv := src.ByLink(ctx, "link1")
	if inv.CampaignName != "The Ashen Reach" || inv.BackdropPath != "bd-1" || inv.NextNight != night {
		t.Errorf("invitation = %+v", inv)
	}
	if len(inv.Members) != 2 || inv.Members[0].Name != "Olive" || inv.Members[0].AvatarPath != "av-1" {
		t.Errorf("members = %+v", inv.Members)
	}
}

func TestInvitationSourceWithoutNight(t *testing.T) {
	camp := &Campaign{ID: "c1", Name: "Quiet"}
	camps := &doorCampaigns{byCode: map[string]*Campaign{"l": camp}, byID: map[string]*Campaign{"c1": camp}}
	codes, _, _ := newGuestCodeRig()
	inv := NewInvitationSource(camps, &doorInvites{}, codes, nil).ByLink(context.Background(), "l")
	if inv == nil || inv.NextNight != nil || len(inv.Members) != 0 {
		t.Fatalf("invitation = %+v", inv)
	}
}
