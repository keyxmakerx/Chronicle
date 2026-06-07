// service.go — the binding service: the precedence resolver + the integrity
// "mitigation kit" (C-WIDGET-BINDING-P1-SPINE).
//
// FK-free polymorphism (see package doc + ADR) means integrity is enforced
// here, as an AND of three mechanisms (precedent refinement #1):
//   - per-plugin DELETE HOOK     → OnInstanceDeleted (owning plugins call it)
//   - always-on RENDER-TIME GUARD → Resolve validates each candidate via
//     WidgetType.InstanceExists and skips dead ones
//   - periodic INTEGRITY SWEEP   → Sweep removes orphaned bindings campaign-wide
package widgetbindings

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

// Sentinel errors.
var (
	ErrUnknownWidgetType = errors.New("widgetbindings: unknown widget type")
	ErrInvalidHostType   = errors.New("widgetbindings: invalid host type")
	ErrCampaignMismatch  = errors.New("widgetbindings: host campaign mismatch")
	// ErrInstanceNotInCampaign is the security stop: a bind whose instance
	// doesn't exist in the host's campaign (cross-campaign or bogus).
	ErrInstanceNotInCampaign = errors.New("widgetbindings: instance not found in campaign")
)

// Service is the binding API consumed by widget blocks + (P4) the assign UI.
type Service interface {
	// Bind upserts a host's binding to an instance, after validating the
	// instance belongs to the host's campaign (security). host.CampaignID is
	// authoritative.
	Bind(ctx context.Context, host HostRef, widgetType, instanceID string) error
	// Unbind removes a host's binding for a widget type.
	Unbind(ctx context.Context, host HostRef, widgetType string) error
	// Resolve runs the precedence chain (own → entity-type template → default)
	// with a render-time orphan guard, returning the winning instance + source.
	Resolve(ctx context.Context, host HostRef, widgetType string) (Resolution, error)
	// OnInstanceDeleted is the per-plugin delete hook: owning plugins call it
	// when an instance is deleted so its bindings are removed promptly. Returns
	// the number of bindings cleaned.
	OnInstanceDeleted(ctx context.Context, campaignID, widgetType, instanceID string) (int, error)
	// Sweep removes bindings whose instance no longer validates (the periodic
	// integrity sweep). Returns the number swept.
	Sweep(ctx context.Context, campaignID string) (int, error)
}

type service struct {
	repo     Repository
	registry *Registry
	newID    func() string
}

// NewService builds the binding service over a repository + widget-type registry.
func NewService(repo Repository, registry *Registry) Service {
	return &service{repo: repo, registry: registry, newID: newUUID}
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *service) Bind(ctx context.Context, host HostRef, widgetType, instanceID string) error {
	if !IsValidHostType(host.Type) {
		return ErrInvalidHostType
	}
	if host.CampaignID == "" || host.ID == "" || instanceID == "" {
		return fmt.Errorf("widgetbindings: campaign, host id, and instance id are required")
	}
	wt, ok := s.registry.Get(widgetType)
	if !ok {
		return ErrUnknownWidgetType
	}
	// SECURITY: the instance must exist within the host's campaign. This is the
	// only guard against a cross-campaign bind (no DB FK to lean on).
	exists, err := wt.InstanceExists(ctx, host.CampaignID, instanceID)
	if err != nil {
		return fmt.Errorf("validate instance: %w", err)
	}
	if !exists {
		return ErrInstanceNotInCampaign
	}
	return s.repo.Upsert(ctx, &WidgetBinding{
		ID:         s.newID(),
		CampaignID: host.CampaignID,
		HostType:   host.Type,
		HostID:     host.ID,
		WidgetType: widgetType,
		InstanceID: instanceID,
	})
}

func (s *service) Unbind(ctx context.Context, host HostRef, widgetType string) error {
	return s.repo.DeleteForHostWidget(ctx, host.CampaignID, host.Type, host.ID, widgetType)
}

// Resolve is the precedence chain. Each bound candidate is validated by the
// render-time guard (InstanceExists) before it wins; a dead binding is swept
// and resolution falls through to the next rung — never crashes, never leaks.
func (s *service) Resolve(ctx context.Context, host HostRef, widgetType string) (Resolution, error) {
	none := Resolution{Source: SourceNone, WidgetType: widgetType}
	wt, ok := s.registry.Get(widgetType)
	if !ok {
		return none, ErrUnknownWidgetType
	}

	// 1) The host's OWN binding (any host type).
	if id, won := s.tryHostBinding(ctx, wt, host.CampaignID, host.Type, host.ID, widgetType); won {
		return Resolution{InstanceID: id, Source: SourceOwn, WidgetType: widgetType}, nil
	}

	// 2) Inheritance: for an ENTITY host, its entity-type's template binding.
	//    Built + tested now (P1); surfaced as data in P4. A type-template must
	//    NEVER override an entity's own binding — that's why this rung is second.
	if host.Type == HostTypeEntity && host.EntityTypeID != "" {
		if id, won := s.tryHostBinding(ctx, wt, host.CampaignID, HostTypeEntityType, host.EntityTypeID, widgetType); won {
			return Resolution{InstanceID: id, Source: SourceEntityType, WidgetType: widgetType}, nil
		}
	}

	// 3) Default = today's behavior (e.g. calendar → campaign default calendar).
	if id, ok, err := wt.DefaultInstance(ctx, host); err == nil && ok && id != "" {
		return Resolution{InstanceID: id, Source: SourceDefault, WidgetType: widgetType}, nil
	}

	return none, nil
}

// tryHostBinding fetches a host's binding and applies the render-time orphan
// guard: if the instance no longer validates, the dead binding is swept and
// the function reports "no win" so Resolve falls through.
func (s *service) tryHostBinding(ctx context.Context, wt WidgetType, campaignID, hostType, hostID, widgetType string) (string, bool) {
	b, err := s.repo.GetForHost(ctx, campaignID, hostType, hostID, widgetType)
	if err != nil || b == nil {
		return "", false
	}
	exists, err := wt.InstanceExists(ctx, campaignID, b.InstanceID)
	if err != nil {
		// Treat a validation error as "can't confirm" → don't win, but don't
		// sweep either (transient DB blips shouldn't delete user data).
		return "", false
	}
	if !exists {
		// Orphan: instance gone or out-of-campaign — sweep the dead binding.
		_ = s.repo.DeleteByID(ctx, campaignID, b.ID)
		return "", false
	}
	return b.InstanceID, true
}

func (s *service) OnInstanceDeleted(ctx context.Context, campaignID, widgetType, instanceID string) (int, error) {
	bindings, err := s.repo.ListForInstance(ctx, campaignID, widgetType, instanceID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, b := range bindings {
		if err := s.repo.DeleteByID(ctx, campaignID, b.ID); err == nil {
			n++
		}
	}
	return n, nil
}

func (s *service) Sweep(ctx context.Context, campaignID string) (int, error) {
	bindings, err := s.repo.ListByCampaign(ctx, campaignID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, b := range bindings {
		wt, ok := s.registry.Get(b.WidgetType)
		if !ok {
			continue // unknown widget type (e.g. addon removed) — leave it be
		}
		exists, err := wt.InstanceExists(ctx, campaignID, b.InstanceID)
		if err != nil {
			continue // transient — don't delete on a blip
		}
		if !exists {
			if err := s.repo.DeleteByID(ctx, campaignID, b.ID); err == nil {
				n++
			}
		}
	}
	return n, nil
}
