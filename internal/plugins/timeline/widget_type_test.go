// widget_type_test.go — C-WIDGET-BINDING-P2. The timeline widget type
// (instance = a timeline record) + the timeline delete-hook.
package timeline

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/widgetbindings"
)

func TestTimelineWidgetType_Slug(t *testing.T) {
	wt := NewTimelineWidgetType(newTestTimelineService(&mockTimelineRepo{}))
	if wt.Slug() != WidgetTypeTimeline {
		t.Errorf("timeline slug = %q", wt.Slug())
	}
}

func TestTimelineWidgetType_InstanceExists(t *testing.T) {
	repo := &mockTimelineRepo{getByIDFn: func(_ context.Context, id string) (*Timeline, error) {
		switch id {
		case "t-1":
			return &Timeline{ID: "t-1", CampaignID: "camp-1"}, nil
		case "t-x":
			return &Timeline{ID: "t-x", CampaignID: "camp-OTHER"}, nil
		default:
			return nil, apperror.NewNotFound("timeline not found")
		}
	}}
	wt := NewTimelineWidgetType(newTestTimelineService(repo))
	ctx := context.Background()

	if ok, err := wt.InstanceExists(ctx, "camp-1", "t-1"); err != nil || !ok {
		t.Errorf("in-campaign timeline should validate; ok=%v err=%v", ok, err)
	}
	if ok, err := wt.InstanceExists(ctx, "camp-1", "t-x"); err != nil || ok {
		t.Errorf("cross-campaign timeline must NOT validate; ok=%v", ok)
	}
	if ok, err := wt.InstanceExists(ctx, "camp-1", "missing"); err != nil || ok {
		t.Errorf("not-found should be (false,nil) so it's sweepable; ok=%v err=%v", ok, err)
	}

	// Transient (non-404) error → (false, err) so the resolver won't sweep.
	blip := &mockTimelineRepo{getByIDFn: func(_ context.Context, _ string) (*Timeline, error) {
		return nil, errors.New("db blip")
	}}
	if ok, err := NewTimelineWidgetType(newTestTimelineService(blip)).InstanceExists(ctx, "camp-1", "t-1"); ok || err == nil {
		t.Errorf("transient error must surface as (false,err); ok=%v err=%v", ok, err)
	}
}

// The entity timeline block is a campaign-level preview list today, so the
// timeline widget type has NO single default — unbound keeps the list.
func TestTimelineWidgetType_NoDefault(t *testing.T) {
	wt := NewTimelineWidgetType(newTestTimelineService(&mockTimelineRepo{}))
	id, ok, err := wt.DefaultInstance(context.Background(), widgetbindings.HostRef{CampaignID: "camp-1", Type: widgetbindings.HostTypeEntity, ID: "e1"})
	if err != nil || ok || id != "" {
		t.Errorf("timeline must have no single default; got %q ok=%v err=%v", id, ok, err)
	}
}

type recordingCleaner struct{ calls [][2]string }

func (c *recordingCleaner) OnInstanceDeleted(_ context.Context, _ string, widgetType, instanceID string) (int, error) {
	c.calls = append(c.calls, [2]string{widgetType, instanceID})
	return 0, nil
}

func TestDeleteTimeline_FiresDeleteHook(t *testing.T) {
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, id string) (*Timeline, error) {
			return &Timeline{ID: id, CampaignID: "camp-1"}, nil
		},
		deleteFn: func(_ context.Context, _ string) error { return nil },
	}
	svc := newTestTimelineService(repo)
	cleaner := &recordingCleaner{}
	svc.(interface{ SetBindingCleaner(BindingCleaner) }).SetBindingCleaner(cleaner)

	if err := svc.DeleteTimeline(context.Background(), "t-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(cleaner.calls) != 1 || cleaner.calls[0][0] != WidgetTypeTimeline || cleaner.calls[0][1] != "t-1" {
		t.Errorf("expected one timeline delete-hook for t-1; got %v", cleaner.calls)
	}
}
