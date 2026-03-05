package campaigns

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockGroupRepo is a test double for GroupRepository.
type mockGroupRepo struct {
	groups  map[int]*CampaignGroup
	members map[int][]GroupMemberInfo
	nextID  int
	errOnCreate error
}

func newMockGroupRepo() *mockGroupRepo {
	return &mockGroupRepo{
		groups:  make(map[int]*CampaignGroup),
		members: make(map[int][]GroupMemberInfo),
		nextID:  1,
	}
}

func (r *mockGroupRepo) CreateGroup(_ context.Context, campaignID, name string, description *string) (*CampaignGroup, error) {
	if r.errOnCreate != nil {
		return nil, r.errOnCreate
	}
	// Simulate duplicate check.
	for _, g := range r.groups {
		if g.CampaignID == campaignID && g.Name == name {
			return nil, errors.New("Duplicate entry")
		}
	}
	g := &CampaignGroup{
		ID:          r.nextID,
		CampaignID:  campaignID,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	r.groups[r.nextID] = g
	r.nextID++
	return g, nil
}

func (r *mockGroupRepo) ListGroups(_ context.Context, campaignID string) ([]CampaignGroup, error) {
	var result []CampaignGroup
	for _, g := range r.groups {
		if g.CampaignID == campaignID {
			result = append(result, *g)
		}
	}
	return result, nil
}

func (r *mockGroupRepo) GetGroup(_ context.Context, groupID int) (*CampaignGroup, error) {
	g, ok := r.groups[groupID]
	if !ok {
		return nil, errors.New("not found")
	}
	return g, nil
}

func (r *mockGroupRepo) UpdateGroup(_ context.Context, groupID int, name string, description *string) error {
	g, ok := r.groups[groupID]
	if !ok {
		return errors.New("not found")
	}
	g.Name = name
	g.Description = description
	return nil
}

func (r *mockGroupRepo) DeleteGroup(_ context.Context, groupID int) error {
	delete(r.groups, groupID)
	delete(r.members, groupID)
	return nil
}

func (r *mockGroupRepo) AddGroupMember(_ context.Context, groupID int, userID string) error {
	r.members[groupID] = append(r.members[groupID], GroupMemberInfo{UserID: userID})
	return nil
}

func (r *mockGroupRepo) RemoveGroupMember(_ context.Context, groupID int, userID string) error {
	members := r.members[groupID]
	for i, m := range members {
		if m.UserID == userID {
			r.members[groupID] = append(members[:i], members[i+1:]...)
			return nil
		}
	}
	return nil
}

func (r *mockGroupRepo) ListGroupMembers(_ context.Context, groupID int) ([]GroupMemberInfo, error) {
	return r.members[groupID], nil
}

// --- Tests ---

func TestGroupService_CreateGroup(t *testing.T) {
	tests := []struct {
		name      string
		groupName string
		wantErr   bool
	}{
		{"valid name", "Party A", false},
		{"empty name", "", true},
		{"whitespace name", "   ", true},
		{"long name", string(make([]byte, 101)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockGroupRepo()
			svc := NewGroupService(repo)

			group, err := svc.CreateGroup(context.Background(), "camp-1", tt.groupName, nil)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if group.Name != tt.groupName {
				t.Errorf("got name %q, want %q", group.Name, tt.groupName)
			}
		})
	}
}

func TestGroupService_CreateGroup_Duplicate(t *testing.T) {
	repo := newMockGroupRepo()
	svc := NewGroupService(repo)

	_, err := svc.CreateGroup(context.Background(), "camp-1", "Party A", nil)
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	_, err = svc.CreateGroup(context.Background(), "camp-1", "Party A", nil)
	if err == nil {
		t.Error("expected duplicate error, got nil")
	}
}

func TestGroupService_UpdateGroup_EmptyName(t *testing.T) {
	repo := newMockGroupRepo()
	svc := NewGroupService(repo)

	group, _ := svc.CreateGroup(context.Background(), "camp-1", "Party A", nil)

	err := svc.UpdateGroup(context.Background(), group.ID, "", nil)
	if err == nil {
		t.Error("expected error for empty name, got nil")
	}
}

func TestGroupService_GetGroup_NotFound(t *testing.T) {
	repo := newMockGroupRepo()
	svc := NewGroupService(repo)

	_, err := svc.GetGroup(context.Background(), 999)
	if err == nil {
		t.Error("expected not found error, got nil")
	}
}

func TestGroupService_AddRemoveMember(t *testing.T) {
	repo := newMockGroupRepo()
	svc := NewGroupService(repo)

	group, _ := svc.CreateGroup(context.Background(), "camp-1", "Party A", nil)

	if err := svc.AddGroupMember(context.Background(), group.ID, "user-1"); err != nil {
		t.Fatalf("add member failed: %v", err)
	}

	members, err := svc.ListGroupMembers(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("list members failed: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("got %d members, want 1", len(members))
	}

	if err := svc.RemoveGroupMember(context.Background(), group.ID, "user-1"); err != nil {
		t.Fatalf("remove member failed: %v", err)
	}

	members, err = svc.ListGroupMembers(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("list members after remove failed: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("got %d members after remove, want 0", len(members))
	}
}

func TestGroupService_DeleteGroup(t *testing.T) {
	repo := newMockGroupRepo()
	svc := NewGroupService(repo)

	group, _ := svc.CreateGroup(context.Background(), "camp-1", "Party A", nil)

	if err := svc.DeleteGroup(context.Background(), group.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err := svc.GetGroup(context.Background(), group.ID)
	if err == nil {
		t.Error("expected not found after delete, got nil")
	}
}
