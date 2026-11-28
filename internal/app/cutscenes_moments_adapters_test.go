package app

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/cutscenes"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// An answer tells the Director's tabs to redraw and the other viewers who is
// answering, never the words; a moment let go carries only the lines that
// joined the story. Each goes to one kind of socket.
func TestCutsceneMomentAudiences(t *testing.T) {
	bus := &cutsceneBus{}
	p := &cutsceneLivePublisher{bus: bus}
	sh := cutscenes.Showing{ID: "sh-1", SceneID: "s-1", Viewers: []cutscenes.ShowingViewer{{UserID: "ana"}, {UserID: "bo"}}}
	p.Answered("camp-1", sh, cutscenes.Answer{UserID: "ana", Name: "Ana", Moment: 0, Text: "ANSWER-WORDS"})
	p.Go("camp-1", sh, 0, []cutscenes.ShownLine{{Line: "It rang.", From: "Ana"}})

	browser := []string{ws.BrowserSource}
	module := []string{foundry_vtt.ModuleSource}
	tests := []struct {
		typ      ws.MessageType
		allowed  []string
		strict   bool
		dm       bool
		sources  []string
		payloads []string
	}{
		{ws.MsgCutsceneMomentAnswer, nil, false, true, browser, []string{"showingId", "sceneId", "moment"}},
		{ws.MsgCutsceneMomentAnswering, []string{"bo"}, true, false, browser, []string{"showingId", "moment", "userId", "name"}},
		{ws.MsgCutsceneFoundryAnswering, nil, false, true, module, []string{"showingId", "moment", "userId", "name", "viewers"}},
		{ws.MsgCutsceneMomentGo, []string{"ana", "bo"}, true, false, browser, []string{"showingId", "moment", "lines"}},
		{ws.MsgCutsceneFoundryGo, nil, false, true, module, []string{"showingId", "moment", "lines", "viewers"}},
	}
	if len(bus.got) != len(tests) {
		t.Fatalf("published %d messages, want %d", len(bus.got), len(tests))
	}
	for i, tt := range tests {
		m := bus.got[i]
		t.Run(string(tt.typ), func(t *testing.T) {
			if m.Type != tt.typ || m.CampaignID != "camp-1" {
				t.Fatalf("message %d = %s", i, m.Type)
			}
			if !reflect.DeepEqual(m.AllowedUsers, tt.allowed) || m.StrictAudience != tt.strict || m.RequiresDM != tt.dm || !reflect.DeepEqual(m.Sources, tt.sources) {
				t.Errorf("audience allowed=%v strict=%v dm=%v sources=%v", m.AllowedUsers, m.StrictAudience, m.RequiresDM, m.Sources)
			}
			if strings.Contains(string(m.Payload), "ANSWER-WORDS") {
				t.Errorf("the answer's words went out: %s", m.Payload)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(m.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload) != len(tt.payloads) {
				t.Errorf("payload keys %v, want %v", payload, tt.payloads)
			}
			for _, k := range tt.payloads {
				if _, ok := payload[k]; !ok {
					t.Errorf("payload lacks %q", k)
				}
			}
		})
	}

	// The only viewer answering: nobody else to tell in Chronicle.
	solo := &cutsceneBus{}
	(&cutsceneLivePublisher{bus: solo}).Answered("camp-1", cutscenes.Showing{ID: "sh-2", Viewers: []cutscenes.ShowingViewer{{UserID: "ana"}}}, cutscenes.Answer{UserID: "ana"})
	for _, m := range solo.got {
		if m.Type == ws.MsgCutsceneMomentAnswering {
			t.Errorf("a lone viewer was told about their own answer")
		}
	}
}

// storyEntities is a page store: one page per id, with a revision that a
// concurrent save can bump once.
type storyEntities struct {
	entities.EntityService
	pages    map[string]*entities.Entity
	rev      int
	bumpOnce bool
	saved    []string
}

func (s *storyEntities) ListEntityNames(context.Context, string, int, string) ([]entities.EntityNameEntry, error) {
	return []entities.EntityNameEntry{{ID: "p1", Name: "The Drowned Bell"}, {ID: "p1", Name: "Bell", IsAlias: true}, {ID: "p2", Name: "Vell"}}, nil
}

func (s *storyEntities) EntryRev(context.Context, string) (int, error) { return s.rev, nil }

func (s *storyEntities) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	e, ok := s.pages[id]
	if !ok {
		return nil, apperror.NewNotFound("page not found")
	}
	return e, nil
}

func (s *storyEntities) SaveEntry(_ context.Context, id, entry, entryHTML string, base *int) (int, *entities.EntryConflict, error) {
	if s.bumpOnce {
		s.bumpOnce = false
		s.rev++
		return s.rev, &entities.EntryConflict{Rev: s.rev}, nil
	}
	if base == nil || *base != s.rev {
		return 0, nil, apperror.NewConflict("stale")
	}
	s.rev++
	e := s.pages[id]
	e.Entry, e.EntryHTML = &entry, &entryHTML
	s.saved = append(s.saved, entry)
	return s.rev, nil, nil
}

func TestCutsceneStoryAdapter(t *testing.T) {
	ctx := context.Background()
	page := func() *entities.Entity {
		return &entities.Entity{ID: "p1", CampaignID: "camp-1",
			Entry:     strp(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Old words."}]}]}`),
			EntryHTML: strp("<p>Old words.</p>")}
	}

	st := &storyEntities{pages: map[string]*entities.Entity{"p1": page()}}
	pages, err := cutsceneStoryAdapter{svc: st}.Pages(ctx, "camp-1")
	if err != nil || len(pages) != 2 || pages[0] != (cutscenes.PageRef{ID: "p1", Name: "The Drowned Bell"}) {
		t.Fatalf("pages %v %v", pages, err)
	}

	// Appended after the old words, against the page's revision, retried
	// once when someone saved in between.
	st = &storyEntities{pages: map[string]*entities.Entity{"p1": page()}, rev: 4, bumpOnce: true}
	if err := (cutsceneStoryAdapter{svc: st}).AddLine(ctx, "camp-1", "p1", "It rang <twice>.", "Ana"); err != nil {
		t.Fatal(err)
	}
	if len(st.saved) != 1 || !strings.Contains(st.saved[0], "Old words.") || !strings.Contains(st.saved[0], `It rang \u003ctwice\u003e.`) || !strings.Contains(st.saved[0], `"from Ana"`) {
		t.Errorf("saved %v", st.saved)
	}
	if h := *st.pages["p1"].EntryHTML; !strings.HasPrefix(h, "<p>Old words.</p>") || !strings.Contains(h, "It rang &lt;twice&gt;.") || !strings.Contains(h, "<em>from Ana</em>") {
		t.Errorf("html %s", h)
	}

	// Another campaign's page, or words that aren't a document, are refused
	// and left as they were.
	st = &storyEntities{pages: map[string]*entities.Entity{"p1": page()}}
	if err := (cutsceneStoryAdapter{svc: st}).AddLine(ctx, "camp-2", "p1", "x", "Ana"); apperror.SafeCode(err) != 404 || len(st.saved) != 0 {
		t.Errorf("another campaign's page: %v", err)
	}
	odd := page()
	odd.Entry = strp("not json")
	st = &storyEntities{pages: map[string]*entities.Entity{"p1": odd}}
	if err := (cutsceneStoryAdapter{svc: st}).AddLine(ctx, "camp-1", "p1", "x", "Ana"); apperror.SafeCode(err) != 409 || len(st.saved) != 0 {
		t.Errorf("unreadable words: %v, saved %v", err, st.saved)
	}
	// An empty page starts a document.
	empty := page()
	empty.Entry, empty.EntryHTML = nil, nil
	st = &storyEntities{pages: map[string]*entities.Entity{"p1": empty}}
	if err := (cutsceneStoryAdapter{svc: st}).AddLine(ctx, "camp-1", "p1", "x", "Ana"); err != nil || !strings.HasPrefix(st.saved[0], `{"content":[{`) || !strings.Contains(st.saved[0], `"type":"doc"`) {
		t.Errorf("empty page: %v %v", err, st.saved)
	}
}
