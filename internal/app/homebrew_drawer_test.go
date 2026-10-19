package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestDMHomebrewAdapter(t *testing.T) {
	render := func(a *dmHomebrewAdapter) string {
		var buf bytes.Buffer
		if err := a.DrawerMount("camp").Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	with := render(&dmHomebrewAdapter{trackers: true})
	for _, w := range []string{`data-widget="homebrew-drawer"`, `data-endpoint="/campaigns/camp/homebrew"`,
		`data-trackers="/campaigns/camp/trackers"`, "/static/plugins/trackers/css/trackers.css", "/static/plugins/homebrew/css/drawer.css"} {
		if !strings.Contains(with, w) {
			t.Errorf("missing %q in %s", w, with)
		}
	}
	without := render(&dmHomebrewAdapter{})
	if strings.Contains(without, "data-trackers") || strings.Contains(without, "trackers.css") {
		t.Errorf("trackers off should leave the pages out: %s", without)
	}
}
