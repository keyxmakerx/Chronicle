package syncapi

import (
	"context"
	"strings"
	"testing"
	"time"
)

func report(ms ...FoundryMonster) *FoundryMonsterList {
	return &FoundryMonsterList{Monsters: ms, ReportedAt: time.Now()}
}

func TestMatchCreature(t *testing.T) {
	stalkerPack := FoundryMonster{Name: "Stalker", Pack: "Monsters", Level: "3"}
	stalkerWorld := FoundryMonster{Name: "stalker"}
	ownActor := FoundryMonster{Name: "Stalker", Linked: true, Page: "p1"}
	tests := []struct {
		name      string
		page      string
		list      *FoundryMonsterList
		ch        *FoundryMatchChoice
		state     string
		monster   string
		pack      string
		use       string
		inPlay    bool
		confirmed bool
	}{
		{name: "no report", page: "Stalker", list: nil, state: MatchNotReported},
		{name: "same name plays Foundry's numbers", page: "Stalker", list: report(stalkerPack), state: MatchMatched, monster: "Stalker", pack: "Monsters", use: MatchUseFoundry},
		{name: "case and spaces aside", page: "  STALKER ", list: report(stalkerPack), state: MatchMatched, monster: "Stalker", use: MatchUseFoundry, pack: "Monsters"},
		{name: "world actor wins over compendium", page: "Stalker", list: report(stalkerPack, stalkerWorld), state: MatchMatched, monster: "stalker", pack: "", use: MatchUseFoundry},
		{name: "already in play keeps the page's numbers", page: "Stalker", list: report(ownActor, stalkerPack), state: MatchMatched, monster: "Stalker", pack: "Monsters", use: MatchUsePage, inPlay: true},
		{name: "the page's own actor is never its match", page: "Stalker", list: report(ownActor), state: MatchNone, inPlay: true},
		{name: "chosen use wins", page: "Stalker", list: report(ownActor, stalkerPack), ch: &FoundryMatchChoice{Use: MatchUseFoundry}, state: MatchMatched, monster: "Stalker", pack: "Monsters", use: MatchUseFoundry, inPlay: true},
		{name: "plural looks alike", page: "Stalkers", list: report(stalkerPack), state: MatchSuggested, monster: "Stalker", pack: "Monsters"},
		{name: "bracketed note looks alike", page: "Goblin Sniper", list: report(FoundryMonster{Name: "Goblin-Sniper (Level 1)", Pack: "Monsters"}), state: MatchSuggested, monster: "Goblin-Sniper (Level 1)", pack: "Monsters"},
		{name: "not the same", page: "Stalker", list: report(stalkerPack), ch: &FoundryMatchChoice{NotSame: []string{"stalker"}}, state: MatchNone},
		{name: "matched by hand", page: "Shadow hunter", list: report(stalkerPack), ch: &FoundryMatchChoice{Name: "Stalker", Pack: "Monsters"}, state: MatchMatched, monster: "Stalker", pack: "Monsters", use: MatchUseFoundry, confirmed: true},
		{name: "matched monster gone", page: "Shadow hunter", list: report(), ch: &FoundryMatchChoice{Name: "Stalker", Pack: "Monsters", Use: MatchUsePage}, state: MatchMissing, monster: "Stalker", pack: "Monsters", use: MatchUsePage, confirmed: true},
		{name: "nothing alike", page: "Bone Wyrm", list: report(stalkerPack), state: MatchNone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := matchCreature("p1", tc.page, tc.list, tc.ch)
			if m.State != tc.state || m.Use != tc.use || m.InPlay != tc.inPlay || m.Confirmed != tc.confirmed {
				t.Fatalf("got %+v", m)
			}
			if tc.monster == "" {
				if m.Monster != nil {
					t.Fatalf("want no monster, got %+v", m.Monster)
				}
				return
			}
			if m.Monster == nil || m.Monster.Name != tc.monster || m.Monster.Pack != tc.pack {
				t.Fatalf("monster %+v, want %q in %q", m.Monster, tc.monster, tc.pack)
			}
		})
	}
}

func TestApplyMatchAction(t *testing.T) {
	list := report(FoundryMonster{Name: "Stalker", Pack: "Monsters"}, FoundryMonster{Name: "Shade", Linked: true, Page: "p9"})
	matched := CreatureMatch{State: MatchMatched}
	tests := []struct {
		name    string
		ch      FoundryMatchChoice
		cur     CreatureMatch
		a       FoundryMatchAction
		want    FoundryMatchChoice
		wantErr string
	}{
		{name: "use the page's numbers", cur: matched, a: FoundryMatchAction{Action: "use", Use: MatchUsePage}, want: FoundryMatchChoice{Use: MatchUsePage}},
		{name: "use needs a match", cur: CreatureMatch{State: MatchSuggested}, a: FoundryMatchAction{Action: "use", Use: MatchUsePage}, wantErr: "not matched"},
		{name: "use takes two words", cur: matched, a: FoundryMatchAction{Action: "use", Use: "mine"}, wantErr: `"foundry" or "page"`},
		{name: "match a reported monster", ch: FoundryMatchChoice{NotSame: []string{"Stalker", "Rat"}}, a: FoundryMatchAction{Action: "match", Name: "stalker", Pack: "Monsters"},
			want: FoundryMatchChoice{Name: "Stalker", Pack: "Monsters", NotSame: []string{"Rat"}}},
		{name: "match must be reported", a: FoundryMatchAction{Action: "match", Name: "Wyrm"}, wantErr: "did not report"},
		{name: "a page's own actor is no match", a: FoundryMatchAction{Action: "match", Name: "Shade"}, wantErr: "did not report"},
		{name: "not the same clears that match", ch: FoundryMatchChoice{Name: "Stalker", Pack: "Monsters", Use: MatchUsePage},
			a: FoundryMatchAction{Action: "not_same", Name: "Stalker"}, want: FoundryMatchChoice{NotSame: []string{"Stalker"}}},
		{name: "not the same once", ch: FoundryMatchChoice{NotSame: []string{"Stalker"}}, a: FoundryMatchAction{Action: "not_same", Name: "STALKER"}, want: FoundryMatchChoice{NotSame: []string{"Stalker"}}},
		{name: "reset", ch: FoundryMatchChoice{Name: "Stalker", Use: MatchUsePage, NotSame: []string{"Rat"}}, a: FoundryMatchAction{Action: "reset"}, want: FoundryMatchChoice{}},
		{name: "unknown action", a: FoundryMatchAction{Action: "merge"}, wantErr: "action must be"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := applyMatchAction(tc.ch, tc.cur, list, tc.a)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != tc.want.Name || got.Pack != tc.want.Pack || got.Use != tc.want.Use || strings.Join(got.NotSame, ",") != strings.Join(tc.want.NotSame, ",") {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// fakeMatches is FoundryMatchRepository in memory.
type fakeMatches map[string]FoundryMatchChoice

func (f fakeMatches) Get(_ context.Context, _, entityID string) (*FoundryMatchChoice, error) {
	if ch, ok := f[entityID]; ok {
		return &ch, nil
	}
	return nil, nil
}
func (f fakeMatches) List(context.Context, string) (map[string]FoundryMatchChoice, error) {
	return f, nil
}
func (f fakeMatches) Put(_ context.Context, _ string, ch FoundryMatchChoice) error {
	f[ch.EntityID] = ch
	return nil
}

type fakeCreatures []CreaturePage

func (f fakeCreatures) CreaturePages(context.Context, string) ([]CreaturePage, bool, error) {
	return f, false, nil
}
func (f fakeCreatures) CreaturePage(_ context.Context, _, id string) (*CreaturePage, error) {
	for _, p := range f {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, nil
}

func TestFoundryMatcher(t *testing.T) {
	ctx := context.Background()
	store := fakeMatches{}
	m := NewFoundryMatcher(&fakeMonsterRepo{stored: *report(FoundryMonster{Name: "Stalker", Pack: "Monsters"}, FoundryMonster{Name: "Stalker Brute", Pack: "Monsters"})},
		store, fakeCreatures{{ID: "p1", Name: "Stalker"}, {ID: "p2", Name: "Wyrm"}})

	all, err := m.All(ctx, "c1")
	if err != nil || all.Total != 2 || all.Matches[0].State != MatchMatched || all.Matches[1].State != MatchNone || all.ReportedAt == nil {
		t.Fatalf("all: %+v %v", all, err)
	}
	got, err := m.Change(ctx, "c1", "p1", FoundryMatchAction{Action: "use", Use: MatchUsePage})
	if err != nil || got.Use != MatchUsePage || !got.UseChosen || store["p1"].Use != MatchUsePage {
		t.Fatalf("change: %+v %v", got, err)
	}
	if _, err := m.Change(ctx, "c1", "nope", FoundryMatchAction{Action: "reset"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("a page that is not a creature must be refused: %v", err)
	}
	found, _ := m.Search(ctx, "c1", "stalker", 1)
	if len(found) != 1 || found[0].Name != "Stalker" {
		t.Fatalf("search: %+v", found)
	}
	got, err = m.Change(ctx, "c1", "p2", FoundryMatchAction{Action: "match", Name: "Stalker Brute", Pack: "Monsters"})
	if err != nil || got.State != MatchMatched || !got.Confirmed || got.EntityName != "Wyrm" {
		t.Fatalf("match another: %+v %v", got, err)
	}
}

func TestCleanMonsters_PageOnlyWhenLinked(t *testing.T) {
	out, err := cleanMonsters(reportMonstersRequest{Monsters: []FoundryMonster{
		{Name: "A", Linked: true, Page: "p1"}, {Name: "B", Page: "p2"},
	}}, time.Now())
	if err != nil || out.Monsters[0].Page != "p1" || out.Monsters[1].Page != "" {
		t.Fatalf("got %+v %v", out, err)
	}
}
