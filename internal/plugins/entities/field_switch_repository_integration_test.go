package entities

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// The switch is worked out before the repository takes its lock, so the
// repository must refuse one whose category changed in between: two "Switch
// back" presses at once must move each page's values once, not twice.
func TestFieldSwitchRepository_MovedBase_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	owner, campaignID := testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		owner, "switch-int-"+owner+"@example.test", "Switch Int Test", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Switch Int Test", "switch-int-"+campaignID[:8], owner)
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, owner)
	}()

	types := NewEntityTypeRepository(db)
	repo := NewFieldSwitchRepository(db)
	offers := func(context.Context, string, *EntityType) *FieldSwitchOffer {
		o := treasureOffer()
		return &o
	}
	svc := NewFieldSwitchService(repo, types, offers)
	original := map[string]any{"type": "Wondrous item", "rarity": "Rare", "weight": "1 lb"}

	newItems := func(t *testing.T, slug string) (*EntityType, string) {
		t.Helper()
		et := &EntityType{
			CampaignID: campaignID, Slug: slug, Name: "Item", NamePlural: "Items",
			Icon: "fa-box", Color: "#111111", Fields: itemFields(), Layout: DefaultLayout(), Enabled: true,
		}
		mustCreate(t, types, ctx, et)
		page := testUUID(t)
		data, _ := json.Marshal(original)
		mustExec(t, db, `INSERT INTO entities (id, campaign_id, entity_type_id, name, slug, fields_data, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`, page, campaignID, et.ID, slug+"-p1", slug+"-p1", data, owner)
		return et, page
	}
	pageData := func(t *testing.T, id string) map[string]any {
		t.Helper()
		var raw []byte
		if err := db.QueryRow(`SELECT fields_data FROM entities WHERE id = ?`, id).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		out := map[string]any{}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	t.Run("a stale base is refused and changes nothing", func(t *testing.T) {
		et, page := newItems(t, "switch-int-stale")
		if _, err := svc.Accept(ctx, campaignID, et.ID, owner); err != nil {
			t.Fatalf("Accept: %v", err)
		}
		switched := pageData(t, page)
		// Worked out from the category as it was before the accept.
		offer := treasureOffer()
		cat := offer.Category
		stale := switchBase{Fields: itemFields()}
		_, err := repo.Switch(ctx, &FieldSwitch{EntityTypeID: et.ID, CampaignID: campaignID, SystemID: "drawsteel", PresetSlug: "drawsteel-item", State: FieldSwitchSwitched, OldFields: itemFields()},
			stale, switchedFieldList(itemFields(), offer), &cat, acceptMove(itemFields(), offer))
		if switchErrCode(err) != 409 {
			t.Fatalf("stale Switch = %v, want a conflict", err)
		}
		if got := pageData(t, page); !reflect.DeepEqual(got, switched) {
			t.Errorf("page after a refused switch = %v, want %v", got, switched)
		}
	})

	t.Run("two switch backs at once move the values once", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			et, page := newItems(t, "switch-int-race-"+string(rune('a'+i)))
			if _, err := svc.Accept(ctx, campaignID, et.ID, owner); err != nil {
				t.Fatalf("Accept: %v", err)
			}
			var wg sync.WaitGroup
			errs := make([]error, 2)
			for j := range errs {
				wg.Add(1)
				go func(j int) {
					defer wg.Done()
					_, errs[j] = svc.Back(ctx, campaignID, et.ID, owner)
				}(j)
			}
			wg.Wait()
			ok, refused := 0, 0
			for _, err := range errs {
				switch {
				case err == nil:
					ok++
				case switchErrCode(err) == 409:
					refused++
				default:
					t.Fatalf("Back: %v", err)
				}
			}
			if ok != 1 || refused != 1 {
				t.Fatalf("round %d: %d succeeded, %d refused (%v); want one each", i, ok, refused, errs)
			}
			if got := pageData(t, page); !reflect.DeepEqual(got, original) {
				t.Fatalf("round %d: page after the race = %v, want %v", i, got, original)
			}
		}
	})
}

// A value the switch sets aside leaves the page's fields, so search must stop
// finding it (and quoting it) for a player as soon as the switch is made, and
// find it again once switching back puts it back.
func TestFieldSwitch_SearchFollowsTheSwitch_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	owner, campaignID := testUUID(t), testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		owner, "switch-search-"+owner+"@example.test", "Switch Search Test", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Switch Search Test", "switch-search-"+campaignID[:8], owner)
	defer func() {
		mustExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
		mustExec(t, db, `DELETE FROM users WHERE id = ?`, owner)
	}()

	types := NewEntityTypeRepository(db)
	pages := NewEntityRepository(db)
	offers := func(context.Context, string, *EntityType) *FieldSwitchOffer {
		o := treasureOffer()
		return &o
	}
	svc := NewFieldSwitchService(NewFieldSwitchRepository(db), types, offers)
	svc.SetSearchReindex(func(ctx context.Context, typeID int) (int, error) {
		return ReindexSearchText(ctx, pages, types, typeID)
	})

	et := &EntityType{
		CampaignID: campaignID, Slug: "switch-search-items", Name: "Item", NamePlural: "Items",
		Icon: "fa-box", Color: "#111111", Fields: itemFields(), Layout: DefaultLayout(), Enabled: true,
	}
	mustCreate(t, types, ctx, et)
	now := time.Now().UTC()
	page := &Entity{
		ID: testUUID(t), CampaignID: campaignID, EntityTypeID: et.ID, Name: "Lantern", Slug: "lantern-" + testUUID(t)[:6],
		FieldsData: map[string]any{"rarity": "Duskglass", "weight": "1 lb"}, CreatedBy: owner, CreatedAt: now, UpdatedAt: now,
	}
	if err := pages.Create(ctx, page); err != nil {
		t.Fatalf("create page: %v", err)
	}
	if _, err := ReindexSearchText(ctx, pages, types, et.ID); err != nil {
		t.Fatalf("index: %v", err)
	}
	playerFinds := func(t *testing.T) bool {
		t.Helper()
		got, _, err := pages.Search(ctx, campaignID, "Duskglass", nil, permissions.RolePlayer, testUUID(t), DefaultListOptions())
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		return len(got) > 0
	}
	if !playerFinds(t) {
		t.Fatal("before the switch a player should find the page by its Rarity")
	}
	if _, err := svc.Accept(ctx, campaignID, et.ID, owner); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if playerFinds(t) {
		t.Error("after the switch a player still finds the page by a value set aside")
	}
	if _, err := svc.Back(ctx, campaignID, et.ID, owner); err != nil {
		t.Fatalf("Back: %v", err)
	}
	if !playerFinds(t) {
		t.Error("after switching back the Rarity is on the page again and should be found")
	}
}
