package entities

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestComputeEffectiveVisibility(t *testing.T) {
	grant := []EntityTagGrantInfo{{TagSlug: "revealed-act-1", SubjectType: "role", SubjectID: "1", SubjectLabel: "Players"}}

	tests := []struct {
		name        string
		entity      *Entity
		grants      []EntityTagGrantInfo
		wantBase    string
		wantWidened bool
	}{
		{"everyone, no grants", &Entity{Visibility: VisibilityDefault, IsPrivate: false}, nil, VisStateEveryone, false},
		{"everyone, with grants stays not-widened", &Entity{Visibility: VisibilityDefault, IsPrivate: false}, grant, VisStateEveryone, false},
		{"dm_only, no grants", &Entity{Visibility: VisibilityDefault, IsPrivate: true}, nil, VisStateDMOnly, false},
		{"dm_only, widened by tag", &Entity{Visibility: VisibilityDefault, IsPrivate: true}, grant, VisStateDMOnly, true},
		{"custom, no grants", &Entity{Visibility: VisibilityCustom}, nil, VisStateCustom, false},
		{"custom, widened by tag", &Entity{Visibility: VisibilityCustom}, grant, VisStateCustom, true},
		// A DM-only category hides the page whatever its own setting, and tag
		// grants cannot widen past it.
		{"everyone in a DM-only category", &Entity{Visibility: VisibilityDefault, TypeDMOnly: true}, nil, VisStateDMOnly, false},
		{"DM-only category ignores tag grants", &Entity{Visibility: VisibilityDefault, TypeDMOnly: true}, grant, VisStateDMOnly, false},
		{"custom in a DM-only category stays custom", &Entity{Visibility: VisibilityCustom, TypeDMOnly: true}, grant, VisStateCustom, false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ev := ComputeEffectiveVisibility(tc.entity, tc.grants)
			if ev.BaseState != tc.wantBase {
				t.Errorf("base = %q, want %q", ev.BaseState, tc.wantBase)
			}
			if ev.WidenedByTags != tc.wantWidened {
				t.Errorf("widened = %v, want %v", ev.WidenedByTags, tc.wantWidened)
			}
		})
	}
}

func TestEffectiveVisibilityTooltip(t *testing.T) {
	ev := &EffectiveVisibility{
		BaseState:     VisStateDMOnly,
		WidenedByTags: true,
		TagGrants: []EntityTagGrantInfo{
			{TagSlug: "revealed-act-1", SubjectLabel: "Players"},
			{TagSlug: "secrets", SubjectLabel: "Lorekeepers"},
		},
	}
	got := effectiveVisibilityTooltip(ev, false)
	// The safety contract: the tooltip must NAME the tag + subject that exposed
	// the entity, never just the base state.
	for _, want := range []string{"DM-Only", "Also visible to", "Players via ‹revealed-act-1›", "Lorekeepers via ‹secrets›"} {
		if !strings.Contains(got, want) {
			t.Errorf("tooltip missing %q\ngot: %s", want, got)
		}
	}

	// Not-widened: tooltip is just the base sentence, no "Also visible".
	base := effectiveVisibilityTooltip(&EffectiveVisibility{BaseState: VisStateDMOnly}, false)
	if strings.Contains(base, "Also visible") {
		t.Errorf("non-widened tooltip must not claim tag exposure: %q", base)
	}
}

// TestEffectiveVisibilityBadge_Markup pins the tag-widening markup this
// component owns: the amber corner dot and the naming tooltip. The base
// state comes from the live entity (via baseVisibilityState), not the
// caller-supplied ev.BaseState, so every call site agrees on one source of
// truth; ev contributes only the tag-widening extras. The role gate itself
// is pinned separately, against real call sites, in
// visibility_glance_render_test.go.
func TestEffectiveVisibilityBadge_Markup(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner}
	dmOnly := &Entity{IsPrivate: true, Visibility: VisibilityDefault}
	render := func(entity *Entity, ev *EffectiveVisibility) string {
		var sb strings.Builder
		if err := effectiveVisibilityBadge(cc, entity, ev).Render(context.Background(), &sb); err != nil {
			t.Fatalf("render: %v", err)
		}
		return sb.String()
	}

	// Widened dm_only entity: lock icon + amber corner dot + widened marker +
	// the naming tooltip.
	widened := render(dmOnly, &EffectiveVisibility{
		BaseState:     VisStateDMOnly,
		WidenedByTags: true,
		TagGrants:     []EntityTagGrantInfo{{TagSlug: "revealed-act-1", SubjectLabel: "Players"}},
	})
	for _, want := range []string{"fa-lock", "bg-amber-400", `data-tag-widened="true"`, "Players via", "revealed-act-1"} {
		if !strings.Contains(widened, want) {
			t.Errorf("widened badge missing %q\ngot: %s", want, widened)
		}
	}

	// Plain dm_only (no grants): lock, but NO corner dot / widened marker.
	plain := render(dmOnly, &EffectiveVisibility{BaseState: VisStateDMOnly})
	if strings.Contains(plain, "bg-amber-400") || strings.Contains(plain, "data-tag-widened") {
		t.Errorf("non-widened badge must not show the tag-widened affordance:\n%s", plain)
	}

	// Nil ev (every call site but the show-page header): still renders the
	// entity's configured base state correctly, it just can't claim a tag
	// widening it was never given.
	noEv := render(dmOnly, nil)
	if !strings.Contains(noEv, "fa-lock") {
		t.Errorf("nil ev must still render the entity's base state, got: %q", noEv)
	}
	if strings.Contains(noEv, "bg-amber-400") || strings.Contains(noEv, "data-tag-widened") {
		t.Errorf("nil ev must not claim a tag widening it was never given, got: %q", noEv)
	}
}

// TestEffectiveVisibilityTooltip_VisitorsOnlyWhenCampaignIsPublic pins that
// an "everyone" entity's tooltip mentions logged-out visitors only when the
// campaign is public: reaching every campaign MEMBER is a property of the
// entity, but reaching a stranger is a property of the CAMPAIGN (only a
// public campaign is readable by RoleNone). Both directions are asserted so
// a test that only checked the public case can't pass against unconditional
// wording.
func TestEffectiveVisibilityTooltip_VisitorsOnlyWhenCampaignIsPublic(t *testing.T) {
	everyone := func() *EffectiveVisibility {
		return &EffectiveVisibility{BaseState: VisStateEveryone}
	}

	public := effectiveVisibilityTooltip(everyone(), true)
	if !strings.Contains(public, "logged-out visitors") {
		t.Errorf("on a PUBLIC campaign the glance must warn that strangers can read it "+
			"(C-PERM-ANON-IDENTITY honesty); got %q", public)
	}

	private := effectiveVisibilityTooltip(everyone(), false)
	if strings.Contains(private, "logged-out visitors") {
		t.Errorf("on a PRIVATE campaign no logged-out visitor can read ANY page, so the "+
			"glance must not claim they can; got %q", private)
	}
	if !strings.Contains(private, "everyone in this campaign") {
		t.Errorf("the private wording must still say it reaches every member; got %q", private)
	}

	// The other two states say nothing about visitors either way, so campaign
	// publicness must not leak into them.
	for _, state := range []string{VisStateDMOnly, VisStateCustom} {
		for _, isPublic := range []bool{true, false} {
			got := effectiveVisibilityTooltip(&EffectiveVisibility{BaseState: state}, isPublic)
			if strings.Contains(got, "logged-out visitors") {
				t.Errorf("state %q (public=%v) must never mention visitors: %q", state, isPublic, got)
			}
		}
	}
}
