package campaigns

import (
	"context"
	"testing"
	"time"
)

// TestGuestVisit_Integration runs a look-around pass through real MariaDB
// with every core migration applied (000061 adds the columns): the pass
// starts on use without a membership, a second pass extends rather than
// shadows the first, End now closes it, Make player turns it into a
// membership, and merging a visiting guest hands the pass to the account.
// Skipped under -short or with no database, like TestNavPins_Integration.
func TestGuestVisit_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openNavPinsTestDB(t)
	ctx := context.Background()
	campRepo := NewCampaignRepository(db)
	svc := NewGuestCodeService(NewGuestCodeRepository(db), campRepo)
	start := time.Now().UTC().Truncate(time.Second)
	svc.now = func() time.Time { return start }

	owner, mira, acct := navPinsUUID(t), navPinsUUID(t), navPinsUUID(t)
	campaignID := navPinsUUID(t)
	for _, u := range []string{owner, mira, acct} {
		navPinsExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
			u, "visit-"+u+"@example.test", "Visit "+u[:4], "x")
	}
	navPinsExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Visits", "visits-"+campaignID[:8], owner)
	navPinsExec(t, db, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'owner')`, campaignID, owner)

	use := func(days int, user string) string {
		t.Helper()
		fresh, err := svc.Create(ctx, campaignID, owner, "", days)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		id, cid, err := svc.ClaimGuestCode(ctx, fresh.Code)
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if err := svc.AdmitWithGuestCode(ctx, id, cid, user); err != nil {
			t.Fatalf("Admit: %v", err)
		}
		return id
	}

	first := use(1, mira)
	if _, err := campRepo.FindMember(ctx, campaignID, mira); err == nil {
		t.Fatal("a visitor was made a member")
	}
	until, live, found, err := svc.VisitPass(ctx, campaignID, mira)
	if err != nil || !live || !found || !until.Equal(start.Add(24*time.Hour)) {
		t.Fatalf("VisitPass = %v %v %v %v", until, live, found, err)
	}
	second := use(7, mira)
	if until, _, _, _ := svc.VisitPass(ctx, campaignID, mira); !until.Equal(start.Add(7 * 24 * time.Hour)) {
		t.Fatalf("the longer pass didn't win: %v", until)
	}

	codes, err := svc.List(ctx, campaignID)
	if err != nil || len(codes) != 2 {
		t.Fatalf("List = %d codes, %v", len(codes), err)
	}
	for _, gc := range codes {
		if gc.Status(start) != "visiting" || gc.VisitDays == 0 {
			t.Fatalf("listed code %+v reads %q", gc, gc.Status(start))
		}
	}

	if err := svc.EndVisit(ctx, campaignID, first); err != nil {
		t.Fatalf("EndVisit: %v", err)
	}
	if err := svc.EndVisit(ctx, campaignID, first); err == nil {
		t.Fatal("ending an ended visit succeeded")
	}

	// Merging the visiting guest moves the pass to the account.
	if err := svc.MoveGuestMembership(ctx, campaignID, mira, acct); err != nil {
		t.Fatalf("MoveGuestMembership: %v", err)
	}
	if _, live, _, _ := svc.VisitPass(ctx, campaignID, acct); !live {
		t.Fatal("the pass didn't follow the merge")
	}

	if err := svc.MakeVisitorPlayer(ctx, campaignID, second); err != nil {
		t.Fatalf("MakeVisitorPlayer: %v", err)
	}
	m, err := campRepo.FindMember(ctx, campaignID, acct)
	if err != nil || m.Role != RolePlayer {
		t.Fatalf("member = %+v, %v", m, err)
	}
	gc, err := NewGuestCodeRepository(db).FindByID(ctx, campaignID, second)
	if err != nil || gc.VisitUntil != nil || gc.Status(start) != "used" {
		t.Fatalf("made-player code = %+v, %v", gc, err)
	}
	if _, err := NewGuestCodeRepository(db).FindByID(ctx, navPinsUUID(t), second); err == nil {
		t.Fatal("another campaign read the code")
	}

	// Someone who became a member another way (an email invite) while their
	// pass ran loses the pass along with the membership when removed.
	tal := navPinsUUID(t)
	navPinsExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		tal, "visit-"+tal+"@example.test", "Tal", "x")
	use(7, tal)
	navPinsExec(t, db, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'player')`, campaignID, tal)
	campSvc := NewCampaignService(campRepo, nil, nil, nil, "")
	ConfigureVisitPasses(campSvc, svc)
	if err := campSvc.RemoveMember(ctx, campaignID, tal); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if _, live, found, _ := svc.VisitPass(ctx, campaignID, tal); live || !found {
		t.Fatalf("after removal: live %v found %v", live, found)
	}
}
