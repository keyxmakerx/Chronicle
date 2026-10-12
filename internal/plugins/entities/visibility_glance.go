package entities

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EntityTagGrantInfo is one tag-derived visibility grant on an entity, resolved
// for display. It mirrors the tags widget's grant summary but lives here so the
// entities plugin needn't import the widget (the EntityTagFetcher seam carries
// it across the boundary). SubjectLabel is the human label shown in the glance
// tooltip ("Players", a member's name, a group name).
type EntityTagGrantInfo struct {
	TagName      string `json:"tag_name"`
	TagSlug      string `json:"tag_slug"`
	TagColor     string `json:"tag_color"`
	SubjectType  string `json:"subject_type"`
	SubjectID    string `json:"subject_id"`
	SubjectLabel string `json:"subject_label"`
}

// Effective-visibility base states. These name the entity's *configured*
// visibility before any tag widening, matching the shipped 3-state badge.
const (
	// VisStateEveryone — public: visible to everyone, including anonymous /
	// logged-out visitors on a public campaign (default mode, not private).
	VisStateEveryone = "everyone"
	// VisStateDMOnly — visible only to Scribe+ (default mode, is_private).
	VisStateDMOnly = "dm_only"
	// VisStateCustom — specific entity_permissions grants (custom mode).
	VisStateCustom = "custom"
)

// EffectiveVisibility is the server-computed glance for one entity: its base
// configured state plus whether tag grants WIDEN it and to whom. A tag must
// never silently expose content, so wherever a Scribe+ sees an entity's
// visibility they must also see any tag that exposed it.
type EffectiveVisibility struct {
	// BaseState is the configured visibility (everyone / dm_only / custom).
	BaseState string
	// WidenedByTags is true when an otherwise-hidden entity (dm_only or custom)
	// is exposed to additional subjects via one or more tag grants.
	WidenedByTags bool
	// TagGrants are the tag-derived grants on the entity, for the tooltip.
	TagGrants []EntityTagGrantInfo
}

// baseVisibilityState derives the configured (pre-tag) visibility state from an
// entity, identical to the shipped 3-state badge's decision logic.
func baseVisibilityState(entity *Entity) string {
	switch {
	case entity.Visibility == VisibilityCustom:
		return VisStateCustom
	case entity.IsPrivate, entity.TypeDMOnly:
		return VisStateDMOnly
	default:
		return VisStateEveryone
	}
}

// ComputeEffectiveVisibility combines an entity's configured visibility with its
// tag-derived grants. Tags only *widen*, so an already-everyone entity is never
// marked widened (it's visible to all members regardless); widening is reported
// only for dm_only / custom entities that tag grants expose further.
func ComputeEffectiveVisibility(entity *Entity, grants []EntityTagGrantInfo) EffectiveVisibility {
	base := baseVisibilityState(entity)
	return EffectiveVisibility{
		BaseState: base,
		// A DM-only category stops tag grants, so they widen nothing there.
		WidenedByTags: base != VisStateEveryone && !entity.TypeDMOnly && len(grants) > 0,
		TagGrants:     grants,
	}
}

// effVisKey is the private context key for the per-request effective-visibility
// glance. It is injected by the entity show handler and read by the show-page
// header badge, mirroring the SetActivePath / WithSingletonTracker context
// pattern so no template signature has to widen.
type effVisKey struct{}

// WithEffectiveVisibility returns a context carrying the entity's glance.
func WithEffectiveVisibility(ctx context.Context, ev *EffectiveVisibility) context.Context {
	return context.WithValue(ctx, effVisKey{}, ev)
}

// GetEffectiveVisibility returns the glance injected for this request, or nil
// when none was computed (e.g. a player view, where the badge is not rendered).
func GetEffectiveVisibility(ctx context.Context) *EffectiveVisibility {
	ev, _ := ctx.Value(effVisKey{}).(*EffectiveVisibility)
	return ev
}

// effectiveVisibilityTooltip builds the glance badge's tooltip: the base-state
// sentence plus, when tags widen the entity, an explicit "Also visible to
// <subject> via ‹tag›" list so the exposure is never silent.
func effectiveVisibilityTooltip(ev *EffectiveVisibility, campaignIsPublic bool) string {
	if ev == nil {
		return ""
	}
	var base string
	switch ev.BaseState {
	case VisStateCustom:
		base = "Custom permissions — specific people"
	case VisStateDMOnly:
		base = "DM-Only — visible to GMs (Scribe + Owner)"
	default:
		// An "everyone" entity reaches every campaign MEMBER. Whether it also
		// reaches a logged-out stranger is a property of the CAMPAIGN, not the
		// entity: only a public campaign is readable by RoleNone at all, so the
		// tooltip must not claim visitor reach on a private campaign.
		if campaignIsPublic {
			base = "Public — visible to everyone in this campaign, including logged-out visitors"
		} else {
			base = "Visible to everyone in this campaign"
		}
	}
	if !ev.WidenedByTags || len(ev.TagGrants) == 0 {
		return base
	}
	parts := make([]string, 0, len(ev.TagGrants))
	for _, g := range ev.TagGrants {
		parts = append(parts, fmt.Sprintf("%s via ‹%s›", g.SubjectLabel, g.TagSlug))
	}
	return base + " · Also visible to " + strings.Join(parts, "; ")
}

// visibilityGlanceTooltip builds the shared visibilityGlance component's
// title text (ADR-057). When ev is available it defers to
// effectiveVisibilityTooltip so the tag-widening safety contract is
// honoured. When no EffectiveVisibility was computed for this call site
// (cards, category-dashboard rows/tree, child-entity lists), it still names
// the entity's *configured* state with the same wording, just without a tag
// widening it was never given.
func visibilityGlanceTooltip(cc *campaigns.CampaignContext, entity *Entity, ev *EffectiveVisibility) string {
	// A nil campaign context cannot happen behind visibilityGlanceVisible, which
	// already rejects nil — but the tooltip must not be the thing that panics if
	// a future call site forgets the gate, and "not public" is the safe answer:
	// it claims LESS reach, never more.
	isPublic := cc != nil && cc.Campaign != nil && cc.Campaign.IsPublic
	if ev != nil {
		return effectiveVisibilityTooltip(ev, isPublic)
	}
	return effectiveVisibilityTooltip(&EffectiveVisibility{BaseState: baseVisibilityState(entity)}, isPublic)
}
