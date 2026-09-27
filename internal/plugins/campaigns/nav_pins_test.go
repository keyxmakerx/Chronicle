package campaigns

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// nav_pins_test.go pins a member's own sidebar pins: only a member who is not
// the owner has them, only rows their own sidebar shows can be pinned, and
// they are stored for that member alone.

// fakeNavSections draws a fixed sidebar for the pin check.
type fakeNavSections struct{ secs []NavSection }

func (f fakeNavSections) NavSectionsFor(context.Context, *CampaignContext) ([]NavSection, error) {
	return f.secs, nil
}

// playerSidebar is a player's sidebar before their own pins: the owner
// pinned Journal and Dates for everyone and hid cat:3, so it is not there.
func playerSidebar() []NavSection {
	items := []SidebarItem{
		{Type: "app", Slug: "notes", Section: NavSectionPinned, Visible: true},
		{Type: "app", Slug: "dates", Section: NavSectionPinned, Visible: true},
		{Type: "category", TypeID: 3, Visible: false},
	}
	return ViewNav(NormalizeNav(items, navTestApps(), navTestCats()), navTestApps(), navTestCats(),
		NavViewer{Access: NavAccessMember})
}

func TestPinnableNavKeys_LeavesOutPinnedAndHiddenRows(t *testing.T) {
	got := PinnableNavKeys(playerSidebar())
	for _, key := range []string{"app:maps", "app:characters", "cat:1", "cat:2"} {
		if !got[key] {
			t.Errorf("%s should be pinnable", key)
		}
	}
	for _, key := range []string{"app:notes", "app:dates", "cat:3", "app:forge", "cat:11"} {
		if got[key] {
			t.Errorf("%s must not be pinnable (already pinned, hidden, turned off, or a sub-category)", key)
		}
	}
}

func TestCleanNavPins(t *testing.T) {
	pinnable := map[string]bool{"app:maps": true, "cat:1": true}
	got, err := CleanNavPins([]string{"cat:1", "app:maps", "cat:1"}, pinnable)
	if err != nil || !reflect.DeepEqual(got, []string{"cat:1", "app:maps"}) {
		t.Fatalf("CleanNavPins = %v, %v; want the pins in order without repeats", got, err)
	}
	if _, err := CleanNavPins([]string{"app:maps", "cat:3"}, pinnable); err == nil {
		t.Errorf("a row the member cannot see must be refused")
	}
	if got, err := CleanNavPins(nil, pinnable); err != nil || len(got) != 0 {
		t.Errorf("no pins must be fine: %v, %v", got, err)
	}
	many := map[string]bool{}
	var keys []string
	for i := 0; i <= maxNavPins; i++ {
		k := NavCategoryKey(100 + i)
		many[k] = true
		keys = append(keys, k)
	}
	if _, err := CleanNavPins(keys, many); err == nil {
		t.Errorf("more than %d pins must be refused", maxNavPins)
	}
}

func navPinsService(repo *mockCampaignRepo) CampaignService {
	svc := NewCampaignService(repo, nil, nil, nil, "")
	svc.SetNavSectionsSource(fakeNavSections{secs: playerSidebar()})
	return svc
}

func memberContext(role Role, member bool) *CampaignContext {
	return &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: role, IsMember: member}
}

func TestUpdateNavPins_WhoMayPin(t *testing.T) {
	tests := []struct {
		name string
		cc   *CampaignContext
		ok   bool
	}{
		{"a player", memberContext(RolePlayer, true), true},
		{"a scribe", memberContext(RoleScribe, true), true},
		{"the owner, who pins for everyone in the editor", memberContext(RoleOwner, true), false},
		{"a signed-in visitor who is not a member", memberContext(RoleNone, false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockCampaignRepo{}
			_, err := navPinsService(repo).UpdateNavPins(context.Background(), tt.cc, "user-1", []string{"app:maps"})
			if tt.ok && err != nil {
				t.Fatalf("UpdateNavPins: %v", err)
			}
			if !tt.ok {
				var appErr *apperror.AppError
				if !errors.As(err, &appErr) || appErr.Code != 403 {
					t.Fatalf("want a 403, got %v", err)
				}
				if len(repo.navPins) != 0 {
					t.Errorf("nothing may be stored: %v", repo.navPins)
				}
			}
		})
	}
}

func TestUpdateNavPins_StoresOnlyTheCallersOwnVisibleRows(t *testing.T) {
	repo := &mockCampaignRepo{}
	svc := navPinsService(repo)
	ctx := context.Background()
	player := memberContext(RolePlayer, true)

	got, err := svc.UpdateNavPins(ctx, player, "user-1", []string{"cat:2", "app:maps", "cat:2"})
	if err != nil || !reflect.DeepEqual(got, []string{"cat:2", "app:maps"}) {
		t.Fatalf("UpdateNavPins = %v, %v", got, err)
	}
	if _, err := svc.UpdateNavPins(ctx, player, "user-1", []string{"cat:3"}); err == nil {
		t.Fatalf("pinning a row hidden from players must be refused")
	}
	if pins, _ := svc.NavPins(ctx, "camp-1", "user-1"); !reflect.DeepEqual(pins, []string{"cat:2", "app:maps"}) {
		t.Errorf("a refused update must leave the stored pins alone: %v", pins)
	}
	if pins, _ := svc.NavPins(ctx, "camp-1", "user-2"); len(pins) != 0 {
		t.Errorf("another member must never see these pins: %v", pins)
	}
	if _, err := svc.UpdateNavPins(ctx, player, "user-1", []string{}); err != nil {
		t.Fatalf("clearing pins: %v", err)
	}
	if pins, _ := svc.NavPins(ctx, "camp-1", "user-1"); len(pins) != 0 {
		t.Errorf("pins must clear: %v", pins)
	}
}

func TestUpdateNavPins_RefusedUntilWired(t *testing.T) {
	svc := NewCampaignService(&mockCampaignRepo{}, nil, nil, nil, "")
	if _, err := svc.UpdateNavPins(context.Background(), memberContext(RolePlayer, true), "user-1", []string{"app:maps"}); err == nil {
		t.Fatalf("without a sidebar to check against, pins must be refused")
	}
}
