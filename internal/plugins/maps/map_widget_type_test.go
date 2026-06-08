// map_widget_type_test.go — C-WIDGET-BINDING-P3a. The map widget type
// (instance = a map record) + the map delete-hook. Mirrors the calendar/
// timeline widget-type tests.
package maps

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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
