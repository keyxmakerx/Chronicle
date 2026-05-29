// calendar_v2_tier_load_test.go covers Wave 1.6.5 handler-side tier
// loading. Activates PR #370 Phase 2 overlay end-to-end via
// loadTierDefinitions: nil-lister fall-back, empty-result fall-back,
// error-on-lookup slog.Warn fall-back, happy-path conversion.

package calendar

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeTierLister implements TierDefinitionsLister for tests; the
// returned defs + err drive each scenario.
type fakeTierLister struct {
	defs []campaigns.TierDefinition
	err  error
}

func (f *fakeTierLister) GetEventTierDefinitions(_ context.Context, _ string) ([]campaigns.TierDefinition, error) {
	return f.defs, f.err
}

func TestLoadTierDefinitions_NilListerReturnsNil(t *testing.T) {
	h := &Handler{}
	got := h.loadTierDefinitions(context.Background(), "camp-1")
	if got != nil {
		t.Errorf("nil lister → nil expected; got %+v", got)
	}
}

func TestLoadTierDefinitions_ErrorReturnsNilAndLogs(t *testing.T) {
	h := &Handler{tierLister: &fakeTierLister{err: errors.New("db down")}}
	got := h.loadTierDefinitions(context.Background(), "camp-1")
	if got != nil {
		t.Errorf("lookup error → nil expected (graceful fall-back); got %+v", got)
	}
	// slog.Warn is fire-and-forget side-effect; assertion focuses on
	// the operator-visible behavior (no crash, nil result).
}

func TestLoadTierDefinitions_EmptyResultReturnsNil(t *testing.T) {
	h := &Handler{tierLister: &fakeTierLister{defs: nil}}
	got := h.loadTierDefinitions(context.Background(), "camp-1")
	if got != nil {
		t.Errorf("empty definitions → nil expected (Phase 2 contract); got %+v", got)
	}
}

func TestLoadTierDefinitions_ConvertsCampaignsTypeToAlias(t *testing.T) {
	h := &Handler{tierLister: &fakeTierLister{
		defs: []campaigns.TierDefinition{
			{Slug: "epic", Name: "Epic", Color: "#ff00ff", Prominence: 95},
			{Slug: "minor", Name: "Minor", Color: "#888", Prominence: 15},
		},
	}}
	got := h.loadTierDefinitions(context.Background(), "camp-1")
	if len(got) != 2 {
		t.Fatalf("expected 2 aliases; got %d", len(got))
	}
	if got[0].Slug != "epic" || got[0].Name != "Epic" || got[0].Color != "#ff00ff" || got[0].Prominence != 95 {
		t.Errorf("first alias mapping wrong; got %+v", got[0])
	}
	if got[1].Slug != "minor" || got[1].Prominence != 15 {
		t.Errorf("second alias mapping wrong; got %+v", got[1])
	}
}

func TestSetTierDefinitionsLister_AcceptsNilForUnwire(t *testing.T) {
	h := &Handler{tierLister: &fakeTierLister{}}
	h.SetTierDefinitionsLister(nil)
	if h.tierLister != nil {
		t.Error("SetTierDefinitionsLister(nil) should clear the lister")
	}
}
