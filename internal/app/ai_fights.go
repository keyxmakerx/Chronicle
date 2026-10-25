package app

// ai_fights.go lends the AI's fight lookups their sources: the Tests block's
// tests, and the monster list Foundry reported. The records package owns the
// narrow interfaces; the adapters live here so it imports neither plugin.

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/pagetests"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

// aiTestsAdapter implements records.TestsAPI over the tests service, as the
// operator; the service applies its own DM rule and page visibility.
type aiTestsAdapter struct{ svc pagetests.Service }

var _ records.TestsAPI = aiTestsAdapter{}

func (a aiTestsAdapter) Tests(ctx context.Context, campaignID string, act records.Actor) ([]records.TestRef, error) {
	v := pagetests.Viewer{UserID: act.UserID, VisibilityRole: act.Role}
	list, err := a.svc.Choices(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(list))
	for i, t := range list {
		ids[i] = t.ID
	}
	outs, err := a.svc.Outcomes(ctx, campaignID, ids, v)
	if err != nil {
		return nil, err
	}
	out := make([]records.TestRef, 0, len(list))
	for _, t := range list {
		ref := records.TestRef{ID: t.ID, Name: t.Name, PageName: t.Page.Name}
		labels := map[string]string{}
		for _, r := range t.Results {
			ref.Results = append(ref.Results, records.TestResult{ID: r.ID, Label: r.Label})
			labels[r.ID] = r.Label
		}
		if o, ok := outs[t.ID]; ok && o.Recorded != "" {
			ref.Recorded = labels[o.Recorded]
		}
		out = append(out, ref)
	}
	return out, nil
}

// aiFoundryMonstersAdapter implements records.FoundryMonstersAPI over the
// list the GM's Foundry client reported. It is the DM's: a player-role
// actor gets an empty report, as if Foundry never sent one.
type aiFoundryMonstersAdapter struct {
	repo syncapi.FoundryMonsterRepository
}

var _ records.FoundryMonstersAPI = aiFoundryMonstersAdapter{}

func (a aiFoundryMonstersAdapter) Monsters(ctx context.Context, campaignID string, act records.Actor) (records.FoundryMonsterReport, error) {
	if !act.CanAuthorDmOnly() {
		return records.FoundryMonsterReport{}, nil
	}
	list, err := a.repo.Get(ctx, campaignID)
	if err != nil || list == nil {
		return records.FoundryMonsterReport{}, err
	}
	at := list.ReportedAt
	out := records.FoundryMonsterReport{ReportedAt: &at, Truncated: list.Truncated, Monsters: make([]records.FoundryMonster, 0, len(list.Monsters))}
	for _, m := range list.Monsters {
		out.Monsters = append(out.Monsters, records.FoundryMonster{
			Name: m.Name, Pack: m.Pack, Level: m.Level, Role: m.Role, Organization: m.Organization, EV: m.EV, Linked: m.Linked,
		})
	}
	return out, nil
}
