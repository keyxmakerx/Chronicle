package campaigns

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildAdminCampaignWhere(t *testing.T) {
	const name = "name LIKE ? ESCAPE '!'"
	tests := []struct {
		name     string
		query    string
		system   string
		wantSQL  string
		wantArgs []any
	}{
		{"no constraints", "", "", "", nil},
		{"name only", "ash", "", " WHERE " + name, []any{"%ash%"}},
		{"none", "", SystemFilterNone, " WHERE " + campaignSystemExpr + " = ''", nil},
		{"custom", "", SystemFilterCustom, " WHERE " + campaignSystemExpr + " LIKE 'custom:%'", nil},
		{"system id is an arg", "", "drawsteel", " WHERE " + campaignSystemExpr + " = ?", []any{"drawsteel"}},
		{"name and system", "ash", "dnd5e", " WHERE " + name + " AND " + campaignSystemExpr + " = ?", []any{"%ash%", "dnd5e"}},
		{"wildcards escaped", "50%_", "", " WHERE " + name, []any{"%50!%!_%"}},
		{"hostile system stays an arg", "", "x' OR '1'='1", " WHERE " + campaignSystemExpr + " = ?", []any{"x' OR '1'='1"}},
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
