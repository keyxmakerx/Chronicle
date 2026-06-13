package tags

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// isNotFound reports whether err is an AppError with a 404 code.
func isNotFound(err error) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == http.StatusNotFound
}

// --- Mocks ---

type mockGrantRepo struct {
	createFn              func(ctx context.Context, p *TagPermission) error
	getByIDFn             func(ctx context.Context, id int) (*TagPermission, error)
	listByTagFn           func(ctx context.Context, tagID int) ([]TagPermission, error)
	deleteFn              func(ctx context.Context, id int) error
	listGrantsForEntityFn func(ctx context.Context, entityID string) ([]EntityTagGrant, error)
}

func (m *mockGrantRepo) Create(ctx context.Context, p *TagPermission) error {
	if m.createFn != nil {
		return m.createFn(ctx, p)
	}
	p.ID = 7
	return nil
}
func (m *mockGrantRepo) GetByID(ctx context.Context, id int) (*TagPermission, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return &TagPermission{ID: id, TagID: 1}, nil
}
func (m *mockGrantRepo) ListByTag(ctx context.Context, tagID int) ([]TagPermission, error) {
	if m.listByTagFn != nil {
		return m.listByTagFn(ctx, tagID)
	}
	return nil, nil
}
func (m *mockGrantRepo) Delete(ctx context.Context, id int) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
func (m *mockGrantRepo) ListGrantsForEntity(ctx context.Context, entityID string) ([]EntityTagGrant, error) {
	if m.listGrantsForEntityFn != nil {
		return m.listGrantsForEntityFn(ctx, entityID)
	}
	return nil, nil
}

type mockMemberChecker struct {
	member *campaigns.CampaignMember
}

func (m *mockMemberChecker) GetMember(_ context.Context, _, _ string) (*campaigns.CampaignMember, error) {
	return m.member, nil
}

type mockGroupChecker struct {
	group *campaigns.CampaignGroup
}

func (m *mockGroupChecker) GetGroup(_ context.Context, _ int) (*campaigns.CampaignGroup, error) {
	return m.group, nil
}

// tagInCampaign returns a TagRepository whose tag belongs to campaignID.
func tagInCampaign(campaignID string) TagRepository {
	return &mockTagRepo{findByIDFn: func(_ context.Context, id int) (*Tag, error) {
		return &Tag{ID: id, CampaignID: campaignID, Name: "Lore", Slug: "lore"}, nil
	}}
}

// --- Tests ---

func TestTagGrantService_Create_SubjectValidation(t *testing.T) {
	ctx := context.Background()
	member := &campaigns.CampaignMember{UserID: "u1", DisplayName: "Alice"}
	group := &campaigns.CampaignGroup{ID: 5, CampaignID: "camp-1", Name: "Lorekeepers"}

	tests := []struct {
		name        string
		subjectType string
		subjectID   string
		member      *campaigns.CampaignMember
		group       *campaigns.CampaignGroup
		wantErr     bool
	}{
		{"valid role player", SubjectRole, "1", nil, nil, false},
		{"valid role scribe", SubjectRole, "2", nil, nil, false},
		{"valid public (no subject id)", SubjectPublic, "", nil, nil, false},
		{"valid public (subject id ignored)", SubjectPublic, "anything", nil, nil, false},
		{"invalid role zero", SubjectRole, "0", nil, nil, true},
		{"invalid role too high", SubjectRole, "9", nil, nil, true},
		{"invalid role non-numeric", SubjectRole, "player", nil, nil, true},
		{"valid user (member)", SubjectUser, "u1", member, nil, false},
		{"invalid user (not a member)", SubjectUser, "u1", nil, nil, true},
		{"valid group in campaign", SubjectGroup, "5", nil, group, false},
		{"group wrong campaign", SubjectGroup, "5", nil, &campaigns.CampaignGroup{ID: 5, CampaignID: "other", Name: "X"}, true},
		{"group non-numeric", SubjectGroup, "abc", nil, group, true},
		{"unknown subject type", "planet", "x", nil, nil, true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			svc := NewTagGrantService(
				&mockGrantRepo{},
				tagInCampaign("camp-1"),
				&mockMemberChecker{member: tc.member},
				&mockGroupChecker{group: tc.group},
			)
			_, err := svc.Create(ctx, "camp-1", 1, tc.subjectType, tc.subjectID, "owner-1")
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestTagGrantService_Create_PublicNormalizesSubjectID pins that a 'public'
// grant stores an empty subject_id regardless of what the client sent, so the
// unique key (tag_id, 'public', '') permits exactly one public grant per tag
// (C-PERM-ANON-IDENTITY).
func TestTagGrantService_Create_PublicNormalizesSubjectID(t *testing.T) {
	var captured *TagPermission
	repo := &mockGrantRepo{createFn: func(_ context.Context, p *TagPermission) error {
		captured = p
		p.ID = 1
		return nil
	}}
	svc := NewTagGrantService(repo, tagInCampaign("camp-1"), &mockMemberChecker{}, &mockGroupChecker{})
	if _, err := svc.Create(context.Background(), "camp-1", 1, SubjectPublic, "junk", "owner-1"); err != nil {
		t.Fatalf("unexpected error creating public grant: %v", err)
	}
	if captured == nil {
		t.Fatal("Create never reached the repo")
	}
	if captured.SubjectType != SubjectPublic {
		t.Errorf("subject_type = %q, want %q", captured.SubjectType, SubjectPublic)
	}
	if captured.SubjectID != "" {
		t.Errorf("public subject_id must normalize to empty, got %q", captured.SubjectID)
	}
}

func TestTagGrantService_Create_TagNotInCampaign(t *testing.T) {
	svc := NewTagGrantService(&mockGrantRepo{}, tagInCampaign("other-camp"),
		&mockMemberChecker{}, &mockGroupChecker{})
	_, err := svc.Create(context.Background(), "camp-1", 1, SubjectRole, "1", "owner-1")
	if !isNotFound(err) {
		t.Fatalf("cross-campaign tag must be NotFound (no probing); got %v", err)
	}
}

func TestTagGrantService_ListByTag_TagNotInCampaign(t *testing.T) {
	svc := NewTagGrantService(&mockGrantRepo{}, tagInCampaign("other-camp"),
		&mockMemberChecker{}, &mockGroupChecker{})
	_, err := svc.ListByTag(context.Background(), "camp-1", 1)
	if !isNotFound(err) {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestTagGrantService_Delete_GrantBelongsToOtherTag(t *testing.T) {
	repo := &mockGrantRepo{getByIDFn: func(_ context.Context, id int) (*TagPermission, error) {
		return &TagPermission{ID: id, TagID: 999}, nil // belongs to a different tag
	}}
	svc := NewTagGrantService(repo, tagInCampaign("camp-1"), &mockMemberChecker{}, &mockGroupChecker{})
	err := svc.Delete(context.Background(), "camp-1", 1, 42)
	if !isNotFound(err) {
		t.Fatalf("deleting a grant from the wrong tag must be NotFound; got %v", err)
	}
}

func TestTagGrantService_GrantsForEntity_ResolvesLabels(t *testing.T) {
	repo := &mockGrantRepo{listGrantsForEntityFn: func(_ context.Context, _ string) ([]EntityTagGrant, error) {
		return []EntityTagGrant{
			{TagSlug: "revealed-act-1", SubjectType: SubjectRole, SubjectID: "1"},
			{TagSlug: "secrets", SubjectType: SubjectGroup, SubjectID: "5"},
			{TagSlug: "personal", SubjectType: SubjectUser, SubjectID: "u1"},
			{TagSlug: "town-board", SubjectType: SubjectPublic, SubjectID: ""},
		}, nil
	}}
	svc := NewTagGrantService(repo, tagInCampaign("camp-1"),
		&mockMemberChecker{member: &campaigns.CampaignMember{DisplayName: "Alice"}},
		&mockGroupChecker{group: &campaigns.CampaignGroup{Name: "Lorekeepers"}},
	)
	grants, err := svc.GrantsForEntity(context.Background(), "camp-1", "ent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A public grant must label as "the public" — distinct from "Players" — so
	// the glance tooltip tells the owner public exposure apart from member-only.
	want := map[string]string{"revealed-act-1": "Players", "secrets": "Lorekeepers", "personal": "Alice", "town-board": "the public"}
	for _, g := range grants {
		if want[g.TagSlug] != g.SubjectLabel {
			t.Errorf("tag %q: label = %q, want %q", g.TagSlug, g.SubjectLabel, want[g.TagSlug])
		}
	}
}
