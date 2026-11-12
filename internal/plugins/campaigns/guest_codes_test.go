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

func (m *memGuestCodes) byID(id string) *GuestCode {
	for _, g := range m.byHash {
		if g.ID == id {
			return g
		}
	}
	return nil
}

func (m *memGuestCodes) FindByID(_ context.Context, campaignID, id string) (*GuestCode, error) {
	g := m.byID(id)
	if g == nil || g.CampaignID != campaignID {
		return nil, apperror.NewNotFound("that code is gone")
	}
	cp := *g
	return &cp, nil
}

func (m *memGuestCodes) VisitDays(_ context.Context, id string) (int, error) {
	return m.byID(id).VisitDays, nil
}

func (m *memGuestCodes) StartVisit(_ context.Context, id, userID string, until time.Time) error {
	g := m.byID(id)
	g.UsedBy, g.VisitUntil = &userID, &until
	m.usedBy[id] = userID
	return nil
}

func (m *memGuestCodes) FindVisit(_ context.Context, campaignID, userID string) (*time.Time, error) {
	var latest *time.Time
	for _, g := range m.byHash {
		if g.CampaignID == campaignID && g.UsedBy != nil && *g.UsedBy == userID && g.VisitUntil != nil &&
			(latest == nil || g.VisitUntil.After(*latest)) {
			u := *g.VisitUntil
			latest = &u
		}
	}
	return latest, nil
}

func (m *memGuestCodes) EndVisit(_ context.Context, campaignID, id string, now time.Time) error {
	g := m.byID(id)
	if g == nil || g.CampaignID != campaignID || g.VisitUntil == nil || !g.VisitUntil.After(now) {
		return apperror.NewNotFound("that visit has already ended")
	}
	g.VisitUntil = &now
	return nil
}

func (m *memGuestCodes) ClearVisits(_ context.Context, campaignID, userID string) error {
	for _, g := range m.byHash {
		if g.CampaignID == campaignID && g.UsedBy != nil && *g.UsedBy == userID {
			g.VisitUntil = nil
		}
	}
	return nil
}

func (m *memGuestCodes) EndVisitsFor(_ context.Context, campaignID, userID string, now time.Time) error {
	for _, g := range m.byHash {
		if g.CampaignID == campaignID && g.UsedBy != nil && *g.UsedBy == userID && g.VisitUntil != nil && g.VisitUntil.After(now) {
			n := now
			g.VisitUntil = &n
		}
	}
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

	fresh, err := svc.Create(ctx, "camp-1", "owner", "  Sam  ", 0)
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
	_, err := svc.Create(ctx, "camp-1", "owner", string(make([]rune, 101)), 0)
	assertGuestErr(t, err, http.StatusBadRequest)

	for i := 0; i < guestCodeMaxLive; i++ {
		if _, err := svc.Create(ctx, "camp-1", "owner", "", 0); err != nil {
			t.Fatal(err)
		}
	}
	_, err = svc.Create(ctx, "camp-1", "owner", "", 0)
	assertGuestErr(t, err, http.StatusBadRequest)

	_, _, err = svc.ClaimGuestCode(ctx, "ABC")
	assertGuestErr(t, err, http.StatusNotFound)

	// An archived campaign's code is put back, not spent.
	svc, repo, camps := newGuestCodeRig()
	fresh, _ := svc.Create(ctx, "camp-1", "owner", "", 0)
	camps.archived = true
	_, _, err = svc.ClaimGuestCode(ctx, fresh.Code)
	assertGuestErr(t, err, http.StatusBadRequest)
	if len(repo.released) != 1 || repo.released[0] != fresh.ID {
		t.Fatalf("released = %v", repo.released)
	}

	// An expired code is refused.
	svc, _, _ = newGuestCodeRig()
	fresh, _ = svc.Create(ctx, "camp-1", "owner", "", 0)
	svc.now = func() time.Time { return time.Now().UTC().Add(guestCodeLife + time.Minute) }
	_, _, err = svc.ClaimGuestCode(ctx, fresh.Code)
	assertGuestErr(t, err, http.StatusNotFound)
}

func TestGuestCodeStatus(t *testing.T) {
	now := time.Now()
	used := now.Add(-time.Hour)
	later := now.Add(time.Hour)
	tests := []struct {
		g    GuestCode
		want string
	}{
		{GuestCode{ExpiresAt: now.Add(time.Hour)}, "open"},
		{GuestCode{ExpiresAt: now}, "expired"},
		{GuestCode{ExpiresAt: now.Add(time.Hour), UsedAt: &used}, "used"},
		{GuestCode{ExpiresAt: now.Add(time.Hour), VisitDays: 3}, "open"},
		{GuestCode{ExpiresAt: now.Add(time.Hour), UsedAt: &used, VisitDays: 3, VisitUntil: &later}, "visiting"},
		{GuestCode{ExpiresAt: now.Add(time.Hour), UsedAt: &used, VisitDays: 3, VisitUntil: &used}, "visit_ended"},
		// A visitor made a player has no end left, and reads as a used code.
		{GuestCode{ExpiresAt: now.Add(time.Hour), UsedAt: &used, VisitDays: 3}, "used"},
	}
	for _, tt := range tests {
		if got := tt.g.Status(now); got != tt.want {
			t.Errorf("Status = %q, want %q", got, tt.want)
		}
	}
}

// A look-around pass lets its holder read the campaign until it runs out,
// never makes them a member, and the owner can end it early or make them a
// player at any point, after it ran out included.
func TestGuestCodeVisit(t *testing.T) {
	svc, repo, camps := newGuestCodeRig()
	ctx := context.Background()
	start := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return start }

	_, err := svc.Create(ctx, "camp-1", "owner", "", 2)
	assertGuestErr(t, err, http.StatusBadRequest)

	fresh, err := svc.Create(ctx, "camp-1", "owner", "Mira", 3)
	if err != nil || !fresh.IsVisit() {
		t.Fatalf("Create = %+v, %v", fresh, err)
	}
	id, campaignID, err := svc.ClaimGuestCode(ctx, fresh.Code)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AdmitWithGuestCode(ctx, id, campaignID, "mira"); err != nil {
		t.Fatal(err)
	}
	if _, ok := camps.members["mira"]; ok {
		t.Fatal("a visitor was made a member")
	}
	until, live, found, err := svc.VisitPass(ctx, "camp-1", "mira")
	if err != nil || !live || !found || !until.Equal(start.Add(72*time.Hour)) {
		t.Fatalf("VisitPass = %v %v %v %v", until, live, found, err)
	}
	if _, live, found, _ := svc.VisitPass(ctx, "camp-2", "mira"); live || found {
		t.Fatal("a pass opened another campaign")
	}

	// Another campaign's owner can't touch it.
	assertGuestErr(t, svc.EndVisit(ctx, "camp-2", id), http.StatusNotFound)

	// Run out: still found, no longer live.
	svc.now = func() time.Time { return start.Add(72*time.Hour + time.Second) }
	if _, live, found, _ := svc.VisitPass(ctx, "camp-1", "mira"); live || !found {
		t.Fatalf("after the end: live %v found %v", live, found)
	}
	assertGuestErr(t, svc.EndVisit(ctx, "camp-1", id), http.StatusNotFound)

	// Making them a player after the end still works, and the pass is gone.
	if err := svc.MakeVisitorPlayer(ctx, "camp-1", id); err != nil {
		t.Fatal(err)
	}
	if camps.members["mira"] != RolePlayer {
		t.Fatalf("members = %v", camps.members)
	}
	if _, _, found, _ := svc.VisitPass(ctx, "camp-1", "mira"); found {
		t.Fatal("the pass outlived the membership")
	}
	assertGuestErr(t, svc.MakeVisitorPlayer(ctx, "camp-1", id), http.StatusNotFound)

	// End now closes a running visit at once.
	svc.now = func() time.Time { return start }
	second, _ := svc.Create(ctx, "camp-1", "owner", "", 1)
	id2, _, _ := svc.ClaimGuestCode(ctx, second.Code)
	if err := svc.AdmitWithGuestCode(ctx, id2, "camp-1", "sam"); err != nil {
		t.Fatal(err)
	}
	if err := svc.EndVisit(ctx, "camp-1", id2); err != nil {
		t.Fatal(err)
	}
	if _, live, found, _ := svc.VisitPass(ctx, "camp-1", "sam"); live || !found {
		t.Fatalf("after End now: live %v found %v", live, found)
	}
	if repo.byID(id2).Status(start) != "visit_ended" {
		t.Fatalf("status = %q", repo.byID(id2).Status(start))
	}

	// A visitor who joins with a player's code is a member now, so their
	// pass stops counting and can't outlive a later removal.
	visit, _ := svc.Create(ctx, "camp-1", "owner", "", 3)
	vid, _, _ := svc.ClaimGuestCode(ctx, visit.Code)
	if err := svc.AdmitWithGuestCode(ctx, vid, "camp-1", "bren"); err != nil {
		t.Fatal(err)
	}
	join, _ := svc.Create(ctx, "camp-1", "owner", "", 0)
	jid, _, _ := svc.ClaimGuestCode(ctx, join.Code)
	if err := svc.AdmitWithGuestCode(ctx, jid, "camp-1", "bren"); err != nil {
		t.Fatal(err)
	}
	if _, _, found, _ := svc.VisitPass(ctx, "camp-1", "bren"); found {
		t.Fatal("joining as a player left the pass in place")
	}

	// EndVisits, run when a member is removed, ends every running pass.
	again, _ := svc.Create(ctx, "camp-1", "owner", "", 7)
	aid, _, _ := svc.ClaimGuestCode(ctx, again.Code)
	_ = svc.AdmitWithGuestCode(ctx, aid, "camp-1", "orsa")
	if err := svc.EndVisits(ctx, "camp-1", "orsa"); err != nil {
		t.Fatal(err)
	}
	if _, live, _, _ := svc.VisitPass(ctx, "camp-1", "orsa"); live {
		t.Fatal("EndVisits left a pass running")
	}

	// A player's code can't be ended or "made a player" again.
	third, _ := svc.Create(ctx, "camp-1", "owner", "", 0)
	id3, _, _ := svc.ClaimGuestCode(ctx, third.Code)
	_ = svc.AdmitWithGuestCode(ctx, id3, "camp-1", "tal")
	assertGuestErr(t, svc.EndVisit(ctx, "camp-1", id3), http.StatusNotFound)
	assertGuestErr(t, svc.MakeVisitorPlayer(ctx, "camp-1", id3), http.StatusNotFound)
}
