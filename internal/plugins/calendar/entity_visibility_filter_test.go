package calendar

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// The calendar mirrors the entities visibility predicate, so it must also keep
// pages of the DM-only Adventures category away from anyone below Owner (a
// Scribe sees what players see), with the role first among the arguments as
// the entities filter does.
func TestEntityVisibilityFilter_DMOnlyCategory(t *testing.T) {
	tests := []struct {
		name       string
		role       int
		wantClause bool
	}{
		{"anonymous", permissions.RoleNone, true},
		{"player", permissions.RolePlayer, true},
		{"scribe", permissions.RoleScribe, true},
		{"owner sees everything, no filter", permissions.RoleOwner, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frag, args := entityVisibilityFilter(tt.role, "u1")
			has := strings.Contains(frag, "dmt.preset_category = 'adventures'") && strings.Contains(frag, " AND NOT EXISTS (")
			if has != tt.wantClause {
				t.Fatalf("DM-only clause present = %v, want %v\n%s", has, tt.wantClause, frag)
			}
			if tt.wantClause {
				if len(args) == 0 || args[0] != tt.role {
					t.Errorf("first arg must be the viewer's role, got %v", args)
				}
				if strings.Count(frag, "?") != len(args) {
					t.Errorf("placeholder count %d != arg count %d", strings.Count(frag, "?"), len(args))
				}
			}
		})
	}
}
