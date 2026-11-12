package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type fakePanelOnlyEntities struct {
	types   []entities.EntityType
	updated map[int]entities.EntityTypeLayout
}

func (f *fakePanelOnlyEntities) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return f.types, nil
}

func (f *fakePanelOnlyEntities) UpdateEntityTypeLayout(_ context.Context, id int, l entities.EntityTypeLayout) error {
	f.updated[id] = l
	return nil
}

func TestRepairPanelOnlyLayoutsOnce(t *testing.T) {
	panelOnly := entities.EntityTypeLayout{Rows: []entities.TemplateRow{{ID: "row-system-panels",
		Columns: []entities.TemplateColumn{{ID: "c", Width: 12, Blocks: []entities.TemplateBlock{{ID: "blk-system-panels", Type: entities.BlockSystemPanels}}}}}}}
	ents := &fakePanelOnlyEntities{
		types: []entities.EntityType{
			{ID: 1, Layout: panelOnly},
			{ID: 2, Layout: entities.DefaultLayout()},
			{ID: 3, Layout: entities.EntityTypeLayout{}},
		},
		updated: map[int]entities.EntityTypeLayout{},
	}
	st := &fakeExtrasSettings{vals: map[string]string{}}
	n, err := repairPanelOnlyLayoutsOnce(context.Background(), st, fakeExtrasCampaigns{ids: []string{"c1"}}, ents)
	if err != nil || n != 1 {
		t.Fatalf("repaired %d, %v; want 1", n, err)
	}
	if _, ok := ents.updated[1]; !ok || len(ents.updated) != 1 {
		t.Errorf("updated %v; want only type 1", ents.updated)
	}
	if st.vals[panelOnlyLayoutsKey] != "1" {
		t.Error("run was not recorded")
	}
	ents.updated = map[int]entities.EntityTypeLayout{}
	if n, err := repairPanelOnlyLayoutsOnce(context.Background(), st, fakeExtrasCampaigns{ids: []string{"c1"}}, ents); err != nil || n != 0 || len(ents.updated) != 0 {
		t.Fatalf("second run = %d, %v, %v; want nothing", n, err, ents.updated)
	}
}
