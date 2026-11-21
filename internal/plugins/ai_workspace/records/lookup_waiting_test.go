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
	// seen records the olderDays the lookup asked for.
	seen *int
}

func (f fakeWaiting) Waiting(_ context.Context, _ string, _ Actor, older int) (WaitingList, error) {
	if f.seen != nil {
		*f.seen = older
	}
	return f.list, f.err
}

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
			"- \"Drop the moon\", turned down by the owner, nothing changed; that was their choice, not an error (2026-10-26)",
			"### Entries players wrote (1 handed in, 1 still being written)",
			"- \"Fairy\" (ancestry) by Sam, for Ash: handed in, waiting for a Director",
			"- \"Gnome\" (ancestry) by Lee: still being written; the Director asked: \"Wings cost 3\"",
			"### Players' stash moves and requests to buy (1)\n\n- Sam: move the rope",
		}},
		{name: "nothing waiting", chip: "Waiting for review · 0", wants: []string{
			"(0)\n\nNone. The owner reviews them on the campaign's Your AI page.",
			"(0 handed in, 0 still being written)\n\nNone.",
			"None answered in the last 7 days. For older ones ask again with `older: 7`",
		}, nots: []string{"Couldn't be read"}},
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
	if !strings.Contains((&Lookups{}).LookupDoc(), "`what: waiting`") {
		t.Error("LookupDoc lacks waiting")
	}
}

// `older` pages the answered requests in seven-day windows and reaches the
// source as a day count; a bad one is refused rather than read as 0.
func TestLookups_WaitingOlder(t *testing.T) {
	at := time.Date(2026, 10, 12, 18, 0, 0, 0, time.UTC)
	list := WaitingList{Decided: []WaitingAsk{{Summary: "Rename the inn", At: at, Outcome: "1 changed"}}}
	hundred := WaitingList{}
	for i := 0; i < WaitingDecidedMax; i++ {
		hundred.Decided = append(hundred.Decided, WaitingAsk{Summary: "x", At: at})
	}
	cases := []struct {
		name    string
		fields  map[string]any
		list    WaitingList
		want    int
		wants   []string
		wantErr string
	}{
		{name: "absent is the last week", list: list, want: 0, wants: []string{"in the last 7 days", "`older: 7` (days 7 to 14)"}},
		{name: "a number", fields: map[string]any{"older": 7}, list: list, want: 7, wants: []string{
			"answered, from 7 to 14 days ago", "Showing 1 answer from 7 to 14 days ago.", "`older: 14` (days 14 to 21)"}},
		{name: "text from a connector tool", fields: map[string]any{"older": "14"}, list: list, want: 14, wants: []string{"from 14 to 21 days ago"}},
		{name: "empty window", fields: map[string]any{"older": 21}, want: 21, wants: []string{"None answered from 21 to 28 days ago.", "`older: 28`"}},
		{name: "the cap is said", list: hundred, want: 0, wants: []string{"Showing the newest 100 answers in the last 7 days, the most one answer shows."}},
		{name: "negative", fields: map[string]any{"older": -1}, wantErr: "`older` is a number of days"},
		{name: "not a number", fields: map[string]any{"older": "last week"}, wantErr: "`older` is a number of days"},
		{name: "fraction", fields: map[string]any{"older": 1.5}, wantErr: "`older` is a number of days"},
		{name: "absurd", fields: map[string]any{"older": 100000}, wantErr: "`older` is a number of days"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := -1
			l := &Lookups{Waiting: fakeWaiting{list: tc.list, seen: &got}}
			ans := l.Answer(context.Background(), camp, owner, []Record{lookup("waiting", tc.fields)})[0]
			if tc.wantErr != "" {
				if !strings.Contains(ans.Error, tc.wantErr) || got != -1 {
					t.Fatalf("error = %q (source asked %d), want %q before asking", ans.Error, got, tc.wantErr)
				}
				return
			}
			if ans.Error != "" {
				t.Fatal(ans.Error)
			}
			if got != tc.want {
				t.Errorf("source asked for older = %d, want %d", got, tc.want)
			}
			for _, w := range tc.wants {
				if !strings.Contains(ans.Text, w) {
					t.Errorf("answer lacks %q:\n%s", w, ans.Text)
				}
			}
		})
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
