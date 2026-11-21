package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// The guest preview's strip names which visitor is shown, offers the switch
// between them only on a public campaign (a visitor with no code cannot open
// a private one), and always has the way back.
func TestGuestPreviewBanner(t *testing.T) {
	base := SetCampaignID(context.Background(), "camp-1")
	tests := []struct {
		name       string
		ctx        context.Context
		wantWords  string
		wantSwitch bool
		pressed    string
	}{
		{"public, no code", SetCampaignPublic(SetPreviewAs(base, "guest"), true), "Viewing as a visitor", true, `"as":"guest"`},
		{"public, guest code", SetCampaignPublic(SetPreviewAs(base, "guestcode"), true), "Viewing as a visitor with a guest code", true, `"as":"guestcode"`},
		{"private, guest code", SetPreviewAs(base, "guestcode"), "Viewing as a visitor with a guest code", false, ""},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		if err := GuestPreviewBanner().Render(tt.ctx, &buf); err != nil {
			t.Fatal(err)
		}
		out := strings.ReplaceAll(buf.String(), "&#34;", `"`)
		if !strings.Contains(out, tt.wantWords) {
			t.Errorf("%s: missing %q", tt.name, tt.wantWords)
		}
		if got := strings.Contains(out, `role="group"`); got != tt.wantSwitch {
			t.Errorf("%s: switch = %v, want %v", tt.name, got, tt.wantSwitch)
		}
		if tt.pressed != "" {
			i := strings.Index(out, `aria-pressed="true"`)
			if i < 0 || !strings.Contains(out[i:], tt.pressed) || strings.Index(out[i:], tt.pressed) > 300 {
				t.Errorf("%s: the pressed choice should be %s", tt.name, tt.pressed)
			}
		}
		if !strings.Contains(out, "Back to your view") || !strings.Contains(out, `"as":"self"`) || !strings.Contains(out, "/campaigns/camp-1/view-as") {
			t.Errorf("%s: no way back", tt.name)
		}
	}
}
