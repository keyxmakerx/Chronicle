// map_widget_type_test.go — C-WIDGET-BINDING-P3a. The map widget type
// (instance = a map record) + the map delete-hook. Mirrors the calendar/
// timeline widget-type tests.
package maps

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/widgetbindings"
)

func TestMapWidgetType_Slug(t *testing.T) {
	wt := NewMapWidgetType(newTestMapService(&mockMapRepo{}))
	if wt.Slug() != WidgetTypeMap {
		t.Errorf("map slug = %q", wt.Slug())
	}
}

func TestMapWidgetType_InstanceExists(t *testing.T) {
	repo := &mockMapRepo{getMapFn: func(_ context.Context, id string) (*Map, error) {
		switch id {
		case "m-1":
			return &Map{ID: "m-1", CampaignID: "camp-1"}, nil
		case "m-x":
			return &Map{ID: "m-x", CampaignID: "camp-OTHER"}, nil
		default:
			return nil, apperror.NewNotFound("map not found")
		}
	}}
	wt := NewMapWidgetType(newTestMapService(repo))
	ctx := context.Background()

	if ok, err := wt.InstanceExists(ctx, "camp-1", "m-1"); err != nil || !ok {
		t.Errorf("in-campaign map should validate; ok=%v err=%v", ok, err)
	}
	if ok, err := wt.InstanceExists(ctx, "camp-1", "m-x"); err != nil || ok {
		t.Errorf("cross-campaign map must NOT validate; ok=%v", ok)
	}
	if ok, err := wt.InstanceExists(ctx, "camp-1", "missing"); err != nil || ok {
		t.Errorf("not-found should be (false,nil) so it's sweepable; ok=%v err=%v", ok, err)
	}

	// Transient (non-404) error → (false, err) so the resolver won't sweep.
	blip := &mockMapRepo{getMapFn: func(_ context.Context, _ string) (*Map, error) {
		return nil, errors.New("db blip")
	}}
	if ok, err := NewMapWidgetType(newTestMapService(blip)).InstanceExists(ctx, "camp-1", "m-1"); ok || err == nil {
		t.Errorf("transient error must surface as (false,err); ok=%v err=%v", ok, err)
	}
}

// Maps has no campaign-level default — the legacy entity.map_id fallback lives
// in the block closure, not here.
func TestMapWidgetType_NoDefault(t *testing.T) {
	wt := NewMapWidgetType(newTestMapService(&mockMapRepo{}))
	id, ok, err := wt.DefaultInstance(context.Background(), widgetbindings.HostRef{CampaignID: "camp-1", Type: widgetbindings.HostTypeEntity, ID: "e1"})
	if err != nil || ok || id != "" {
		t.Errorf("map must have no single default; got %q ok=%v err=%v", id, ok, err)
	}
}

// ListInstances (C-WIDGET-BINDING-P4b) maps the campaign's maps to picker
// InstanceRefs; CreateInstance creates a named map and returns its id.
func TestMapWidgetType_ListAndCreateInstances(t *testing.T) {
	var created *Map
	repo := &mockMapRepo{
		listMapsFn: func(_ context.Context, _ string) ([]Map, error) {
			return []Map{
				{ID: "m-1", CampaignID: "camp-1", Name: "Overworld"},
				{ID: "m-2", CampaignID: "camp-1", Name: "Dungeon"},
			}, nil
		},
		createMapFn: func(_ context.Context, m *Map) error { created = m; return nil },
	}
	wt := NewMapWidgetType(newTestMapService(repo))
	ctx := context.Background()

	refs, err := wt.ListInstances(ctx, "camp-1", 0)
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(refs) != 2 || refs[0].ID != "m-1" || refs[0].Name != "Overworld" || refs[0].Icon != "fa-map-location-dot" {
		t.Errorf("unexpected refs: %+v", refs)
	}

	id, err := wt.CreateInstance(ctx, "camp-1", widgetbindings.CreateInput{Name: "  Cavern  "})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if created == nil || created.Name != "Cavern" {
		t.Errorf("name should be trimmed and passed to CreateMap; got %+v", created)
	}
	if id != created.ID {
		t.Errorf("CreateInstance should return the new map id; got %q", id)
	}

	errRepo := &mockMapRepo{listMapsFn: func(_ context.Context, _ string) ([]Map, error) {
		return nil, errors.New("db down")
	}}
	if _, err := NewMapWidgetType(newTestMapService(errRepo)).ListInstances(ctx, "camp-1", 0); err == nil {
		t.Errorf("ListInstances must surface a service error")
	}
}

// RenderBlock: a bound, in-campaign map renders the embed (with the binding
// affordance for Scribe); no map → the Scribe "choose" state; players → empty.
func TestMapWidgetType_RenderBlock(t *testing.T) {
	repo := &mockMapRepo{
		getMapFn: func(_ context.Context, id string) (*Map, error) {
			return &Map{ID: id, CampaignID: "camp-1", Name: "Overworld"}, nil
		},
		listMarkersFn: func(_ context.Context, _ string, _ int) ([]Marker, error) { return nil, nil },
	}
	wt := NewMapWidgetType(newTestMapService(repo))
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe}
	ctx := context.Background()

	// Bound map → embed, wrapped in BlockHost, with the affordance.
	embed := render(t, wt.RenderBlock(ctx, widgetbindings.BlockRenderContext{
		CC: cc, HostID: "ent-1", Role: int(campaigns.RoleScribe),
		Resolution: widgetbindings.Resolution{InstanceID: "m-1", Source: widgetbindings.SourceOwn},
	}))
	if !strings.Contains(embed, `id="`+widgetbindings.BlockHostID(WidgetTypeMap, "ent-1")+`"`) {
		t.Errorf("RenderBlock should wrap in BlockHost; got %q", embed)
	}
	if !strings.Contains(embed, "entity-map-embed") || !strings.Contains(embed, `data-binding-affordance="map"`) {
		t.Errorf("bound map should render the embed + affordance; got %q", embed)
	}

	// No map + Scribe → the choose state with the affordance.
	choose := render(t, wt.RenderBlock(ctx, widgetbindings.BlockRenderContext{
		CC: cc, HostID: "ent-1", Role: int(campaigns.RoleScribe),
		Resolution: widgetbindings.Resolution{Source: widgetbindings.SourceNone},
	}))
	if !strings.Contains(choose, "data-entity-map-choose") || !strings.Contains(choose, `data-binding-affordance="map"`) {
		t.Errorf("no-map Scribe should render the choose state; got %q", choose)
	}

	// No map + player → the friendly empty state, NO affordance.
	ccP := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer}
	empty := render(t, wt.RenderBlock(ctx, widgetbindings.BlockRenderContext{
		CC: ccP, HostID: "ent-1", Role: int(campaigns.RolePlayer),
		Resolution: widgetbindings.Resolution{Source: widgetbindings.SourceNone},
	}))
	if strings.Contains(empty, "data-binding-affordance") {
		t.Errorf("players must not see the binding affordance; got %q", empty)
	}
}

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var sb strings.Builder
	if err := c.Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

type recordingCleaner struct{ calls [][2]string }

func (c *recordingCleaner) OnInstanceDeleted(_ context.Context, _ string, widgetType, instanceID string) (int, error) {
	c.calls = append(c.calls, [2]string{widgetType, instanceID})
	return 0, nil
}

func TestDeleteMap_FiresDeleteHook(t *testing.T) {
	repo := &mockMapRepo{
		getMapFn: func(_ context.Context, id string) (*Map, error) {
			return &Map{ID: id, CampaignID: "camp-1"}, nil
		},
		deleteMapFn: func(_ context.Context, _ string) error { return nil },
	}
	svc := newTestMapService(repo)
	cleaner := &recordingCleaner{}
	svc.(interface{ SetBindingCleaner(BindingCleaner) }).SetBindingCleaner(cleaner)

	if err := svc.DeleteMap(context.Background(), "m-1", nil); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(cleaner.calls) != 1 || cleaner.calls[0][0] != WidgetTypeMap || cleaner.calls[0][1] != "m-1" {
		t.Errorf("expected one map delete-hook for m-1; got %v", cleaner.calls)
	}
}
