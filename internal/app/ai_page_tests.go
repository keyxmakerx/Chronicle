package app

// ai_page_tests.go lets AI Import set up the Tests block's tests: a montage
// the AI plans can be written onto its page, and an encounter's versions can
// then name its results. The records package owns the narrow interface.

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/pagetests"
)

func init() {
	// Before encounters, so the prompt explains a test before the
	// versions that name one.
	addAIKinds(165, func(d *aiDeps) []records.Kind {
		if d.PageTests == nil {
			return nil
		}
		return []records.Kind{records.TestKind{Svc: aiTestWriter{svc: d.PageTests, ents: d.Entities}, Entities: d.Entities}}
	})
}

// aiTestWriter implements records.TestWriteAPI over the tests service, as
// the operator; the service applies its DM rule, page access and limits.
type aiTestWriter struct {
	svc  pagetests.Service
	ents entities.EntityService
}

var _ records.TestWriteAPI = aiTestWriter{}

func testViewer(a records.Actor) pagetests.Viewer {
	return pagetests.Viewer{UserID: a.UserID, VisibilityRole: a.Role}
}

func (w aiTestWriter) OnPage(ctx context.Context, campaignID, pageID string, a records.Actor) ([]records.TestSetup, error) {
	list, err := w.svc.ListForPage(ctx, campaignID, pageID, testViewer(a))
	if err != nil {
		return nil, err
	}
	out := make([]records.TestSetup, 0, len(list.Tests))
	for _, t := range list.Tests {
		s := records.TestSetup{ID: t.ID, Name: t.Name, Type: t.Kind, Shown: t.Shown,
			Successes: t.Needed, Failures: t.Limit, Version: t.Version}
		for _, r := range t.Results {
			s.Results = append(s.Results, records.TestSetupResult{ID: r.ID, Label: r.Label, On: r.On})
		}
		out = append(out, s)
	}
	return out, nil
}

func testInput(s records.TestSetup) pagetests.SaveInput {
	in := pagetests.SaveInput{Name: s.Name, Kind: s.Type, Shown: s.Shown, Needed: s.Successes, Limit: s.Failures}
	for _, r := range s.Results {
		in.Results = append(in.Results, pagetests.Result{ID: r.ID, Label: r.Label, On: r.On})
	}
	return in
}

func (w aiTestWriter) Create(ctx context.Context, campaignID, pageID string, a records.Actor, s records.TestSetup) error {
	_, err := w.svc.Create(ctx, campaignID, pageID, testViewer(a), testInput(s))
	return err
}

func (w aiTestWriter) Update(ctx context.Context, campaignID, id string, a records.Actor, s records.TestSetup) error {
	in := testInput(s)
	v := s.Version
	in.Version = &v
	_, err := w.svc.Update(ctx, campaignID, id, testViewer(a), in)
	return err
}

func (w aiTestWriter) Delete(ctx context.Context, campaignID, id string, a records.Actor) error {
	return w.svc.Delete(ctx, campaignID, id, testViewer(a))
}

func (w aiTestWriter) HasBlock(ctx context.Context, campaignID, pageID string) (bool, error) {
	return categoryLayoutHas(ctx, w.ents, campaignID, pageID, pagetests.BlockType)
}

// Pages is each page with tests, once, in the order the encounter builder
// lists the tests.
func (w aiTestWriter) Pages(ctx context.Context, campaignID string, a records.Actor) ([]entities.Entity, error) {
	list, err := w.svc.Choices(ctx, campaignID, testViewer(a))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []entities.Entity
	for _, c := range list {
		if seen[c.Page.ID] {
			continue
		}
		seen[c.Page.ID] = true
		out = append(out, entities.Entity{ID: c.Page.ID, Name: c.Page.Name})
	}
	return out, nil
}

// categoryLayoutHas says whether the layout of a page's category holds a
// block of blockType, in a column or a tile section. A page that can't be
// read answers true, so the review never warns on a guess.
func categoryLayoutHas(ctx context.Context, ents entities.EntityService, campaignID, pageID, blockType string) (bool, error) {
	if ents == nil {
		return true, nil
	}
	e, err := ents.GetByID(ctx, pageID)
	if err != nil || e == nil || e.CampaignID != campaignID {
		return true, err
	}
	et, err := ents.GetEntityTypeByID(ctx, e.EntityTypeID)
	if err != nil || et == nil {
		return true, err
	}
	for _, row := range et.Layout.Rows {
		for _, col := range row.Columns {
			for _, b := range col.Blocks {
				if b.Type == blockType {
					return true, nil
				}
			}
		}
		for _, t := range row.Tiles {
			if t.Type == blockType {
				return true, nil
			}
		}
	}
	return false, nil
}
