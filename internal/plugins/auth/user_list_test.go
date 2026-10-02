package auth

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseUserFilter(t *testing.T) {
	tests := []struct {
		raw  string
		want UserFilter
	}{
		{"", UserFilterAll},
		{"all", UserFilterAll},
		{"admins", UserFilterAdmins},
		{"disabled", UserFilterDisabled},
		{"ADMINS", UserFilterAll},
		{"admins; DROP TABLE users", UserFilterAll},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := ParseUserFilter(tt.raw); got != tt.want {
				t.Errorf("ParseUserFilter(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestBuildUserSearchWhere(t *testing.T) {
	const search = " WHERE (display_name LIKE ? ESCAPE '!' OR email LIKE ? ESCAPE '!')"
	tests := []struct {
		name     string
		query    string
		filter   UserFilter
		wantSQL  string
		wantArgs []any
	}{
		{"no constraints", "", UserFilterAll, "", nil},
		{"admins only", "", UserFilterAdmins, " WHERE is_admin = 1", nil},
		{"disabled only", "", UserFilterDisabled, " WHERE is_disabled = 1", nil},
		{"search", "ann", UserFilterAll, search, []any{"%ann%", "%ann%"}},
		{"search and chip", "ann", UserFilterDisabled, search + " AND is_disabled = 1", []any{"%ann%", "%ann%"}},
		{"wildcards are escaped", "100%_", UserFilterAll, search, []any{"%100!%!_%", "%100!%!_%"}},
		{"quotes never reach the SQL text", "x' OR 1=1 --", UserFilterAdmins, search + " AND is_admin = 1",
			[]any{"%x' OR 1=1 --%", "%x' OR 1=1 --%"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args := buildUserSearchWhere(tt.query, tt.filter)
			if sql != tt.wantSQL {
				t.Errorf("sql = %q, want %q", sql, tt.wantSQL)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args = %#v, want %#v", args, tt.wantArgs)
			}
			if strings.Contains(sql, "OR 1=1") {
				t.Errorf("user text leaked into SQL: %q", sql)
			}
		})
	}
}

func TestUserFilterCounts_For(t *testing.T) {
	c := UserFilterCounts{All: 10, Admins: 2, Disabled: 3}
	tests := []struct {
		f    UserFilter
		want int
	}{{UserFilterAll, 10}, {UserFilterAdmins, 2}, {UserFilterDisabled, 3}}
	for _, tt := range tests {
		if got := c.For(tt.f); got != tt.want {
			t.Errorf("For(%q) = %d, want %d", tt.f, got, tt.want)
		}
	}
}
