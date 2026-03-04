package campaigns

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mock Repository ---

type mockCampaignRepo struct {
	createFn                func(ctx context.Context, campaign *Campaign) error
	findByIDFn              func(ctx context.Context, id string) (*Campaign, error)
	findBySlugFn            func(ctx context.Context, slug string) (*Campaign, error)
	listByUserFn            func(ctx context.Context, userID string, opts ListOptions) ([]Campaign, int, error)
	listAllFn               func(ctx context.Context, opts ListOptions) ([]Campaign, int, error)
	listPublicFn            func(ctx context.Context, limit int) ([]Campaign, error)
	updateFn                func(ctx context.Context, campaign *Campaign) error
	deleteFn                func(ctx context.Context, id string) error
	slugExistsFn            func(ctx context.Context, slug string) (bool, error)
	countAllFn              func(ctx context.Context) (int, error)
	addMemberFn             func(ctx context.Context, member *CampaignMember) error
	removeMemberFn          func(ctx context.Context, campaignID, userID string) error
	findMemberFn            func(ctx context.Context, campaignID, userID string) (*CampaignMember, error)
	listMembersFn           func(ctx context.Context, campaignID string) ([]CampaignMember, error)
	updateMemberRoleFn      func(ctx context.Context, campaignID, userID string, role Role) error
	findOwnerMemberFn       func(ctx context.Context, campaignID string) (*CampaignMember, error)
	createTransferFn        func(ctx context.Context, transfer *OwnershipTransfer) error
	findTransferByTokenFn   func(ctx context.Context, token string) (*OwnershipTransfer, error)
	findTransferByCampaignFn func(ctx context.Context, campaignID string) (*OwnershipTransfer, error)
	deleteTransferFn        func(ctx context.Context, id string) error
	updateSidebarConfigFn   func(ctx context.Context, campaignID, configJSON string) error
	updateDashboardLayoutFn func(ctx context.Context, campaignID string, layoutJSON *string) error
	transferOwnershipFn     func(ctx context.Context, campaignID, fromUserID, toUserID string) error
	forceTransferOwnershipFn func(ctx context.Context, campaignID, newOwnerID string) error
}

func (m *mockCampaignRepo) Create(ctx context.Context, campaign *Campaign) error {
	if m.createFn != nil {
		return m.createFn(ctx, campaign)
	}
	return nil
}

func (m *mockCampaignRepo) FindByID(ctx context.Context, id string) (*Campaign, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return &Campaign{ID: id, Name: "Test Campaign", Slug: "test-campaign", Settings: "{}", SidebarConfig: "{}"}, nil
}

func (m *mockCampaignRepo) FindBySlug(ctx context.Context, slug string) (*Campaign, error) {
	if m.findBySlugFn != nil {
		return m.findBySlugFn(ctx, slug)
	}
	return nil, apperror.NewNotFound("campaign not found")
}

func (m *mockCampaignRepo) ListByUser(ctx context.Context, userID string, opts ListOptions) ([]Campaign, int, error) {
	if m.listByUserFn != nil {
		return m.listByUserFn(ctx, userID, opts)
	}
	return nil, 0, nil
}

func (m *mockCampaignRepo) ListAll(ctx context.Context, opts ListOptions) ([]Campaign, int, error) {
	if m.listAllFn != nil {
		return m.listAllFn(ctx, opts)
	}
	return nil, 0, nil
}

func (m *mockCampaignRepo) ListPublic(ctx context.Context, limit int) ([]Campaign, error) {
	if m.listPublicFn != nil {
		return m.listPublicFn(ctx, limit)
	}
	return nil, nil
}

func (m *mockCampaignRepo) Update(ctx context.Context, campaign *Campaign) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, campaign)
	}
	return nil
}

func (m *mockCampaignRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockCampaignRepo) SlugExists(ctx context.Context, slug string) (bool, error) {
	if m.slugExistsFn != nil {
		return m.slugExistsFn(ctx, slug)
	}
	return false, nil
}

func (m *mockCampaignRepo) CountAll(ctx context.Context) (int, error) {
	if m.countAllFn != nil {
		return m.countAllFn(ctx)
	}
	return 0, nil
}

func (m *mockCampaignRepo) AddMember(ctx context.Context, member *CampaignMember) error {
	if m.addMemberFn != nil {
		return m.addMemberFn(ctx, member)
	}
	return nil
}

func (m *mockCampaignRepo) RemoveMember(ctx context.Context, campaignID, userID string) error {
	if m.removeMemberFn != nil {
		return m.removeMemberFn(ctx, campaignID, userID)
	}
	return nil
}

func (m *mockCampaignRepo) FindMember(ctx context.Context, campaignID, userID string) (*CampaignMember, error) {
	if m.findMemberFn != nil {
		return m.findMemberFn(ctx, campaignID, userID)
	}
	return nil, apperror.NewNotFound("member not found")
}

func (m *mockCampaignRepo) ListMembers(ctx context.Context, campaignID string) ([]CampaignMember, error) {
	if m.listMembersFn != nil {
		return m.listMembersFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockCampaignRepo) UpdateMemberRole(ctx context.Context, campaignID, userID string, role Role) error {
	if m.updateMemberRoleFn != nil {
		return m.updateMemberRoleFn(ctx, campaignID, userID, role)
	}
	return nil
}

func (m *mockCampaignRepo) FindOwnerMember(ctx context.Context, campaignID string) (*CampaignMember, error) {
	if m.findOwnerMemberFn != nil {
		return m.findOwnerMemberFn(ctx, campaignID)
	}
	return nil, apperror.NewNotFound("owner not found")
}

func (m *mockCampaignRepo) CreateTransfer(ctx context.Context, transfer *OwnershipTransfer) error {
	if m.createTransferFn != nil {
		return m.createTransferFn(ctx, transfer)
	}
	return nil
}

func (m *mockCampaignRepo) FindTransferByToken(ctx context.Context, token string) (*OwnershipTransfer, error) {
	if m.findTransferByTokenFn != nil {
		return m.findTransferByTokenFn(ctx, token)
	}
	return nil, apperror.NewNotFound("transfer not found")
}

func (m *mockCampaignRepo) FindTransferByCampaign(ctx context.Context, campaignID string) (*OwnershipTransfer, error) {
	if m.findTransferByCampaignFn != nil {
		return m.findTransferByCampaignFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockCampaignRepo) DeleteTransfer(ctx context.Context, id string) error {
	if m.deleteTransferFn != nil {
		return m.deleteTransferFn(ctx, id)
	}
	return nil
}

func (m *mockCampaignRepo) UpdateSidebarConfig(ctx context.Context, campaignID, configJSON string) error {
	if m.updateSidebarConfigFn != nil {
		return m.updateSidebarConfigFn(ctx, campaignID, configJSON)
	}
	return nil
}

func (m *mockCampaignRepo) UpdateDashboardLayout(ctx context.Context, campaignID string, layoutJSON *string) error {
	if m.updateDashboardLayoutFn != nil {
		return m.updateDashboardLayoutFn(ctx, campaignID, layoutJSON)
	}
	return nil
}

func (m *mockCampaignRepo) TransferOwnership(ctx context.Context, campaignID, fromUserID, toUserID string) error {
	if m.transferOwnershipFn != nil {
		return m.transferOwnershipFn(ctx, campaignID, fromUserID, toUserID)
	}
	return nil
}

func (m *mockCampaignRepo) ForceTransferOwnership(ctx context.Context, campaignID, newOwnerID string) error {
	if m.forceTransferOwnershipFn != nil {
		return m.forceTransferOwnershipFn(ctx, campaignID, newOwnerID)
	}
	return nil
}

// --- Mock UserFinder ---

type mockUserFinder struct {
	findByEmailFn func(ctx context.Context, email string) (*MemberUser, error)
	findByIDFn    func(ctx context.Context, id string) (*MemberUser, error)
}

func (m *mockUserFinder) FindUserByEmail(ctx context.Context, email string) (*MemberUser, error) {
	if m.findByEmailFn != nil {
		return m.findByEmailFn(ctx, email)
	}
	return nil, apperror.NewNotFound("user not found")
}

func (m *mockUserFinder) FindUserByID(ctx context.Context, id string) (*MemberUser, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, apperror.NewNotFound("user not found")
}

// --- Mock MailService ---

type mockMailService struct {
	sendMailFn     func(ctx context.Context, to []string, subject, body string) error
	isConfiguredFn func(ctx context.Context) bool
}

func (m *mockMailService) SendMail(ctx context.Context, to []string, subject, body string) error {
	if m.sendMailFn != nil {
		return m.sendMailFn(ctx, to, subject, body)
	}
	return nil
}

func (m *mockMailService) IsConfigured(ctx context.Context) bool {
	if m.isConfiguredFn != nil {
		return m.isConfiguredFn(ctx)
	}
	return false
}

// --- Mock EntityTypeSeeder ---

type mockSeeder struct {
	seedDefaultsFn func(ctx context.Context, campaignID string) error
}

func (m *mockSeeder) SeedDefaults(ctx context.Context, campaignID string) error {
	if m.seedDefaultsFn != nil {
		return m.seedDefaultsFn(ctx, campaignID)
	}
	return nil
}

// --- Test Helpers ---

func newTestCampaignService(repo *mockCampaignRepo, users *mockUserFinder) CampaignService {
	return NewCampaignService(repo, users, nil, nil, "http://localhost:8080")
}

func newTestCampaignServiceFull(repo *mockCampaignRepo, users *mockUserFinder, mail MailService, seeder EntityTypeSeeder) CampaignService {
	return NewCampaignService(repo, users, mail, seeder, "http://localhost:8080")
}

// assertAppError checks that an error is an AppError with the expected code.
func assertAppError(t *testing.T, err error, expectedCode int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != expectedCode {
		t.Errorf("expected error code %d, got %d (message: %s)", expectedCode, appErr.Code, appErr.Message)
	}
}

// ============================================================
// Create Tests
// ============================================================

func TestCreate_Success(t *testing.T) {
	var createdCampaign *Campaign
	var addedMember *CampaignMember
	seederCalled := false

	repo := &mockCampaignRepo{
		createFn: func(_ context.Context, c *Campaign) error {
			createdCampaign = c
			return nil
		},
		addMemberFn: func(_ context.Context, m *CampaignMember) error {
			addedMember = m
			return nil
		},
	}
	seeder := &mockSeeder{
		seedDefaultsFn: func(_ context.Context, _ string) error {
			seederCalled = true
			return nil
		},
	}

	svc := newTestCampaignServiceFull(repo, &mockUserFinder{}, nil, seeder)
	campaign, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{
		Name:        "My Campaign",
		Description: "A great campaign",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if campaign == nil {
		t.Fatal("expected campaign, got nil")
	}
	if campaign.Name != "My Campaign" {
		t.Errorf("expected name 'My Campaign', got %q", campaign.Name)
	}
	if campaign.Slug != "my-campaign" {
		t.Errorf("expected slug 'my-campaign', got %q", campaign.Slug)
	}
	if campaign.Description == nil || *campaign.Description != "A great campaign" {
		t.Error("expected description to be set")
	}
	if createdCampaign == nil {
		t.Fatal("repo.Create was not called")
	}
	if addedMember == nil {
		t.Fatal("repo.AddMember was not called for owner")
	}
	if addedMember.Role != RoleOwner {
		t.Errorf("expected owner role, got %v", addedMember.Role)
	}
	if addedMember.UserID != "user-1" {
		t.Errorf("expected user-1, got %q", addedMember.UserID)
	}
	if !seederCalled {
		t.Error("seeder.SeedDefaults was not called")
	}
}

func TestCreate_EmptyName(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	_, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{Name: ""})
	assertAppError(t, err, 400)
}

func TestCreate_WhitespaceOnlyName(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	_, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{Name: "   "})
	assertAppError(t, err, 400)
}

func TestCreate_NameTooLong(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	longName := make([]byte, 201)
	for i := range longName {
		longName[i] = 'a'
	}
	_, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{Name: string(longName)})
	assertAppError(t, err, 400)
}

func TestCreate_DescriptionTooLong(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	longDesc := make([]byte, 5001)
	for i := range longDesc {
		longDesc[i] = 'a'
	}
	_, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{
		Name:        "Test",
		Description: string(longDesc),
	})
	assertAppError(t, err, 400)
}

func TestCreate_EmptyDescription(t *testing.T) {
	repo := &mockCampaignRepo{}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	campaign, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{
		Name: "Test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if campaign.Description != nil {
		t.Error("expected nil description for empty input")
	}
}

func TestCreate_TrimsName(t *testing.T) {
	var captured *Campaign
	repo := &mockCampaignRepo{
		createFn: func(_ context.Context, c *Campaign) error {
			captured = c
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	_, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{Name: "  Test Campaign  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Name != "Test Campaign" {
		t.Errorf("expected trimmed name 'Test Campaign', got %q", captured.Name)
	}
}

func TestCreate_SeederFailureNonFatal(t *testing.T) {
	seeder := &mockSeeder{
		seedDefaultsFn: func(_ context.Context, _ string) error {
			return errors.New("seeder broke")
		},
	}
	repo := &mockCampaignRepo{}
	svc := newTestCampaignServiceFull(repo, &mockUserFinder{}, nil, seeder)
	campaign, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{Name: "Test"})
	if err != nil {
		t.Fatalf("seeder failure should be non-fatal, got: %v", err)
	}
	if campaign == nil {
		t.Fatal("expected campaign even when seeder fails")
	}
}

func TestCreate_NilSeeder(t *testing.T) {
	repo := &mockCampaignRepo{}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	campaign, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{Name: "Test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if campaign == nil {
		t.Fatal("expected campaign")
	}
}

func TestCreate_SlugDedup(t *testing.T) {
	calls := 0
	repo := &mockCampaignRepo{
		slugExistsFn: func(_ context.Context, slug string) (bool, error) {
			calls++
			// First two slugs taken, third is free.
			if slug == "test" || slug == "test-2" {
				return true, nil
			}
			return false, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	campaign, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{Name: "Test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if campaign.Slug != "test-3" {
		t.Errorf("expected slug 'test-3', got %q", campaign.Slug)
	}
	if calls != 3 {
		t.Errorf("expected 3 slug checks, got %d", calls)
	}
}

func TestCreate_RepoError(t *testing.T) {
	repo := &mockCampaignRepo{
		createFn: func(_ context.Context, _ *Campaign) error {
			return errors.New("db error")
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	_, err := svc.Create(context.Background(), "user-1", CreateCampaignInput{Name: "Test"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// ============================================================
// Update Tests
// ============================================================

func TestUpdate_Success(t *testing.T) {
	repo := &mockCampaignRepo{
		findByIDFn: func(_ context.Context, _ string) (*Campaign, error) {
			return &Campaign{ID: "camp-1", Name: "Old Name", Slug: "old-name", Settings: "{}", SidebarConfig: "{}"}, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	campaign, err := svc.Update(context.Background(), "camp-1", UpdateCampaignInput{
		Name:        "New Name",
		Description: "New description",
		IsPublic:    true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if campaign.Name != "New Name" {
		t.Errorf("expected 'New Name', got %q", campaign.Name)
	}
	if campaign.Slug != "new-name" {
		t.Errorf("expected regenerated slug 'new-name', got %q", campaign.Slug)
	}
	if !campaign.IsPublic {
		t.Error("expected IsPublic to be true")
	}
}

func TestUpdate_KeepsSlugWhenNameUnchanged(t *testing.T) {
	repo := &mockCampaignRepo{
		findByIDFn: func(_ context.Context, _ string) (*Campaign, error) {
			return &Campaign{ID: "camp-1", Name: "Same Name", Slug: "same-name", Settings: "{}", SidebarConfig: "{}"}, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	campaign, err := svc.Update(context.Background(), "camp-1", UpdateCampaignInput{
		Name:        "Same Name",
		Description: "Updated desc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if campaign.Slug != "same-name" {
		t.Errorf("expected slug to stay 'same-name', got %q", campaign.Slug)
	}
}

func TestUpdate_EmptyName(t *testing.T) {
	repo := &mockCampaignRepo{
		findByIDFn: func(_ context.Context, _ string) (*Campaign, error) {
			return &Campaign{ID: "camp-1", Name: "Test", Slug: "test", Settings: "{}", SidebarConfig: "{}"}, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	_, err := svc.Update(context.Background(), "camp-1", UpdateCampaignInput{Name: ""})
	assertAppError(t, err, 400)
}

func TestUpdate_NotFound(t *testing.T) {
	repo := &mockCampaignRepo{
		findByIDFn: func(_ context.Context, _ string) (*Campaign, error) {
			return nil, apperror.NewNotFound("not found")
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	_, err := svc.Update(context.Background(), "nonexistent", UpdateCampaignInput{Name: "Test"})
	assertAppError(t, err, 404)
}

func TestUpdate_ClearsDescription(t *testing.T) {
	desc := "old desc"
	repo := &mockCampaignRepo{
		findByIDFn: func(_ context.Context, _ string) (*Campaign, error) {
			return &Campaign{ID: "camp-1", Name: "Test", Slug: "test", Description: &desc, Settings: "{}", SidebarConfig: "{}"}, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	campaign, err := svc.Update(context.Background(), "camp-1", UpdateCampaignInput{
		Name:        "Test",
		Description: "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if campaign.Description != nil {
		t.Error("expected nil description when empty string provided")
	}
}

// ============================================================
// List Tests
// ============================================================

func TestList_DefaultPagination(t *testing.T) {
	var capturedOpts ListOptions
	repo := &mockCampaignRepo{
		listByUserFn: func(_ context.Context, _ string, opts ListOptions) ([]Campaign, int, error) {
			capturedOpts = opts
			return nil, 0, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	_, _, _ = svc.List(context.Background(), "user-1", ListOptions{PerPage: 0})
	if capturedOpts.PerPage != 24 {
		t.Errorf("expected default PerPage 24, got %d", capturedOpts.PerPage)
	}
}

func TestList_ClampsPerPage(t *testing.T) {
	var capturedOpts ListOptions
	repo := &mockCampaignRepo{
		listByUserFn: func(_ context.Context, _ string, opts ListOptions) ([]Campaign, int, error) {
			capturedOpts = opts
			return nil, 0, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	_, _, _ = svc.List(context.Background(), "user-1", ListOptions{PerPage: 500})
	if capturedOpts.PerPage != 24 {
		t.Errorf("expected clamped PerPage 24, got %d", capturedOpts.PerPage)
	}
}

func TestList_ClampsPage(t *testing.T) {
	var capturedOpts ListOptions
	repo := &mockCampaignRepo{
		listByUserFn: func(_ context.Context, _ string, opts ListOptions) ([]Campaign, int, error) {
			capturedOpts = opts
			return nil, 0, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	_, _, _ = svc.List(context.Background(), "user-1", ListOptions{Page: -1, PerPage: 24})
	if capturedOpts.Page != 1 {
		t.Errorf("expected clamped Page 1, got %d", capturedOpts.Page)
	}
}

func TestListPublic_ClampsLimit(t *testing.T) {
	var capturedLimit int
	repo := &mockCampaignRepo{
		listPublicFn: func(_ context.Context, limit int) ([]Campaign, error) {
			capturedLimit = limit
			return nil, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	_, _ = svc.ListPublic(context.Background(), 999)
	if capturedLimit != 12 {
		t.Errorf("expected clamped limit 12, got %d", capturedLimit)
	}
}

func TestListPublic_DefaultsZero(t *testing.T) {
	var capturedLimit int
	repo := &mockCampaignRepo{
		listPublicFn: func(_ context.Context, limit int) ([]Campaign, error) {
			capturedLimit = limit
			return nil, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	_, _ = svc.ListPublic(context.Background(), 0)
	if capturedLimit != 12 {
		t.Errorf("expected default limit 12, got %d", capturedLimit)
	}
}

// ============================================================
// Membership Tests
// ============================================================

func TestAddMember_Success(t *testing.T) {
	var capturedMember *CampaignMember
	repo := &mockCampaignRepo{
		addMemberFn: func(_ context.Context, m *CampaignMember) error {
			capturedMember = m
			return nil
		},
	}
	users := &mockUserFinder{
		findByEmailFn: func(_ context.Context, email string) (*MemberUser, error) {
			return &MemberUser{ID: "user-2", Email: email}, nil
		},
	}
	svc := newTestCampaignService(repo, users)
	err := svc.AddMember(context.Background(), "camp-1", "player@example.com", RolePlayer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedMember == nil {
		t.Fatal("AddMember was not called")
	}
	if capturedMember.Role != RolePlayer {
		t.Errorf("expected Player role, got %v", capturedMember.Role)
	}
	if capturedMember.UserID != "user-2" {
		t.Errorf("expected user-2, got %q", capturedMember.UserID)
	}
}

func TestAddMember_InvalidRole(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.AddMember(context.Background(), "camp-1", "test@example.com", RoleNone)
	assertAppError(t, err, 400)
}

func TestAddMember_OwnerRoleForbidden(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.AddMember(context.Background(), "camp-1", "test@example.com", RoleOwner)
	assertAppError(t, err, 400)
}

func TestAddMember_UserNotFound(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.AddMember(context.Background(), "camp-1", "nobody@example.com", RolePlayer)
	assertAppError(t, err, 400)
}

func TestAddMember_AlreadyMember(t *testing.T) {
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, _ string) (*CampaignMember, error) {
			return &CampaignMember{Role: RolePlayer}, nil
		},
	}
	users := &mockUserFinder{
		findByEmailFn: func(_ context.Context, _ string) (*MemberUser, error) {
			return &MemberUser{ID: "user-2"}, nil
		},
	}
	svc := newTestCampaignService(repo, users)
	err := svc.AddMember(context.Background(), "camp-1", "existing@example.com", RolePlayer)
	assertAppError(t, err, 409)
}

func TestAddMember_NormalizesEmail(t *testing.T) {
	var capturedEmail string
	users := &mockUserFinder{
		findByEmailFn: func(_ context.Context, email string) (*MemberUser, error) {
			capturedEmail = email
			return &MemberUser{ID: "user-2", Email: email}, nil
		},
	}
	svc := newTestCampaignService(&mockCampaignRepo{}, users)
	_ = svc.AddMember(context.Background(), "camp-1", "  TEST@Example.COM  ", RoleScribe)
	if capturedEmail != "test@example.com" {
		t.Errorf("expected normalized email 'test@example.com', got %q", capturedEmail)
	}
}

func TestRemoveMember_Success(t *testing.T) {
	removeCalled := false
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, _ string) (*CampaignMember, error) {
			return &CampaignMember{Role: RolePlayer}, nil
		},
		removeMemberFn: func(_ context.Context, _, _ string) error {
			removeCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.RemoveMember(context.Background(), "camp-1", "user-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !removeCalled {
		t.Error("RemoveMember was not called on repo")
	}
}

func TestRemoveMember_OwnerCannotBeRemoved(t *testing.T) {
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, _ string) (*CampaignMember, error) {
			return &CampaignMember{Role: RoleOwner}, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.RemoveMember(context.Background(), "camp-1", "owner-user")
	assertAppError(t, err, 400)
}

func TestRemoveMember_NotFound(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.RemoveMember(context.Background(), "camp-1", "nonexistent")
	assertAppError(t, err, 404)
}

func TestUpdateMemberRole_Success(t *testing.T) {
	updateCalled := false
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, _ string) (*CampaignMember, error) {
			return &CampaignMember{Role: RolePlayer}, nil
		},
		updateMemberRoleFn: func(_ context.Context, _, _ string, _ Role) error {
			updateCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.UpdateMemberRole(context.Background(), "camp-1", "user-2", RoleScribe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updateCalled {
		t.Error("UpdateMemberRole was not called on repo")
	}
}

func TestUpdateMemberRole_InvalidRole(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.UpdateMemberRole(context.Background(), "camp-1", "user-2", RoleNone)
	assertAppError(t, err, 400)
}

func TestUpdateMemberRole_CannotPromoteToOwner(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.UpdateMemberRole(context.Background(), "camp-1", "user-2", RoleOwner)
	assertAppError(t, err, 400)
}

func TestUpdateMemberRole_CannotChangeOwnerRole(t *testing.T) {
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, _ string) (*CampaignMember, error) {
			return &CampaignMember{Role: RoleOwner}, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.UpdateMemberRole(context.Background(), "camp-1", "owner-user", RoleScribe)
	assertAppError(t, err, 400)
}

// ============================================================
// Ownership Transfer Tests
// ============================================================

func TestInitiateTransfer_Success(t *testing.T) {
	var capturedTransfer *OwnershipTransfer
	repo := &mockCampaignRepo{
		createTransferFn: func(_ context.Context, tr *OwnershipTransfer) error {
			capturedTransfer = tr
			return nil
		},
	}
	users := &mockUserFinder{
		findByEmailFn: func(_ context.Context, _ string) (*MemberUser, error) {
			return &MemberUser{ID: "user-2", Email: "target@example.com"}, nil
		},
	}
	svc := newTestCampaignService(repo, users)
	transfer, err := svc.InitiateTransfer(context.Background(), "camp-1", "user-1", "target@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transfer == nil {
		t.Fatal("expected transfer, got nil")
	}
	if capturedTransfer.FromUserID != "user-1" {
		t.Errorf("expected from user-1, got %q", capturedTransfer.FromUserID)
	}
	if capturedTransfer.ToUserID != "user-2" {
		t.Errorf("expected to user-2, got %q", capturedTransfer.ToUserID)
	}
	if capturedTransfer.Token == "" {
		t.Error("expected token to be generated")
	}
	if capturedTransfer.ExpiresAt.Before(time.Now()) {
		t.Error("expected expiry to be in the future")
	}
}

func TestInitiateTransfer_SelfTransfer(t *testing.T) {
	users := &mockUserFinder{
		findByEmailFn: func(_ context.Context, _ string) (*MemberUser, error) {
			return &MemberUser{ID: "user-1"}, nil
		},
	}
	svc := newTestCampaignService(&mockCampaignRepo{}, users)
	_, err := svc.InitiateTransfer(context.Background(), "camp-1", "user-1", "self@example.com")
	assertAppError(t, err, 400)
}

func TestInitiateTransfer_TargetNotFound(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	_, err := svc.InitiateTransfer(context.Background(), "camp-1", "user-1", "nobody@example.com")
	assertAppError(t, err, 400)
}

func TestInitiateTransfer_AlreadyPending(t *testing.T) {
	repo := &mockCampaignRepo{
		findTransferByCampaignFn: func(_ context.Context, _ string) (*OwnershipTransfer, error) {
			return &OwnershipTransfer{ID: "existing"}, nil
		},
	}
	users := &mockUserFinder{
		findByEmailFn: func(_ context.Context, _ string) (*MemberUser, error) {
			return &MemberUser{ID: "user-2"}, nil
		},
	}
	svc := newTestCampaignService(repo, users)
	_, err := svc.InitiateTransfer(context.Background(), "camp-1", "user-1", "target@example.com")
	assertAppError(t, err, 409)
}

func TestInitiateTransfer_SendsEmailWhenConfigured(t *testing.T) {
	emailSent := false
	repo := &mockCampaignRepo{
		findByIDFn: func(_ context.Context, _ string) (*Campaign, error) {
			return &Campaign{ID: "camp-1", Name: "Test Campaign", Settings: "{}", SidebarConfig: "{}"}, nil
		},
	}
	users := &mockUserFinder{
		findByEmailFn: func(_ context.Context, _ string) (*MemberUser, error) {
			return &MemberUser{ID: "user-2"}, nil
		},
	}
	mail := &mockMailService{
		isConfiguredFn: func(_ context.Context) bool { return true },
		sendMailFn: func(_ context.Context, _ []string, _, _ string) error {
			emailSent = true
			return nil
		},
	}
	svc := newTestCampaignServiceFull(repo, users, mail, nil)
	_, err := svc.InitiateTransfer(context.Background(), "camp-1", "user-1", "target@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !emailSent {
		t.Error("expected email to be sent")
	}
}

func TestInitiateTransfer_EmailFailureNonFatal(t *testing.T) {
	repo := &mockCampaignRepo{
		findByIDFn: func(_ context.Context, _ string) (*Campaign, error) {
			return &Campaign{ID: "camp-1", Name: "Test", Settings: "{}", SidebarConfig: "{}"}, nil
		},
	}
	users := &mockUserFinder{
		findByEmailFn: func(_ context.Context, _ string) (*MemberUser, error) {
			return &MemberUser{ID: "user-2"}, nil
		},
	}
	mail := &mockMailService{
		isConfiguredFn: func(_ context.Context) bool { return true },
		sendMailFn: func(_ context.Context, _ []string, _, _ string) error {
			return errors.New("smtp broke")
		},
	}
	svc := newTestCampaignServiceFull(repo, users, mail, nil)
	transfer, err := svc.InitiateTransfer(context.Background(), "camp-1", "user-1", "target@example.com")
	if err != nil {
		t.Fatalf("email failure should be non-fatal, got: %v", err)
	}
	if transfer == nil {
		t.Fatal("expected transfer to succeed despite email failure")
	}
}

func TestAcceptTransfer_Success(t *testing.T) {
	transferCalled := false
	repo := &mockCampaignRepo{
		findTransferByTokenFn: func(_ context.Context, _ string) (*OwnershipTransfer, error) {
			return &OwnershipTransfer{
				ID:         "transfer-1",
				CampaignID: "camp-1",
				FromUserID: "user-1",
				ToUserID:   "user-2",
				ExpiresAt:  time.Now().Add(24 * time.Hour),
			}, nil
		},
		transferOwnershipFn: func(_ context.Context, _, _, _ string) error {
			transferCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.AcceptTransfer(context.Background(), "valid-token", "user-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !transferCalled {
		t.Error("TransferOwnership was not called")
	}
}

func TestAcceptTransfer_InvalidToken(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.AcceptTransfer(context.Background(), "bad-token", "user-2")
	assertAppError(t, err, 400)
}

func TestAcceptTransfer_ExpiredToken(t *testing.T) {
	deleteCalled := false
	repo := &mockCampaignRepo{
		findTransferByTokenFn: func(_ context.Context, _ string) (*OwnershipTransfer, error) {
			return &OwnershipTransfer{
				ID:        "transfer-1",
				ToUserID:  "user-2",
				ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired.
			}, nil
		},
		deleteTransferFn: func(_ context.Context, _ string) error {
			deleteCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.AcceptTransfer(context.Background(), "expired-token", "user-2")
	assertAppError(t, err, 400)
	if !deleteCalled {
		t.Error("expected expired transfer to be cleaned up")
	}
}

func TestAcceptTransfer_WrongUser(t *testing.T) {
	repo := &mockCampaignRepo{
		findTransferByTokenFn: func(_ context.Context, _ string) (*OwnershipTransfer, error) {
			return &OwnershipTransfer{
				ID:        "transfer-1",
				ToUserID:  "user-2",
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.AcceptTransfer(context.Background(), "valid-token", "wrong-user")
	assertAppError(t, err, 403)
}

func TestCancelTransfer_Success(t *testing.T) {
	deleteCalled := false
	repo := &mockCampaignRepo{
		findTransferByCampaignFn: func(_ context.Context, _ string) (*OwnershipTransfer, error) {
			return &OwnershipTransfer{ID: "transfer-1"}, nil
		},
		deleteTransferFn: func(_ context.Context, _ string) error {
			deleteCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.CancelTransfer(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleteCalled {
		t.Error("DeleteTransfer was not called")
	}
}

func TestCancelTransfer_NoPending(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.CancelTransfer(context.Background(), "camp-1")
	assertAppError(t, err, 404)
}

// ============================================================
// Sidebar Config Tests
// ============================================================

func TestUpdateSidebarConfig_Success(t *testing.T) {
	updateCalled := false
	repo := &mockCampaignRepo{
		updateSidebarConfigFn: func(_ context.Context, _, _ string) error {
			updateCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.UpdateSidebarConfig(context.Background(), "camp-1", SidebarConfig{
		EntityTypeOrder: []int{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updateCalled {
		t.Error("UpdateSidebarConfig was not called")
	}
}

func TestUpdateSidebarConfig_EntityTypeOrderTooLong(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	longOrder := make([]int, 101)
	err := svc.UpdateSidebarConfig(context.Background(), "camp-1", SidebarConfig{
		EntityTypeOrder: longOrder,
	})
	assertAppError(t, err, 400)
}

func TestUpdateSidebarConfig_HiddenTooLong(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	longHidden := make([]int, 101)
	err := svc.UpdateSidebarConfig(context.Background(), "camp-1", SidebarConfig{
		HiddenTypeIDs: longHidden,
	})
	assertAppError(t, err, 400)
}

// ============================================================
// Dashboard Layout Tests
// ============================================================

func TestUpdateDashboardLayout_Success(t *testing.T) {
	updateCalled := false
	repo := &mockCampaignRepo{
		updateDashboardLayoutFn: func(_ context.Context, _ string, _ *string) error {
			updateCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.UpdateDashboardLayout(context.Background(), "camp-1", &DashboardLayout{
		Rows: []DashboardRow{
			{
				Columns: []DashboardColumn{
					{Width: 6, Blocks: []DashboardBlock{{Type: BlockWelcomeBanner}}},
					{Width: 6, Blocks: []DashboardBlock{{Type: BlockRecentPages}}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updateCalled {
		t.Error("UpdateDashboardLayout was not called")
	}
}

func TestUpdateDashboardLayout_NilResetsToDefault(t *testing.T) {
	var captured *string
	repo := &mockCampaignRepo{
		updateDashboardLayoutFn: func(_ context.Context, _ string, layoutJSON *string) error {
			captured = layoutJSON
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.UpdateDashboardLayout(context.Background(), "camp-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured != nil {
		t.Error("expected nil layout to be passed to repo")
	}
}

func TestUpdateDashboardLayout_TooManyRows(t *testing.T) {
	rows := make([]DashboardRow, 51)
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.UpdateDashboardLayout(context.Background(), "camp-1", &DashboardLayout{Rows: rows})
	assertAppError(t, err, 400)
}

func TestUpdateDashboardLayout_ColumnWidthTooSmall(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.UpdateDashboardLayout(context.Background(), "camp-1", &DashboardLayout{
		Rows: []DashboardRow{
			{Columns: []DashboardColumn{{Width: 0}}},
		},
	})
	assertAppError(t, err, 400)
}

func TestUpdateDashboardLayout_ColumnWidthTooLarge(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.UpdateDashboardLayout(context.Background(), "camp-1", &DashboardLayout{
		Rows: []DashboardRow{
			{Columns: []DashboardColumn{{Width: 13}}},
		},
	})
	assertAppError(t, err, 400)
}

func TestUpdateDashboardLayout_TotalWidthExceeds12(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.UpdateDashboardLayout(context.Background(), "camp-1", &DashboardLayout{
		Rows: []DashboardRow{
			{Columns: []DashboardColumn{
				{Width: 8},
				{Width: 6},
			}},
		},
	})
	assertAppError(t, err, 400)
}

func TestUpdateDashboardLayout_InvalidBlockType(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.UpdateDashboardLayout(context.Background(), "camp-1", &DashboardLayout{
		Rows: []DashboardRow{
			{Columns: []DashboardColumn{
				{Width: 12, Blocks: []DashboardBlock{{Type: "invalid_type"}}},
			}},
		},
	})
	assertAppError(t, err, 400)
}

func TestUpdateDashboardLayout_TooManyBlocksPerRow(t *testing.T) {
	blocks := make([]DashboardBlock, 21)
	for i := range blocks {
		blocks[i] = DashboardBlock{Type: BlockTextBlock}
	}
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.UpdateDashboardLayout(context.Background(), "camp-1", &DashboardLayout{
		Rows: []DashboardRow{
			{Columns: []DashboardColumn{
				{Width: 12, Blocks: blocks},
			}},
		},
	})
	assertAppError(t, err, 400)
}

func TestResetDashboardLayout_Success(t *testing.T) {
	var captured *string
	capturedCalled := false
	repo := &mockCampaignRepo{
		updateDashboardLayoutFn: func(_ context.Context, _ string, layoutJSON *string) error {
			captured = layoutJSON
			capturedCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.ResetDashboardLayout(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !capturedCalled {
		t.Error("UpdateDashboardLayout was not called")
	}
	if captured != nil {
		t.Error("expected nil layout for reset")
	}
}

// ============================================================
// Admin Operations Tests
// ============================================================

func TestForceTransferOwnership_Success(t *testing.T) {
	transferCalled := false
	repo := &mockCampaignRepo{
		forceTransferOwnershipFn: func(_ context.Context, _, _ string) error {
			transferCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.ForceTransferOwnership(context.Background(), "camp-1", "admin-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !transferCalled {
		t.Error("ForceTransferOwnership was not called on repo")
	}
}

func TestForceTransferOwnership_RepoError(t *testing.T) {
	repo := &mockCampaignRepo{
		forceTransferOwnershipFn: func(_ context.Context, _, _ string) error {
			return errors.New("db error")
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.ForceTransferOwnership(context.Background(), "camp-1", "admin-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAdminAddMember_NewMember(t *testing.T) {
	var capturedMember *CampaignMember
	repo := &mockCampaignRepo{
		addMemberFn: func(_ context.Context, m *CampaignMember) error {
			capturedMember = m
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.AdminAddMember(context.Background(), "camp-1", "user-2", RoleScribe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedMember == nil {
		t.Fatal("AddMember was not called")
	}
	if capturedMember.Role != RoleScribe {
		t.Errorf("expected Scribe role, got %v", capturedMember.Role)
	}
}

func TestAdminAddMember_InvalidRole(t *testing.T) {
	svc := newTestCampaignService(&mockCampaignRepo{}, &mockUserFinder{})
	err := svc.AdminAddMember(context.Background(), "camp-1", "user-2", RoleNone)
	assertAppError(t, err, 400)
}

func TestAdminAddMember_ExistingSameRole(t *testing.T) {
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, _ string) (*CampaignMember, error) {
			return &CampaignMember{Role: RoleScribe}, nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.AdminAddMember(context.Background(), "camp-1", "user-2", RoleScribe)
	if err != nil {
		t.Fatalf("expected no error for same role, got: %v", err)
	}
}

func TestAdminAddMember_ExistingDifferentRole(t *testing.T) {
	updateCalled := false
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, _ string) (*CampaignMember, error) {
			return &CampaignMember{Role: RolePlayer}, nil
		},
		updateMemberRoleFn: func(_ context.Context, _, _ string, _ Role) error {
			updateCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.AdminAddMember(context.Background(), "camp-1", "user-2", RoleScribe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updateCalled {
		t.Error("UpdateMemberRole was not called")
	}
}

func TestAdminAddMember_PromoteToOwnerTriggersForceTransfer(t *testing.T) {
	forceTransferCalled := false
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, _ string) (*CampaignMember, error) {
			return &CampaignMember{Role: RoleScribe}, nil
		},
		forceTransferOwnershipFn: func(_ context.Context, _, _ string) error {
			forceTransferCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.AdminAddMember(context.Background(), "camp-1", "user-2", RoleOwner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !forceTransferCalled {
		t.Error("ForceTransferOwnership was not called for Owner promotion")
	}
}

func TestAdminAddMember_NewMemberAsOwnerTriggersForceTransfer(t *testing.T) {
	forceTransferCalled := false
	repo := &mockCampaignRepo{
		forceTransferOwnershipFn: func(_ context.Context, _, _ string) error {
			forceTransferCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.AdminAddMember(context.Background(), "camp-1", "user-2", RoleOwner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !forceTransferCalled {
		t.Error("ForceTransferOwnership was not called for new Owner member")
	}
}

// ============================================================
// Delete Tests
// ============================================================

func TestDelete_Success(t *testing.T) {
	deleteCalled := false
	repo := &mockCampaignRepo{
		deleteFn: func(_ context.Context, _ string) error {
			deleteCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.Delete(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleteCalled {
		t.Error("Delete was not called on repo")
	}
}

func TestDelete_RepoError(t *testing.T) {
	repo := &mockCampaignRepo{
		deleteFn: func(_ context.Context, _ string) error {
			return apperror.NewNotFound("campaign not found")
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	err := svc.Delete(context.Background(), "nonexistent")
	assertAppError(t, err, 404)
}

// ============================================================
// Model Tests
// ============================================================

func TestSlugify(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "Hello World", "hello-world"},
		{"special chars", "My Campaign! #1", "my-campaign-1"},
		{"leading trailing spaces", "  Test  ", "test"},
		{"multiple spaces", "A   B   C", "a-b-c"},
		{"numbers", "Campaign 42", "campaign-42"},
		{"all special", "!@#$%", "campaign"},
		{"empty", "", "campaign"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Slugify(tt.input)
			if got != tt.expected {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRole_IsValid(t *testing.T) {
	tests := []struct {
		role  Role
		valid bool
	}{
		{RoleNone, false},
		{RolePlayer, true},
		{RoleScribe, true},
		{RoleOwner, true},
		{Role(99), false},
		{Role(-1), false},
	}
	for _, tt := range tests {
		if got := tt.role.IsValid(); got != tt.valid {
			t.Errorf("Role(%d).IsValid() = %v, want %v", tt.role, got, tt.valid)
		}
	}
}

func TestRole_String(t *testing.T) {
	tests := []struct {
		role     Role
		expected string
	}{
		{RoleOwner, "owner"},
		{RoleScribe, "scribe"},
		{RolePlayer, "player"},
		{RoleNone, ""},
	}
	for _, tt := range tests {
		if got := tt.role.String(); got != tt.expected {
			t.Errorf("Role(%d).String() = %q, want %q", tt.role, got, tt.expected)
		}
	}
}

func TestRoleFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected Role
	}{
		{"owner", RoleOwner},
		{"scribe", RoleScribe},
		{"player", RolePlayer},
		{"invalid", RoleNone},
		{"", RoleNone},
	}
	for _, tt := range tests {
		if got := RoleFromString(tt.input); got != tt.expected {
			t.Errorf("RoleFromString(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestListOptions_Offset(t *testing.T) {
	tests := []struct {
		name     string
		opts     ListOptions
		expected int
	}{
		{"page 1", ListOptions{Page: 1, PerPage: 24}, 0},
		{"page 2", ListOptions{Page: 2, PerPage: 24}, 24},
		{"page 3 with 10", ListOptions{Page: 3, PerPage: 10}, 20},
		{"page 0 treated as 1", ListOptions{Page: 0, PerPage: 24}, 0},
		{"negative page treated as 1", ListOptions{Page: -1, PerPage: 24}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.Offset(); got != tt.expected {
				t.Errorf("Offset() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestParseSidebarConfig_Empty(t *testing.T) {
	c := &Campaign{SidebarConfig: "{}"}
	cfg := c.ParseSidebarConfig()
	if len(cfg.EntityTypeOrder) != 0 {
		t.Error("expected empty entity type order")
	}
}

func TestParseSidebarConfig_InvalidJSON(t *testing.T) {
	c := &Campaign{SidebarConfig: "invalid"}
	cfg := c.ParseSidebarConfig()
	// Should not panic, returns empty config.
	if len(cfg.EntityTypeOrder) != 0 {
		t.Error("expected empty config on invalid JSON")
	}
}

func TestParseDashboardLayout_Nil(t *testing.T) {
	c := &Campaign{}
	layout := c.ParseDashboardLayout()
	if layout != nil {
		t.Error("expected nil layout when column is NULL")
	}
}

func TestParseDashboardLayout_Empty(t *testing.T) {
	empty := ""
	c := &Campaign{DashboardLayout: &empty}
	layout := c.ParseDashboardLayout()
	if layout != nil {
		t.Error("expected nil layout for empty string")
	}
}

func TestParseDashboardLayout_Valid(t *testing.T) {
	layoutJSON := `{"rows":[{"id":"r1","columns":[{"id":"c1","width":12,"blocks":[{"id":"b1","type":"welcome_banner"}]}]}]}`
	c := &Campaign{DashboardLayout: &layoutJSON}
	layout := c.ParseDashboardLayout()
	if layout == nil {
		t.Fatal("expected parsed layout")
	}
	if len(layout.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(layout.Rows))
	}
}
