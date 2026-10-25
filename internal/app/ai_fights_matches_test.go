package app

import (
	"context"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

type aiMonsterListRepo struct{ list *syncapi.FoundryMonsterList }

func (r aiMonsterListRepo) Replace(context.Context, string, syncapi.FoundryMonsterList) error {
	return nil
}
func (r aiMonsterListRepo) Get(context.Context, string) (*syncapi.FoundryMonsterList, error) {
	return r.list, nil
}
func (r aiMonsterListRepo) PruneOlderThan(context.Context, time.Time) (int64, error) { return 0, nil }

type aiMatchStore map[string]syncapi.FoundryMatchChoice

func (s aiMatchStore) Get(_ context.Context, _, id string) (*syncapi.FoundryMatchChoice, error) {
	if ch, ok := s[id]; ok {
		return &ch, nil
	}
	return nil, nil
}
func (s aiMatchStore) List(context.Context, string) (map[string]syncapi.FoundryMatchChoice, error) {
	return s, nil
}
func (s aiMatchStore) Put(context.Context, string, syncapi.FoundryMatchChoice) error { return nil }

type aiCreaturePages []syncapi.CreaturePage

func (p aiCreaturePages) CreaturePages(context.Context, string) ([]syncapi.CreaturePage, bool, error) {
	return p, false, nil
}
func (p aiCreaturePages) CreaturePage(context.Context, string, string) (*syncapi.CreaturePage, error) {
	return nil, nil
}

func TestAIFoundryMonsters_Matches(t *testing.T) {
	repo := aiMonsterListRepo{&syncapi.FoundryMonsterList{ReportedAt: time.Now(), Monsters: []syncapi.FoundryMonster{
		{Name: "Stalker", Pack: "Monsters"}, {Name: "Captain Vex"},
	}}}
	matcher := syncapi.NewFoundryMatcher(repo, aiMatchStore{"p2": {EntityID: "p2", Use: syncapi.MatchUsePage}},
		aiCreaturePages{{ID: "p1", Name: "Stalkers"}, {ID: "p2", Name: "Captain Vex"}, {ID: "p3", Name: "Wyrm"}})
	rep, err := aiFoundryMonstersAdapter{repo: repo, matcher: matcher}.Monsters(context.Background(), "c1", records.Actor{UserID: "u1", Role: int(campaigns.RoleOwner)})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Matches) != 2 {
		t.Fatalf("a page with no match is left out: %+v", rep.Matches)
	}
	if m := rep.Matches[0]; m.PageName != "Stalkers" || !m.Suggested {
		t.Errorf("look-alike: %+v", m)
	}
	if m := rep.Matches[1]; m.PageName != "Captain Vex" || m.Suggested || m.FoundryNumbers {
		t.Errorf("matched, page's numbers: %+v", m)
	}
	if rep, _ := (aiFoundryMonstersAdapter{repo: repo, matcher: matcher}).Monsters(context.Background(), "c1", records.Actor{UserID: "u2", Role: int(campaigns.RolePlayer)}); rep.ReportedAt != nil || rep.Matches != nil {
		t.Errorf("a player sees nothing: %+v", rep)
	}
}
