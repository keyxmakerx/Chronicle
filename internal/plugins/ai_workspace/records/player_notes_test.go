package records

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/entity_notes"
)

// fakePlayerNotes holds one page's Player Notes and records what was asked.
type fakePlayerNotes struct {
	notes   []entity_notes.Note
	created []entity_notes.CreateNoteRequest
	updated map[string]entity_notes.UpdateNoteRequest
	deleted []string
	viewer  entity_notes.ViewerContext
	// swap makes Get report a different writer, as if the note changed hands.
	swap bool
}

func (f *fakePlayerNotes) List(_ context.Context, entityID string, v entity_notes.ViewerContext) ([]entity_notes.Note, error) {
	f.viewer = v
	var out []entity_notes.Note
	for _, n := range f.notes {
		if n.EntityID == entityID {
			out = append(out, n)
		}
	}
	return out, nil
}
func (f *fakePlayerNotes) Get(_ context.Context, id string, _ entity_notes.ViewerContext) (*entity_notes.Note, error) {
	for _, n := range f.notes {
		if n.ID == id {
			if f.swap {
				n.AuthorUserID = "intruder"
			}
			return &n, nil
		}
	}
	return nil, errors.New("not found")
}
func (f *fakePlayerNotes) Create(_ context.Context, entityID string, v entity_notes.ViewerContext, req entity_notes.CreateNoteRequest) (*entity_notes.Note, error) {
	f.created = append(f.created, req)
	return &entity_notes.Note{ID: "new", EntityID: entityID, AuthorUserID: v.UserID}, nil
}
func (f *fakePlayerNotes) Update(_ context.Context, id string, _ entity_notes.ViewerContext, req entity_notes.UpdateNoteRequest) (*entity_notes.Note, error) {
	if f.updated == nil {
		f.updated = map[string]entity_notes.UpdateNoteRequest{}
	}
	f.updated[id] = req
	return nil, nil
}
func (f *fakePlayerNotes) Delete(_ context.Context, id string, _ entity_notes.ViewerContext) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func TestPlayerNoteKind(t *testing.T) {
	inn := &entities.Entity{ID: "inn", CampaignID: camp, Name: "The Rusty Anchor"}
	pages := fakeEntities{"the-rusty-anchor": inn}
	notesOnInn := []entity_notes.Note{
		{ID: "mine", EntityID: "inn", CampaignID: camp, AuthorUserID: "owner", Title: "Prices", Audience: entity_notes.AudiencePrivate},
		{ID: "theirs", EntityID: "inn", CampaignID: camp, AuthorUserID: "mira", Title: "Can I buy rope?", Audience: entity_notes.AudienceDMOnly},
	}
	at := map[string]any{"page": "The Rusty Anchor"}
	with := func(extra map[string]any) map[string]any {
		out := map[string]any{"page": "The Rusty Anchor"}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	tests := []struct {
		name    string
		r       Record
		wantErr string
		check   func(t *testing.T, f *fakePlayerNotes)
	}{
		{"needs a page", rec(KindPlayerNote, ActionCreate, "Prices", nil, "x"), "needs the `page`", nil},
		{"unknown page", rec(KindPlayerNote, ActionCreate, "Prices", map[string]any{"page": "Nowhere"}, "x"), "no page called", nil},
		{"bad share", rec(KindPlayerNote, ActionCreate, "Prices", with(map[string]any{"share": "party"}), "x"), "share must be", nil},
		{"adds mine, private by default", rec(KindPlayerNote, ActionCreate, "Rumours", at, "The cook *knows*."), "", func(t *testing.T, f *fakePlayerNotes) {
			if len(f.created) != 1 || f.created[0].Title != "Rumours" || f.created[0].Audience != "" || !strings.Contains(f.created[0].BodyHTML, "<em>knows</em>") || len(f.created[0].Body) == 0 {
				t.Fatalf("created %+v", f.created)
			}
		}},
		{"shares with the DM and Scribes", rec(KindPlayerNote, ActionCreate, "Rumours", with(map[string]any{"share": "gm-scribes"}), ""), "", func(t *testing.T, f *fakePlayerNotes) {
			if f.created[0].Audience != entity_notes.AudienceDMScribe || f.created[0].BodyHTML != "" {
				t.Fatalf("created %+v", f.created[0])
			}
		}},
		{"changes mine", rec(KindPlayerNote, ActionUpdate, "prices", with(map[string]any{"rename_to": "Prices here", "share": "gm"}), "Too cheap."), "", func(t *testing.T, f *fakePlayerNotes) {
			u, ok := f.updated["mine"]
			if !ok || *u.Title != "Prices here" || *u.Audience != entity_notes.AudienceDMOnly || !strings.Contains(*u.BodyHTML, "Too cheap.") {
				t.Fatalf("updated %+v", f.updated)
			}
		}},
		{"removes mine", rec(KindPlayerNote, ActionDelete, "Prices", at, ""), "", func(t *testing.T, f *fakePlayerNotes) {
			if strings.Join(f.deleted, ",") != "mine" {
				t.Fatalf("deleted %v", f.deleted)
			}
		}},
		{"cannot change someone else's", rec(KindPlayerNote, ActionUpdate, "Can I buy rope?", at, "x"), "you have no Player Note", nil},
		{"cannot remove someone else's", rec(KindPlayerNote, ActionDelete, "Can I buy rope?", at, ""), "you have no Player Note", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakePlayerNotes{notes: notesOnInn}
			err := PlayerNoteKind{Svc: f, Entities: pages}.Apply(context.Background(), camp, owner, tc.r)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				if len(f.updated)+len(f.deleted)+len(f.created) != 0 {
					t.Fatal("a refused block reached the service")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if f.viewer.UserID != "owner" || !f.viewer.IsDMGranted {
				t.Errorf("listed as %+v, want the owner's audience", f.viewer)
			}
			tc.check(t, f)
		})
	}
}

// A note the list says is mine but the store says is not is refused.
func TestPlayerNoteKind_RechecksStoredWriter(t *testing.T) {
	f := &fakePlayerNotes{swap: true, notes: []entity_notes.Note{{ID: "n", EntityID: "inn", CampaignID: camp, AuthorUserID: "owner", Title: "T"}}}
	k := PlayerNoteKind{Svc: f, Entities: fakeEntities{"inn": {ID: "inn", CampaignID: camp, Name: "Inn"}}}
	if err := k.Apply(context.Background(), camp, owner, rec(KindPlayerNote, ActionDelete, "T", map[string]any{"page": "Inn"}, "")); err == nil {
		t.Fatal("removed a Player Note whose stored writer is someone else")
	}
	if len(f.deleted) != 0 {
		t.Fatal("delete reached the service")
	}
}
