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
		{ID: "vault", Label: "Vault notes", Icon: "fa-solid fa-book", MinRole: RolePlayer},
		{ID: settingsTabImport, Label: "Import", Icon: "fa-solid fa-file-import", MinRole: RoleOwner},
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
			[]string{"People", "Trash", "Customize", "Game & features", "Your AI", "What players changed", "Name", "Description", "Colours", "Vault notes", "Campaign", "Connections", "Data",
				"Preview as a player", "Preview as a guest", "New pages", "Private or public", "Game system", "Keys", "Export", "Import", "Activity log",
				"Archive", "Transfer ownership", "Delete"},
			[]string{"Media", "Foundry", "Overview", "Hidden from the owner's own page", "Unarchive"}},
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
			imports := 0
			for _, e := range got {
				if e.Label == "Import" {
					imports++
				}
			}
			if imports != 1 {
				t.Errorf("Import is offered %d times, want once: its part and its row are one place", imports)
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
		{"guest", []string{"Preview as a guest"}},
		{"preview", []string{"Preview as a player", "Preview as a guest"}},
		{"export", []string{"Export"}},
		{"delete", []string{"Delete"}},
		{"archive", []string{"Archive"}},
		{"transfer", []string{"Transfer ownership"}},
		{"invite", []string{"People"}},
		{"rename", []string{"Name"}},
		{"blurb", []string{"Description"}},
		{"rules", []string{"Game system"}},
		// The names the page used to carry still find their rows.
		{"safety net", []string{"Export", "Import", "Activity log", "Trash"}},
		{"who can read", []string{"Private or public"}},
		{"archive or delete", []string{"Archive", "Delete", "Transfer ownership"}},
		{"what players and guests see", []string{"Private or public", "New pages", "Preview as a guest"}},
		{"look and game", []string{"Name", "Description", "Media", "Game & features"}},
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
	got := buildDeskFind(deskFindInput{Base: "/campaigns/c1", Media: true, Foundry: true, Archived: true})
	type where struct{ path, href string }
	want := map[string]where{
		"Campaign":             {"Campaign settings", "/campaigns/c1/settings"},
		"Connections":          {"Campaign settings", "/campaigns/c1/connections"},
		"Data":                 {"Campaign settings", "/campaigns/c1/data"},
		"Name":                 {"Campaign settings › Campaign", "/campaigns/c1/settings#line-name"},
		"Description":          {"Campaign settings › Campaign", "/campaigns/c1/settings#line-description"},
		"Private or public":    {"Campaign settings › Campaign", "/campaigns/c1/settings#line-public"},
		"New pages":            {"Campaign settings › Campaign", "/campaigns/c1/settings#line-pages"},
		"Preview as a player":  {"Campaign settings › Campaign", "/campaigns/c1/settings#line-preview"},
		"Preview as a guest":   {"Campaign settings › Campaign", "/campaigns/c1/settings#line-preview"},
		"Customize":            {"Campaign settings › Campaign", "/campaigns/c1/customize"},
		"People":               {"Campaign settings › People", "/campaigns/c1/members"},
		"What players changed": {"Campaign settings › People", "/campaigns/c1/players/changes"},
		"Transfer ownership":   {"Campaign settings › People", "/campaigns/c1/members#transfer"},
		"Game system":          {"Campaign settings › Game", "/campaigns/c1/extensions"},
		"Game & features":      {"Campaign settings › Game", "/campaigns/c1/extensions"},
		"Foundry":              {"Campaign settings › Connections", "/campaigns/c1/foundry"},
		"Your AI":              {"Campaign settings › Connections", "/campaigns/c1/ai"},
		"Keys":                 {"Campaign settings › Connections", "/campaigns/c1/connections#keys"},
		"Media":                {"Campaign settings › Data", "/campaigns/c1/media"},
		"Export":               {"Campaign settings › Data", "/campaigns/c1/data#export"},
		"Import":               {"Campaign settings › Data", "/campaigns/c1/data#import"},
		"Activity log":         {"Campaign settings › Data", "/campaigns/c1/data#activity"},
		"Trash":                {"Campaign settings › Data", "/campaigns/c1/trash"},
		"Unarchive":            {"Campaign settings › Data › Danger zone", "/campaigns/c1/data#danger"},
		"Delete":               {"Campaign settings › Data › Danger zone", "/campaigns/c1/data#danger"},
	}
	for _, e := range got {
		if w, ok := want[e.Label]; ok {
			if e.Path != w.path || e.Href != w.href {
				t.Errorf("%q = %q → %q, want %q → %q", e.Label, e.Path, e.Href, w.path, w.href)
			}
			delete(want, e.Label)
		}
		if e.Label == "Archive" {
			t.Error("an archived campaign offers Unarchive, not Archive")
		}
	}
	for l := range want {
		t.Errorf("%q is not offered", l)
	}
}

// Typing "settings" finds this place first, ahead of the Customizer's category
// entries, which sort after the page's own: the board, then its three tabs of
// settings, each with the words of what it holds, the old page's among them.
func TestBuildDeskFind_SettingsLeadsWithThisPlace(t *testing.T) {
	got := buildDeskFind(deskFindInput{Base: "/campaigns/c1", Customize: []customizeFindEntry{
		{Label: "Characters settings", Path: "Categories › Characters", Href: "/campaigns/c1/customize/categories/1/settings"},
	}})
	labels := findLabels(got)
	if len(got) < 4 || strings.Join(labels[:4], ",") != "Campaign settings,Campaign,Connections,Data" || got[0].Href != "/campaigns/c1/dashboard" {
		t.Fatalf("first entries = %v", labels)
	}
	for i, words := range map[int][]string{
		0: {"settings", "overview"},
		1: {"settings", "details", "description"},
		2: {"settings", "keys", "ai"},
		3: {"settings", "details", "export", "import", "activity", "danger"},
	} {
		for _, w := range words {
			if !strings.Contains(" "+got[i].Words+" ", " "+w+" ") {
				t.Errorf("%s lacks the word %q", got[i].Label, w)
			}
		}
	}
	for _, w := range []string{"description", "about", "summary", "blurb"} {
		found := false
		for _, e := range got {
			found = found || (e.Label == "Description" && strings.Contains(e.Words, w))
		}
		if !found {
			t.Errorf("Description is not found by %q", w)
		}
	}
}
