package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// homesFake is a campaign with a saved Players home holding written blocks, a
// Directors home with a tile section, and nothing saved for the rest.
func homesFake() *fakeHome {
	return &fakeHome{owner: "owner", homes: map[string]HomeLayout{
		"player": roundTripHome(),
		"director": {Tiles: true, TileBlocks: []string{"activity_feed", "text_block"},
			Rows: []HomeRow{{ID: "r", Columns: []HomeColumn{{ID: "c", Width: 12, Blocks: []HomeBlock{{ID: "b", Type: "welcome_banner"}}}}}}},
		// Unsaved homes read as what they borrow.
		"default": {Rows: []HomeRow{{ID: "r", Columns: []HomeColumn{{ID: "c", Width: 12, Blocks: []HomeBlock{{ID: "b", Type: "recent_pages"}}}}}}},
	}}
}

// homePages is the campaign's pages for a brochure, in the lookup's campaign.
func homePages() fakeBrochurePages {
	return fakeBrochurePages{bySlug: map[string]*entities.Entity{
		"new-player-brochure": {ID: "page-1", CampaignID: camp, Name: "New player brochure"},
	}}
}

// Home reports "saved" only for the audiences the fake holds as saved.
type savedOnly struct {
	*fakeHome
	saved map[string]bool
}

func (s savedOnly) Home(ctx context.Context, c, aud string) (HomeLayout, bool, error) {
	l, _, err := s.fakeHome.Home(ctx, c, aud)
	if !s.saved[aud] {
		// An unsaved audience reads as the Everyone home.
		l, _, err = s.fakeHome.Home(ctx, c, "default")
	}
	return l, s.saved[aud], err
}

func TestLookups_Home(t *testing.T) {
	svc := savedOnly{homesFake(), map[string]bool{"default": true, "player": true, "director": true}}
	l := &Lookups{Homes: &HomeKind{Svc: svc, Pages: homePages()}}
	coDM := Actor{UserID: "co", Role: 3}
	cases := []struct {
		name     string
		l        *Lookups
		a        Actor
		fields   map[string]any
		want     []string
		lacks    []string
		wantErr  string
		wantChip string
	}{
		{name: "every audience", l: l, a: owner, wantChip: "Home pages · 3 saved", want: []string{
			"## Home pages",
			"### Everyone home\n\nIts own arrangement, saved:",
			"### Players home\n\nIts own arrangement, saved:",
			"audience: players",
			`{type: where_we_play, foundry_url: "foundry.example.com", voice: "Discord, the Ember Coast channel"}`,
			"{type: table_agreement, text: ",
			"{type: making_a_hero, text: ",
			`{type: brochure, page: "New player brochure", text: "Read this first."}`,
			"### Scribes home\n\nNot saved, so it uses the Everyone home.",
			"### Guests home\n\nNot saved, so it uses the Everyone home.",
			"### Directors home\n\nArranged in Customize; can't be written back. Its rows hold: welcome_banner. Its tile section holds: activity_feed, text_block.",
		}, lacks: []string{"audience: directors"}},
		{name: "one audience by word or key", l: l, a: owner, fields: map[string]any{"audience": "player"}, wantChip: "Home · players",
			want: []string{"### Players home"}, lacks: []string{"### Scribes", "### Directors"}},
		{name: "an unsaved one", l: l, a: owner, fields: map[string]any{"audience": "Scribes"}, wantChip: "Home · scribes",
			want: []string{"Not saved, so it uses the Everyone home. A `home` block for this audience gives it its own."}, lacks: []string{"It shows", "built-in"}},
		{name: "a tile home is not offered as a block", l: l, a: owner, fields: map[string]any{"audience": "directors"},
			want: []string{"can't be written back", "would be refused"}, lacks: []string{"audience: directors"}},
		{name: "everyone unsaved too", a: owner, fields: map[string]any{"audience": "scribes"},
			l:    &Lookups{Homes: &HomeKind{Svc: savedOnly{homesFake(), map[string]bool{}}}},
			want: []string{"which is not saved either, so everyone sees Chronicle's built-in home", "It shows: recent_pages"}},
		{name: "nothing in a saved home", a: owner, fields: map[string]any{"audience": "guests"},
			l:    &Lookups{Homes: &HomeKind{Svc: &fakeHome{owner: "owner", homes: map[string]HomeLayout{"guest": {}}}}},
			want: []string{"Saved with no blocks"}},
		{name: "not an audience", l: l, a: owner, fields: map[string]any{"audience": "elves"}, wantErr: "not an audience"},
		{name: "a player is refused", l: l, a: player, wantErr: "choose Permitted"},
		{name: "a co-DM is not the owner", l: l, a: coDM, wantErr: "only the campaign's owner"},
		{name: "unwired", l: &Lookups{}, a: owner, wantErr: "can't be looked up"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ans := tc.l.Answer(context.Background(), camp, tc.a, []Record{lookup("home", tc.fields)})[0]
			if tc.wantErr != "" {
				if !strings.Contains(ans.Error, tc.wantErr) || ans.Text != "" {
					t.Fatalf("got %+v", ans)
				}
				return
			}
			if ans.Error != "" {
				t.Fatal(ans.Error)
			}
			for _, w := range tc.want {
				if !strings.Contains(ans.Text, w) {
					t.Errorf("answer lacks %q:\n%s", w, ans.Text)
				}
			}
			for _, w := range tc.lacks {
				if strings.Contains(ans.Text, w) {
					t.Errorf("answer has %q:\n%s", w, ans.Text)
				}
			}
			if tc.wantChip != "" && ans.Chip != tc.wantChip {
				t.Errorf("chip = %q, want %q", ans.Chip, tc.wantChip)
			}
		})
	}
	if !strings.Contains((&Lookups{}).LookupDoc(), "`what: home`") {
		t.Error("LookupDoc lacks the home lookup")
	}
}

// What the lookup writes for a saved home is a block the review reads back:
// the same layout, words and settings after the trip.
func TestLookups_HomeAnswerSendsBack(t *testing.T) {
	ctx := context.Background()
	f := homesFake()
	k := &HomeKind{Svc: savedOnly{f, map[string]bool{"player": true, "director": true}}, Pages: homePages()}
	ans := (&Lookups{Homes: k}).Answer(ctx, camp, owner, []Record{lookup("home", map[string]any{"audience": "players"})})[0]
	if ans.Error != "" {
		t.Fatal(ans.Error)
	}
	recs := parseBlocks(ans.Text)
	if len(recs) != 1 || recs[0].Kind != "home" {
		t.Fatalf("parsed %d records from:\n%s", len(recs), ans.Text)
	}
	rec := recs[0]
	rec.Action = ActionUpdate
	rec.Text = BodyText(nil)
	// Unchanged, it is already the home; a moved column is what gets saved.
	if p := k.Plan(ctx, camp, owner, rec); !strings.Contains(p.Error, "already has these blocks") {
		t.Fatalf("unchanged: plan = %+v", p)
	}
	swapRowTwoColumns(rec)
	if err := k.Apply(ctx, camp, owner, rec); err != nil {
		t.Fatalf("apply: %v\n%s", err, ans.Text)
	}
	got, want := f.saved["player"], roundTripHome()
	cols := want.Rows[1].Columns
	cols[0], cols[1] = cols[1], cols[0]
	if homeSummary(got) != homeSummary(want) {
		t.Errorf("layout = %s, want %s", homeSummary(got), homeSummary(want))
	}
	if v := got.Rows[1].Columns[0].Blocks[0].Config["voice"]; v != "Discord, the Ember Coast channel" {
		t.Errorf("voice = %v", v)
	}
}
