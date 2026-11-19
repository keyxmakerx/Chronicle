package campaigns

import (
	"strings"
	"testing"
)

func findLabels(es []customizeFindEntry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Label)
	}
	return out
}

func hasLabel(es []customizeFindEntry, label string) bool {
	for _, e := range es {
		if e.Label == label {
			return true
		}
	}
	return false
}

func TestBuildDeskFind(t *testing.T) {
	custom := []customizeFindEntry{
		{Label: "Colours", Path: "Look", Href: "/campaigns/c1/customize/look#colours", Icon: "fa-palette"},
		{Label: "Overview", Path: "Customize", Href: "/campaigns/c1/customize", Icon: "fa-compass"},
	}
	tabs := []SettingsTab{
		{ID: "general", Label: "Details", Icon: "fa-solid fa-gear", MinRole: RolePlayer},
		{ID: "data", Label: "Export", Icon: "fa-solid fa-database", MinRole: RolePlayer},
		{ID: "secret", Label: "Hidden from the owner's own page", MinRole: RoleOwner + 1},
		{ID: "blank"},
	}
	tests := []struct {
		name           string
		media, foundry bool
		want           []string
		wantNot        []string
	}{
		{"nothing switched on", false, false,
			[]string{"People", "Trash", "Customize", "Game & features", "Your AI", "What players changed", "Name", "Colours", "Details",
				"Players' dashboard", "Guests' dashboard", "New pages start", "Who can read it", "Keys", "Export a copy", "Import", "Activity log",
				"Archive this campaign", "Delete this campaign"},
			[]string{"Media", "Foundry", "Overview", "Hidden from the owner's own page", "Export"}},
		{"media on", true, false, []string{"Media"}, []string{"Foundry"}},
		{"foundry on", false, true, []string{"Foundry"}, []string{"Media"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDeskFind(deskFindInput{Base: "/campaigns/c1", Media: tt.media, Foundry: tt.foundry, Customize: custom, SettingsTabs: tabs})
			for _, l := range tt.want {
				if !hasLabel(got, l) {
					t.Errorf("missing %q in %v", l, findLabels(got))
				}
			}
			for _, l := range tt.wantNot {
				if hasLabel(got, l) {
					t.Errorf("%q must not be offered: %v", l, findLabels(got))
				}
			}
			for _, e := range got {
				if !strings.HasPrefix(e.Href, "/campaigns/c1/") {
					t.Errorf("%q leaves the campaign: %s", e.Label, e.Href)
				}
				if e.Label == "" || e.Path == "" {
					t.Errorf("an entry with no words to show: %+v", e)
				}
			}
		})
	}
}

// What the campaign switched off is not in the list, including words that
// would match it: Foundry's name and its search words appear only with it on.
func TestBuildDeskFind_SwitchedOffToolsLeaveNoTrace(t *testing.T) {
	got := buildDeskFind(deskFindInput{Base: "/campaigns/c1"})
	for _, e := range got {
		hay := strings.ToLower(e.Label + " " + e.Path + " " + e.Words + " " + e.Href)
		for _, off := range []string{"foundry", "/media", "gallery"} {
			if strings.Contains(hay, off) {
				t.Errorf("%q carries %q while its add-on is off", e.Label, off)
			}
		}
	}
}

func TestBuildDeskFind_CustomizeEntriesKeepTheirPlace(t *testing.T) {
	got := buildDeskFind(deskFindInput{Base: "/campaigns/c1", Customize: []customizeFindEntry{
		{Label: "Colours", Path: "Look", Href: "/campaigns/c1/customize/look#colours"},
		{Label: "Home pages", Path: "Customize", Href: "/campaigns/c1/customize/home"},
	}})
	for _, e := range got {
		switch e.Label {
		case "Colours":
			if e.Path != "Customize › Look" {
				t.Errorf("Colours path = %q", e.Path)
			}
		case "Home pages":
			if e.Path != "Customize" {
				t.Errorf("Home pages path = %q", e.Path)
			}
		}
	}
}

// The words people type find the rows they mean, and a result's path names the
// page and the group.
func TestBuildDeskFind_WordsFindTheirRows(t *testing.T) {
	got := buildDeskFind(deskFindInput{Base: "/campaigns/c1", Media: true, Foundry: true})
	find := func(word string) []string {
		var labels []string
		for _, e := range got {
			hay := strings.ToLower(e.Label + " " + e.Path + " " + e.Words)
			if strings.Contains(hay, word) {
				labels = append(labels, e.Label)
			}
		}
		return labels
	}
	tests := []struct {
		word string
		want []string
	}{
		{"guest", []string{"Guests' dashboard"}},
		{"dashboard", []string{"Players' dashboard", "Guests' dashboard"}},
		{"export", []string{"Export a copy"}},
		{"delete", []string{"Delete this campaign"}},
		{"archive", []string{"Archive this campaign"}},
		{"invite", []string{"People"}},
		{"rename", []string{"Name"}},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			labels := find(tt.word)
			for _, want := range tt.want {
				ok := false
				for _, l := range labels {
					ok = ok || l == want
				}
				if !ok {
					t.Errorf("%q does not find %q, only %v", tt.word, want, labels)
				}
			}
		})
	}
	// "settings" is in every path, so it lists the page's rows rather than nothing.
	if n := len(find("settings")); n < 15 {
		t.Errorf("settings finds only %d rows", n)
	}
	for _, e := range got {
		if strings.HasPrefix(e.Path, "Manage") {
			t.Errorf("%q still lives under Manage: %q", e.Label, e.Path)
		}
	}
}

func TestBuildDeskFind_RowPathsNameTheGroup(t *testing.T) {
	got := buildDeskFind(deskFindInput{Base: "/campaigns/c1", Archived: true})
	want := map[string]string{
		"People":                  "Campaign settings › People",
		"Guests' dashboard":       "Campaign settings › What players and guests see",
		"Export a copy":           "Campaign settings › Safety net",
		"Unarchive this campaign": "Campaign settings › Archive or delete",
		"Delete this campaign":    "Campaign settings › Archive or delete",
		"Your AI":                 "Campaign settings › Connections",
	}
	for _, e := range got {
		if p, ok := want[e.Label]; ok {
			if e.Path != p {
				t.Errorf("%q path = %q, want %q", e.Label, e.Path, p)
			}
			delete(want, e.Label)
		}
	}
	for l := range want {
		t.Errorf("%q is not offered", l)
	}
}
