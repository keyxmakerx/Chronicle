package armory

import (
	"context"
	"errors"
	"testing"
)

// --- Mocks ---

type mockArmoryRepo struct {
	listItemsFn  func(ctx context.Context, campaignID string, typeIDs []int, role int, userID string, opts ItemListOptions) ([]ItemCard, int, error)
	countItemsFn func(ctx context.Context, campaignID string, typeIDs []int, role int, userID string) (int, error)
}

func (m *mockArmoryRepo) ListItems(ctx context.Context, campaignID string, typeIDs []int, role int, userID string, opts ItemListOptions) ([]ItemCard, int, error) {
	if m.listItemsFn != nil {
		return m.listItemsFn(ctx, campaignID, typeIDs, role, userID, opts)
	}
	return nil, 0, nil
}

func (m *mockArmoryRepo) CountItems(ctx context.Context, campaignID string, typeIDs []int, role int, userID string) (int, error) {
	if m.countItemsFn != nil {
		return m.countItemsFn(ctx, campaignID, typeIDs, role, userID)
	}
	return 0, nil
}

type mockTypeFinder struct {
	findIDsFn   func(ctx context.Context, campaignID string) ([]int, error)
	findTypesFn func(ctx context.Context, campaignID string) ([]ItemTypeInfo, error)
}

func (m *mockTypeFinder) FindItemTypeIDs(ctx context.Context, campaignID string) ([]int, error) {
	if m.findIDsFn != nil {
		return m.findIDsFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockTypeFinder) FindItemTypes(ctx context.Context, campaignID string) ([]ItemTypeInfo, error) {
	if m.findTypesFn != nil {
		return m.findTypesFn(ctx, campaignID)
	}
	return nil, nil
}

type mockTagLister struct {
	listFn func(ctx context.Context, entityIDs []string) (map[string][]TagInfo, error)
}

func (m *mockTagLister) ListTagsForEntities(ctx context.Context, entityIDs []string) (map[string][]TagInfo, error) {
	if m.listFn != nil {
		return m.listFn(ctx, entityIDs)
	}
	return nil, nil
}

func newTestArmoryService(repo *mockArmoryRepo, tf *mockTypeFinder) *armoryService {
	return &armoryService{repo: repo, typeFinder: tf}
}

// --- Tests ---

func TestListItems_Success(t *testing.T) {
	repo := &mockArmoryRepo{
		listItemsFn: func(_ context.Context, _ string, typeIDs []int, _ int, _ string, _ ItemListOptions) ([]ItemCard, int, error) {
			if len(typeIDs) != 1 || typeIDs[0] != 5 {
				t.Errorf("expected typeIDs [5], got %v", typeIDs)
			}
			return []ItemCard{{ID: "item-1", Name: "Sword"}}, 1, nil
		},
	}
	tf := &mockTypeFinder{
		findIDsFn: func(_ context.Context, _ string) ([]int, error) { return []int{5}, nil },
	}
	svc := newTestArmoryService(repo, tf)
	cards, total, err := svc.ListItems(context.Background(), "camp-1", 2, "user-1", DefaultItemListOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 || len(cards) != 1 || cards[0].Name != "Sword" {
		t.Errorf("unexpected result: %d cards, total=%d", len(cards), total)
	}
}

func TestListItems_NoItemTypes(t *testing.T) {
	tf := &mockTypeFinder{
		findIDsFn: func(_ context.Context, _ string) ([]int, error) { return nil, nil },
	}
	svc := newTestArmoryService(&mockArmoryRepo{}, tf)
	cards, total, err := svc.ListItems(context.Background(), "camp-1", 2, "user-1", DefaultItemListOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 || len(cards) != 0 {
		t.Error("expected empty result for no item types")
	}
}

func TestListItems_TypeFinderError(t *testing.T) {
	tf := &mockTypeFinder{
		findIDsFn: func(_ context.Context, _ string) ([]int, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestArmoryService(&mockArmoryRepo{}, tf)
	_, _, err := svc.ListItems(context.Background(), "camp-1", 2, "user-1", DefaultItemListOptions())
	if err == nil {
		t.Error("expected error from type finder")
	}
}

func TestListItems_RepoError(t *testing.T) {
	tf := &mockTypeFinder{
		findIDsFn: func(_ context.Context, _ string) ([]int, error) { return []int{1}, nil },
	}
	repo := &mockArmoryRepo{
		listItemsFn: func(_ context.Context, _ string, _ []int, _ int, _ string, _ ItemListOptions) ([]ItemCard, int, error) {
			return nil, 0, errors.New("repo error")
		},
	}
	svc := newTestArmoryService(repo, tf)
	_, _, err := svc.ListItems(context.Background(), "camp-1", 2, "user-1", DefaultItemListOptions())
	if err == nil {
		t.Error("expected repo error")
	}
}

func TestListItems_WithTags(t *testing.T) {
	repo := &mockArmoryRepo{
		listItemsFn: func(_ context.Context, _ string, _ []int, _ int, _ string, _ ItemListOptions) ([]ItemCard, int, error) {
			return []ItemCard{{ID: "item-1", Name: "Sword"}, {ID: "item-2", Name: "Shield"}}, 2, nil
		},
	}
	tf := &mockTypeFinder{
		findIDsFn: func(_ context.Context, _ string) ([]int, error) { return []int{1}, nil },
	}
	svc := newTestArmoryService(repo, tf)
	svc.tagLister = &mockTagLister{
		listFn: func(_ context.Context, ids []string) (map[string][]TagInfo, error) {
			return map[string][]TagInfo{
				"item-1": {{Name: "Weapon", Color: "#ff0000"}},
			}, nil
		},
	}
	cards, _, err := svc.ListItems(context.Background(), "camp-1", 2, "user-1", DefaultItemListOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cards[0].Tags) != 1 {
		t.Errorf("expected 1 tag on item-1, got %d", len(cards[0].Tags))
	}
	if len(cards[1].Tags) != 0 {
		t.Errorf("expected 0 tags on item-2, got %d", len(cards[1].Tags))
	}
}

func TestCountItems_Success(t *testing.T) {
	repo := &mockArmoryRepo{
		countItemsFn: func(_ context.Context, _ string, _ []int, _ int, _ string) (int, error) {
			return 42, nil
		},
	}
	tf := &mockTypeFinder{
		findIDsFn: func(_ context.Context, _ string) ([]int, error) { return []int{1}, nil },
	}
	svc := newTestArmoryService(repo, tf)
	count, err := svc.CountItems(context.Background(), "camp-1", 2, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 42 {
		t.Errorf("expected 42, got %d", count)
	}
}

func TestCountItems_NoTypes(t *testing.T) {
	tf := &mockTypeFinder{
		findIDsFn: func(_ context.Context, _ string) ([]int, error) { return nil, nil },
	}
	svc := newTestArmoryService(&mockArmoryRepo{}, tf)
	count, err := svc.CountItems(context.Background(), "camp-1", 2, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestGetItemTypes_Success(t *testing.T) {
	tf := &mockTypeFinder{
		findTypesFn: func(_ context.Context, _ string) ([]ItemTypeInfo, error) {
			return []ItemTypeInfo{{ID: 1, Name: "Weapon"}}, nil
		},
	}
	svc := newTestArmoryService(&mockArmoryRepo{}, tf)
	types, err := svc.GetItemTypes(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(types) != 1 || types[0].Name != "Weapon" {
		t.Errorf("unexpected types: %v", types)
	}
}
