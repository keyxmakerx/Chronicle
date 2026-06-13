package entities

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// TestVisibilityFilter pins the entities-side half of the mirrored entity
// visibility policy (the calendar plugin holds the verbatim other half in
// entity_ties_test.go::TestEntityVisibilityFilter). Both suites assert the SAME
// token set + arg shape so the two SQL fragments cannot silently diverge
// (cordinator#32/#455). This is the most security-sensitive query in the
// product — a drift here is a data-leak.
func TestVisibilityFilter(t *testing.T) {
	t.Run("owner is unfiltered (sees all, including dm_only)", func(t *testing.T) {
		frag, args := visibilityFilter(permissions.RoleOwner, "owner-1")
		if frag != "" || args != nil {
			t.Fatalf("owner must get no filter; got frag=%q args=%v", frag, args)
		}
	})

	// RoleNone is the anonymous / public-visitor identity (C-PERM-ANON-IDENTITY)
	// and must be filtered exactly like any other non-owner — same token set,
	// same arg shape — with role=0 fed into every role comparison.
	for _, role := range []int{permissions.RoleNone, permissions.RolePlayer, permissions.RoleScribe} {
		role := role
		t.Run("non-owner is filtered (with additive tag-grant branch)", func(t *testing.T) {
			frag, args := visibilityFilter(role, "user-9")
			if frag == "" {
				t.Fatalf("role %d must get a restrictive filter", role)
			}
			// default-mode is_private + custom entity_permissions (role/user/
			// group/public) + the additive tag-grant branch (entity_tags ⋈
			// tag_permissions). MUST match the calendar mirror's token set.
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
					t.Errorf("filter missing %q (drift from policy / calendar mirror?)\nfrag=%s", want, frag)
				}
			}
			// entity_permissions branch: role, role, userID, userID.
			// tag_permissions branch: role, userID, userID.
			// The 'public' subject matches unconditionally, so it adds NO arg.
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

	// Lock the additive branch's exact subject-match shape so a refactor can't
	// quietly weaken it (e.g. dropping the role ceiling, the group join, or
	// flipping the comparison direction). Both grant tables use subject_id <= ?
	// (viewer role): a Player-role grant (subject_id "1") is therefore visible
	// to role >= 1 but NOT to an anonymous viewer (role 0) — the leak this fix
	// closes. The 'public' subject matches every viewer, anonymous included.
	t.Run("subject-match shape: role ceiling, user, group, public", func(t *testing.T) {
		frag, _ := visibilityFilter(permissions.RolePlayer, "u")
		for _, want := range []string{
			"ep.subject_type = 'role' AND CAST(ep.subject_id AS UNSIGNED) <= ?",
			"ep.subject_type = 'user' AND ep.subject_id = ?",
			"ep.subject_type = 'public'",
			"tp.subject_type = 'role' AND CAST(tp.subject_id AS UNSIGNED) <= ?",
			"tp.subject_type = 'user' AND tp.subject_id = ?",
			"tp.subject_type = 'public'",
			"tp.subject_type = 'group'",
		} {
			if !strings.Contains(frag, want) {
				t.Errorf("subject-match missing %q\nfrag=%s", want, frag)
			}
		}
		// Direction guard: the role tier must use `<= ?`, never `>= ?`, or an
		// anonymous viewer (role 0) would match a Player grant.
		if strings.Contains(frag, "subject_id AS UNSIGNED) >= ?") {
			t.Errorf("role-tier comparison must be `<= ?` (anon-below-Player); found `>= ?`\nfrag=%s", frag)
		}
	})
}
