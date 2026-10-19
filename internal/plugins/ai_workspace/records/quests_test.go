package records

import (
	"context"
	"strings"
	"testing"
)

// fakeQuests records what the kinds ask the quests plugin to do.
type fakeQuests struct {
	quests  []QuestRef
	boards  []BoardRef
	written map[string]QuestInput
	updated []BoardInput
	added   []BoardItemRef
	removed []string
	deleted []string
	created []string
}

func (f *fakeQuests) Quests(context.Context, string, Actor) ([]QuestRef, error) { return f.quests, nil }
func (f *fakeQuests) WriteQuest(_ context.Context, _, pageID string, _ Actor, in QuestInput) error {
	if f.written == nil {
		f.written = map[string]QuestInput{}
	}
	f.written[pageID] = in
	return nil
}
func (f *fakeQuests) Categories(context.Context, string) ([]FolderCategory, error) {
	return []FolderCategory{{ID: 7, Name: "Quest", Plural: "Quests", Slug: "quest"}}, nil
}
func (f *fakeQuests) Boards(context.Context, string, Actor) ([]BoardRef, error) { return f.boards, nil }
func (f *fakeQuests) CreateBoard(_ context.Context, _ string, _ BoardHome, _ Actor, name, who string) (string, error) {
	f.created = append(f.created, name+"/"+who)
	return "new-board", nil
}
func (f *fakeQuests) UpdateBoard(_ context.Context, _ string, _ BoardHome, _ string, _ Actor, in BoardInput) error {
	f.updated = append(f.updated, in)
	return nil
}
func (f *fakeQuests) DeleteBoard(_ context.Context, _ string, _ BoardHome, id string, _ Actor) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeQuests) AddItem(_ context.Context, _ string, _ BoardHome, _ string, _ Actor, it BoardItemRef) error {
	f.added = append(f.added, it)
	return nil
}
func (f *fakeQuests) RemoveItem(_ context.Context, _ string, _ BoardHome, _, id string, _ Actor) error {
	f.removed = append(f.removed, id)
	return nil
}

func questPages() fakeEntities {
	return fakeEntities{
		"the-missing-goat": {ID: "goat", Name: "The Missing Goat", CampaignID: camp},
		"wolves":           {ID: "wolves", Name: "Wolves", CampaignID: camp},
		"the-rusty-anchor": {ID: "anchor", Name: "The Rusty Anchor", CampaignID: camp},
		"elsewhere":        {ID: "x", Name: "Elsewhere", CampaignID: "other"},
	}
}

func TestQuestKindPlan(t *testing.T) {
	saved := []QuestRef{{PageID: "goat", PageName: "The Missing Goat", Status: "not_started"}}
	tests := []struct {
		name    string
		a       Actor
		r       Record
		wantErr string
		want    string
	}{
		{"player refused", player, Record{Action: ActionUpdate, Name: "The Missing Goat"}, "only the owner", ""},
		{"delete refused", owner, Record{Action: ActionDelete, Name: "The Missing Goat"}, "part of its page", ""},
		{"no page", owner, Record{Action: ActionCreate, Name: "Nowhere"}, "no page called", ""},
		{"another campaign's page", owner, Record{Action: ActionCreate, Name: "Elsewhere"}, "no page called", ""},
		{"page made in the paste", owner, Record{Action: ActionCreate, Name: "Dragons", Made: map[string]bool{"dragons": true}}, "", "a new quest"},
		{"create over a saved sheet", owner, Record{Action: ActionCreate, Name: "The Missing Goat"}, "already has a quest", ""},
		{"update with no sheet", owner, Record{Action: ActionUpdate, Name: "Wolves", Fields: map[string]any{"status": "taken"}}, "no quest on", ""},
		{"bad status", owner, Record{Action: ActionUpdate, Name: "The Missing Goat", Fields: map[string]any{"status": "maybe"}}, "is not open", ""},
		{"status word", owner, Record{Action: ActionUpdate, Name: "The Missing Goat", Fields: map[string]any{"status": "Taken"}}, "", "status taken"},
		{"nothing to change", owner, Record{Action: ActionUpdate, Name: "The Missing Goat"}, "nothing to change", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := QuestKind{Svc: &fakeQuests{quests: saved}, Entities: questPages()}
			p := k.Plan(context.Background(), camp, tt.a, tt.r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("error %q, want it to contain %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tt.want) {
				t.Fatalf("plan %+v, want summary containing %q", p, tt.want)
			}
		})
	}
}

func TestQuestKindApplyReadsStepsAndBody(t *testing.T) {
	f := &fakeQuests{}
	k := QuestKind{Svc: f, Entities: questPages()}
	r := Record{Action: ActionCreate, Name: "Wolves", Fields: map[string]any{
		"status": "open", "reward": "20 gp",
		"steps": []any{"Ask the farmer (shown)", "[x] Find tracks", map[string]any{"text": "Kill the alpha", "shown": true}},
	}, Body: "Wolves at night.\nThey howl.\n\nPay on proof."}
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	in := f.written["wolves"]
	if in.Status == nil || *in.Status != "not_started" || in.Reward == nil || *in.Reward != "20 gp" || in.Title != nil {
		t.Fatalf("written %+v", in)
	}
	want := []QuestStep{{Text: "Ask the farmer", Shown: true}, {Text: "Find tracks", Done: true}, {Text: "Kill the alpha", Shown: true}}
	if in.Steps == nil || len(*in.Steps) != 3 {
		t.Fatalf("steps %+v", in.Steps)
	}
	for i, s := range *in.Steps {
		if s != want[i] {
			t.Errorf("step %d = %+v, want %+v", i, s, want[i])
		}
	}
	if in.Body == nil || strings.Join(*in.Body, "|") != "Wolves at night. They howl.|Pay on proof." {
		t.Fatalf("body %+v", in.Body)
	}
}

func TestQuestKindExportReadsBack(t *testing.T) {
	f := &fakeQuests{quests: []QuestRef{{PageID: "goat", PageName: "The Missing Goat", Status: "active", Title: "The Missing Goat",
		Kicker: "Help wanted", Reward: "20 gp: and cheese", Steps: []QuestStep{{Text: "Ask", Done: true, Shown: true}}, Body: []string{"Gone three nights."}}}}
	out, err := QuestKind{Svc: f, Entities: questPages()}.Export(context.Background(), camp, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kind: quest\n", "name: The Missing Goat\n", "status: taken\n", `reward: "20 gp: and cheese"`, `- "[x] Ask (shown)"`, "Gone three nights."} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "title:") {
		t.Error("a title equal to the page name should be left out")
	}
	if s, _ := (QuestKind{Svc: f}).Export(context.Background(), camp, player); s != "" {
		t.Error("a player must not get the quest export")
	}
}

func boardFixture() *fakeQuests {
	return &fakeQuests{boards: []BoardRef{{
		ID: "b1", Name: "Back wall", Who: "dm", Home: BoardHome{PageID: "anchor"}, HomeName: "The Rusty Anchor",
		Items: []BoardItemRef{
			{ID: "n-goat", Kind: "notice", QuestID: "goat", QuestName: "The Missing Goat", ByDM: true},
			{ID: "n-auto", Kind: "notice", QuestID: "wolves", QuestName: "Wolves", ByDM: true, Auto: true},
			{ID: "note-dm", Kind: "note", Text: "Ask Mara", ByDM: true},
			{ID: "note-pl", Kind: "note", Text: "the goat knows"},
		},
	}}}
}

func TestNoticeBoardKind(t *testing.T) {
	tests := []struct {
		name    string
		r       Record
		wantErr string
		check   func(t *testing.T, f *fakeQuests)
	}{
		{name: "needs a home", r: Record{Action: ActionCreate, Name: "B"}, wantErr: "say where"},
		{name: "not both homes", r: Record{Action: ActionCreate, Name: "B", Fields: map[string]any{"page": "The Rusty Anchor", "category": "Quests"}}, wantErr: "not both"},
		{name: "bad fill word", r: Record{Action: ActionCreate, Name: "B", Fields: map[string]any{"category": "Quests", "fills_with": []any{"open", "soon"}}}, wantErr: "is not open"},
		{name: "duplicate name", r: Record{Action: ActionCreate, Name: "back wall", Fields: map[string]any{"page": "The Rusty Anchor"}}, wantErr: "already exists"},
		{
			name: "create on a category, filling, with a note",
			r:    Record{Action: ActionCreate, Name: "Open work", Fields: map[string]any{"category": "quests", "who": "everyone", "fills_with": "open, taken", "notes": []any{"Read me"}}},
			check: func(t *testing.T, f *fakeQuests) {
				if len(f.created) != 1 || f.created[0] != "Open work/all" {
					t.Fatalf("created %v", f.created)
				}
				if len(f.updated) != 1 || f.updated[0].Fill == nil || strings.Join(*f.updated[0].Fill, ",") != "not_started,active" || f.updated[0].Who != nil {
					t.Fatalf("updated %+v", f.updated)
				}
				if len(f.added) != 1 || f.added[0].Kind != "note" || f.added[0].Text != "Read me" {
					t.Fatalf("added %+v", f.added)
				}
			},
		},
		{
			name: "update replaces only the DM's hand pins and notes",
			r:    Record{Action: ActionUpdate, Name: "Back wall", Fields: map[string]any{"page": "The Rusty Anchor", "quests": []any{"Wolves"}, "notes": []any{"Bring rope"}}},
			check: func(t *testing.T, f *fakeQuests) {
				if strings.Join(f.removed, ",") != "n-goat,note-dm" {
					t.Fatalf("removed %v; want the hand-pinned goat and the DM's note only", f.removed)
				}
				if len(f.added) != 1 || f.added[0].Text != "Bring rope" {
					t.Fatalf("added %+v; Wolves is already shown by the board itself", f.added)
				}
			},
		},
		{
			name: "stop filling",
			r:    Record{Action: ActionUpdate, Name: "Back wall", Fields: map[string]any{"page": "The Rusty Anchor", "fills_with": "nothing"}},
			check: func(t *testing.T, f *fakeQuests) {
				if len(f.updated) != 1 || f.updated[0].Fill == nil || len(*f.updated[0].Fill) != 0 {
					t.Fatalf("updated %+v", f.updated)
				}
			},
		},
		{
			name: "delete",
			r:    Record{Action: ActionDelete, Name: "Back wall", Fields: map[string]any{"page": "The Rusty Anchor"}},
			check: func(t *testing.T, f *fakeQuests) {
				if strings.Join(f.deleted, ",") != "b1" {
					t.Fatalf("deleted %v", f.deleted)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := boardFixture()
			k := NoticeBoardKind{Svc: f, Entities: questPages()}
			err := k.Apply(context.Background(), camp, owner, tt.r)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got %v, want an error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, f)
		})
	}
}

func TestNoticeBoardKindExport(t *testing.T) {
	out, err := NoticeBoardKind{Svc: boardFixture(), Entities: questPages()}.Export(context.Background(), camp, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"page: The Rusty Anchor\n", "name: Back wall\n", "who: dm\n", "quests:\n  - The Missing Goat\n", "notes:\n  - Ask Mara\n", "players pinned 1 things"} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Wolves") || strings.Contains(out, "the goat knows") {
		t.Errorf("auto notices and players' notes are not the DM's to write back:\n%s", out)
	}
}
