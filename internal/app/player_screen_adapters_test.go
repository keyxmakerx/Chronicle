package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// plsEntities answers ListClaimed with a fixed party and records who asked,
// so a test can see the adapter asks as the viewer and nobody else.
type plsEntities struct {
	entities.EntityService
	party     []entities.Entity
	types     map[int]*entities.EntityType
	askedRole int
	askedUser string
}

func (f *plsEntities) ListClaimed(_ context.Context, _ string, role int, userID string) ([]entities.Entity, error) {
	f.askedRole, f.askedUser = role, userID
	return f.party, nil
}

func (f *plsEntities) GetEntityTypeByID(_ context.Context, id int) (*entities.EntityType, error) {
	if t, ok := f.types[id]; ok {
		return t, nil
	}
	return nil, apperror.NewNotFound("no such category")
}

func plsOwner(id string) *string { return &id }

func TestPlayerHeroes(t *testing.T) {
	ctx := context.Background()
	newEnts := func() *plsEntities {
		return &plsEntities{
			party: []entities.Entity{
				{ID: "h2", Name: "Tyne", EntityTypeID: 1, OwnerUserID: plsOwner("u2"), FieldsData: map[string]any{"class": "Fury"},
					FieldOverrides: &entities.FieldOverrides{Added: []entities.FieldDefinition{{Key: "vow", OwnerOnly: true}}}},
				{ID: "h1", Name: "Orrin", EntityTypeID: 1, OwnerUserID: plsOwner("u1")},
				{ID: "h3", Name: "Ash", EntityTypeID: 9, OwnerUserID: plsOwner("u3")},
				{ID: "hx", Name: "Elsewhere", CampaignID: "c2", EntityTypeID: 1, OwnerUserID: plsOwner("u4")},
			},
			types: map[int]*entities.EntityType{1: {ID: 1, Fields: []entities.FieldDefinition{
				{Key: "class"}, {Key: "secret", GMOnly: true}, {Key: "backstory", OwnerOnly: true},
			}}},
		}
	}

	t.Run("the DM team is refused", func(t *testing.T) {
		ents := newEnts()
		a := &dmPartyAdapter{entities: ents}
		if _, err := a.PlayerHeroes(ctx, "c1", dmscreen.Viewer{UserID: "own", Role: 3}); err == nil {
			t.Fatal("want a refusal for role 3")
		}
		if ents.askedUser != "" {
			t.Error("the party was listed for the DM team")
		}
	})

	t.Run("asks as the viewer and carries each hero's restricted keys", func(t *testing.T) {
		ents := newEnts()
		a := &dmPartyAdapter{entities: ents}
		got, err := a.PlayerHeroes(ctx, "c1", dmscreen.Viewer{UserID: "u1", Role: 1})
		if err != nil {
			t.Fatal(err)
		}
		if ents.askedRole != 1 || ents.askedUser != "u1" {
			t.Errorf("ListClaimed asked as role %d user %q, want the viewer", ents.askedRole, ents.askedUser)
		}
		byID := map[string]dmscreen.Hero{}
		for _, h := range got {
			byID[h.ID] = h
		}
		if _, ok := byID["hx"]; ok {
			t.Error("a hero from another campaign was listed")
		}
		if len(got) != 3 || got[0].Name != "Ash" || got[2].Name != "Tyne" {
			t.Errorf("heroes = %+v, want Ash, Orrin, Tyne", got)
		}
		tyne := byID["h2"]
		if !tyne.RestrictionsKnown || len(tyne.GMOnly) != 1 || tyne.GMOnly[0] != "secret" {
			t.Errorf("Tyne GMOnly = %v known %v", tyne.GMOnly, tyne.RestrictionsKnown)
		}
		if len(tyne.OwnerOnly) != 2 || tyne.OwnerOnly[0] != "backstory" || tyne.OwnerOnly[1] != "vow" {
			t.Errorf("Tyne OwnerOnly = %v, want the category's and the page's own", tyne.OwnerOnly)
		}
		if tyne.PlayerUserID != "u2" {
			t.Errorf("Tyne's player = %q", tyne.PlayerUserID)
		}
		// Ash's category cannot be read, so the screen must withhold all of
		// Ash's values rather than guess which are safe.
		if byID["h3"].RestrictionsKnown {
			t.Error("a hero whose category is unreadable claims known restrictions")
		}
	})
}

func TestPlayerNotesAdapter(t *testing.T) {
	ctx := context.Background()
	p1 := dmscreen.Viewer{UserID: "p1", Role: 1}

	t.Run("the first save creates a private note of the viewer's at their own id", func(t *testing.T) {
		f := &fakeNoteStore{byID: map[string]*notes.Note{}}
		a := &playerNotesAdapter{notes: f}
		got, err := a.SaveMine(ctx, "c1", p1, "My notes", `{"v":1}`, "<p>1</p>", "")
		if err != nil {
			t.Fatal(err)
		}
		n := f.byID[playerNoteID("c1", "p1")]
		if n == nil || n.UserID != "p1" || n.SharedWithGM || n.Title != "My notes" {
			t.Fatalf("stored note = %+v", n)
		}
		if got.Entry != `{"v":1}` || got.Shared {
			t.Errorf("returned %+v", got)
		}
		again, err := a.SaveMine(ctx, "c1", p1, "ignored", `{"v":2}`, "", dmscreen.BodyVersion(got.Entry))
		if err != nil || again.Entry != `{"v":2}` || f.creates != 1 || f.byID[playerNoteID("c1", "p1")].Title != "My notes" {
			t.Fatalf("second save: %v %+v creates %d", err, again, f.creates)
		}
	})

	t.Run("each member's id is their own, per campaign", func(t *testing.T) {
		if playerNoteID("c1", "p1") == playerNoteID("c1", "p2") || playerNoteID("c1", "p1") == playerNoteID("c2", "p1") {
			t.Error("two members or two campaigns share a note id")
		}
		if playerNoteID("c1", "s1") == dmNoteID("c1", "s1") {
			t.Error("a player's note id collides with a DM note id")
		}
	})

	t.Run("a stale version is refused", func(t *testing.T) {
		body := `{"v":"newer"}`
		id := playerNoteID("c1", "p1")
		f := &fakeNoteStore{byID: map[string]*notes.Note{id: {ID: id, CampaignID: "c1", UserID: "p1", Entry: &body}}}
		a := &playerNotesAdapter{notes: f}
		if _, err := a.SaveMine(ctx, "c1", p1, "t", `{"v":1}`, "", dmscreen.BodyVersion(`{"v":"older"}`)); !errors.Is(err, dmscreen.ErrNoteChanged) {
			t.Fatalf("err = %v, want ErrNoteChanged", err)
		}
		if _, err := a.SaveMine(ctx, "c1", p1, "t", `{"v":1}`, "", ""); !errors.Is(err, dmscreen.ErrNoteChanged) {
			t.Fatalf("a tab that saw no note overwrote one: %v", err)
		}
		if *f.byID[id].Entry != body {
			t.Error("the newer body was overwritten")
		}
	})

	t.Run("a note at the id that the viewer does not own is hidden and never written", func(t *testing.T) {
		id := playerNoteID("c1", "p1")
		body := `{"v":"theirs"}`
		f := &fakeNoteStore{byID: map[string]*notes.Note{id: {ID: id, CampaignID: "c1", UserID: "own", Entry: &body}}}
		a := &playerNotesAdapter{notes: f}
		if got, err := a.FindMine(ctx, "c1", p1); err != nil || got != nil {
			t.Fatalf("FindMine = %+v, %v; want absent", got, err)
		}
		if _, err := a.SaveMine(ctx, "c1", p1, "t", `{"v":1}`, "", ""); err == nil {
			t.Fatal("want a refusal")
		}
		if *f.byID[id].Entry != body || f.creates != 0 {
			t.Error("the other note was touched")
		}
	})

	t.Run("the DM reads only their own place, never a player's note", func(t *testing.T) {
		body := `{"v":"p1 private"}`
		id := playerNoteID("c1", "p1")
		f := &fakeNoteStore{byID: map[string]*notes.Note{id: {ID: id, CampaignID: "c1", UserID: "p1", Entry: &body}}}
		a := &playerNotesAdapter{notes: f}
		got, err := a.FindMine(ctx, "c1", dmscreen.Viewer{UserID: "own", Role: 1})
		if err != nil || got != nil {
			t.Fatalf("owner viewing as a player got %+v, %v; want their own (absent) note", got, err)
		}
		if _, err := a.FindMine(ctx, "c1", dmscreen.Viewer{}); err == nil {
			t.Error("a viewer with no user id was answered")
		}
	})

	t.Run("a shared note says so", func(t *testing.T) {
		id := playerNoteID("c1", "p1")
		f := &fakeNoteStore{byID: map[string]*notes.Note{id: {ID: id, CampaignID: "c1", UserID: "p1", SharedWithGM: true}}}
		got, err := (&playerNotesAdapter{notes: f}).FindMine(ctx, "c1", p1)
		if err != nil || got == nil || !got.Shared {
			t.Fatalf("got %+v, %v; want Shared", got, err)
		}
	})
}
