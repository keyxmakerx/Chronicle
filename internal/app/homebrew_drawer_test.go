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
		if err := a.DrawerMount("camp", true).Render(context.Background(), &buf); err != nil {
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
	if !strings.Contains(with, `data-can-place="true"`) {
		t.Errorf("the owner's drawer should offer placing a Trackers block: %s", with)
	}
	var codm bytes.Buffer
	if err := (&dmHomebrewAdapter{trackers: true}).DrawerMount("camp", false).Render(context.Background(), &codm); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(codm.String(), "data-can-place") {
		t.Errorf("a co-DM's drawer must not offer placing (the routes are the owner's): %s", codm.String())
	}
	without := render(&dmHomebrewAdapter{})
	if strings.Contains(without, "data-trackers") || strings.Contains(without, "trackers.css") {
		t.Errorf("trackers off should leave the pages out: %s", without)
	}
}
