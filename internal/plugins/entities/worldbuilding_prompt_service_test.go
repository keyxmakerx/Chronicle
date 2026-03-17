package entities

import (
	"context"
	"fmt"
	"testing"
)

// --- Mock Worldbuilding Prompt Repository ---

type mockWBPromptRepo struct {
	prompts map[int]*WorldbuildingPrompt
	nextID  int
}

func newMockWBPromptRepo() *mockWBPromptRepo {
	return &mockWBPromptRepo{prompts: make(map[int]*WorldbuildingPrompt), nextID: 1}
}

func (m *mockWBPromptRepo) Create(_ context.Context, p *WorldbuildingPrompt) error {
	p.ID = m.nextID
	m.nextID++
	m.prompts[p.ID] = p
	return nil
}

func (m *mockWBPromptRepo) FindByID(_ context.Context, id int) (*WorldbuildingPrompt, error) {
	if p, ok := m.prompts[id]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockWBPromptRepo) ListForCampaign(_ context.Context, campaignID string) ([]WorldbuildingPrompt, error) {
	var result []WorldbuildingPrompt
	for _, p := range m.prompts {
		if p.CampaignID != nil && *p.CampaignID == campaignID {
			result = append(result, *p)
		}
	}
	return result, nil
}

func (m *mockWBPromptRepo) ListForCampaignAndType(_ context.Context, campaignID string, entityTypeID int) ([]WorldbuildingPrompt, error) {
	var result []WorldbuildingPrompt
	for _, p := range m.prompts {
		if p.CampaignID != nil && *p.CampaignID == campaignID {
			if p.EntityTypeID == nil || *p.EntityTypeID == entityTypeID {
				result = append(result, *p)
			}
		}
	}
	return result, nil
}

func (m *mockWBPromptRepo) Update(_ context.Context, p *WorldbuildingPrompt) error {
	if _, ok := m.prompts[p.ID]; !ok {
		return fmt.Errorf("not found")
	}
	m.prompts[p.ID] = p
	return nil
}

func (m *mockWBPromptRepo) Delete(_ context.Context, id int) error {
	delete(m.prompts, id)
	return nil
}

// --- Mock Entity Type Lister (minimal for prompt tests) ---

type mockEntityTypeListerForPrompts struct {
	types []EntityType
}

func (m *mockEntityTypeListerForPrompts) ListByCampaign(_ context.Context, _ string) ([]EntityType, error) {
	return m.types, nil
}

// --- Tests ---

func TestCreatePrompt_Success(t *testing.T) {
	repo := newMockWBPromptRepo()
	typeRepo := &mockEntityTypeListerForPrompts{}
	svc := NewWorldbuildingPromptService(repo, typeRepo)

	etID := 1
	p, err := svc.Create(context.Background(), "camp-1", CreatePromptInput{
		Name:         "Motivation",
		PromptText:   "What motivates this character?",
		EntityTypeID: &etID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "Motivation" {
		t.Errorf("expected name Motivation, got %s", p.Name)
	}
	if p.Icon != "fa-lightbulb" {
		t.Errorf("expected default icon, got %s", p.Icon)
	}
}

func TestCreatePrompt_EmptyName(t *testing.T) {
	repo := newMockWBPromptRepo()
	typeRepo := &mockEntityTypeListerForPrompts{}
	svc := NewWorldbuildingPromptService(repo, typeRepo)

	_, err := svc.Create(context.Background(), "camp-1", CreatePromptInput{
		Name:       "",
		PromptText: "Some prompt",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCreatePrompt_EmptyText(t *testing.T) {
	repo := newMockWBPromptRepo()
	typeRepo := &mockEntityTypeListerForPrompts{}
	svc := NewWorldbuildingPromptService(repo, typeRepo)

	_, err := svc.Create(context.Background(), "camp-1", CreatePromptInput{
		Name:       "Test",
		PromptText: "",
	})
	if err == nil {
		t.Fatal("expected error for empty prompt text")
	}
}

func TestListForType(t *testing.T) {
	repo := newMockWBPromptRepo()
	typeRepo := &mockEntityTypeListerForPrompts{}
	svc := NewWorldbuildingPromptService(repo, typeRepo)

	etID := 1
	_, _ = svc.Create(context.Background(), "camp-1", CreatePromptInput{
		Name: "A", PromptText: "prompt a", EntityTypeID: &etID,
	})
	etID2 := 2
	_, _ = svc.Create(context.Background(), "camp-1", CreatePromptInput{
		Name: "B", PromptText: "prompt b", EntityTypeID: &etID2,
	})

	prompts, err := svc.ListForType(context.Background(), "camp-1", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prompts) != 1 {
		t.Errorf("expected 1 prompt for type 1, got %d", len(prompts))
	}
}

func TestDeletePrompt(t *testing.T) {
	repo := newMockWBPromptRepo()
	typeRepo := &mockEntityTypeListerForPrompts{}
	svc := NewWorldbuildingPromptService(repo, typeRepo)

	p, _ := svc.Create(context.Background(), "camp-1", CreatePromptInput{
		Name: "Test", PromptText: "test prompt",
	})

	if err := svc.Delete(context.Background(), p.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prompts, _ := svc.ListForCampaign(context.Background(), "camp-1")
	if len(prompts) != 0 {
		t.Errorf("expected 0 prompts after delete, got %d", len(prompts))
	}
}

func TestSeedDefaults(t *testing.T) {
	repo := newMockWBPromptRepo()
	typeRepo := &mockEntityTypeListerForPrompts{
		types: []EntityType{
			{ID: 1, Slug: "character", Name: "Character"},
			{ID: 2, Slug: "location", Name: "Location"},
			{ID: 3, Slug: "organization", Name: "Organization"},
			{ID: 4, Slug: "item", Name: "Item"},
			{ID: 5, Slug: "quest", Name: "Quest"},
		},
	}
	svc := NewWorldbuildingPromptService(repo, typeRepo)

	if err := svc.SeedDefaults(context.Background(), "camp-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prompts, _ := svc.ListForCampaign(context.Background(), "camp-1")
	// 4 character + 4 location + 3 org + 2 item + 3 quest = 16
	if len(prompts) != 16 {
		t.Errorf("expected 16 seeded prompts, got %d", len(prompts))
	}
}

func TestUpdatePrompt(t *testing.T) {
	repo := newMockWBPromptRepo()
	typeRepo := &mockEntityTypeListerForPrompts{}
	svc := NewWorldbuildingPromptService(repo, typeRepo)

	p, _ := svc.Create(context.Background(), "camp-1", CreatePromptInput{
		Name: "Original", PromptText: "original text",
	})

	err := svc.Update(context.Background(), p.ID, UpdatePromptInput{
		Name: "Updated", PromptText: "updated text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := svc.GetByID(context.Background(), p.ID)
	if got.Name != "Updated" {
		t.Errorf("expected name Updated, got %s", got.Name)
	}
	if got.PromptText != "updated text" {
		t.Errorf("expected updated text, got %s", got.PromptText)
	}
}
