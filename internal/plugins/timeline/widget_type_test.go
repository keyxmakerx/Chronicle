// widget_type_test.go — C-WIDGET-BINDING-P2. The timeline widget type
// (instance = a timeline record) + the timeline delete-hook.
package timeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
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

// ListInstances (C-WIDGET-BINDING-P4b) maps the campaign's timelines to picker
// InstanceRefs (id + name + icon + color). CreateInstance creates a named
// timeline via the service (defaults applied) and returns its id.
func TestTimelineWidgetType_ListAndCreateInstances(t *testing.T) {
	var created *Timeline
	repo := &mockTimelineRepo{
		listFn: func(_ context.Context, _ string, _ int) ([]Timeline, error) {
			return []Timeline{
				{ID: "t-1", CampaignID: "camp-1", Name: "Ages", Icon: "fa-timeline", Color: "#6366f1"},
				{ID: "t-2", CampaignID: "camp-1", Name: "Wars", Icon: "fa-bolt", Color: "#ef4444"},
			}, nil
		},
		createFn: func(_ context.Context, tl *Timeline) error { created = tl; return nil },
	}
	wt := NewTimelineWidgetType(newTestTimelineService(repo))
	ctx := context.Background()

	refs, err := wt.ListInstances(ctx, "camp-1", int(0))
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(refs) != 2 || refs[0].ID != "t-1" || refs[0].Name != "Ages" || refs[0].Icon != "fa-timeline" {
		t.Errorf("unexpected refs: %+v", refs)
	}

	id, err := wt.CreateInstance(ctx, "camp-1", widgetbindings.CreateInput{Name: "  New Era  "})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if created == nil || created.Name != "New Era" {
		t.Errorf("name should be trimmed and passed to CreateTimeline; got %+v", created)
	}
	if id != created.ID {
		t.Errorf("CreateInstance should return the new timeline id; got %q", id)
	}

	// A list error surfaces.
	errRepo := &mockTimelineRepo{listFn: func(_ context.Context, _ string, _ int) ([]Timeline, error) {
		return nil, errors.New("db down")
	}}
	if _, err := NewTimelineWidgetType(newTestTimelineService(errRepo)).ListInstances(ctx, "camp-1", 0); err == nil {
		t.Errorf("ListInstances must surface a service error")
	}
}

// RenderBlock returns a non-nil component wrapped in the stable BlockHost id.
func TestTimelineWidgetType_RenderBlock(t *testing.T) {
	wt := NewTimelineWidgetType(newTestTimelineService(&mockTimelineRepo{}))
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe}
	comp := wt.RenderBlock(context.Background(), widgetbindings.BlockRenderContext{
		CC: cc, HostID: "ent-1", Role: int(campaigns.RoleScribe),
		Resolution: widgetbindings.Resolution{InstanceID: "t-1", Source: widgetbindings.SourceOwn},
	})
	if comp == nil {
		t.Fatal("RenderBlock returned nil")
	}
	var sb strings.Builder
	if err := comp.Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()
	if !strings.Contains(html, `id="`+widgetbindings.BlockHostID(WidgetTypeTimeline, "ent-1")+`"`) {
		t.Errorf("RenderBlock should wrap in BlockHost; got %q", html)
	}
	if !strings.Contains(html, `data-binding-affordance="timeline"`) {
		t.Errorf("Scribe timeline block should carry the binding affordance; got %q", html)
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
