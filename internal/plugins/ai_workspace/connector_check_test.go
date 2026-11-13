package ai_workspace

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// typesNoPages answers the category list and fails the test on any page
// lookup, since what a connected AI hears must not say whether a page exists.
type typesNoPages struct {
	t *testing.T
	// pages lists the pages that exist, by slug; nil makes any lookup a failure.
	pages map[string]*entities.Entity
}

func (s typesNoPages) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return []entities.EntityType{
		{ID: 1, Slug: "lore", Name: "Lore", NamePlural: "Lore"},
		{ID: 2, Slug: "places", Name: "Places", NamePlural: "Places"},
		{ID: 3, Slug: "race", Name: "Race", NamePlural: "Races", Fields: []entities.FieldDefinition{{Key: "size", Label: "Size", Type: "text"}}},
	}, nil
}
func (s typesNoPages) GetBySlug(_ context.Context, _, slug string) (*entities.Entity, error) {
	if s.pages == nil {
		s.t.Error("the check looked up a page")
	}
	return s.pages[slug], nil
}
func (s typesNoPages) GetEntityTypeBySlug(context.Context, string, string) (*entities.EntityType, error) {
	return nil, nil
}
func (s typesNoPages) CheckEntityAccess(context.Context, string, int, string) (*entities.EffectivePermission, error) {
	s.t.Error("the check looked up a page")
	return nil, nil
}

// foldersIn has a Races folder in Lore and a Docks folder in Places.
type foldersIn struct{}

func (foldersIn) FindFolder(_ context.Context, _ string, typeID int, name string) (*entities.SidebarNode, error) {
	for _, f := range []struct {
		typeID int
		name   string
	}{{1, "races"}, {2, "docks"}} {
		if f.typeID == typeID && strings.EqualFold(name, f.name) {
			return &entities.SidebarNode{ID: "f", Name: name, EntityTypeID: typeID}, nil
		}
	}
	return nil, nil
}
func (foldersIn) SetEntityFolder(context.Context, string, string, *string) error { return nil }

// A new page filed in a folder its category doesn't have is told to the AI,
// with where the folder is and the fix.
func TestConnectorCheck(t *testing.T) {
	khepri := func(typ, folder string) string {
		return "---\nname: The Khepri\ntype: " + typ + "\nfolder: " + folder + "\n---\n\n# The Khepri\n\nBeetle folk.\n"
	}
	tests := []struct {
		name    string
		changes string
		want    []string
	}{
		{"folder in another category", khepri("races", "Races"),
			[]string{`block 1 (The Khepri): the Race category has no folder "Races"; Lore has one. Use type: Lore, or leave the folder out`}},
		{"folder nowhere", khepri("race", "Desert folk"),
			[]string{`block 1 (The Khepri): the Race category has no folder "Desert folk". Make it first with a folder block in this request, or leave the folder out`}},
		{"new category", khepri("peoples", "Docks"),
			[]string{`block 1 (The Khepri): the category "peoples" is new, so it has no folder "Docks"; Places has one. Use type: Places, or leave the folder out`}},
		{"new category, folder nowhere", khepri("peoples", "Desert folk"),
			[]string{`block 1 (The Khepri): the category "peoples" is new, so it has no folder "Desert folk". Use an existing category, or leave the folder out`}},
		{"right category", khepri("lore", "Races"), nil},
		{"folder made in the request", "---\nkind: folder\nname: Desert folk\ncategory: Race\n---\n\n" + khepri("race", "Desert folk"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{importLookup: typesNoPages{t: t, pages: map[string]*entities.Entity{}}, importFolders: foldersIn{}}
			got, err := h.ConnectorCheck(context.Background(), &connector.Connection{CampaignID: "camp-1", UserID: "owner-1"}, tt.changes)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) == 0 {
				got = nil
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q\nwant %q", got, tt.want)
			}
		})
	}
}

// benchKindStub refuses a bench whose kind it doesn't know and records the
// role it was planned as.
type benchKindStub struct{ roles *[]int }

func (benchKindStub) Name() string  { return "bench" }
func (benchKindStub) Label() string { return "Bench" }
func (benchKindStub) Doc() string   { return "" }
func (k benchKindStub) Plan(_ context.Context, _ string, a records.Actor, r records.Record) records.Plan {
	*k.roles = append(*k.roles, a.Role)
	if r.Str("kind_of_bench") != "forge" {
		return records.Plan{Error: `kind_of_bench: "teleporter" is not a kind of bench`}
	}
	return records.Plan{Summary: "a new bench"}
}
func (benchKindStub) Apply(context.Context, string, records.Actor, records.Record) error { return nil }
func (benchKindStub) Export(context.Context, string, records.Actor) (string, error)      { return "", nil }

// A block the review would mark unusable is refused when it is asked for,
// with the review's reason, for a key that reads as the owner. A Safe key
// reads as a player and isn't told, so a reason can't reveal hidden pages.
func TestConnectorCheckPlansRecords(t *testing.T) {
	bench := func(kind string) string {
		return "---\nkind: bench\nname: Lab\nkind_of_bench: " + kind + "\n---\n"
	}
	tests := []struct {
		name    string
		privacy aiexport.PrivacyMode
		off     bool
		changes string
		want    []string
	}{
		{"broken block", aiexport.PrivacyModePermitted, false, bench("teleporter"),
			[]string{`block 1 (bench Lab): kind_of_bench: "teleporter" is not a kind of bench`}},
		{"good block", aiexport.PrivacyModePermitted, false, bench("forge"), nil},
		{"feature switched off", aiexport.PrivacyModePermitted, true, bench("forge"),
			[]string{"block 1 (bench Lab): " + records.OffFeatureReason("Crafting")}},
		{"a Safe key isn't told", aiexport.PrivacyModeSafe, false, bench("teleporter"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var roles []int
			h := &Handler{importLookup: typesNoPages{t: t, pages: map[string]*entities.Entity{}}, importFolders: foldersIn{}}
			h.SetRecords(records.NewRegistry(benchKindStub{&roles}))
			if tt.off {
				h.featureGate = func(context.Context, string) records.OffFeatures { return records.OffFeatures{"bench": "Crafting"} }
			}
			got, err := h.ConnectorCheck(context.Background(), &connector.Connection{CampaignID: "camp-1", UserID: "owner-1", Privacy: tt.privacy}, tt.changes)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) == 0 {
				got = nil
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q\nwant %q", got, tt.want)
			}
			for _, r := range roles {
				if r != int(campaigns.RoleOwner) {
					t.Errorf("planned as role %d, want the owner", r)
				}
			}
		})
	}
}

// An update or delete of a page that doesn't exist, and sheet fields the
// page's category lacks, are refused for a key that reads as the owner. A
// Safe key reads as a player: it isn't told, and no page is looked up.
func TestConnectorCheckPageProblems(t *testing.T) {
	block := func(action, typ, fields string) string {
		return "---\nname: Nobody Here\ntype: " + typ + "\naction: " + action + "\n" + fields + "---\nx"
	}
	exists := map[string]*entities.Entity{"khepri": {ID: "e1", Name: "Khepri", Slug: "khepri", EntityTypeID: 3}}
	tests := []struct {
		name    string
		privacy aiexport.PrivacyMode
		changes string
		want    string
	}{
		{"update of a missing page", aiexport.PrivacyModePermitted, block("update", "race", ""), `Cannot update: no existing entity named "Nobody Here"`},
		{"delete of a missing page", aiexport.PrivacyModePermitted, block("delete", "race", ""), `Cannot delete: no existing entity named "Nobody Here"`},
		{"unknown sheet field", aiexport.PrivacyModePermitted, block("create", "race", "fields:\n  nonsense: 1\n"), `"nonsense" is not one of this category's fields`},
		{"known sheet field", aiexport.PrivacyModePermitted, block("create", "race", "fields:\n  size: big\n"), ""},
		{"new category is left to the review", aiexport.PrivacyModePermitted, block("create", "peoples", "fields:\n  size: big\n"), ""},
		{"update of a page that exists", aiexport.PrivacyModePermitted, "---\nname: Khepri\ntype: race\naction: update\n---\nx", ""},
		{"Safe key is not told", aiexport.PrivacyModeSafe, block("update", "race", ""), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pages := exists
			if tt.privacy == aiexport.PrivacyModeSafe {
				pages = nil // any lookup fails the test
			}
			h := &Handler{importLookup: typesNoPages{t: t, pages: pages}, importFolders: foldersIn{}}
			got, err := h.ConnectorCheck(context.Background(), &connector.Connection{CampaignID: "camp-1", UserID: "owner-1", Privacy: tt.privacy}, tt.changes)
			if err != nil {
				t.Fatal(err)
			}
			if tt.want == "" {
				if len(got) != 0 {
					t.Errorf("got %q, want no problem", got)
				}
				return
			}
			if _, did, _ := h.ConnectorCheckDid(context.Background(), &connector.Connection{CampaignID: "camp-1", UserID: "owner-1", Privacy: tt.privacy}, tt.changes); !strings.HasPrefix(did, "Asked to ") || !strings.Contains(did, "Nobody Here, refused: ") {
				t.Errorf("did = %q", did)
			}
			if len(got) != 1 || !strings.Contains(got[0], tt.want) || !strings.HasPrefix(got[0], "block 1 (Nobody Here): ") {
				t.Errorf("got %q, want one line with %q", got, tt.want)
			}
		})
	}
}
