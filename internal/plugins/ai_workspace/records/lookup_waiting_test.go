package records

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeWaiting struct {
	list WaitingList
	err  error
}

func (f fakeWaiting) Waiting(context.Context, string, Actor) (WaitingList, error) { return f.list, f.err }

type fakeEntryStatus struct{}

func (fakeEntryStatus) Entries(context.Context, string, string, bool) (string, []SystemEntry, error) {
	return "Draw Steel", []SystemEntry{
		{Name: "Dwarf", Slug: "dwarf", Source: "package"},
		{Name: "Fairy", Slug: "fairy", Source: "campaign", Status: "sent"},
		{Name: "Gnome", Slug: "gnome", Source: "campaign", Status: "draft"},
	}, nil
}
func (fakeEntryStatus) Fields(context.Context, string, bool) (string, []SystemEntryField, error) {
	return "", nil, nil
}

func TestLookups_Waiting(t *testing.T) {
	at := time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC)
	full := WaitingList{
		Asks:    []WaitingAsk{{Summary: "Five characters for Coral Court", Counts: "5 to add", At: at}},
		Decided: []WaitingAsk{{Summary: "Rename the inn", At: at, Outcome: "1 changed"}, {Summary: "Drop the moon", At: at, TurnedDown: true}},
		Entries: []WaitingEntry{
			{Name: "Fairy", Field: "ancestry", Writer: "Sam", For: "Ash", HandedIn: true},
			{Name: "Gnome", Field: "ancestry", Writer: "Lee", Note: "Wings cost 3"},
		},
		Requests: []WaitingRequest{{Text: "Sam: move the rope from the party stash to Ash", At: at}},
	}
	cases := []struct {
		name  string
		list  WaitingList
		err   error
		chip  string
		wants []string
		nots  []string
		fail  string
	}{
		{name: "every queue", list: full, chip: "Waiting for review · 3", wants: []string{
			"Only a Director can say yes or no",
			"### Changes your AI asked for, waiting on the owner (1)\n\n- \"Five characters for Coral Court\" (5 to add), sent 2026-10-26",
			"- \"Rename the inn\", applied: 1 changed (2026-10-26)",
			"- \"Drop the moon\", turned down, nothing changed (2026-10-26)",
			"### Entries players wrote (1 handed in, 1 still being written)",
			"- \"Fairy\" (ancestry) by Sam, for Ash: handed in, waiting for a Director",
			"- \"Gnome\" (ancestry) by Lee: still being written; the Director asked: \"Wings cost 3\"",
			"### Players' stash moves and requests to buy (1)\n\n- Sam: move the rope",
		}},
		{name: "nothing waiting", chip: "Waiting for review · 0", wants: []string{
			"(0)\n\nNone. The owner reviews them on the campaign's Your AI page.",
			"(0 handed in, 0 still being written)\n\nNone.",
		}, nots: []string{"answered lately", "Couldn't be read"}},
		{name: "a queue unread", list: WaitingList{Missing: []string{"players' entries"}}, wants: []string{
			"Couldn't be read just now, so may also have items waiting: players' entries.",
		}},
		{name: "source fails", err: errors.New("boom"), fail: "try again"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := &Lookups{Waiting: fakeWaiting{list: tc.list, err: tc.err}}
			ans := l.Answer(context.Background(), camp, owner, []Record{lookup("waiting", nil)})[0]
			if tc.fail != "" {
				if !strings.Contains(ans.Error, tc.fail) {
					t.Fatalf("error = %q, want %q", ans.Error, tc.fail)
				}
				return
			}
			if ans.Error != "" {
				t.Fatal(ans.Error)
			}
			if tc.chip != "" && ans.Chip != tc.chip {
				t.Errorf("chip = %q, want %q", ans.Chip, tc.chip)
			}
			for _, w := range tc.wants {
				if !strings.Contains(ans.Text, w) {
					t.Errorf("answer lacks %q:\n%s", w, ans.Text)
				}
			}
			for _, n := range tc.nots {
				if strings.Contains(ans.Text, n) {
					t.Errorf("answer has %q:\n%s", n, ans.Text)
				}
			}
		})
	}

	// Safe privacy and an unwired server are refused, never answered empty.
	l := &Lookups{Waiting: fakeWaiting{list: full}}
	if got := l.Answer(context.Background(), camp, player, []Record{lookup("waiting", nil)})[0]; !strings.Contains(got.Error, "choose Permitted") || got.Text != "" {
		t.Errorf("a player got %+v", got)
	}
	if got := (&Lookups{}).Answer(context.Background(), camp, owner, []Record{lookup("pending", nil)})[0]; !strings.Contains(got.Error, "can't be looked up") {
		t.Errorf("unwired: %+v", got)
	}
	if !strings.Contains(LookupDoc(), "`what: waiting`") {
		t.Error("LookupDoc lacks waiting")
	}
}

func TestLookups_SystemEntriesMarkDrafts(t *testing.T) {
	l := &Lookups{Entries: fakeEntryStatus{}}
	ans := l.Answer(context.Background(), camp, owner, []Record{lookup("system-entries", map[string]any{"field": "ancestry"})})[0]
	if ans.Error != "" {
		t.Fatal(ans.Error)
	}
	for _, want := range []string{
		"- Dwarf (dwarf), package\n",
		"- Fairy (fairy), campaign, a player's entry handed in and not accepted yet",
		"- Gnome (gnome), campaign, a player's draft, not accepted yet",
	} {
		if !strings.Contains(ans.Text, want) {
			t.Errorf("answer lacks %q:\n%s", want, ans.Text)
		}
	}
}
