package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// attrJSON reads one data attribute off the page and parses it.
func attrJSON(t *testing.T, page, attr string) map[string]any {
	t.Helper()
	m := regexp.MustCompile(` ` + attr + `="([^"]*)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("%s is not on the mount", attr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &out); err != nil {
		t.Fatalf("%s is not JSON: %v", attr, err)
	}
	return out
}

// Every mount the entry form can open on carries the border library and the
// picker's lists as JSON the form can parse.
func TestEntryFormMounts_CarryBorderLibrary(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test"}, MemberRole: campaigns.RolePlayer, IsMember: true}
	pages := map[string]string{}
	var b bytes.Buffer
	if err := heroBuilder(&HeroBuilderView{CampaignID: "camp-1"}).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	pages["hero builder"] = b.String()
	b.Reset()
	if err := OwnEntriesPage(cc, OwnEntriesView{Director: true, UserID: "u1"}).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	pages["own entries"] = b.String()
	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			lib := attrJSON(t, page, "data-borders")
			pick := attrJSON(t, page, "data-border-picker")
			if len(lib) == 0 {
				t.Error("data-borders is empty")
			}
			bs, _ := pick["borders"].(map[string]any)
			if len(bs) == 0 || pick["groups"] == nil {
				t.Errorf("data-border-picker lacks borders or groups: %v", pick)
			}
			for k := range bs {
				if _, ok := lib[k]; !ok {
					t.Errorf("picker border %q is not in the library", k)
				}
			}
			if strings.Contains(page, `data-look="stage"`) {
				t.Error("only the creator names a look")
			}
		})
	}
}
