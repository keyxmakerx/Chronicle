package app

// trash_followup_integration_test.go covers the second round of site Trash
// safety rules against a real database: the claim re-checks age in SQL, the
// sessions token routes and the write paths refuse a trashed campaign, and the
// retention setting needs the password re-check.
//
// Skipped under -short and without CHRONICLE_TEST_DB_DSN.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/admin"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/settings"
)

func TestSiteTrash_ClaimRechecksAgeInSQL(t *testing.T) {
	fx := newTrashFixture(t)
	ctx := context.Background()
	cutoff := time.Now().UTC().AddDate(0, 0, -30)

	if err := fx.campaigns.MoveToTrash(ctx, fx.campaignID, fx.adminID); err != nil {
		t.Fatal(err)
	}
	mustGalleryExec(t, fx.db, `UPDATE campaigns SET deleted_at = ? WHERE id = ?`, time.Now().UTC().AddDate(0, 0, -40), fx.campaignID)

	// The run lists the campaign as due ...
	ids, err := fx.campaigns.ListPurgeDue(ctx, cutoff, false)
	if err != nil || len(ids) != 1 {
		t.Fatalf("ListPurgeDue = %v, %v; want the aged campaign", ids, err)
	}
	// ... then an admin undoes it and someone trashes it again ...
	if err := fx.campaigns.RestoreFromTrash(ctx, fx.campaignID); err != nil {
		t.Fatal(err)
	}
	if err := fx.campaigns.MoveToTrash(ctx, fx.campaignID, fx.ownerID); err != nil {
		t.Fatal(err)
	}
	// ... and the claim, with the run's cutoff, must leave it alone.
	purged, err := fx.campaigns.PurgeTrashed(ctx, fx.campaignID, cutoff)
	if err != nil || purged {
		t.Fatalf("PurgeTrashed after re-trash = %v, %v; want false, nil", purged, err)
	}
	if !fx.rowExists("campaigns", fx.campaignID) {
		t.Fatal("a re-trashed campaign was purged by a stale run")
	}
	var started *time.Time
	if err := fx.db.QueryRow(`SELECT purge_started_at FROM campaigns WHERE id = ?`, fx.campaignID).Scan(&started); err != nil || started != nil {
		t.Fatalf("purge_started_at = %v, %v; want unset so Undo still works", started, err)
	}
	if err := fx.campaigns.RestoreFromTrash(ctx, fx.campaignID); err != nil {
		t.Errorf("Undo after the refused claim: %v", err)
	}

	// Empty now has no age condition, but still needs the campaign to be in
	// the Trash.
	if purged, err := fx.campaigns.PurgeTrashed(ctx, fx.campaignID, time.Time{}); err != nil || purged {
		t.Errorf("zero-cutoff purge of a live campaign = %v, %v; want false, nil", purged, err)
	}
	if err := fx.campaigns.MoveToTrash(ctx, fx.campaignID, fx.ownerID); err != nil {
		t.Fatal(err)
	}
	if purged, err := fx.campaigns.PurgeTrashed(ctx, fx.campaignID, time.Time{}); err != nil || !purged {
		t.Errorf("zero-cutoff purge of a trashed campaign = %v, %v; want true", purged, err)
	}

	// The same rule for a file clean-up batch.
	batches := admin.NewTrashBatchRepository(fx.db)
	b := &admin.TrashBatch{ID: galleryTestUUID(t), Kind: "files", Label: "files", CreatedAt: time.Now().UTC()}
	if err := batches.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	if ok, err := batches.Claim(ctx, b.ID, "trashed", "purging", cutoff); err != nil || ok {
		t.Errorf("claiming a young batch with an old cutoff = %v, %v; want false", ok, err)
	}
	if ok, err := batches.Claim(ctx, b.ID, "trashed", "purging", time.Now().UTC().Add(time.Minute)); err != nil || !ok {
		t.Errorf("claiming an aged batch = %v, %v; want true", ok, err)
	}
}

// feedStub is the sessions service as far as the iCal feed needs it.
type feedStub struct {
	sessions.SessionService
	campaignID, userID string
	listed             bool
}

func (f *feedStub) ResolveFeedToken(context.Context, string) (string, string, error) {
	return f.campaignID, f.userID, nil
}
func (f *feedStub) ListFeedSessions(context.Context, string) ([]sessions.Session, error) {
	f.listed = true
	return nil, nil
}
func (f *feedStub) BuildFeedICS([]sessions.Session) string { return "BEGIN:VCALENDAR\nEND:VCALENDAR\n" }

func TestSiteTrash_SessionsTokenRoutesStopForATrashedCampaign(t *testing.T) {
	fx := newTrashFixture(t)
	ctx := context.Background()
	stub := &feedStub{campaignID: fx.campaignID, userID: fx.memberID}
	h := sessions.NewHandler(stub)
	h.SetMemberLister(fx.campaigns)
	e := trashEcho()
	e.GET("/sessions/feed/:token", h.GameNightFeedICS)

	if got := trashStatus(e, http.MethodGet, "/sessions/feed/tok.ics", ""); got != http.StatusOK {
		t.Fatalf("feed before trashing = %d, want 200", got)
	}
	if err := fx.campaigns.MoveToTrash(ctx, fx.campaignID, fx.adminID); err != nil {
		t.Fatal(err)
	}
	stub.listed = false
	if got := trashStatus(e, http.MethodGet, "/sessions/feed/tok.ics", ""); got != http.StatusNotFound {
		t.Errorf("feed of a trashed campaign = %d, want 404", got)
	}
	if stub.listed {
		t.Error("the feed read the trashed campaign's sessions")
	}
	if members, err := fx.campaigns.ListMembers(ctx, fx.campaignID); err != nil || len(members) != 0 {
		t.Errorf("ListMembers of a trashed campaign = %d, %v; want none", len(members), err)
	}
	if err := fx.campaigns.RestoreFromTrash(ctx, fx.campaignID); err != nil {
		t.Fatal(err)
	}
	if got := trashStatus(e, http.MethodGet, "/sessions/feed/tok.ics", ""); got != http.StatusOK {
		t.Errorf("feed after Undo = %d, want 200", got)
	}
}

func TestSiteTrash_NoWritesIntoATrashedCampaign(t *testing.T) {
	fx := newTrashFixture(t)
	ctx := context.Background()

	newcomer := galleryTestUUID(t)
	mustGalleryExec(t, fx.db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, 'New', 'x')`, newcomer, newcomer+"@example.test")
	mustGalleryExec(t, fx.db, `INSERT INTO campaign_invites (id, campaign_id, email, role, token, created_by, expires_at) VALUES (?, ?, ?, 'player', 'invite-tok', ?, ?)`,
		galleryTestUUID(t), fx.campaignID, newcomer+"@example.test", fx.ownerID, time.Now().UTC().Add(time.Hour))
	mustGalleryExec(t, fx.db, `INSERT INTO ownership_transfers (id, campaign_id, from_user_id, to_user_id, token, expires_at) VALUES (?, ?, ?, ?, 'xfer-tok', ?)`,
		galleryTestUUID(t), fx.campaignID, fx.ownerID, fx.memberID, time.Now().UTC().Add(time.Hour))

	invites := campaigns.NewInviteService(campaigns.NewInviteRepository(fx.db), campaigns.NewCampaignRepository(fx.db), nil, "http://localhost")

	if err := fx.campaigns.MoveToTrash(ctx, fx.campaignID, fx.adminID); err != nil {
		t.Fatal(err)
	}
	if _, err := invites.AcceptInvite(ctx, "invite-tok", newcomer); !isNotFoundErr(err) {
		t.Errorf("AcceptInvite into a trashed campaign = %v, want not found", err)
	}
	var n int
	if err := fx.db.QueryRow(`SELECT COUNT(*) FROM campaign_members WHERE campaign_id = ? AND user_id = ?`, fx.campaignID, newcomer).Scan(&n); err != nil || n != 0 {
		t.Errorf("the newcomer was added to a trashed campaign (%d, %v)", n, err)
	}
	if err := fx.campaigns.AcceptTransfer(ctx, "xfer-tok", fx.memberID); !isNotFoundErr(err) {
		t.Errorf("AcceptTransfer on a trashed campaign = %v, want not found", err)
	}
	var role string
	if err := fx.db.QueryRow(`SELECT role FROM campaign_members WHERE campaign_id = ? AND user_id = ?`, fx.campaignID, fx.ownerID).Scan(&role); err != nil || role != "owner" {
		t.Errorf("owner role = %q, %v; want unchanged", role, err)
	}

	// The Foundry auto-pin never lists a trashed campaign.
	repo := foundry_vtt.NewRepository(fx.db)
	listed, err := repo.CampaignsWithEmptyPin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range listed {
		if c.CampaignID == fx.campaignID {
			t.Error("CampaignsWithEmptyPin lists a trashed campaign")
		}
	}

	// Once Undone, both work again.
	if err := fx.campaigns.RestoreFromTrash(ctx, fx.campaignID); err != nil {
		t.Fatal(err)
	}
	listed, _ = repo.CampaignsWithEmptyPin(ctx)
	found := false
	for _, c := range listed {
		found = found || c.CampaignID == fx.campaignID
	}
	if !found {
		t.Error("an undone campaign is missing from the auto-pin list")
	}
	if _, err := invites.AcceptInvite(ctx, "invite-tok", newcomer); err != nil {
		t.Errorf("AcceptInvite after Undo: %v", err)
	}
}

func TestSiteTrash_RetentionChangeNeedsThePasswordRecheck(t *testing.T) {
	fx := newTrashFixture(t)
	ctx := context.Background()
	settingsSvc := settings.NewSettingsService(settings.NewSettingsRepository(fx.db))

	authStub := &trashAuth{adminID: fx.adminID}
	h := admin.NewHandler(nil, fx.campaigns, nil)
	h.SetTrashService(admin.NewTrashService(fx.campaigns, fx.media, trashFinder{}, admin.NewTrashBatchRepository(fx.db), settingsSvc))
	h.SetSettingsDeps(settingsSvc)
	e := trashEcho()
	admin.RegisterRoutes(e, h, authStub, nil)

	post := func() *httptest.ResponseRecorder {
		form := url.Values{"trash_retention_days": {"7"}}
		req := httptest.NewRequest(http.MethodPost, "/admin/trash/retention", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: fx.adminID})
		req.Header.Set("HX-Request", "true")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	rec := post()
	if rec.Code != http.StatusForbidden || rec.Header().Get("HX-Trigger") != "reauth-required" {
		t.Fatalf("without re-check: %d, HX-Trigger %q; want 403 reauth-required", rec.Code, rec.Header().Get("HX-Trigger"))
	}
	if days, err := settingsSvc.SiteTrashRetentionDays(ctx); err != nil || days != settings.DefaultSiteTrashRetentionDays {
		t.Fatalf("retention after a refused change = %d, %v; want the default", days, err)
	}

	authStub.reauth = true
	if rec := post(); rec.Code != http.StatusOK {
		t.Fatalf("with re-check: %d, body %s", rec.Code, rec.Body.String())
	}
	if days, err := settingsSvc.SiteTrashRetentionDays(ctx); err != nil || days != 7 {
		t.Errorf("retention after the change = %d, %v; want 7", days, err)
	}
}
