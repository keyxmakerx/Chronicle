package layouts

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The owner's phrases reach the places people actually type for, only for the
// owner, each once, and each carries the words that also find it.
func TestNavCommandsJSON_OwnerPhrases(t *testing.T) {
	decode := func(c navStateCase) []navCommand {
		var out []navCommand
		if err := json.Unmarshal([]byte(NavCommandsJSON(c.ctx())), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out
	}
	byLabel := func(cmds []navCommand, label string) *navCommand {
		for i := range cmds {
			if cmds[i].Label == label {
				return &cmds[i]
			}
		}
		return nil
	}

	owner := decode(navStateCase{role: 3, path: "/campaigns/c1"})
	want := []struct{ label, href string }{
		{"Campaign settings", "/campaigns/c1/dashboard"},
		{"Preview as a player or guest", "/campaigns/c1/settings#line-preview"},
		{"Export a copy", "/campaigns/c1/data#export"},
		{"Import", "/campaigns/c1/data#import"},
		{"Activity log", "/campaigns/c1/data#activity"},
		{"Keys", "/campaigns/c1/connections#keys"},
		{"Invite someone", "/campaigns/c1/members"},
		{"Archive or delete this campaign", "/campaigns/c1/data#danger"},
		{"People", "/campaigns/c1/members"},
		{"Trash", "/campaigns/c1/trash"},
		{"Quest board", "/campaigns/c1/extensions#quest-board"},
		{"Notice boards", "/campaigns/c1/extensions#notice-boards"},
		{"Trackers", "/campaigns/c1/extensions#trackers"},
		{"Tests", "/campaigns/c1/extensions#tests"},
	}
	for _, w := range want {
		c := byLabel(owner, w.label)
		if c == nil || c.Href != w.href {
			t.Errorf("owner: %q = %+v, want href %s", w.label, c, w.href)
		}
	}
	for _, gone := range []string{"Overview", "Settings", "Details and data", "Manage"} {
		if c := byLabel(owner, gone); c != nil {
			t.Errorf("owner: plain %q entry should not be listed, got %+v", gone, c)
		}
	}
	seen := map[string]string{}
	for _, c := range owner {
		// Invite someone is a second name for the People page on purpose.
		if c.Label == "Invite someone" {
			continue
		}
		if prev, dup := seen[c.Href]; dup {
			t.Errorf("owner: %q and %q both point at %s", prev, c.Label, c.Href)
		}
		seen[c.Href] = c.Label
	}

	// What a person types finds something: the label or one of the words.
	for _, typed := range []string{"settings", "guest", "dashboard", "export", "delete", "invite", "quest board", "notice board", "tracker", "test", "history"} {
		found := false
		for _, c := range owner {
			if strings.Contains(strings.ToLower(c.Label), typed) || strings.Contains(" "+c.Words+" ", " "+typed+" ") {
				found = true
			}
		}
		if !found {
			t.Errorf("owner: typing %q finds nothing", typed)
		}
	}

	for _, who := range []struct {
		name string
		c    navStateCase
	}{
		{"scribe", navStateCase{role: 2, path: "/campaigns/c1"}},
		{"player", navStateCase{role: 1, path: "/campaigns/c1"}},
		{"owner viewing as player", navStateCase{role: 1, player: true, path: "/campaigns/c1"}},
		{"visitor", navStateCase{role: 0, path: "/campaigns/c1"}},
	} {
		got := decode(who.c)
		for _, w := range want[:9] {
			if c := byLabel(got, w.label); c != nil {
				t.Errorf("%s must not get %q", who.name, w.label)
			}
		}
		for _, c := range got {
			if c.Words != "" {
				t.Errorf("%s: unexpected words on %q", who.name, c.Label)
			}
		}
	}
}

// Campaign settings is a row of its own: it is not inside the categories
// scroller, it is the owner's alone, and it keeps the manage key and URL.
func TestCampaignNavManage_Placement(t *testing.T) {
	render := func(c navStateCase) string {
		var buf bytes.Buffer
		if err := campaignNavManage().Render(c.ctx(), &buf); err != nil {
			t.Fatalf("render: %v", err)
		}
		return buf.String()
	}
	owner := render(navStateCase{role: 3, path: "/campaigns/c1"})
	for _, want := range []string{`class="nav-manage"`, `data-nav-section="manage"`, `data-nav-key="manage"`, `href="/campaigns/c1/dashboard"`, "Campaign settings", "fa-sliders"} {
		if !strings.Contains(owner, want) {
			t.Errorf("owner row is missing %q in %s", want, owner)
		}
	}
	if strings.Contains(owner, ">Manage<") {
		t.Errorf("the old word is still drawn: %s", owner)
	}
	for name, c := range map[string]navStateCase{
		"scribe":  {role: 2, path: "/campaigns/c1"},
		"player":  {role: 1, path: "/campaigns/c1"},
		"preview": {role: 1, player: true, path: "/campaigns/c1"},
		"visitor": {role: 0, path: "/campaigns/c1"},
	} {
		if got := render(c); strings.TrimSpace(got) != "" {
			t.Errorf("%s gets a row: %s", name, got)
		}
	}
}

// "delete" and "history" say where those live today: deleted pages wait in the
// Trash, and the record of changes is the Activity log and What players
// changed. A page's own Delete and History are on the page, so they are not
// listed here.
func TestNavCommandsJSON_DeleteAndHistoryWords(t *testing.T) {
	var out []navCommand
	if err := json.Unmarshal([]byte(NavCommandsJSON(navStateCase{role: 3, path: "/campaigns/c1"}.ctx())), &out); err != nil {
		t.Fatal(err)
	}
	finds := func(typed string) map[string]bool {
		got := map[string]bool{}
		for _, c := range out {
			if strings.Contains(strings.ToLower(c.Label), typed) || strings.Contains(" "+c.Words+" ", " "+typed+" ") {
				got[c.Label] = true
			}
		}
		return got
	}
	for typed, labels := range map[string][]string{
		"delete":  {"Trash", "Archive or delete this campaign"},
		"history": {"Activity log", "What players changed"},
	} {
		got := finds(typed)
		for _, l := range labels {
			if !got[l] {
				t.Errorf("typing %q should find %q, found %v", typed, l, got)
			}
		}
	}
}
