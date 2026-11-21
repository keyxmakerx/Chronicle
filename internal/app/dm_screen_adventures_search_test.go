package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// pagedAdventures serves n adventures of one type, honouring Page/PerPage the
// way the real listing does, so the adapter's page walk is exercised.
type pagedAdventures struct {
	entities.EntityService
	n int
}

func (p *pagedAdventures) GetEntityTypesByPresetCategory(context.Context, string, string) ([]entities.EntityType, error) {
	return []entities.EntityType{{ID: 5, Name: "Adventure"}}, nil
}

func (p *pagedAdventures) List(_ context.Context, camp string, typeID int, _ int, _ string, o entities.ListOptions) ([]entities.Entity, int, error) {
	var out []entities.Entity
	for i := (o.Page - 1) * o.PerPage; i < o.Page*o.PerPage && i < p.n; i++ {
		out = append(out, entities.Entity{ID: fmt.Sprintf("a%d", i), CampaignID: camp, EntityTypeID: typeID, Name: fmt.Sprintf("Adv %d", i)})
	}
	return out, p.n, nil
}

func TestDMAdventuresSearch_NoLimitListsEveryAdventure(t *testing.T) {
	for _, n := range []int{0, 12, 13, 100, 101, 250} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			a := &dmAdventuresAdapter{entities: &pagedAdventures{n: n}}
			got, has, err := a.Search(context.Background(), "c1", "", dmscreen.Viewer{Role: 3}, 0)
			if err != nil || !has {
				t.Fatalf("err=%v has=%v", err, has)
			}
			if len(got) != n {
				t.Errorf("listed %d of %d adventures", len(got), n)
			}
		})
	}
	t.Run("a positive limit still caps", func(t *testing.T) {
		a := &dmAdventuresAdapter{entities: &pagedAdventures{n: 30}}
		got, _, _ := a.Search(context.Background(), "c1", "", dmscreen.Viewer{Role: 3}, 12)
		if len(got) != 12 {
			t.Errorf("got %d, want 12", len(got))
		}
	})
}
