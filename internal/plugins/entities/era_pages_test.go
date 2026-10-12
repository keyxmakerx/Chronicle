package entities

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeEraPages serves one set of eras and a version per era id.
type fakeEraPages struct {
	EraPages
	set      *PageEraSet
	versions map[int]*PageEraVersion
}

func (f *fakeEraPages) PageEras(context.Context, string, permissions.Viewer) (*PageEraSet, error) {
	return f.set, nil
}
func (f *fakeEraPages) Version(_ context.Context, _ string, eraID int) (*PageEraVersion, error) {
	return f.versions[eraID], nil
}
func (f *fakeEraPages) VersionEraIDs(context.Context, string) ([]int, error) {
	var ids []int
	for id := range f.versions {
		ids = append(ids, id)
	}
	return ids, nil
}

func TestResolveEraView(t *testing.T) {
	name := func(s string) *string { return &s }
	pages := &fakeEraPages{
		set: &PageEraSet{CurrentID: 2, Eras: []PageEra{{ID: 1, Name: "Embers"}, {ID: 2, Name: "Ash"}, {ID: 3, Name: "Frost"}}},
		versions: map[int]*PageEraVersion{
			2: {Name: name("Ashgate")},
			3: {Name: name("Frostgate")},
		},
	}
	h := &Handler{eraPages: pages}
	camp := &campaigns.Campaign{ID: "c1"}
	owner := &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RoleOwner}
	player := &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RolePlayer}
	e := &Entity{ID: "e1", Name: "Emberfall"}
	tests := []struct {
		name          string
		cc            *campaigns.CampaignContext
		era, hl       string
		wantShown     int
		wantHighlight bool
	}{
		{"a player sees the current era's version", player, "", "", 2, false},
		{"a player can't pick another era", player, "3", "", 2, false},
		{"the GM picks another era, highlighted", owner, "3", "", 3, true},
		{"the GM turns highlights off to edit", owner, "3", "0", 3, false},
		{"the GM's usual page shows no version", owner, "usual", "", 0, false},
		{"an era with no version shows the usual page", owner, "1", "", 0, false},
		{"an unknown era is ignored", owner, "99", "", 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := h.resolveEraView(context.Background(), tt.cc, e, "u", tt.era, tt.hl)
			if v.ShownID != tt.wantShown || v.Highlight != tt.wantHighlight {
				t.Errorf("shown=%d highlight=%v, want %d %v", v.ShownID, v.Highlight, tt.wantShown, tt.wantHighlight)
			}
		})
	}
}

func TestMayWriteEraVersion(t *testing.T) {
	set := &PageEraSet{CurrentID: 2, Eras: []PageEra{{ID: 1}, {ID: 2}}}
	camp := &campaigns.Campaign{ID: "c1"}
	owner := &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RoleOwner}
	scribe := &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RoleScribe}
	tests := []struct {
		name string
		cc   *campaigns.CampaignContext
		era  int
		want bool
	}{
		{"the GM writes any era", owner, 1, true},
		{"the GM can't write an era not in the set", owner, 9, false},
		{"a scribe writes the current era", scribe, 2, true},
		{"a scribe can't write another era", scribe, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mayWriteEraVersion(tt.cc, set, tt.era); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWidgetEndpoint(t *testing.T) {
	ctx := withEraView(context.Background(), &eraView{Version: &PageEraVersion{}, ShownID: 4})
	if got := widgetEndpoint(ctx, "c1", "e1", "entry"); got != "/campaigns/c1/entities/e1/eras/4/entry" {
		t.Errorf("version endpoint = %s", got)
	}
	if got := widgetEndpoint(context.Background(), "c1", "e1", "entry"); got != "/campaigns/c1/entities/e1/entry" {
		t.Errorf("usual endpoint = %s", got)
	}
}
