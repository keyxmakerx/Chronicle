package armory

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// --- Mocks ---
//
// These mocks stub the REPOSITORY's SQL and the entities plugin's real
// visibility predicate, so nothing here proves the real SQL enforces
// visibility — see the real-database test in internal/app for that. What
// IS unit-testable here is the SERVICE's own orchestration: whether it
// calls the visibility filter for the right roles, fails closed with no
// filter wired, and whether ListItems/CountItems agree for the same inputs.

type mockArmoryRepo struct {
	listIDsFn    func(ctx context.Context, campaignID string, typeIDs []int, opts ItemListOptions) ([]string, error)
	getByIDsFn   func(ctx context.Context, campaignID string, ids []string) ([]ItemCard, error)
	lastByIDsArg []string // records the ids GetItemCardsByIDs was called with, for pagination assertions
}

func (m *mockArmoryRepo) ListItemIDs(ctx context.Context, campaignID string, typeIDs []int, opts ItemListOptions) ([]string, error) {
	if m.listIDsFn != nil {
		return m.listIDsFn(ctx, campaignID, typeIDs, opts)
	}
	return nil, nil
}

func (m *mockArmoryRepo) GetItemCardsByIDs(ctx context.Context, campaignID string, ids []string) ([]ItemCard, error) {
	m.lastByIDsArg = ids
	if m.getByIDsFn != nil {
		return m.getByIDsFn(ctx, campaignID, ids)
	}
	cards := make([]ItemCard, len(ids))
	for i, id := range ids {
		cards[i] = ItemCard{ID: id, Name: id}
	}
	return cards, nil
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
	listFn func(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]TagInfo, error)
}

func (m *mockTagLister) ListTagsForEntities(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]TagInfo, error) {
	if m.listFn != nil {
		return m.listFn(ctx, entityIDs, includeDmOnly)
	}
	return nil, nil
}

// mockVisibilityFilter records every call it receives and returns a
// caller-supplied viewable set, so tests can assert exactly which ids were
// sent to it (never more than the candidate set) and control the outcome.
type mockVisibilityFilter struct {
	viewable   map[string]bool
	err        error
	calls      int
	lastRole   int
	lastUserID string
	lastEntIDs []string
}

func (m *mockVisibilityFilter) FilterViewableEntityIDs(_ context.Context, _ string, entityIDs []string, role int, userID string) (map[string]bool, error) {
	m.calls++
	m.lastRole = role
	m.lastUserID = userID
	m.lastEntIDs = entityIDs
	if m.err != nil {
		return nil, m.err
	}
	return m.viewable, nil
}

func newTestArmoryService(repo *mockArmoryRepo, tf *mockTypeFinder, vf EntityVisibilityFilter) *armoryService {
	return &armoryService{repo: repo, typeFinder: tf, entityVisibility: vf}
}

// --- Tests ---

func TestListItems_Success(t *testing.T) {
	repo := &mockArmoryRepo{
		listIDsFn: func(_ context.Context, _ string, typeIDs []int, _ ItemListOptions) ([]string, error) {
			if len(typeIDs) != 1 || typeIDs[0] != 5 {
				t.Errorf("expected typeIDs [5], got %v", typeIDs)
			}
			return []string{"item-1"}, nil
		},
	}
	tf := &mockTypeFinder{
		findIDsFn: func(_ context.Context, _ string) ([]int, error) { return []int{5}, nil },
	}
	// Owner is the only role that bypasses the visibility filter, matching
	// entities' own visibilityFilter, so these list-mechanics fixtures run as
	// Owner. A Scribe with a nil filter now fails CLOSED and would return
	// nothing — correct behaviour, but it would stop this test exercising
	// pagination and card assembly, which is what it is for.
	svc := newTestArmoryService(repo, tf, nil)
	cards, total, err := svc.ListItems(context.Background(), "camp-1", permissions.RoleOwner, "user-1", DefaultItemListOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 || len(cards) != 1 || cards[0].ID != "item-1" {
		t.Errorf("unexpected result: %d cards, total=%d", len(cards), total)
	}
}

func TestListItems_NoItemTypes(t *testing.T) {
	tf := &mockTypeFinder{
		findIDsFn: func(_ context.Context, _ string) ([]int, error) { return nil, nil },
	}
	svc := newTestArmoryService(&mockArmoryRepo{}, tf, nil)
	cards, total, err := svc.ListItems(context.Background(), "camp-1", permissions.RoleOwner, "user-1", DefaultItemListOptions())
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
	svc := newTestArmoryService(&mockArmoryRepo{}, tf, nil)
	_, _, err := svc.ListItems(context.Background(), "camp-1", permissions.RoleOwner, "user-1", DefaultItemListOptions())
	if err == nil {
		t.Error("expected error from type finder")
	}
}

func TestListItems_RepoError(t *testing.T) {
	tf := &mockTypeFinder{
		findIDsFn: func(_ context.Context, _ string) ([]int, error) { return []int{1}, nil },
	}
	repo := &mockArmoryRepo{
		listIDsFn: func(_ context.Context, _ string, _ []int, _ ItemListOptions) ([]string, error) {
			return nil, errors.New("repo error")
		},
	}
	svc := newTestArmoryService(repo, tf, nil)
	_, _, err := svc.ListItems(context.Background(), "camp-1", permissions.RoleOwner, "user-1", DefaultItemListOptions())
	if err == nil {
		t.Error("expected repo error")
	}
}

func TestListItems_WithTags(t *testing.T) {
	repo := &mockArmoryRepo{
		listIDsFn: func(_ context.Context, _ string, _ []int, _ ItemListOptions) ([]string, error) {
			return []string{"item-1", "item-2"}, nil
		},
	}
	tf := &mockTypeFinder{
		findIDsFn: func(_ context.Context, _ string) ([]int, error) { return []int{1}, nil },
	}
	svc := newTestArmoryService(repo, tf, nil)
	svc.tagLister = &mockTagLister{
		listFn: func(_ context.Context, ids []string, _ bool) (map[string][]TagInfo, error) {
			return map[string][]TagInfo{
				"item-1": {{Name: "Weapon", Color: "#ff0000"}},
			}, nil
		},
	}
	cards, _, err := svc.ListItems(context.Background(), "camp-1", permissions.RoleOwner, "user-1", DefaultItemListOptions())
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

// GM-only tags follow the tags widget's rule: only Owner visibility (the
// Owner or a DM-granted co-DM) gets them, never a Scribe or Player.
func TestListItems_DmOnlyTagsOnlyAtOwnerVisibility(t *testing.T) {
	tests := []struct {
		name string
		role int
		want bool
	}{
		{"player", permissions.RolePlayer, false},
		{"scribe", permissions.RoleScribe, false},
		{"owner or DM-granted co-DM", permissions.RoleOwner, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockArmoryRepo{
				listIDsFn: func(_ context.Context, _ string, _ []int, _ ItemListOptions) ([]string, error) {
					return []string{"item-1"}, nil
				},
			}
			tf := &mockTypeFinder{
				findIDsFn: func(_ context.Context, _ string) ([]int, error) { return []int{1}, nil },
			}
			allowAll := &mockVisibilityFilter{viewable: map[string]bool{"item-1": true}}
			svc := newTestArmoryService(repo, tf, allowAll)
			var got *bool
			svc.tagLister = &mockTagLister{
				listFn: func(_ context.Context, _ []string, includeDmOnly bool) (map[string][]TagInfo, error) {
					got = &includeDmOnly
					return nil, nil
				},
			}
			if _, _, err := svc.ListItems(context.Background(), "camp-1", tt.role, "user-1", DefaultItemListOptions()); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil || *got != tt.want {
				t.Errorf("includeDmOnly = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCountItems_Success(t *testing.T) {
	repo := &mockArmoryRepo{
		listIDsFn: func(_ context.Context, _ string, _ []int, _ ItemListOptions) ([]string, error) {
			ids := make([]string, 42)
			for i := range ids {
				ids[i] = string(rune('a' + i%26))
			}
			return ids, nil
		},
	}
	tf := &mockTypeFinder{
		findIDsFn: func(_ context.Context, _ string) ([]int, error) { return []int{1}, nil },
	}
	svc := newTestArmoryService(repo, tf, nil)
	count, err := svc.CountItems(context.Background(), "camp-1", permissions.RoleOwner, "user-1")
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
	svc := newTestArmoryService(&mockArmoryRepo{}, tf, nil)
	count, err := svc.CountItems(context.Background(), "camp-1", permissions.RoleOwner, "user-1")
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
	svc := newTestArmoryService(&mockArmoryRepo{}, tf, nil)
	types, err := svc.GetItemTypes(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(types) != 1 || types[0].Name != "Weapon" {
		t.Errorf("unexpected types: %v", types)
	}
}

// --- Visibility orchestration tests ---

// TestVisibleItemIDs_OnlyOwnerBypassesFilter pins the bypass at OWNER, not
// Scribe: visibilityFilter (entities/repository.go) returns an empty
// predicate only for role >= RoleOwner. For a Scribe it still evaluates,
// and its custom branch requires a matching grant, so a visibility='custom'
// entity is NOT automatically visible to a Scribe — bypassing at Scribe
// would show them the name and artwork of a page they cannot open. A
// Scribe still sees every plain is_private page (the filter's default
// branch admits role >= 2); only custom-without-grant is withheld.
func TestVisibleItemIDs_OnlyOwnerBypassesFilter(t *testing.T) {
	t.Run("owner bypasses entirely", func(t *testing.T) {
		repo := &mockArmoryRepo{
			listIDsFn: func(_ context.Context, _ string, _ []int, _ ItemListOptions) ([]string, error) {
				return []string{"public-item", "restricted-item"}, nil
			},
		}
		vf := &mockVisibilityFilter{viewable: map[string]bool{}} // would hide everything if consulted
		svc := &armoryService{repo: repo, typeFinder: &mockTypeFinder{}, entityVisibility: vf}

		role := permissions.RoleOwner
		ids, err := svc.visibleItemIDs(context.Background(), "camp-1", []int{1}, role, "u1", ItemListOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vf.calls != 0 {
			t.Errorf("an Owner must not be filtered at all, got %d filter calls", vf.calls)
		}
		if len(ids) != 2 {
			t.Errorf("an Owner must see both ids, got %v", ids)
		}
	})

	t.Run("scribe is filtered, not bypassed", func(t *testing.T) {
		repo := &mockArmoryRepo{
			listIDsFn: func(_ context.Context, _ string, _ []int, _ ItemListOptions) ([]string, error) {
				return []string{"public-item", "restricted-item"}, nil
			},
		}
		// The canonical filter admits the public one and withholds the
		// custom-restricted one, which is what it does for a real Scribe with
		// no matching grant.
		vf := &mockVisibilityFilter{viewable: map[string]bool{"public-item": true}}
		svc := &armoryService{repo: repo, typeFinder: &mockTypeFinder{}, entityVisibility: vf}

		role := permissions.RoleScribe
		ids, err := svc.visibleItemIDs(context.Background(), "camp-1", []int{1}, role, "u1", ItemListOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vf.calls != 1 {
			t.Errorf("a Scribe must be run through the canonical filter exactly once, got %d calls", vf.calls)
		}
		if len(ids) != 1 || ids[0] != "public-item" {
			t.Errorf("a Scribe must not see the custom-restricted id, got %v", ids)
		}
		if vf.lastRole != permissions.RoleScribe {
			t.Errorf("the Scribe's own role must reach the filter, got %d", vf.lastRole)
		}
	})
}

// TestVisibleItemIDs_PlayerAndAnonymousUseCanonicalFilter proves the service
// consults EntityVisibilityFilter (not a hand-rolled is_private check) for
// Player and anonymous, and narrows to exactly what it reports viewable.
func TestVisibleItemIDs_PlayerAndAnonymousUseCanonicalFilter(t *testing.T) {
	for _, tc := range []struct {
		role   int
		userID string
	}{
		{permissions.RolePlayer, "player-1"},
		{permissions.RoleNone, ""},
	} {
		repo := &mockArmoryRepo{
			listIDsFn: func(_ context.Context, _ string, _ []int, _ ItemListOptions) ([]string, error) {
				return []string{"public-item", "restricted-item"}, nil
			},
		}
		vf := &mockVisibilityFilter{viewable: map[string]bool{"public-item": true}}
		svc := newTestArmoryService(repo, &mockTypeFinder{}, vf)

		ids, err := svc.visibleItemIDs(context.Background(), "camp-1", []int{1}, tc.role, tc.userID, ItemListOptions{})
		if err != nil {
			t.Fatalf("role %d: unexpected error: %v", tc.role, err)
		}
		if vf.calls != 1 {
			t.Fatalf("role %d: expected EntityVisibilityFilter to be consulted exactly once, got %d", tc.role, vf.calls)
		}
		if vf.lastRole != tc.role || vf.lastUserID != tc.userID {
			t.Errorf("role %d: filter called with role=%d userID=%q, want role=%d userID=%q", tc.role, vf.lastRole, vf.lastUserID, tc.role, tc.userID)
		}
		if len(ids) != 1 || ids[0] != "public-item" {
			t.Errorf("role %d: expected only public-item, got %v", tc.role, ids)
		}
	}
}

// TestVisibleItemIDs_FailsClosedWithNoFilterWired is the defense-in-depth
// case: if the visibility gate is somehow not wired for a Player/anonymous
// viewer, the result must be EMPTY, never "everything" — the opposite of the
// pre-fix bug's failure direction.
func TestVisibleItemIDs_FailsClosedWithNoFilterWired(t *testing.T) {
	repo := &mockArmoryRepo{
		listIDsFn: func(_ context.Context, _ string, _ []int, _ ItemListOptions) ([]string, error) {
			return []string{"item-1"}, nil
		},
	}
	svc := newTestArmoryService(repo, &mockTypeFinder{}, nil) // entityVisibility == nil
	ids, err := svc.visibleItemIDs(context.Background(), "camp-1", []int{1}, permissions.RolePlayer, "player-1", ItemListOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected fail-CLOSED (no ids) with no visibility filter wired, got %v", ids)
	}
}

// TestListAndCountItems_NeverDisagree drives ListItems and CountItems off the
// same fixture and requires their totals to match, for every role — the
// "an inflated count is itself a leak" half of finding 2.
func TestListAndCountItems_NeverDisagree(t *testing.T) {
	candidateIDs := []string{"public-item", "restricted-item"}
	tf := &mockTypeFinder{findIDsFn: func(_ context.Context, _ string) ([]int, error) { return []int{1}, nil }}

	for _, tc := range []struct {
		name string
		role int
	}{
		{"anonymous", permissions.RoleNone},
		{"player", permissions.RolePlayer},
		{"scribe", permissions.RoleScribe},
		{"owner", permissions.RoleOwner},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockArmoryRepo{
				listIDsFn: func(_ context.Context, _ string, _ []int, _ ItemListOptions) ([]string, error) {
					return candidateIDs, nil
				},
			}
			vf := &mockVisibilityFilter{viewable: map[string]bool{"public-item": true}}
			svc := newTestArmoryService(repo, tf, vf)

			_, total, err := svc.ListItems(context.Background(), "camp-1", tc.role, "u1", DefaultItemListOptions())
			if err != nil {
				t.Fatalf("ListItems: %v", err)
			}
			count, err := svc.CountItems(context.Background(), "camp-1", tc.role, "u1")
			if err != nil {
				t.Fatalf("CountItems: %v", err)
			}
			if total != count {
				t.Errorf("ListItems total=%d, CountItems=%d — must never disagree", total, count)
			}
		})
	}
}

// TestListItems_PaginatesTheFilteredSet proves pagination is applied AFTER
// visibility narrowing, not before — the pre-fix pagination happened in SQL
// against the unfiltered set, which is exactly the shape that let a
// restricted row occupy a page slot a visible row should have had.
func TestListItems_PaginatesTheFilteredSet(t *testing.T) {
	repo := &mockArmoryRepo{
		listIDsFn: func(_ context.Context, _ string, _ []int, _ ItemListOptions) ([]string, error) {
			return []string{"a", "restricted", "b", "c"}, nil
		},
	}
	tf := &mockTypeFinder{findIDsFn: func(_ context.Context, _ string) ([]int, error) { return []int{1}, nil }}
	vf := &mockVisibilityFilter{viewable: map[string]bool{"a": true, "b": true, "c": true}}
	svc := newTestArmoryService(repo, tf, vf)

	opts := ItemListOptions{Page: 1, PerPage: 2, Sort: "name"}
	cards, total, err := svc.ListItems(context.Background(), "camp-1", permissions.RolePlayer, "player-1", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3 (restricted must not count)", total)
	}
	if len(cards) != 2 || cards[0].ID != "a" || cards[1].ID != "b" {
		t.Fatalf("page 1 = %v, want [a b] (restricted skipped, no gap left in the page)", repo.lastByIDsArg)
	}
}
