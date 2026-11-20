package systems

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// beforeWords renders the gathered page as "run: Name[Kind]!" rows, with
// "!" on a row only the Director sees and "~" on a row with a change line.
func beforeWords(v *BeforeView) string {
	var runs []string
	for _, run := range v.Runs {
		var rows []string
		for _, r := range run.Rows {
			w := r.Name + "[" + r.Kind + "]"
			if r.Line != "" && r.Quoted {
				w += "“" + r.Line + "”"
			}
			if r.DirectorsOnly {
				w += "!"
			}
			if r.Change != nil {
				w += "~"
			}
			rows = append(rows, w)
		}
		runs = append(runs, run.ID+": "+strings.Join(rows, ","))
	}
	return strings.Join(runs, " | ")
}

func TestBeforePage_PerReader(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		setup  func(t *testing.T, svc *systemEntryService, repo *fakeEntryRepo)
		reader EntryActor
		want   string
	}{
		{"untouched campaign, a player", nil, playerActor,
			"only: Ashborn[Ancestry],Dwarf[Ancestry]"},
		{"untouched campaign, the Director", nil, dirActor,
			"only: Ashborn[Ancestry],Dwarf[Ancestry],Fairy[Ancestry],Tideling[Ancestry]!"},
		{"Wren's player sees the entry kept to Wren", nil, tyneActor,
			"only: Ashborn[Ancestry],Dwarf[Ancestry],Fairy[Ancestry]"},
		{"a Directors-only stand-in is the Director's row only", func(t *testing.T, svc *systemEntryService, _ *fakeEntryRepo) {
			if _, err := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "elf"}); err != nil {
				t.Fatal(err)
			}
		}, playerActor, "only: Ashborn[Ancestry],Dwarf[Ancestry]"},
		{"the same stand-in for the Director", func(t *testing.T, svc *systemEntryService, _ *fakeEntryRepo) {
			if _, err := svc.Rewrite(ctx, "c1", dirActor, RewriteInput{FieldKey: "ancestry", Slug: "elf"}); err != nil {
				t.Fatal(err)
			}
		}, dirActor, "only: Ashborn[Ancestry],Dwarf[Ancestry],Fairy[Ancestry],Tideling[Ancestry]! | changed: Elf[Ancestry]!"},
		{"a stand-in in play", func(t *testing.T, svc *systemEntryService, repo *fakeEntryRepo) {
			d := entryByName(t, repo, "Dwarf")
			if _, err := svc.Tie(ctx, "c1", dirActor, d.ID, "dwarf"); err != nil {
				t.Fatal(err)
			}
		}, playerActor, "only: Ashborn[Ancestry] | changed: Dwarf[Ancestry]"},
		{"left out with and without a sentence, a player", leaveTwoOut, playerActor,
			"only: Ashborn[Ancestry],Dwarf[Ancestry] | not-in: Polder[Ancestry]“The halflings drowned.”"},
		{"left out with and without a sentence, the Director", leaveTwoOut, dirActor,
			"only: Ashborn[Ancestry],Dwarf[Ancestry],Fairy[Ancestry],Tideling[Ancestry]! | not-in: Devil[Ancestry]!,Polder[Ancestry]“The halflings drowned.”"},
		{"a player's draft is not drawn", func(t *testing.T, _ *systemEntryService, repo *fakeEntryRepo) {
			for i := range repo.rows {
				if repo.rows[i].Name == "Ashborn" {
					repo.rows[i].Status = EntryStatusSent
					repo.rows[i].CreatedBy = "pl"
				}
			}
		}, dirActor, "only: Dwarf[Ancestry],Fairy[Ancestry],Tideling[Ancestry]!"},
		{"the package changed a rewritten entry", func(t *testing.T, svc *systemEntryService, repo *fakeEntryRepo) {
			d := entryByName(t, repo, "Dwarf")
			if _, err := svc.Tie(ctx, "c1", dirActor, d.ID, "dwarf"); err != nil {
				t.Fatal(err)
			}
			staleSnapshot(t, repo, d.ID)
		}, dirActor, "only: Ashborn[Ancestry],Fairy[Ancestry],Tideling[Ancestry]! | changed: Dwarf[Ancestry]~"},
		{"the change line is never a player's", func(t *testing.T, svc *systemEntryService, repo *fakeEntryRepo) {
			d := entryByName(t, repo, "Dwarf")
			if _, err := svc.Tie(ctx, "c1", dirActor, d.ID, "dwarf"); err != nil {
				t.Fatal(err)
			}
			staleSnapshot(t, repo, d.ID)
		}, playerActor, "only: Ashborn[Ancestry] | changed: Dwarf[Ancestry]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, svc, repo, sys := newRulesPages(t)
			if tt.setup != nil {
				tt.setup(t, svc, repo)
			}
			v, err := p.Before(ctx, sys, reader(tt.reader))
			if err != nil {
				t.Fatal(err)
			}
			if got := beforeWords(v); got != tt.want {
				t.Fatalf("page = %s\nwant   %s", got, tt.want)
			}
			if !tt.reader.IsDirector && (len(v.NewKinds) > 0 || v.DraftsURL != "" || len(v.OldNames) > 0) {
				t.Fatalf("a player got the Director's words: %+v", v)
			}
		})
	}
}

func leaveTwoOut(t *testing.T, svc *systemEntryService, _ *fakeEntryRepo) {
	t.Helper()
	ctx := context.Background()
	if _, err := svc.LeaveOut(ctx, "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "polder", Sentence: "The halflings drowned."}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LeaveOut(ctx, "c1", dirActor, LeftOutInput{FieldKey: "ancestry", Slug: "devil"}); err != nil {
		t.Fatal(err)
	}
}

// staleSnapshot makes the stored snapshot an older Dwarf, as if the package
// had since reworded Grounded.
func staleSnapshot(t *testing.T, repo *fakeEntryRepo, id int64) {
	t.Helper()
	old, _ := json.Marshal(StandardSnapshot{Name: "Dwarf", Summary: "Stone folk.", Description: "Dense and sturdy.",
		Properties: map[string]any{"signature_traits": []any{map[string]any{"name": "Grounded", "description": "Very hard to move."}}}})
	for i := range repo.rows {
		if repo.rows[i].ID == id {
			repo.rows[i].StandsForSeen = string(old)
		}
	}
}

func TestBeforePage_DirectorWords(t *testing.T) {
	p, svc, repo, sys := newRulesPages(t)
	ctx := context.Background()
	d := entryByName(t, repo, "Dwarf")
	if _, err := svc.Tie(ctx, "c1", dirActor, d.ID, "dwarf"); err != nil {
		t.Fatal(err)
	}
	staleSnapshot(t, repo, d.ID)
	if _, err := svc.Update(ctx, "c1", dirActor, d.ID, UpdateSystemEntryInput{Name: patch.Of("Sea Dwarf")}); err != nil {
		t.Fatal(err)
	}
	v, err := p.Before(ctx, sys, reader(dirActor))
	if err != nil {
		t.Fatal(err)
	}
	var ch *changeLine
	for _, run := range v.Runs {
		for _, r := range run.Rows {
			if r.Change != nil {
				ch = r.Change
			}
		}
	}
	if ch == nil || !strings.HasPrefix(ch.Text, "Draw Steel changed its Dwarf since you wrote yours: ") ||
		ch.CompareURL != "/campaigns/c1/systems/sys/ancestries/dwarf?printed=1" ||
		ch.AsideLabel != "Set Therin’s Sea Dwarf aside" || !strings.HasSuffix(ch.AsideURL, "/ancestries/dwarf#set-aside") {
		t.Fatalf("change line = %+v", ch)
	}
	if len(v.OldNames) != 1 || v.OldNames[0].Name != "Dwarf" || v.OldNames[0].Leads != "Sea Dwarf" {
		t.Fatalf("old names = %+v", v.OldNames)
	}
	if len(v.NewKinds) != 2 || v.NewKinds[0].Label != "Ancestry" || !strings.Contains(v.NewKinds[0].URL, "new=1") {
		t.Fatalf("new kinds = %+v", v.NewKinds)
	}
	// Seen clears the line.
	if _, err := svc.MarkSeen(ctx, "c1", dirActor, d.ID); err != nil {
		t.Fatal(err)
	}
	v, _ = p.Before(ctx, sys, reader(dirActor))
	if strings.Contains(beforeWords(v), "~") {
		t.Fatalf("still a change line after Seen: %s", beforeWords(v))
	}
}

func TestBeforePage_Empty(t *testing.T) {
	p, svc, repo, sys := newRulesPages(t)
	ctx := context.Background()
	for _, r := range append([]SystemEntry(nil), repo.rows...) {
		if err := svc.Delete(ctx, "c1", dirActor, r.ID); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range []EntryActor{playerActor, dirActor} {
		v, err := p.Before(ctx, sys, reader(a))
		if err != nil || !v.Empty() {
			t.Fatalf("empty page for %+v = %s %v", a, beforeWords(v), err)
		}
	}
}
