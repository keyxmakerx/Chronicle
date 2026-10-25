package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type menuCampaignStub struct {
	campaigns.CampaignService
	config  string
	member  *campaigns.CampaignMember
	err     error
	ordered []int
}

func (m *menuCampaignStub) GetByID(context.Context, string) (*campaigns.Campaign, error) {
	return &campaigns.Campaign{ID: "c", SidebarConfig: m.config}, nil
}
func (m *menuCampaignStub) GetMember(context.Context, string, string) (*campaigns.CampaignMember, error) {
	return m.member, m.err
}
func (m *menuCampaignStub) OrderNavCategories(_ context.Context, _ string, _ []campaigns.NavApp, _ []campaigns.NavCategory, order []int) error {
	m.ordered = order
	return nil
}

type menuTypesStub []entities.EntityType

func (t menuTypesStub) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return t, nil
}

func TestAIMenuAdapter_IsOwner(t *testing.T) {
	tests := []struct {
		name string
		stub menuCampaignStub
		user string
		want bool
		err  bool
	}{
		{"owner", menuCampaignStub{member: &campaigns.CampaignMember{Role: campaigns.RoleOwner}}, "u", true, false},
		{"scribe", menuCampaignStub{member: &campaigns.CampaignMember{Role: campaigns.RoleScribe}}, "u", false, false},
		{"not a member", menuCampaignStub{err: apperror.NewNotFound("member not found")}, "u", false, false},
		{"lookup failed", menuCampaignStub{err: errors.New("db down")}, "u", false, true},
		{"no user", menuCampaignStub{member: &campaigns.CampaignMember{Role: campaigns.RoleOwner}}, "", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := aiMenuAdapter{campaigns: &tc.stub}.IsOwner(context.Background(), "c", tc.user)
			if got != tc.want || (err != nil) != tc.err {
				t.Errorf("got %v, %v; want %v, error %v", got, err, tc.want, tc.err)
			}
		})
	}
}

// The menu reads in the sidebar's order: a stored arrangement first, then
// categories it doesn't name; disabled categories are not in it.
func TestAIMenuAdapter_MenuCategories(t *testing.T) {
	types := menuTypesStub{
		{ID: 1, Name: "Character", NamePlural: "Characters", Slug: "character", Enabled: true},
		{ID: 2, Name: "Location", NamePlural: "Places", Slug: "location", Enabled: true},
		{ID: 3, Name: "Lore", NamePlural: "Lore", Slug: "lore", Enabled: true},
		{ID: 4, Name: "Old", NamePlural: "Olds", Slug: "old", Enabled: false},
	}
	stub := &menuCampaignStub{config: `{"items":[{"type":"category","type_id":3,"visible":true}]}`}
	m := aiMenuAdapter{campaigns: stub, types: types}
	got, err := m.MenuCategories(context.Background(), "c")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range got {
		names = append(names, c.Plural+"/"+c.Section)
	}
	if want := []string{"Lore/Categories", "Characters/Categories", "Places/Categories"}; !reflect.DeepEqual(names, want) {
		t.Errorf("menu %v, want %v", names, want)
	}

	after, err := m.PreviewOrder(context.Background(), "c", []int{2, 3})
	if err != nil {
		t.Fatal(err)
	}
	names = names[:0]
	for _, c := range after {
		names = append(names, c.Plural)
	}
	if want := []string{"Places", "Characters", "Lore"}; !reflect.DeepEqual(names, want) {
		t.Errorf("preview %v, want %v", names, want)
	}
	if err := m.OrderCategories(context.Background(), "c", []int{2, 3}); err != nil || !reflect.DeepEqual(stub.ordered, []int{2, 3}) {
		t.Errorf("ordered %v, %v", stub.ordered, err)
	}
}
