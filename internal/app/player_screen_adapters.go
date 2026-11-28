package app

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// The players' screen reads the same plugins as the DM Screen, through these
// adapters, always as the player: hidden pages are filtered by the entities
// plugin's own visibility rule, and every hero carries the keys of the sheet
// values a player may not see, so the screen can withhold them.

// errPlayersOnly refuses a players' screen read asked as the DM team; their
// view of a hero is not a player's, so it is never handed to this screen.
var errPlayersOnly = apperror.NewForbidden("the players' screen is for the campaign's players")

// PlayerHeroes lists the claimed heroes the viewer may open, each with the
// keys its category and its own page restrict. A category that cannot be read
// leaves RestrictionsKnown false, so the screen shows none of that hero's
// values rather than guess.
func (a *dmPartyAdapter) PlayerHeroes(ctx context.Context, campaignID string, v dmscreen.Viewer) ([]dmscreen.Hero, error) {
	if !v.CanOpenPlayerScreen() {
		return nil, errPlayersOnly
	}
	ents := a.entities
	claimed, err := ents.ListClaimed(ctx, campaignID, v.Role, v.UserID)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	if a.campaigns != nil {
		if members, err := a.campaigns.ListMembers(ctx, campaignID); err == nil {
			for _, m := range members {
				names[m.UserID] = m.DisplayName
			}
		}
	}
	types := map[int]*entities.EntityType{}
	out := make([]dmscreen.Hero, 0, len(claimed))
	for _, e := range claimed {
		if e.CampaignID != "" && e.CampaignID != campaignID {
			continue
		}
		h := dmscreen.Hero{ID: e.ID, TypeID: e.EntityTypeID, Name: e.Name, Fields: e.FieldsData, UpdatedAt: e.UpdatedAt}
		if e.OwnerUserID != nil {
			h.PlayerName = names[*e.OwnerUserID]
			h.PlayerUserID = *e.OwnerUserID
		}
		t, seen := types[e.EntityTypeID]
		if !seen {
			if got, err := ents.GetEntityTypeByID(ctx, e.EntityTypeID); err == nil {
				t = got
			}
			types[e.EntityTypeID] = t
		}
		if t != nil {
			h.RestrictionsKnown = true
			for _, f := range entities.RestrictionDefs(t.Fields, e.FieldOverrides) {
				switch {
				case f.GMOnly:
					h.GMOnly = append(h.GMOnly, f.Key)
				case f.OwnerOnly:
					h.OwnerOnly = append(h.OwnerOnly, f.Key)
				}
			}
		}
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// playerNotesNamespace seeds the players' notes' ids; any fixed UUID other
// than the DM notes' would do.
var playerNotesNamespace = uuid.MustParse("b3d7a0c4-2e6f-4f19-8a5d-71c9e04f2b86")

// playerNoteID is one member's note id in one campaign. Deriving it lets the
// screen find the note again without a table, and two tabs saving at once
// collide on the primary key rather than making two notes.
func playerNoteID(campaignID, userID string) string {
	return uuid.NewSHA1(playerNotesNamespace, []byte(campaignID+"\x00"+userID)).String()
}

// errPlayerNoteTaken is the refusal when a note the viewer does not own sits
// at their screen note's id. Ids are never taken from a request, so this is
// a guard rather than an expected case.
var errPlayerNoteTaken = apperror.NewValidation("another note already uses your screen note's place; open your Journal instead")

// playerNotesAdapter keeps each member's players' screen note as an ordinary
// private note of theirs, so it also shows in their Journal. Only a note the
// viewer owns, in this campaign, is ever read or written: the id is derived
// from the viewer's own user id, and the owner is checked on every read.
type playerNotesAdapter struct {
	notes notes.NoteService
}

// mine returns the note at the viewer's id and whether something there is not
// theirs.
func (a *playerNotesAdapter) mine(ctx context.Context, campaignID, userID string) (*notes.Note, bool, error) {
	n, err := a.notes.GetByID(ctx, playerNoteID(campaignID, userID))
	if err != nil {
		if isNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !n.IsOwnedBy(userID, campaignID) {
		return nil, true, nil
	}
	return n, false, nil
}

func (a *playerNotesAdapter) FindMine(ctx context.Context, campaignID string, v dmscreen.Viewer) (*dmscreen.ScreenNote, error) {
	if v.UserID == "" {
		return nil, errPlayersOnly
	}
	n, _, err := a.mine(ctx, campaignID, v.UserID)
	if err != nil || n == nil {
		return nil, err
	}
	return playerScreenNote(n), nil
}

func (a *playerNotesAdapter) SaveMine(ctx context.Context, campaignID string, v dmscreen.Viewer, title, entry, entryHTML, version string) (*dmscreen.ScreenNote, error) {
	if v.UserID == "" {
		return nil, errPlayersOnly
	}
	viewer := permissions.RequestViewer(v.Role, v.UserID)
	id := playerNoteID(campaignID, v.UserID)
	cur, foreign, err := a.mine(ctx, campaignID, v.UserID)
	if err != nil {
		return nil, err
	}
	if foreign {
		return nil, errPlayerNoteTaken
	}
	// A tab that saw no note (version "") may only create it; one that saw a
	// note may only replace that same body.
	verify := cur != nil
	if cur == nil {
		if version != "" {
			return nil, dmscreen.ErrNoteChanged
		}
		// Private: the viewer alone, not even the GM. The title is set here and
		// never changed by later saves.
		if _, cerr := a.notes.Create(ctx, campaignID, viewer, notes.CreateNoteRequest{
			ID: id, Title: title, Visibility: notes.VisibilityPrivate,
		}); cerr != nil {
			// Another tab of theirs created it a moment ago: its body is not
			// what this tab loaded (nothing), so the version check reports it.
			if again, _, err := a.mine(ctx, campaignID, v.UserID); err != nil || again == nil {
				return nil, cerr
			}
			verify = true
		}
	}
	n, foreign, err := a.mine(ctx, campaignID, v.UserID)
	if err != nil {
		return nil, err
	}
	if foreign || n == nil {
		return nil, errPlayerNoteTaken
	}
	loaded := ""
	if n.Entry != nil {
		loaded = *n.Entry
	}
	if verify && version != dmscreen.BodyVersion(loaded) {
		return nil, dmscreen.ErrNoteChanged
	}
	// Only the body changes: who the note is shared with stays as the owner
	// set it in the Journal.
	req := notes.UpdateNoteRequest{Entry: &entry, EntryHTML: &entryHTML}
	req.StripOwnerOnly()
	saved, err := a.notes.Update(ctx, id, viewer, req)
	if err != nil {
		return nil, err
	}
	return playerScreenNote(saved), nil
}

// playerScreenNote is the note in the screen's shape, saying whether its
// owner has shared it with anyone.
func playerScreenNote(n *notes.Note) *dmscreen.ScreenNote {
	out := toScreenNote(n)
	out.Shared = n.IsShared || len(n.SharedWith) > 0 || n.SharedWithGM
	return out
}
