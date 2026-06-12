package entities

import (
	"context"
	"strings"
	"testing"
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
	got := effectiveVisibilityTooltip(ev)
	// The safety contract: the tooltip must NAME the tag + subject that exposed
	// the entity, never just the base state.
	for _, want := range []string{"DM-Only", "Also visible to", "Players via ‹revealed-act-1›", "Lorekeepers via ‹secrets›"} {
		if !strings.Contains(got, want) {
			t.Errorf("tooltip missing %q\ngot: %s", want, got)
		}
	}

	// Not-widened: tooltip is just the base sentence, no "Also visible".
	base := effectiveVisibilityTooltip(&EffectiveVisibility{BaseState: VisStateDMOnly})
	if strings.Contains(base, "Also visible") {
		t.Errorf("non-widened tooltip must not claim tag exposure: %q", base)
	}
}

func TestEffectiveVisibilityBadge_Markup(t *testing.T) {
	render := func(ev *EffectiveVisibility) string {
		var sb strings.Builder
		if err := effectiveVisibilityBadge(ev).Render(context.Background(), &sb); err != nil {
			t.Fatalf("render: %v", err)
		}
		return sb.String()
	}

	// Widened dm_only entity: lock icon + amber corner dot + widened marker +
	// the naming tooltip.
	widened := render(&EffectiveVisibility{
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
	plain := render(&EffectiveVisibility{BaseState: VisStateDMOnly})
	if strings.Contains(plain, "bg-amber-400") || strings.Contains(plain, "data-tag-widened") {
		t.Errorf("non-widened badge must not show the tag-widened affordance:\n%s", plain)
	}

	// Nil glance (e.g. a player view): renders nothing.
	if got := render(nil); strings.TrimSpace(got) != "" {
		t.Errorf("nil glance must render empty, got: %q", got)
	}
}
