package notes

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestSendToJournal(t *testing.T) {
	page := "11111111-2222-3333-4444-555555555555"
	jot := &Note{
		ID: "jot-1", CampaignID: "c1", UserID: "u-writer", EntityID: &page, Title: "Trap checklist",
		EntryHTML: strp("<p>Second gate, DC 15</p>"),
		Content: []Block{{Type: "checklist", Items: []ChecklistItem{
			{Text: "Portcullis <disarmed>", Checked: true}, {Text: "Cistern rim", Checked: false},
		}}},
		IsShared: true,
	}
	repo, store, _ := memRepo(jot)
	pub := &recordingPublisher{}
	svc := &noteService{repo: repo, events: pub}

	res, err := svc.SendToJournal(context.Background(), "c1", player("u-writer"), "jot-1", "Ashkeep <Ruins>")
	if err != nil {
		t.Fatalf("SendToJournal: %v", err)
	}
	n := res.Note
	if res.Existing || n.EntityID != nil || n.Title != "Trap checklist" || n.Visibility != VisibilityPrivate || n.UserID != "u-writer" {
		t.Fatalf("want a new private Journal note of the writer's, got %+v", n)
	}
	body := *store[n.ID].EntryHTML
	for _, want := range []string{
		`data-mention-id="` + page + `"`, "Ashkeep &lt;Ruins&gt;", "Second gate, DC 15",
		`data-type="taskList"`, `data-checked="true"`, "Portcullis &lt;disarmed&gt;", "Cistern rim",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the note body lacks %q:\n%s", want, body)
		}
	}
	if got := store["jot-1"].LinkedNoteID; got == nil || *got != n.ID {
		t.Errorf("the jot must remember the note it became, got %v", got)
	}
	if len(pub.events) != 2 || pub.events[0].Type != "created" || pub.events[1].Type != "updated" {
		t.Errorf("want created then updated events, got %+v", pub.events)
	}
	if !reflectAudience(pub.events[0].Audience, []string{"u-writer"}) {
		t.Errorf("the new private note may reach only its writer, got %+v", pub.events[0].Audience)
	}

	again, err := svc.SendToJournal(context.Background(), "c1", player("u-writer"), "jot-1", "")
	if err != nil || !again.Existing || again.Note.ID != n.ID {
		t.Fatalf("sending twice must return the same note: %+v %v", again, err)
	}

	// The note it became is gone: a new one is made.
	delete(store, n.ID)
	third, err := svc.SendToJournal(context.Background(), "c1", player("u-writer"), "jot-1", "")
	if err != nil || third.Existing || third.Note.ID == n.ID {
		t.Fatalf("a deleted link must be replaced: %+v %v", third, err)
	}
	if !strings.Contains(*store[third.Note.ID].EntryHTML, "its page") {
		t.Error("an unnamed page reads as its page")
	}
}

func reflectAudience(a Audience, users []string) bool {
	if a.Everyone || a.GMs || len(a.Users) != len(users) {
		return false
	}
	for i := range users {
		if a.Users[i] != users[i] {
			return false
		}
	}
	return true
}

func TestSendToJournal_Refuses(t *testing.T) {
	page := "11111111-2222-3333-4444-555555555555"
	repo, _, _ := memRepo(
		&Note{ID: "jot-party", CampaignID: "c1", UserID: "u-writer", EntityID: &page, IsShared: true},
		&Note{ID: "journal", CampaignID: "c1", UserID: "u-writer"},
		&Note{ID: "other-campaign", CampaignID: "c2", UserID: "u-writer", EntityID: &page},
	)
	svc := &noteService{repo: repo, events: NoopNoteEventPublisher{}}
	for _, tc := range []struct {
		name, id string
		v        permissions.Viewer
	}{
		{"someone else's jot, even shared with them", "jot-party", player("u-reader")},
		{"the GM on a player's jot", "jot-party", gm("u-gm")},
		{"a Journal note is not a jot", "journal", player("u-writer")},
		{"a jot of another campaign", "other-campaign", player("u-writer")},
	} {
		if _, err := svc.SendToJournal(context.Background(), "c1", tc.v, tc.id, ""); err == nil {
			t.Errorf("%s: must be refused", tc.name)
		}
	}
}

type stubNamer struct {
	names map[string]string
	asked [][]string
}

func (s *stubNamer) PageNames(_ context.Context, _ string, _ permissions.Viewer, ids []string) (map[string]string, error) {
	s.asked = append(s.asked, ids)
	out := map[string]string{}
	for _, id := range ids {
		if n, ok := s.names[id]; ok {
			out[id] = n
		}
	}
	return out, nil
}

type sendStub struct {
	*stubAccessSvc
	gotName string
}

func (s *sendStub) SendToJournal(_ context.Context, _ string, _ permissions.Viewer, jotID, pageName string) (*SendResult, error) {
	s.gotName = pageName
	return &SendResult{Jot: s.notes[jotID], Note: &Note{ID: "new"}}, nil
}

func TestSendToJournalRoute_OwnerOnly(t *testing.T) {
	page := "11111111-2222-3333-4444-555555555555"
	base := newAccessSvc()
	base.notes["jot"] = &Note{ID: "jot", CampaignID: "c1", UserID: "u-player", EntityID: &page, IsShared: true}
	svc := &sendStub{stubAccessSvc: base}
	h := NewHandler(svc)
	h.SetPageNamer(&stubNamer{names: map[string]string{page: "Ashkeep Ruins"}})

	c, _ := ctxFor(http.MethodPost, "/", "u-other", campaigns.RolePlayer, map[string]string{"noteId": "jot"}, nil)
	wantStatus(t, h.SendToJournal(c), http.StatusNotFound)
	c, _ = ctxFor(http.MethodPost, "/", "u-gm", campaigns.RoleOwner, map[string]string{"noteId": "jot"}, nil)
	wantStatus(t, h.SendToJournal(c), http.StatusNotFound)

	c, rec := ctxFor(http.MethodPost, "/", "u-player", campaigns.RolePlayer, map[string]string{"noteId": "jot"}, nil)
	if err := h.SendToJournal(c); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("the owner sends their jot: %v %d", err, rec.Code)
	}
	if svc.gotName != "Ashkeep Ruins" {
		t.Errorf("the page name comes from the server's lookup, got %q", svc.gotName)
	}
}

func TestPageNamesRoute(t *testing.T) {
	a, b := "11111111-2222-3333-4444-555555555555", "66666666-7777-8888-9999-000000000000"
	namer := &stubNamer{names: map[string]string{a: "Ashkeep Ruins"}}
	h := NewHandler(newAccessSvc())
	h.SetPageNamer(namer)
	c, rec := ctxFor(http.MethodGet, "/?ids="+a+","+b+",not%20an%20id,"+a, "u-player", campaigns.RolePlayer, nil, nil)
	if err := h.PageNames(c); err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[a] != "Ashkeep Ruins" {
		t.Errorf("only visible pages are named, got %v", got)
	}
	if len(namer.asked) != 1 || len(namer.asked[0]) != 2 {
		t.Errorf("ids are deduplicated and id-shaped only, asked %v", namer.asked)
	}
}
