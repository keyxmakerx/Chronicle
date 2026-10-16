package layouts

import (
	"bytes"
	"context"
	"regexp"
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
