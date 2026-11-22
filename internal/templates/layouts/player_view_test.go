package layouts

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
)

// The page marks <html> only while an owner views as a player, so the
// Director-only hiding (input.css, "Player view") never reaches anyone else.
func TestBase_ViewAsPlayerMark(t *testing.T) {
	htmlTag := regexp.MustCompile(`<html[^>]*>`)
	for _, viewing := range []bool{false, true} {
		ctx := SetViewingAsPlayer(context.Background(), viewing)
		var buf bytes.Buffer
		if err := Base("Page").Render(ctx, &buf); err != nil {
			t.Fatalf("render: %v", err)
		}
		tag := htmlTag.FindString(buf.String())
		if tag == "" {
			t.Fatal("no <html> tag rendered")
		}
		got := regexp.MustCompile(`\sdata-view-as-player[\s>=]`).MatchString(tag)
		if got != viewing {
			t.Errorf("viewing=%v: <html> carries data-view-as-player = %v (%s)", viewing, got, tag)
		}
	}
}

// The eye shows as on during a guest preview, says how to go back, and does
// not take the player-view animation; the strip names the guest view. A
// plain visitor gets no eye at all.
func TestTopbar_GuestPreviewEye(t *testing.T) {
	base := SetCampaignID(context.Background(), "camp-1")
	tests := []struct {
		name     string
		ctx      context.Context
		wantEye  bool
		wantOn   bool
		wantAnim bool
	}{
		{"visitor", base, false, false, false},
		{"owner", SetIsOwner(base, true), true, false, true},
		{"player preview", SetViewingAsPlayer(base, true), true, true, true},
		{"guest preview", SetPreviewAs(base, "guest"), true, true, false},
		{"guest-code preview", SetPreviewAs(base, "guestcode"), true, true, false},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		if err := Topbar().Render(tt.ctx, &buf); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if got := strings.Contains(out, "/toggle-view-mode"); got != tt.wantEye {
			t.Errorf("%s: eye = %v, want %v", tt.name, got, tt.wantEye)
		}
		if got := strings.Contains(out, "Viewing as a guest — click to go back to your own view"); got != strings.HasPrefix(tt.name, "guest") {
			t.Errorf("%s: guest title = %v", tt.name, got)
		}
		if got := strings.Contains(out, "data-pv-toggle"); got != tt.wantAnim {
			t.Errorf("%s: data-pv-toggle = %v, want %v", tt.name, got, tt.wantAnim)
		}
		if got := strings.Contains(out, "!text-amber-400"); got != tt.wantOn {
			t.Errorf("%s: eye drawn on = %v, want %v", tt.name, got, tt.wantOn)
		}
	}
}
