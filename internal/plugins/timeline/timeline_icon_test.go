// timeline_icon_test.go pins the icon name check on UpdateTimeline.
// CreateTimeline's check is already covered by TestCreateTimeline_InvalidIcon
// in service_test.go.
package timeline

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// badTimelineIconNames are inputs that must fail sanitize.ValidateIcon.
var badTimelineIconNames = []string{
	`fa-x" data-y="z`,
	`<b>`,
	`FA-BOOK`,
	`fa-`,
}

func TestUpdateTimeline_InvalidIcon(t *testing.T) {
	for _, icon := range badTimelineIconNames {
		t.Run(icon, func(t *testing.T) {
			updated := false
			repo := &mockTimelineRepo{
				getByIDFn: func(_ context.Context, id string) (*Timeline, error) {
					return &Timeline{ID: id, Name: "Main", CampaignID: "camp-1", Icon: "fa-timeline"}, nil
				},
				updateFn: func(_ context.Context, _ *Timeline) error {
					updated = true
					return nil
				},
			}
			svc := newTestTimelineService(repo)

			err := svc.UpdateTimeline(context.Background(), "tl-1", UpdateTimelineInput{
				Name:        "Main",
				Visibility:  patch.Of("everyone"),
				ZoomDefault: patch.Of(ZoomYear),
				Icon:        patch.Of(icon),
			})
			assertAppError(t, err, 400)
			if updated {
				t.Error("expected repo.Update not to be called for an invalid icon")
			}
		})
	}
}

func TestUpdateTimeline_ValidIcon(t *testing.T) {
	var captured *Timeline
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, id string) (*Timeline, error) {
			return &Timeline{ID: id, Name: "Main", CampaignID: "camp-1", Icon: "fa-timeline"}, nil
		},
		updateFn: func(_ context.Context, tl *Timeline) error {
			captured = tl
			return nil
		},
	}
	svc := newTestTimelineService(repo)

	err := svc.UpdateTimeline(context.Background(), "tl-1", UpdateTimelineInput{
		Name:        "Main",
		Visibility:  patch.Of("everyone"),
		ZoomDefault: patch.Of(ZoomYear),
		Icon:        patch.Of("fa-dragon"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil || captured.Icon != "fa-dragon" {
		t.Errorf("expected icon fa-dragon to be persisted, got %+v", captured)
	}
}
