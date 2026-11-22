package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// presetStore stands in for the layout preset service: it serves presets by id
// and counts the writes that reach it.
type presetStore struct {
	entities.LayoutPresetService
	byID   map[int]*entities.LayoutPreset
	writes int
}

func (p *presetStore) GetByID(_ context.Context, id int) (*entities.LayoutPreset, error) {
	if x, ok := p.byID[id]; ok {
		return x, nil
	}
	return nil, apperror.NewNotFound("layout preset not found")
}
func (p *presetStore) Update(context.Context, int, entities.UpdateLayoutPresetInput) (*entities.LayoutPreset, error) {
	p.writes++
	return nil, nil
}
func (p *presetStore) Delete(context.Context, int) error { p.writes++; return nil }

// templateStore does the same for content templates.
type templateStore struct {
	entities.ContentTemplateService
	byID   map[int]*entities.ContentTemplate
	writes int
}

func (s *templateStore) GetByID(_ context.Context, id int) (*entities.ContentTemplate, error) {
	if x, ok := s.byID[id]; ok {
		return x, nil
	}
	return nil, apperror.NewNotFound("content template not found")
}
func (s *templateStore) Update(context.Context, int, entities.UpdateContentTemplateInput) (*entities.ContentTemplate, error) {
	s.writes++
	return nil, nil
}
func (s *templateStore) Delete(context.Context, int) error { s.writes++; return nil }

// The services do not look at whose a preset or template is, or whether it is
// built in or shared; the routes do. The adapter does the same, so a change
// the route would refuse never reaches the service.
func TestAITemplateAdaptersRefuseWhatTheRoutesRefuse(t *testing.T) {
	ctx := context.Background()
	camp := "c1"
	other := "c2"

	presets := &presetStore{byID: map[int]*entities.LayoutPreset{
		1: {ID: 1, CampaignID: camp, Name: "Standard", IsBuiltin: true},
		2: {ID: 2, CampaignID: camp, Name: "Mine"},
		3: {ID: 3, CampaignID: other, Name: "Theirs"},
	}}
	sl := aiSavedLayoutAdapter{presets: presets}
	for _, tc := range []struct {
		id      int
		allowed bool
	}{{1, false}, {2, true}, {3, false}, {9, false}} {
		before := presets.writes
		errU := sl.Update(ctx, camp, tc.id, entities.UpdateLayoutPresetInput{})
		errD := sl.Delete(ctx, camp, tc.id)
		wrote := presets.writes - before
		if tc.allowed && (errU != nil || errD != nil || wrote != 2) {
			t.Errorf("preset %d: %v %v, %d writes", tc.id, errU, errD, wrote)
		}
		if !tc.allowed && (errU == nil || errD == nil || wrote != 0) {
			t.Errorf("preset %d went ahead: %v %v, %d writes", tc.id, errU, errD, wrote)
		}
	}

	templates := &templateStore{byID: map[int]*entities.ContentTemplate{
		1: {ID: 1, Name: "Shared", IsGlobal: true},
		2: {ID: 2, CampaignID: &camp, Name: "Mine"},
		3: {ID: 3, CampaignID: &other, Name: "Theirs"},
		4: {ID: 4, CampaignID: &camp, Name: "Flagged", IsGlobal: true},
	}}
	pt := aiPageTemplateAdapter{templates: templates}
	for _, tc := range []struct {
		id      int
		allowed bool
	}{{1, false}, {2, true}, {3, false}, {4, false}, {9, false}} {
		before := templates.writes
		errU := pt.Update(ctx, camp, tc.id, entities.UpdateContentTemplateInput{})
		errD := pt.Delete(ctx, camp, tc.id)
		wrote := templates.writes - before
		if tc.allowed && (errU != nil || errD != nil || wrote != 2) {
			t.Errorf("template %d: %v %v, %d writes", tc.id, errU, errD, wrote)
		}
		if !tc.allowed && (errU == nil || errD == nil || wrote != 0) {
			t.Errorf("template %d went ahead: %v %v, %d writes", tc.id, errU, errD, wrote)
		}
	}
}
