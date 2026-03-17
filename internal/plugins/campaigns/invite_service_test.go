package campaigns

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

// --- Mock Invite Repository ---

type mockInviteRepo struct {
	invites map[string]*Invite // keyed by ID
}

func newMockInviteRepo() *mockInviteRepo {
	return &mockInviteRepo{invites: make(map[string]*Invite)}
}

func (m *mockInviteRepo) Create(_ context.Context, inv *Invite) error {
	m.invites[inv.ID] = inv
	return nil
}

func (m *mockInviteRepo) GetByToken(_ context.Context, token string) (*Invite, error) {
	for _, inv := range m.invites {
		if inv.Token == token {
			return inv, nil
		}
	}
	return nil, fmt.Errorf("fetching invite by token: %w", sql.ErrNoRows)
}

func (m *mockInviteRepo) ListByCampaign(_ context.Context, campaignID string) ([]Invite, error) {
	var result []Invite
	for _, inv := range m.invites {
		if inv.CampaignID == campaignID {
			result = append(result, *inv)
		}
	}
	return result, nil
}

func (m *mockInviteRepo) MarkAccepted(_ context.Context, id string) error {
	if inv, ok := m.invites[id]; ok {
		now := time.Now().UTC()
		inv.AcceptedAt = &now
	}
	return nil
}

func (m *mockInviteRepo) Delete(_ context.Context, id string) error {
	delete(m.invites, id)
	return nil
}

func (m *mockInviteRepo) DeleteExpired(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (m *mockInviteRepo) GetByEmailAndCampaign(_ context.Context, email, campaignID string) (*Invite, error) {
	for _, inv := range m.invites {
		if inv.Email == email && inv.CampaignID == campaignID && inv.AcceptedAt == nil && !inv.IsExpired() {
			return inv, nil
		}
	}
	return nil, fmt.Errorf("fetching invite by email: %w", sql.ErrNoRows)
}

// --- Mock Campaign Repository (minimal for invite tests) ---

type mockCampaignRepoForInvites struct {
	campaigns map[string]*Campaign
	members   map[string]map[string]*CampaignMember // campaignID -> userID -> member
}

func newMockCampaignRepoForInvites() *mockCampaignRepoForInvites {
	return &mockCampaignRepoForInvites{
		campaigns: map[string]*Campaign{
			"camp-1": {ID: "camp-1", Name: "Test Campaign"},
		},
		members: make(map[string]map[string]*CampaignMember),
	}
}

func (m *mockCampaignRepoForInvites) FindByID(_ context.Context, id string) (*Campaign, error) {
	if c, ok := m.campaigns[id]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("campaign not found")
}

func (m *mockCampaignRepoForInvites) FindMember(_ context.Context, campaignID, userID string) (*CampaignMember, error) {
	if members, ok := m.members[campaignID]; ok {
		if member, ok := members[userID]; ok {
			return member, nil
		}
	}
	return nil, fmt.Errorf("member not found")
}

func (m *mockCampaignRepoForInvites) AddMember(_ context.Context, member *CampaignMember) error {
	if _, ok := m.members[member.CampaignID]; !ok {
		m.members[member.CampaignID] = make(map[string]*CampaignMember)
	}
	m.members[member.CampaignID][member.UserID] = member
	return nil
}

// Unused interface methods — required by CampaignRepository.
func (m *mockCampaignRepoForInvites) Create(context.Context, *Campaign) error { return nil }
func (m *mockCampaignRepoForInvites) FindBySlug(context.Context, string) (*Campaign, error) {
	return nil, nil
}
func (m *mockCampaignRepoForInvites) ListByUser(context.Context, string, ListOptions) ([]Campaign, int, error) {
	return nil, 0, nil
}
func (m *mockCampaignRepoForInvites) ListAll(context.Context, ListOptions) ([]Campaign, int, error) {
	return nil, 0, nil
}
func (m *mockCampaignRepoForInvites) ListPublic(context.Context, int) ([]Campaign, error) {
	return nil, nil
}
func (m *mockCampaignRepoForInvites) Update(context.Context, *Campaign) error { return nil }
func (m *mockCampaignRepoForInvites) Delete(context.Context, string) error    { return nil }
func (m *mockCampaignRepoForInvites) SlugExists(context.Context, string) (bool, error) {
	return false, nil
}
func (m *mockCampaignRepoForInvites) CountAll(context.Context) (int, error) { return 0, nil }
func (m *mockCampaignRepoForInvites) RemoveMember(context.Context, string, string) error {
	return nil
}
func (m *mockCampaignRepoForInvites) ListMembers(context.Context, string) ([]CampaignMember, error) {
	return nil, nil
}
func (m *mockCampaignRepoForInvites) UpdateMemberRole(context.Context, string, string, Role) error {
	return nil
}
func (m *mockCampaignRepoForInvites) UpdateMemberCharacter(context.Context, string, string, *string) error {
	return nil
}
func (m *mockCampaignRepoForInvites) FindOwnerMember(context.Context, string) (*CampaignMember, error) {
	return nil, nil
}
func (m *mockCampaignRepoForInvites) CreateTransfer(context.Context, *OwnershipTransfer) error {
	return nil
}
func (m *mockCampaignRepoForInvites) FindTransferByToken(context.Context, string) (*OwnershipTransfer, error) {
	return nil, nil
}
func (m *mockCampaignRepoForInvites) FindTransferByCampaign(context.Context, string) (*OwnershipTransfer, error) {
	return nil, nil
}
func (m *mockCampaignRepoForInvites) DeleteTransfer(context.Context, string) error { return nil }
func (m *mockCampaignRepoForInvites) UpdateBackdropPath(context.Context, string, *string) error {
	return nil
}
func (m *mockCampaignRepoForInvites) UpdateSettings(context.Context, string, string) error {
	return nil
}
func (m *mockCampaignRepoForInvites) UpdateSidebarConfig(context.Context, string, string) error {
	return nil
}
func (m *mockCampaignRepoForInvites) UpdateDashboardLayout(context.Context, string, *string) error {
	return nil
}
func (m *mockCampaignRepoForInvites) UpdateOwnerDashboardLayout(context.Context, string, *string) error {
	return nil
}
func (m *mockCampaignRepoForInvites) TransferOwnership(context.Context, string, string, string) error {
	return nil
}
func (m *mockCampaignRepoForInvites) ForceTransferOwnership(context.Context, string, string) error {
	return nil
}

// --- Tests ---

func TestCreateInvite_Success(t *testing.T) {
	inviteRepo := newMockInviteRepo()
	campaignRepo := newMockCampaignRepoForInvites()
	svc := NewInviteService(inviteRepo, campaignRepo, nil, "http://localhost:3000")

	invite, err := svc.CreateInvite(context.Background(), "camp-1", "user-1", CreateInviteInput{
		Email: "player@example.com",
		Role:  "player",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if invite.Email != "player@example.com" {
		t.Errorf("expected email player@example.com, got %s", invite.Email)
	}
	if invite.Role != "player" {
		t.Errorf("expected role player, got %s", invite.Role)
	}
	if invite.CampaignID != "camp-1" {
		t.Errorf("expected campaign camp-1, got %s", invite.CampaignID)
	}
	if invite.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestCreateInvite_InvalidRole(t *testing.T) {
	inviteRepo := newMockInviteRepo()
	campaignRepo := newMockCampaignRepoForInvites()
	svc := NewInviteService(inviteRepo, campaignRepo, nil, "http://localhost:3000")

	_, err := svc.CreateInvite(context.Background(), "camp-1", "user-1", CreateInviteInput{
		Email: "player@example.com",
		Role:  "owner",
	})
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestCreateInvite_EmptyEmail(t *testing.T) {
	inviteRepo := newMockInviteRepo()
	campaignRepo := newMockCampaignRepoForInvites()
	svc := NewInviteService(inviteRepo, campaignRepo, nil, "http://localhost:3000")

	_, err := svc.CreateInvite(context.Background(), "camp-1", "user-1", CreateInviteInput{
		Email: "",
		Role:  "player",
	})
	if err == nil {
		t.Fatal("expected error for empty email")
	}
}

func TestCreateInvite_DuplicatePending(t *testing.T) {
	inviteRepo := newMockInviteRepo()
	campaignRepo := newMockCampaignRepoForInvites()
	svc := NewInviteService(inviteRepo, campaignRepo, nil, "http://localhost:3000")

	// Create first invite.
	_, err := svc.CreateInvite(context.Background(), "camp-1", "user-1", CreateInviteInput{
		Email: "player@example.com",
		Role:  "player",
	})
	if err != nil {
		t.Fatalf("first invite failed: %v", err)
	}

	// Try duplicate.
	_, err = svc.CreateInvite(context.Background(), "camp-1", "user-1", CreateInviteInput{
		Email: "player@example.com",
		Role:  "player",
	})
	if err == nil {
		t.Fatal("expected error for duplicate invite")
	}
}

func TestAcceptInvite_Success(t *testing.T) {
	inviteRepo := newMockInviteRepo()
	campaignRepo := newMockCampaignRepoForInvites()
	svc := NewInviteService(inviteRepo, campaignRepo, nil, "http://localhost:3000")

	// Create invite.
	invite, _ := svc.CreateInvite(context.Background(), "camp-1", "user-1", CreateInviteInput{
		Email: "player@example.com",
		Role:  "scribe",
	})

	// Accept.
	accepted, err := svc.AcceptInvite(context.Background(), invite.Token, "user-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if accepted.CampaignID != "camp-1" {
		t.Errorf("expected campaign camp-1, got %s", accepted.CampaignID)
	}

	// Verify member was added.
	member, err := campaignRepo.FindMember(context.Background(), "camp-1", "user-2")
	if err != nil {
		t.Fatalf("member not found: %v", err)
	}
	if member.Role != RoleScribe {
		t.Errorf("expected role scribe, got %s", member.Role.String())
	}
}

func TestAcceptInvite_Expired(t *testing.T) {
	inviteRepo := newMockInviteRepo()
	campaignRepo := newMockCampaignRepoForInvites()
	svc := NewInviteService(inviteRepo, campaignRepo, nil, "http://localhost:3000")

	// Create invite and manually expire it.
	invite, _ := svc.CreateInvite(context.Background(), "camp-1", "user-1", CreateInviteInput{
		Email: "player@example.com",
		Role:  "player",
	})
	// Set expiry to the past.
	inviteRepo.invites[invite.ID].ExpiresAt = time.Now().UTC().Add(-time.Hour)

	_, err := svc.AcceptInvite(context.Background(), invite.Token, "user-2")
	if err == nil {
		t.Fatal("expected error for expired invite")
	}
}

func TestAcceptInvite_AlreadyAccepted(t *testing.T) {
	inviteRepo := newMockInviteRepo()
	campaignRepo := newMockCampaignRepoForInvites()
	svc := NewInviteService(inviteRepo, campaignRepo, nil, "http://localhost:3000")

	invite, _ := svc.CreateInvite(context.Background(), "camp-1", "user-1", CreateInviteInput{
		Email: "player@example.com",
		Role:  "player",
	})

	// Accept once.
	_, _ = svc.AcceptInvite(context.Background(), invite.Token, "user-2")

	// Try again.
	_, err := svc.AcceptInvite(context.Background(), invite.Token, "user-3")
	if err == nil {
		t.Fatal("expected error for already accepted invite")
	}
}

func TestRevokeInvite(t *testing.T) {
	inviteRepo := newMockInviteRepo()
	campaignRepo := newMockCampaignRepoForInvites()
	svc := NewInviteService(inviteRepo, campaignRepo, nil, "http://localhost:3000")

	invite, _ := svc.CreateInvite(context.Background(), "camp-1", "user-1", CreateInviteInput{
		Email: "player@example.com",
		Role:  "player",
	})

	err := svc.RevokeInvite(context.Background(), invite.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's gone.
	invites, _ := svc.ListInvites(context.Background(), "camp-1")
	if len(invites) != 0 {
		t.Errorf("expected 0 invites after revoke, got %d", len(invites))
	}
}

func TestListInvites(t *testing.T) {
	inviteRepo := newMockInviteRepo()
	campaignRepo := newMockCampaignRepoForInvites()
	svc := NewInviteService(inviteRepo, campaignRepo, nil, "http://localhost:3000")

	_, _ = svc.CreateInvite(context.Background(), "camp-1", "user-1", CreateInviteInput{
		Email: "p1@example.com", Role: "player",
	})
	_, _ = svc.CreateInvite(context.Background(), "camp-1", "user-1", CreateInviteInput{
		Email: "p2@example.com", Role: "scribe",
	})

	invites, err := svc.ListInvites(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(invites) != 2 {
		t.Errorf("expected 2 invites, got %d", len(invites))
	}
}

func TestCreateInvite_DefaultsToPlayer(t *testing.T) {
	inviteRepo := newMockInviteRepo()
	campaignRepo := newMockCampaignRepoForInvites()
	svc := NewInviteService(inviteRepo, campaignRepo, nil, "http://localhost:3000")

	invite, err := svc.CreateInvite(context.Background(), "camp-1", "user-1", CreateInviteInput{
		Email: "player@example.com",
		Role:  "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if invite.Role != "player" {
		t.Errorf("expected role player (default), got %s", invite.Role)
	}
}
