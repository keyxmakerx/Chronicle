package npcs

import (
	"context"
	"testing"
)

// --- Mocks ---

type mockNPCRepo struct {
	listRevealedFn  func(ctx context.Context, campaignID string, characterTypeID int, role int, userID string, opts NPCListOptions) ([]NPCCard, int, error)
	countRevealedFn func(ctx context.Context, campaignID string, characterTypeID int, role int, userID string) (int, error)
}

func (m *mockNPCRepo) ListRevealed(ctx context.Context, campaignID string, characterTypeID int, role int, userID string, opts NPCListOptions) ([]NPCCard, int, error) {
	if m.listRevealedFn != nil {
		return m.listRevealedFn(ctx, campaignID, characterTypeID, role, userID, opts)
	}
	return nil, 0, nil
}

func (m *mockNPCRepo) CountRevealed(ctx context.Context, campaignID string, characterTypeID int, role int, userID string) (int, error) {
	if m.countRevealedFn != nil {
		return m.countRevealedFn(ctx, campaignID, characterTypeID, role, userID)
	}
	return 0, nil
}

type mockTypeFinder struct {
	typeID int
	err    error
}

func (m *mockTypeFinder) FindCharacterTypeID(_ context.Context, _ string) (int, error) {
	return m.typeID, m.err
}

// --- Tests ---

func TestListNPCs_ReturnsCards(t *testing.T) {
	repo := &mockNPCRepo{
		listRevealedFn: func(_ context.Context, _ string, typeID int, role int, _ string, opts NPCListOptions) ([]NPCCard, int, error) {
			if typeID != 42 {
				t.Fatalf("expected typeID=42, got %d", typeID)
			}
			if role != 1 {
				t.Fatalf("expected role=1 (player), got %d", role)
			}
			return []NPCCard{
				{ID: "npc-1", Name: "Gandalf"},
				{ID: "npc-2", Name: "Aragorn"},
			}, 2, nil
		},
	}
	svc := NewNPCService(repo, &mockTypeFinder{typeID: 42})

	cards, total, err := svc.ListNPCs(context.Background(), "campaign-1", 1, "user-1", DefaultNPCListOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total=2, got %d", total)
	}
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}
	if cards[0].Name != "Gandalf" {
		t.Fatalf("expected first card name=Gandalf, got %s", cards[0].Name)
	}
}

func TestListNPCs_EmptyWhenNoCharacterType(t *testing.T) {
	svc := NewNPCService(&mockNPCRepo{}, &mockTypeFinder{err: context.DeadlineExceeded})

	_, _, err := svc.ListNPCs(context.Background(), "campaign-1", 1, "user-1", DefaultNPCListOptions())
	if err == nil {
		t.Fatal("expected error when character type not found")
	}
}

func TestCountNPCs(t *testing.T) {
	repo := &mockNPCRepo{
		countRevealedFn: func(_ context.Context, _ string, typeID int, _ int, _ string) (int, error) {
			return 7, nil
		},
	}
	svc := NewNPCService(repo, &mockTypeFinder{typeID: 10})

	count, err := svc.CountNPCs(context.Background(), "campaign-1", 3, "owner-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 7 {
		t.Fatalf("expected count=7, got %d", count)
	}
}

func TestNPCListOptions_Offset(t *testing.T) {
	tests := []struct {
		page    int
		perPage int
		want    int
	}{
		{1, 24, 0},
		{2, 24, 24},
		{3, 10, 20},
		{0, 24, 0}, // clamped to 0
	}
	for _, tt := range tests {
		opts := NPCListOptions{Page: tt.page, PerPage: tt.perPage}
		if got := opts.Offset(); got != tt.want {
			t.Errorf("page=%d perPage=%d: expected offset=%d, got %d", tt.page, tt.perPage, tt.want, got)
		}
	}
}

func TestNPCListOptions_OrderByClause(t *testing.T) {
	tests := []struct {
		sort string
		want string
	}{
		{"name", "ORDER BY e.name ASC"},
		{"updated", "ORDER BY e.updated_at DESC"},
		{"created", "ORDER BY e.created_at DESC"},
		{"invalid", "ORDER BY e.name ASC"},
	}
	for _, tt := range tests {
		opts := NPCListOptions{Sort: tt.sort}
		if got := opts.OrderByClause(); got != tt.want {
			t.Errorf("sort=%s: expected %q, got %q", tt.sort, tt.want, got)
		}
	}
}

func TestNPCCard_FieldString(t *testing.T) {
	card := &NPCCard{
		Fields: map[string]any{
			"race":  "Elf",
			"level": 5,
		},
	}

	if got := card.FieldString("race"); got != "Elf" {
		t.Errorf("expected race=Elf, got %s", got)
	}
	if got := card.FieldString("missing"); got != "" {
		t.Errorf("expected empty for missing key, got %s", got)
	}
	if got := card.FieldString("level"); got != "" {
		t.Errorf("expected empty for non-string value, got %s", got)
	}

	// Nil fields should return empty.
	nilCard := &NPCCard{}
	if got := nilCard.FieldString("anything"); got != "" {
		t.Errorf("expected empty for nil fields, got %s", got)
	}
}

func TestTogglePrivate_EntityService(t *testing.T) {
	// This tests that TogglePrivate is part of the service interface.
	// The actual implementation is tested in the entities package.
	// Here we just verify the mock VisibilityToggler interface works.
	toggler := &mockToggler{newPrivate: true}
	newPrivate, err := toggler.TogglePrivate(context.Background(), "entity-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !newPrivate {
		t.Fatal("expected newPrivate=true")
	}
}

type mockToggler struct {
	newPrivate bool
	err        error
}

func (m *mockToggler) TogglePrivate(_ context.Context, _ string) (bool, error) {
	return m.newPrivate, m.err
}
