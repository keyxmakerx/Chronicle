package campaigns

import (
	"context"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// memGuestCodes keeps codes in memory, by hash.
type memGuestCodes struct {
	GuestCodeRepository
	byHash   map[string]*GuestCode
	released []string
	usedBy   map[string]string
}

func (m *memGuestCodes) Create(_ context.Context, gc *GuestCode, hash string) error {
	cp := *gc
	m.byHash[hash] = &cp
	return nil
}

func (m *memGuestCodes) CountLive(_ context.Context, _ string, now time.Time) (int, error) {
	n := 0
	for _, g := range m.byHash {
		if g.Status(now) == "open" {
			n++
		}
	}
	return n, nil
}

func (m *memGuestCodes) Claim(_ context.Context, hash string, now time.Time) (string, string, error) {
	g, ok := m.byHash[hash]
	if !ok || g.Status(now) != "open" {
		return "", "", apperror.NewNotFound("no such code")
	}
	g.UsedAt = &now
	return g.ID, g.CampaignID, nil
}

func (m *memGuestCodes) Release(_ context.Context, id string) error {
	m.released = append(m.released, id)
	for _, g := range m.byHash {
		if g.ID == id {
			g.UsedAt = nil
		}
	}
	return nil
}

func (m *memGuestCodes) SetUsedBy(_ context.Context, id, userID string) error {
	m.usedBy[id] = userID
	return nil
}

// guestCampaigns is the slice of CampaignRepository guest codes use.
type guestCampaigns struct {
	CampaignRepository
	archived bool
	members  map[string]Role
}

func (g *guestCampaigns) FindByID(_ context.Context, id string) (*Campaign, error) {
	c := &Campaign{ID: id}
	if g.archived {
		now := time.Now()
		c.ArchivedAt = &now
	}
	return c, nil
}

func (g *guestCampaigns) FindMember(_ context.Context, _, userID string) (*CampaignMember, error) {
	if r, ok := g.members[userID]; ok {
		return &CampaignMember{UserID: userID, Role: r}, nil
	}
	return nil, apperror.NewNotFound("not a member")
}

func (g *guestCampaigns) AddMember(_ context.Context, m *CampaignMember) error {
	g.members[m.UserID] = m.Role
	return nil
}

func newGuestCodeRig() (*GuestCodeService, *memGuestCodes, *guestCampaigns) {
	repo := &memGuestCodes{byHash: map[string]*GuestCode{}, usedBy: map[string]string{}}
	camps := &guestCampaigns{members: map[string]Role{}}
	return NewGuestCodeService(repo, camps), repo, camps
}

func assertGuestErr(t *testing.T, err error, code int) {
	t.Helper()
	appErr, ok := err.(*apperror.AppError)
	if !ok || appErr.Code != code {
		t.Fatalf("err = %v, want code %d", err, code)
	}
}

func TestRandomGuestCode(t *testing.T) {
	shape := regexp.MustCompile(`^[ABCDEFGHJKMNPQRSTUVWXYZ23456789]{4}-[ABCDEFGHJKMNPQRSTUVWXYZ23456789]{4}$`)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, err := randomGuestCode()
		if err != nil || !shape.MatchString(c) {
			t.Fatalf("code %q, %v", c, err)
		}
		seen[c] = true
	}
	if len(seen) < 199 {
		t.Fatalf("codes repeat: %d distinct of 200", len(seen))
	}
	if hashGuestCode("k7qm 3wht") != hashGuestCode("K7QM-3WHT") {
		t.Fatal("the code's spelling changes its hash")
	}
}

func TestGuestCodeLifecycle(t *testing.T) {
	svc, repo, camps := newGuestCodeRig()
	ctx := context.Background()

	fresh, err := svc.Create(ctx, "camp-1", "owner", "  Sam  ")
	if err != nil || fresh.Note != "Sam" || fresh.ExpiresAt.Sub(fresh.CreatedAt) != guestCodeLife {
		t.Fatalf("Create = %+v, %v", fresh, err)
	}
	for h := range repo.byHash {
		if h == fresh.Code {
			t.Fatal("the code was stored as itself")
		}
	}

	id, campaignID, err := svc.ClaimGuestCode(ctx, fresh.Code)
	if err != nil || id != fresh.ID || campaignID != "camp-1" {
		t.Fatalf("Claim = %q %q %v", id, campaignID, err)
	}
	_, _, err = svc.ClaimGuestCode(ctx, fresh.Code)
	assertGuestErr(t, err, http.StatusNotFound)

	if err := svc.AdmitWithGuestCode(ctx, id, campaignID, "guest-1"); err != nil {
		t.Fatal(err)
	}
	if camps.members["guest-1"] != RolePlayer || repo.usedBy[id] != "guest-1" {
		t.Fatalf("admit: members %v usedBy %v", camps.members, repo.usedBy)
	}
	// Someone already in the campaign is refused, so the caller releases.
	assertGuestErr(t, svc.AdmitWithGuestCode(ctx, id, campaignID, "guest-1"), http.StatusConflict)
}

func TestGuestCodeRefusals(t *testing.T) {
	ctx := context.Background()

	svc, _, _ := newGuestCodeRig()
	_, err := svc.Create(ctx, "camp-1", "owner", string(make([]rune, 101)))
	assertGuestErr(t, err, http.StatusBadRequest)

	for i := 0; i < guestCodeMaxLive; i++ {
		if _, err := svc.Create(ctx, "camp-1", "owner", ""); err != nil {
			t.Fatal(err)
		}
	}
	_, err = svc.Create(ctx, "camp-1", "owner", "")
	assertGuestErr(t, err, http.StatusBadRequest)

	_, _, err = svc.ClaimGuestCode(ctx, "ABC")
	assertGuestErr(t, err, http.StatusNotFound)

	// An archived campaign's code is put back, not spent.
	svc, repo, camps := newGuestCodeRig()
	fresh, _ := svc.Create(ctx, "camp-1", "owner", "")
	camps.archived = true
	_, _, err = svc.ClaimGuestCode(ctx, fresh.Code)
	assertGuestErr(t, err, http.StatusBadRequest)
	if len(repo.released) != 1 || repo.released[0] != fresh.ID {
		t.Fatalf("released = %v", repo.released)
	}

	// An expired code is refused.
	svc, _, _ = newGuestCodeRig()
	fresh, _ = svc.Create(ctx, "camp-1", "owner", "")
	svc.now = func() time.Time { return time.Now().UTC().Add(guestCodeLife + time.Minute) }
	_, _, err = svc.ClaimGuestCode(ctx, fresh.Code)
	assertGuestErr(t, err, http.StatusNotFound)
}

func TestGuestCodeStatus(t *testing.T) {
	now := time.Now()
	used := now.Add(-time.Hour)
	tests := []struct {
		g    GuestCode
		want string
	}{
		{GuestCode{ExpiresAt: now.Add(time.Hour)}, "open"},
		{GuestCode{ExpiresAt: now}, "expired"},
		{GuestCode{ExpiresAt: now.Add(time.Hour), UsedAt: &used}, "used"},
	}
	for _, tt := range tests {
		if got := tt.g.Status(now); got != tt.want {
			t.Errorf("Status = %q, want %q", got, tt.want)
		}
	}
}
