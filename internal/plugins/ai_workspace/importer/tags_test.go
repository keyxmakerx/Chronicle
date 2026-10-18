package importer

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// fakeTags is an in-memory campaign tag set plus each page's tags.
type fakeTags struct {
	campaign []tags.Tag
	onPage   map[string][]int // entity ID → tag IDs
	setCalls int
	created  []string
	setErr   error
}

func (f *fakeTags) ListByCampaign(_ context.Context, _ string, _ bool) ([]tags.Tag, error) {
	return f.campaign, nil
}

func (f *fakeTags) Create(_ context.Context, campaignID, name, _ string, dmOnly bool) (*tags.Tag, error) {
	t := tags.Tag{ID: 100 + len(f.campaign), CampaignID: campaignID, Name: name, Slug: tagSlug(name), DmOnly: dmOnly}
	f.campaign = append(f.campaign, t)
	f.created = append(f.created, name)
	return &t, nil
}

func (f *fakeTags) GetEntityTags(_ context.Context, entityID string, _ bool) ([]tags.Tag, error) {
	var out []tags.Tag
	for _, id := range f.onPage[entityID] {
		for _, t := range f.campaign {
			if t.ID == id {
				out = append(out, t)
			}
		}
	}
	return out, nil
}

func (f *fakeTags) SetEntityTags(_ context.Context, entityID, _ string, ids []int) error {
	f.setCalls++
	if f.setErr != nil {
		return f.setErr
	}
	if f.onPage == nil {
		f.onPage = map[string][]int{}
	}
	f.onPage[entityID] = append([]int(nil), ids...)
	return nil
}

func campaignTags() []tags.Tag {
	return []tags.Tag{
		{ID: 1, Name: "Trade Hub", Slug: "trade-hub"},
		{ID: 2, Name: "Coastal", Slug: "coastal"},
		{ID: 3, Name: "Secret Cult", Slug: "secret-cult", DmOnly: true},
	}
}

func sortedIDs(ids []int) []int {
	out := append([]int(nil), ids...)
	sort.Ints(out)
	return out
}

// Pins that a page block's `tags:` reaches the page: matched by slug, new
// names made, absent keeps, [] clears, and an update never drops GM-only tags
// the AI may not have been shown.
func TestCommit_PageTags(t *testing.T) {
	tests := []struct {
		name        string
		tags        []string
		update      bool
		before      []int
		wantIDs     []int
		wantSet     bool
		wantCreated []string
	}{
		{name: "create matches by slug", tags: []string{"trade hub", "Coastal"}, wantIDs: []int{1, 2}, wantSet: true},
		{name: "create makes a new tag", tags: []string{"Haunted"}, wantIDs: []int{103}, wantSet: true, wantCreated: []string{"Haunted"}},
		{name: "duplicates collapse", tags: []string{"coastal", "COASTAL"}, wantIDs: []int{2}, wantSet: true},
		{name: "no tags line on create sets nothing", tags: nil, wantSet: false},
		{name: "no tags line on update keeps them", tags: nil, update: true, before: []int{1, 3}, wantIDs: []int{1, 3}, wantSet: false},
		{name: "update replaces but keeps GM-only", tags: []string{"coastal"}, update: true, before: []int{1, 3}, wantIDs: []int{2, 3}, wantSet: true},
		{name: "empty list clears all but GM-only", tags: []string{}, update: true, before: []int{1, 2, 3}, wantIDs: []int{3}, wantSet: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft := &fakeTags{campaign: campaignTags(), onPage: map[string][]int{}}
			f := &fakeCreator{types: charTypes()}
			id := "ent-lyra-vance"
			conflict := "rename"
			if tt.update {
				f.existing = map[string]*entities.Entity{"lyra-vance": {ID: id, Name: "Lyra Vance", Slug: "lyra-vance"}}
				ft.onPage[id] = tt.before
				conflict = "update"
			}
			p := page("Lyra Vance", "character", "Body.")
			p.FrontMatter.Tags = tt.tags
			c := NewCommitter(f)
			c.SetTagWriter(ft)
			res, err := c.Commit(context.Background(), "camp-1", CommitInput{
				OwnerID:   "u-1",
				Pages:     []ParsedPage{p},
				Decisions: []RowDecision{decision(true, "Lyra Vance", "character", "", conflict)},
			})
			if err != nil {
				t.Fatalf("Commit: %v", err)
			}
			if s := res.Rows[0].Status; s == StatusFailed || s == StatusSkipped {
				t.Fatalf("row %s: %s", s, res.Rows[0].Reason)
			}
			if (ft.setCalls > 0) != tt.wantSet {
				t.Fatalf("SetEntityTags called %d times; want called=%v", ft.setCalls, tt.wantSet)
			}
			if tt.wantSet || tt.update {
				if got := sortedIDs(ft.onPage[id]); !reflect.DeepEqual(got, sortedIDs(tt.wantIDs)) {
					t.Errorf("page tags = %v; want %v", got, tt.wantIDs)
				}
			}
			if !reflect.DeepEqual(ft.created, tt.wantCreated) {
				t.Errorf("created tags = %v; want %v", ft.created, tt.wantCreated)
			}
		})
	}
}

// A page saved without its tags must say so on the row, never pass silently.
func TestCommit_PageTagsFailureIsReported(t *testing.T) {
	tests := []struct {
		name   string
		writer TagWriter
		tags   []string
		want   string
	}{
		{name: "not wired", writer: nil, tags: []string{"coastal"}, want: "tags could not be set"},
		{name: "write fails", writer: &fakeTags{campaign: campaignTags(), setErr: errors.New("db down")}, tags: []string{"coastal"}, want: "could not set its tags"},
		{name: "name too long", writer: &fakeTags{campaign: campaignTags()}, tags: []string{strings.Repeat("x", 101)}, want: "longer than 100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeCreator{types: charTypes()}
			c := NewCommitter(f)
			if tt.writer != nil {
				c.SetTagWriter(tt.writer)
			}
			p := page("Lyra Vance", "character", "Body.")
			p.FrontMatter.Tags = tt.tags
			res, err := c.Commit(context.Background(), "camp-1", CommitInput{
				OwnerID:   "u-1",
				Pages:     []ParsedPage{p},
				Decisions: []RowDecision{decision(true, "Lyra Vance", "character", "", "rename")},
			})
			if err != nil {
				t.Fatalf("Commit: %v", err)
			}
			row := res.Rows[0]
			if row.Status != StatusFailed || !strings.Contains(row.Reason, tt.want) {
				t.Errorf("row = %s %q; want failed containing %q", row.Status, row.Reason, tt.want)
			}
			if row.EntityID == "" {
				t.Error("the page was saved, so the row should still link it")
			}
		})
	}
}

// tagSlug must agree with the tags service's own slug, or a name would make a
// second copy of a tag the campaign already has.
func TestTagSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Trade Hub":     "trade-hub",
		"  coastal  ":   "coastal",
		"Dragon's Lair": "dragon-s-lair",
		"--":            "tag",
		"Élan":          "lan",
	} {
		if got := tagSlug(in); got != want {
			t.Errorf("tagSlug(%q) = %q; want %q", in, got, want)
		}
	}
}

// `tags: []` must reach the committer as an empty list (clear), not nil (keep).
func TestParse_EmptyTagsListIsNotAbsent(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantNil bool
		wantLen int
	}{
		{name: "absent", yaml: "name: Lyra\ntype: character", wantNil: true},
		{name: "empty", yaml: "name: Lyra\ntype: character\ntags: []", wantLen: 0},
		{name: "two", yaml: "name: Lyra\ntype: character\ntags: [a, b]", wantLen: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pages := Parse("---\n" + tt.yaml + "\n---\n# Lyra\n\nBody.\n")
			if len(pages) != 1 {
				t.Fatalf("got %d pages", len(pages))
			}
			got := pages[0].FrontMatter.Tags
			if (got == nil) != tt.wantNil || len(got) != tt.wantLen {
				t.Errorf("Tags = %#v; want nil=%v len=%d", got, tt.wantNil, tt.wantLen)
			}
		})
	}
}
