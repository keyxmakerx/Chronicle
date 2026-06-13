// entity_ties_test.go — C-CAL-ENTITY-TIES-DATA-MODEL.
//
// Covers the participation-role enum (which MUST match Phase 1.5's showcase
// picker), enum validation on the write path, both-direction queries, the
// optional-both-ways property, and the role round-trip. Cascade-on-delete is
// DB-enforced (FK ON DELETE CASCADE) and is exercised by the operator's local
// migrate gate, noted in the PR.
package calendar

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// --- mockCalendarRepo entity-ties method stubs ---

func (m *mockCalendarRepo) LinkEntityEvent(ctx context.Context, entityID, eventID, role string) error {
	if m.linkEntityEventFn != nil {
		return m.linkEntityEventFn(ctx, entityID, eventID, role)
	}
	return nil
}
func (m *mockCalendarRepo) UnlinkEntityEvent(ctx context.Context, entityID, eventID string) error {
	if m.unlinkEntityEventFn != nil {
		return m.unlinkEntityEventFn(ctx, entityID, eventID)
	}
	return nil
}
func (m *mockCalendarRepo) LinkEntityEra(ctx context.Context, entityID string, eraID int, role *string) error {
	if m.linkEntityEraFn != nil {
		return m.linkEntityEraFn(ctx, entityID, eraID, role)
	}
	return nil
}
func (m *mockCalendarRepo) UnlinkEntityEra(ctx context.Context, entityID string, eraID int) error {
	if m.unlinkEntityEraFn != nil {
		return m.unlinkEntityEraFn(ctx, entityID, eraID)
	}
	return nil
}
func (m *mockCalendarRepo) EntitiesForEvent(ctx context.Context, eventID string) ([]EntityTieRef, error) {
	if m.entitiesForEventFn != nil {
		return m.entitiesForEventFn(ctx, eventID)
	}
	return nil, nil
}
func (m *mockCalendarRepo) EntitiesForEra(ctx context.Context, eraID int) ([]EntityTieRef, error) {
	if m.entitiesForEraFn != nil {
		return m.entitiesForEraFn(ctx, eraID)
	}
	return nil, nil
}
func (m *mockCalendarRepo) EntitiesForCalendar(ctx context.Context, calendarID string, role int, userID string) ([]EntityTieRef, error) {
	if m.entitiesForCalendarFn != nil {
		return m.entitiesForCalendarFn(ctx, calendarID, role, userID)
	}
	return nil, nil
}
func (m *mockCalendarRepo) EventsForEntity(ctx context.Context, entityID string) ([]EntityEventTie, error) {
	if m.eventsForEntityFn != nil {
		return m.eventsForEntityFn(ctx, entityID)
	}
	return nil, nil
}
func (m *mockCalendarRepo) ErasForEntity(ctx context.Context, entityID string) ([]EntityEraTie, error) {
	if m.erasForEntityFn != nil {
		return m.erasForEntityFn(ctx, entityID)
	}
	return nil, nil
}

// TestParticipationRoles_MatchShowcaseEnum is the load-bearing guard: the four
// roles + their order MUST match Phase 1.5's showcase picker exactly so the
// Phase-2 port wires mock→real with no translation. The canonical pin is the
// dispatch (involved · present · affected · mentioned) + Phase 1.5 R3's
// __calParticipationRoles. NB: R3 (PR #400) is NOT on main at this PR's base —
// only #399 (R1+R2) is — so this asserts the verbatim dispatch values rather
// than coupling the backend test to the demo branch's JS. Flagged in the PR.
func TestParticipationRoles_MatchShowcaseEnum(t *testing.T) {
	want := []ParticipationRole{"involved", "present", "affected", "mentioned"}
	if len(ParticipationRoles) != len(want) {
		t.Fatalf("role count drift: got %v", ParticipationRoles)
	}
	for i := range want {
		if ParticipationRoles[i] != want[i] {
			t.Errorf("role[%d] = %q, want %q (order matters)", i, ParticipationRoles[i], want[i])
		}
	}
}

func TestParticipationRole_Validation(t *testing.T) {
	for _, r := range ParticipationRoles {
		if !r.IsValid() {
			t.Errorf("%q should be valid", r)
		}
	}
	for _, bad := range []string{"killed", "born", "x", "Involved"} {
		if ParticipationRole(bad).IsValid() {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestLinkEntityToEvent_RoleValidationAndDefault(t *testing.T) {
	var gotRole string
	repo := &mockCalendarRepo{
		linkEntityEventFn: func(_ context.Context, _, _, role string) error {
			gotRole = role
			return nil
		},
	}
	svc := NewCalendarService(repo)

	// Empty role defaults to "involved".
	if err := svc.LinkEntityToEvent(context.Background(), "ent-1", "evt-1", ""); err != nil {
		t.Fatalf("link with empty role: %v", err)
	}
	if gotRole != "involved" {
		t.Errorf("empty role should default to involved, got %q", gotRole)
	}

	// Valid role round-trips verbatim.
	if err := svc.LinkEntityToEvent(context.Background(), "ent-1", "evt-1", "affected"); err != nil {
		t.Fatalf("link with valid role: %v", err)
	}
	if gotRole != "affected" {
		t.Errorf("valid role should round-trip, got %q", gotRole)
	}

	// Invalid role is rejected (and never reaches the repo).
	gotRole = ""
	if err := svc.LinkEntityToEvent(context.Background(), "ent-1", "evt-1", "bogus"); err == nil {
		t.Errorf("invalid role should be rejected")
	}
	if gotRole != "" {
		t.Errorf("invalid role must not reach the repo")
	}
}

func TestLinkEntityToEra_OptionalRole(t *testing.T) {
	var gotRole *string
	called := false
	repo := &mockCalendarRepo{
		linkEntityEraFn: func(_ context.Context, _ string, _ int, role *string) error {
			gotRole, called = role, true
			return nil
		},
	}
	svc := NewCalendarService(repo)

	// nil role allowed (era ties are coarser) → stored NULL.
	if err := svc.LinkEntityToEra(context.Background(), "ent-1", 7, nil); err != nil {
		t.Fatalf("era link with nil role: %v", err)
	}
	if !called || gotRole != nil {
		t.Errorf("nil era role should pass through as NULL, got %v", gotRole)
	}

	// Empty-string role also normalizes to NULL.
	empty := ""
	if err := svc.LinkEntityToEra(context.Background(), "ent-1", 7, &empty); err != nil {
		t.Fatalf("era link with empty role: %v", err)
	}
	if gotRole != nil {
		t.Errorf("empty era role should normalize to NULL, got %v", *gotRole)
	}

	// Valid role passes through.
	valid := "present"
	if err := svc.LinkEntityToEra(context.Background(), "ent-1", 7, &valid); err != nil {
		t.Fatalf("era link with valid role: %v", err)
	}
	if gotRole == nil || *gotRole != "present" {
		t.Errorf("valid era role should pass through, got %v", gotRole)
	}

	// Invalid role rejected.
	bad := "haunted"
	if err := svc.LinkEntityToEra(context.Background(), "ent-1", 7, &bad); err == nil {
		t.Errorf("invalid era role should be rejected")
	}
}

func TestEntityTies_BothDirectionQueries(t *testing.T) {
	repo := &mockCalendarRepo{
		eventsForEntityFn: func(_ context.Context, entityID string) ([]EntityEventTie, error) {
			return []EntityEventTie{{Event: Event{ID: "evt-1", Name: "Siege"}, ParticipationRole: "involved"}}, nil
		},
		entitiesForEventFn: func(_ context.Context, eventID string) ([]EntityTieRef, error) {
			role := "involved"
			return []EntityTieRef{{EntityID: "ent-1", EntityName: "Marisha", EntityType: "npc", ParticipationRole: &role}}, nil
		},
		erasForEntityFn: func(_ context.Context, entityID string) ([]EntityEraTie, error) {
			return []EntityEraTie{{Era: Era{ID: 3, Name: "Age of Fire"}}}, nil // nil role
		},
		entitiesForEraFn: func(_ context.Context, eraID int) ([]EntityTieRef, error) {
			return []EntityTieRef{{EntityID: "ent-1", EntityName: "Marisha", EntityType: "npc"}}, nil
		},
	}
	svc := NewCalendarService(repo)
	ctx := context.Background()

	evs, _ := svc.EventsForEntity(ctx, "ent-1")
	if len(evs) != 1 || evs[0].Event.ID != "evt-1" || evs[0].ParticipationRole != "involved" {
		t.Errorf("EventsForEntity wrong: %+v", evs)
	}
	ents, _ := svc.EntitiesForEvent(ctx, "evt-1")
	if len(ents) != 1 || ents[0].EntityID != "ent-1" || ents[0].ParticipationRole == nil {
		t.Errorf("EntitiesForEvent wrong: %+v", ents)
	}
	eras, _ := svc.ErasForEntity(ctx, "ent-1")
	if len(eras) != 1 || eras[0].Era.ID != 3 || eras[0].ParticipationRole != nil {
		t.Errorf("ErasForEntity wrong (era role should be nil-able): %+v", eras)
	}
	eraEnts, _ := svc.EntitiesForEra(ctx, 3)
	if len(eraEnts) != 1 || eraEnts[0].ParticipationRole != nil {
		t.Errorf("EntitiesForEra wrong: %+v", eraEnts)
	}
}

// TestEntityVisibilityFilter pins cordinator#32 gap #1: the calendar
// associations panel must apply the SAME entity-visibility policy the entities
// plugin uses, so a player can't learn the NAME of a dm_only / custom-restricted
// entity through it. Owners/co-DMs (role >= RoleOwner) get NO filter — they see
// every tied entity; lower roles get the restrictive fragment that drops
// private "default" entities and "custom" entities without a matching
// entity_permissions grant. This mirrors entities/repository.go::visibilityFilter
// (unexported there) — the two MUST stay in lockstep.
func TestEntityVisibilityFilter(t *testing.T) {
	t.Run("owner is unfiltered (sees all, including dm_only)", func(t *testing.T) {
		frag, args := entityVisibilityFilter(permissions.RoleOwner, "owner-1")
		if frag != "" || args != nil {
			t.Fatalf("owner must get no filter; got frag=%q args=%v", frag, args)
		}
	})

	// RoleNone is the anonymous / public-visitor identity (C-PERM-ANON-IDENTITY)
	// — filtered like any other non-owner. Mirrors entities/visibility_filter_test.
	for _, role := range []int{permissions.RoleNone, permissions.RolePlayer, permissions.RoleScribe} {
		role := role
		t.Run("non-owner is filtered", func(t *testing.T) {
			frag, args := entityVisibilityFilter(role, "user-9")
			if frag == "" {
				t.Fatalf("role %d must get a restrictive filter", role)
			}
			// Gate on the same columns/tables the entities plugin uses: the
			// legacy is_private flag (default mode) + custom entity_permissions
			// (user/role/group/public grants) + the additive tag-grant branch
			// (entity_tags ⋈ tag_permissions). Drift from these = drift from
			// policy, and the two mirror sites would silently diverge.
			for _, want := range []string{
				"e.visibility = 'default'",
				"e.is_private = false",
				"e.visibility = 'custom'",
				"entity_permissions",
				"campaign_group_members",
				"entity_tags",
				"tag_permissions",
				"ep.subject_type = 'public'",
				"tp.subject_type = 'public'",
			} {
				if !strings.Contains(frag, want) {
					t.Errorf("filter missing %q (drift from entities policy?)\nfrag=%s", want, frag)
				}
			}
			// Args bind role twice + userID twice for the entity_permissions
			// branch (default threshold, role grant, user grant, group
			// membership), then role + userID + userID again for the additive
			// tag_permissions branch (role grant, user grant, group membership).
			// The 'public' subject matches unconditionally and adds NO arg.
			wantArgs := []any{role, role, "user-9", "user-9", role, "user-9", "user-9"}
			if len(args) != len(wantArgs) {
				t.Fatalf("arg count = %d, want %d (%v)", len(args), len(wantArgs), args)
			}
			for i := range wantArgs {
				if args[i] != wantArgs[i] {
					t.Errorf("arg[%d] = %v, want %v", i, args[i], wantArgs[i])
				}
			}
		})
	}

	// Direction + 'public' guard, mirroring the entities suite: the role tier
	// must use `<= ?` so anonymous (role 0) never matches a Player grant.
	t.Run("subject-match shape: role ceiling <= ? and public", func(t *testing.T) {
		frag, _ := entityVisibilityFilter(permissions.RolePlayer, "u")
		for _, want := range []string{
			"tp.subject_type = 'role' AND CAST(tp.subject_id AS UNSIGNED) <= ?",
			"tp.subject_type = 'public'",
			"ep.subject_type = 'public'",
		} {
			if !strings.Contains(frag, want) {
				t.Errorf("subject-match missing %q\nfrag=%s", want, frag)
			}
		}
		if strings.Contains(frag, "subject_id AS UNSIGNED) >= ?") {
			t.Errorf("role-tier comparison must be `<= ?` (anon-below-Player); found `>= ?`\nfrag=%s", frag)
		}
	})
}

// TestEntityTies_OptionalBothWays: zero ties is a valid state on both sides
// (an event with no entities, an entity with no events) — the property the
// operator pinned ("can be but does not have to be tied").
func TestEntityTies_OptionalBothWays(t *testing.T) {
	svc := NewCalendarService(&mockCalendarRepo{}) // all stubs return nil, nil
	ctx := context.Background()
	if evs, err := svc.EventsForEntity(ctx, "lonely-entity"); err != nil || len(evs) != 0 {
		t.Errorf("entity with no events should yield empty, got %v err=%v", evs, err)
	}
	if ents, err := svc.EntitiesForEvent(ctx, "lonely-event"); err != nil || len(ents) != 0 {
		t.Errorf("event with no entities should yield empty, got %v err=%v", ents, err)
	}
}
