package layouts

import (
	"context"
	"testing"
)

func TestViewPrefAttrsTimezone(t *testing.T) {
	tests := []struct {
		name string
		tz   string
		want string
	}{
		{"saved zone", "America/Chicago", "America/Chicago"},
		{"none saved", "", ""},
		{"bad value never becomes an attribute", `"><script>`, ""},
	}
	for _, tc := range tests {
		ctx := SetViewPrefs(context.Background(), &ViewPrefsData{Theme: "device", Timezone: tc.tz})
		got, _ := ViewPrefAttrs(ctx)["data-view-tz"].(string)
		if got != tc.want {
			t.Errorf("%s: data-view-tz = %q, want %q", tc.name, got, tc.want)
		}
	}
}
