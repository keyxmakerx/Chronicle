package campaigns

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildAdminCampaignWhere(t *testing.T) {
	const name = "name LIKE ? ESCAPE '!'"
	// The Trash page lists trashed campaigns; this list never does.
	const live = "deleted_at IS NULL"
	tests := []struct {
		name     string
		query    string
		system   string
		wantSQL  string
		wantArgs []any
	}{
		{"no constraints", "", "", " WHERE " + live, nil},
		{"name only", "ash", "", " WHERE " + live + " AND " + name, []any{"%ash%"}},
		{"none", "", SystemFilterNone, " WHERE " + live + " AND " + campaignSystemExpr + " = ''", nil},
		{"custom", "", SystemFilterCustom, " WHERE " + live + " AND " + campaignSystemExpr + " LIKE 'custom:%'", nil},
		{"system id is an arg", "", "drawsteel", " WHERE " + live + " AND " + campaignSystemExpr + " = ?", []any{"drawsteel"}},
		{"name and system", "ash", "dnd5e", " WHERE " + live + " AND " + name + " AND " + campaignSystemExpr + " = ?", []any{"%ash%", "dnd5e"}},
		{"wildcards escaped", "50%_", "", " WHERE " + live + " AND " + name, []any{"%50!%!_%"}},
		{"hostile system stays an arg", "", "x' OR '1'='1", " WHERE " + live + " AND " + campaignSystemExpr + " = ?", []any{"x' OR '1'='1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args := buildAdminCampaignWhere(tt.query, tt.system)
			if sql != tt.wantSQL {
				t.Errorf("sql = %q, want %q", sql, tt.wantSQL)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args = %#v, want %#v", args, tt.wantArgs)
			}
			if strings.Contains(sql, "OR '1'") {
				t.Errorf("user text leaked into SQL: %q", sql)
			}
		})
	}
}
