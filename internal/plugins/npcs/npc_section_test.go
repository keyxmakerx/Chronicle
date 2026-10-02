package npcs

// npc_section_test.go covers the Characters-page NPC list controls: query
// parsing, the "Showing X of N" pager, tag-option visibility, and the section
// fragment's access rules.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestNewNPCPager(t *testing.T) {
	tests := []struct {
		name                 string
		page, perPage, total int
		shown                int
		hasMore              bool
		nextPage, nextCount  int
	}{
		{"empty", 1, 60, 0, 0, false, 0, 0},
		{"fits on one page", 1, 60, 45, 45, false, 0, 0},
		{"exactly one full page", 1, 60, 60, 60, false, 0, 0},
		{"more after page 1", 1, 60, 150, 60, true, 2, 60},
		{"short final page shows everything", 2, 60, 100, 100, false, 0, 0},
		{"page 2 of 3, last page short", 2, 60, 150, 120, true, 3, 30},
		{"page below 1 is clamped", 0, 60, 90, 60, true, 2, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewNPCPager(tt.page, tt.perPage, tt.total)
			if p.Shown != tt.shown || p.Total != tt.total || p.HasMore != tt.hasMore || p.NextPage != tt.nextPage || p.NextCount != tt.nextCount {
				t.Errorf("NewNPCPager(%d,%d,%d) = %+v, want shown=%d hasMore=%v next=%d count=%d",
					tt.page, tt.perPage, tt.total, p, tt.shown, tt.hasMore, tt.nextPage, tt.nextCount)
			}
		})
	}
}

func TestParseNPCSectionQuery(t *testing.T) {
	offered := []NPCTagInfo{{ID: 1, Slug: "villain"}, {ID: 2, Slug: "merchant"}}
	long := strings.Repeat("a", 250)

	tests := []struct {
		name         string
		q, tag, page string
		wantSearch   string
		wantTag      string
		wantPage     int
	}{
		{"defaults", "", "", "", "", "", 1},
		{"trims search", "  bob  ", "", "", "bob", "", 1},
		{"search is bounded", long, "", "", long[:100], "", 1},
		{"offered tag kept", "", "villain", "", "", "villain", 1},
		{"unoffered tag dropped (e.g. a GM-only slug)", "", "secret-plot", "", "", "", 1},
		{"valid page", "", "", "3", "", "", 3},
		{"zero page", "", "", "0", "", "", 1},
		{"negative page", "", "", "-4", "", "", 1},
		{"garbage page", "", "", "abc", "", "", 1},
		{"huge page is capped", "", "", "99999999", "", "", maxNPCPage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, tag, page := ParseNPCSectionQuery(tt.q, tt.tag, tt.page, offered)
			if s != tt.wantSearch || tag != tt.wantTag || page != tt.wantPage {
				t.Errorf("got (%q, %q, %d), want (%q, %q, %d)", s, tag, page, tt.wantSearch, tt.wantTag, tt.wantPage)
			}
		})
	}
}

func TestSectionURL_CarriesFilters(t *testing.T) {
	tests := []struct {
		name, search, tag string
		page              int
		want              string
	}{
		{"page only", "", "", 2, "/campaigns/c1/npcs/section?page=2"},
		{"search and tag", "old man", "merchant", 3, "/campaigns/c1/npcs/section?page=3&q=old+man&tag=merchant"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sectionURL("c1", tt.search, tt.tag, tt.page); got != tt.want {
				t.Errorf("sectionURL = %q, want %q", got, tt.want)
			}
		})
	}
}

// fakeTagLister returns per-entity tags and records the includeDmOnly flag.
type fakeTagLister struct {
	tags          map[string][]TagInfo
	includeDmOnly bool
}

func (f *fakeTagLister) ListTagsForEntities(_ context.Context, ids []string, includeDmOnly bool) (map[string][]TagInfo, error) {
	f.includeDmOnly = includeDmOnly
	out := map[string][]TagInfo{}
	for _, id := range ids {
		if t, ok := f.tags[id]; ok {
			out[id] = t
		}
	}
	return out, nil
}

// TestListTags_OnlyTagsOnVisibleNPCs pins that a tag carried solely by an NPC
// the viewer cannot see is not offered, and that the dm_only flag is passed on.
func TestListTags_OnlyTagsOnVisibleNPCs(t *testing.T) {
	repo := &mockNPCRepo{
		listIDsFn: func(context.Context, string, []int, NPCListOptions) ([]string, error) {
			return []string{"a", "b", "hidden"}, nil
		},
	}
	vf := &mockVisibilityFilter{viewable: map[string]bool{"a": true, "b": true}}
	svc := NewNPCService(repo, &mockTypeFinder{typeID: 1}, vf)
	tl := &fakeTagLister{tags: map[string][]TagInfo{
		"a":      {{ID: 2, Name: "Merchant", Slug: "merchant"}, {ID: 1, Name: "Ally", Slug: "ally"}},
		"b":      {{ID: 2, Name: "Merchant", Slug: "merchant"}},
		"hidden": {{ID: 9, Name: "Traitor", Slug: "traitor"}},
	}}
	svc.SetTagLister(tl)

	got, err := svc.ListTags(context.Background(), "camp", 1, "u1", false)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(got) != 2 || got[0].Slug != "ally" || got[1].Slug != "merchant" {
		t.Errorf("tags = %+v, want [ally merchant] (deduped, name-sorted, no hidden-only tag)", got)
	}
	if tl.includeDmOnly {
		t.Error("a player-facing call must not request dm_only tags")
	}

	if _, err := svc.ListTags(context.Background(), "camp", 1, "u1", true); err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if !tl.includeDmOnly {
		t.Error("includeDmOnly=true should reach the tag lister")
	}
}

func TestListTags_NoTagListerIsEmpty(t *testing.T) {
	svc := NewNPCService(&mockNPCRepo{}, &mockTypeFinder{typeID: 1}, nil)
	got, err := svc.ListTags(context.Background(), "camp", 1, "u1", false)
	if err != nil || len(got) != 0 {
		t.Errorf("ListTags = %v, %v; want empty, nil", got, err)
	}
}

func TestCanSeeDmOnlyTags(t *testing.T) {
	tests := []struct {
		name string
		cc   campaigns.CampaignContext
		want bool
	}{
		{"player", campaigns.CampaignContext{MemberRole: campaigns.RolePlayer}, false},
		{"scribe", campaigns.CampaignContext{MemberRole: campaigns.RoleScribe}, false},
		{"anonymous", campaigns.CampaignContext{}, false},
		{"owner", campaigns.CampaignContext{MemberRole: campaigns.RoleOwner}, true},
		{"co-DM", campaigns.CampaignContext{MemberRole: campaigns.RolePlayer, IsDmGranted: true}, true},
		{"site admin", campaigns.CampaignContext{IsSiteAdmin: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canSeeDmOnlyTags(&tt.cc); got != tt.want {
				t.Errorf("canSeeDmOnlyTags = %v, want %v", got, tt.want)
			}
		})
	}
}

// serveSection drives GET /npcs/section as an anonymous visitor.
func serveSection(t *testing.T, campaign *campaigns.Campaign, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	RegisterRoutes(e, NewHandler(stubNPCSvc{}),
		stubCampaignSvc{campaign: campaign}, stubAuthSvc{}, stubAddonSvc{enabled: true})
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/npcs/section?q=bob&page=1", nil)
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestSectionFragment_Access(t *testing.T) {
	tests := []struct {
		name     string
		campaign *campaigns.Campaign
		htmx     bool
		wantCode int
		wantLoc  string
	}{
		{"anonymous on a public campaign gets the fragment", &campaigns.Campaign{ID: "camp-1", IsPublic: true}, true, http.StatusOK, ""},
		{"direct hit goes back to the Characters page", &campaigns.Campaign{ID: "camp-1", IsPublic: true}, false, http.StatusFound, "/campaigns/camp-1/characters"},
		{"anonymous on a private campaign is sent to login", &campaigns.Campaign{ID: "camp-1"}, true, http.StatusFound, "/login"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveSection(t, tt.campaign, tt.htmx)
			if rec.Code != tt.wantCode {
				t.Fatalf("code = %d, want %d", rec.Code, tt.wantCode)
			}
			if loc := rec.Header().Get("Location"); loc != tt.wantLoc {
				t.Errorf("Location = %q, want %q", loc, tt.wantLoc)
			}
		})
	}
}
