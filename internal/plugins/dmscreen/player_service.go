package dmscreen

import (
	"context"
	"fmt"
	"log/slog"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// errNotPlayer refuses the players' screen to anyone it is not for. The route
// refuses first; this is the second check, so a wiring mistake cannot open
// it to a non-member or hand the DM team a player-shaped view of their own
// secrets.
var errNotPlayer = apperror.NewForbidden("the players' screen is for the campaign's players")

// playerNoteTitle is the title the player's note is created with; it is never
// changed by later saves, so a rename in the Journal stands.
const playerNoteTitle = "My notes"

// BuildPlayer assembles the players' screen. Every source is asked as the
// viewer, and a failing source leaves its section out, as on the DM Screen.
func (s *service) BuildPlayer(ctx context.Context, campaignID string, v Viewer) (*PlayerView, error) {
	if !v.CanOpenPlayerScreen() {
		return nil, errNotPlayer
	}
	view := &PlayerView{CampaignID: campaignID}
	warn := func(section string, err error) {
		slog.Warn("players' screen: section unavailable", slog.String("section", section), slog.String("campaign_id", campaignID), slog.Any("error", err))
	}

	var def *systems.DMScreenDef
	var sys systems.System
	if s.src.System != nil {
		if sys = s.src.System.EnabledSystem(ctx, campaignID); sys != nil {
			if info := sys.Info(); info != nil {
				view.SystemName = info.Name
				def = info.DMScreen
				if rules, ok := info.PowerRoll(); ok {
					view.PowerRoll = rules.Dice
				}
			}
		}
	}

	if s.src.PlayerParty != nil {
		heroes, err := s.src.PlayerParty.PlayerHeroes(ctx, campaignID, v)
		if err != nil {
			warn("party", err)
		} else {
			view.HasParty = true
			view.PartyFilled = def != nil && len(def.Party) > 0
			view.Hero, view.Party = playerParty(def, heroes, v.UserID)
		}
	}

	if s.src.World != nil {
		w, err := s.src.World.World(ctx, campaignID, v)
		if err != nil {
			warn("world", err)
		}
		if w != nil {
			// Players never move the date.
			w.CanStep = false
		}
		view.World = w
	}

	if sys != nil && def != nil && def.Conditions != nil {
		if dp := sys.DataProvider(); dp != nil {
			items, err := dp.List(def.Conditions.Category)
			if err != nil {
				warn("conditions", err)
			}
			view.Conditions = pickConditions(items, def.Conditions)
		}
	}

	if s.src.PlayerNotes != nil {
		nv, err := s.playerNoteView(ctx, campaignID, v)
		if err != nil {
			warn("notes", err)
		} else {
			view.Notes = nv
		}
	}
	return view, nil
}

// playerParty splits the heroes into the viewer's own (the most recently
// played, when they have several) and everyone else's, each built from only
// the sheet values a player may see: never a GM-only value, and an owner-only
// value only on the viewer's own hero.
func playerParty(def *systems.DMScreenDef, heroes []Hero, userID string) (*HeroView, []HeroView) {
	own := -1
	for i, h := range heroes {
		if h.PlayerUserID == userID && (own < 0 || h.UpdatedAt.After(heroes[own].UpdatedAt)) {
			own = i
		}
	}
	narrow := func(h Hero) map[string]any { return playerFields(h, h.PlayerUserID == userID) }
	var mine *HeroView
	var others []Hero
	for i, h := range heroes {
		if i == own {
			hv := heroViews(def, []Hero{h}, nil, narrow)[0]
			mine = &hv
			continue
		}
		others = append(others, h)
	}
	return mine, heroViews(def, others, nil, narrow)
}

// playerFields is a hero's sheet values as a player sees them. A hero whose
// restrictions could not be read gives no values at all.
func playerFields(h Hero, own bool) map[string]any {
	if !h.RestrictionsKnown {
		return nil
	}
	drop := map[string]bool{}
	for _, k := range h.GMOnly {
		drop[k] = true
	}
	if !own {
		for _, k := range h.OwnerOnly {
			drop[k] = true
		}
	}
	out := make(map[string]any, len(h.Fields))
	for k, val := range h.Fields {
		if !drop[k] {
			out[k] = val
		}
	}
	return out
}

// playerNoteView reads the viewer's own note into the card's shape.
func (s *service) playerNoteView(ctx context.Context, campaignID string, v Viewer) (*NotesView, error) {
	n, err := s.src.PlayerNotes.FindMine(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	return playerNotesView(campaignID, n), nil
}

func playerNotesView(campaignID string, n *ScreenNote) *NotesView {
	nv := &NotesView{Label: "Only you can see this"}
	if n == nil {
		return nv
	}
	if n.Shared {
		nv.Label = "Shared from your Journal"
	}
	nv.NoteID = n.ID
	nv.Link = fmt.Sprintf("/campaigns/%s/journal/%s", campaignID, n.ID)
	nv.Version = BodyVersion(n.Entry)
	switch {
	case n.Legacy != "":
		nv.Text, nv.ReadOnly = n.Legacy, true
	case !isPlainProse(n.Entry):
		nv.Text, nv.ReadOnly = plainFromProse(n.Entry), true
	default:
		nv.Text = plainFromProse(n.Entry)
	}
	return nv
}

// PlayerNote reads the viewer's own note.
func (s *service) PlayerNote(ctx context.Context, campaignID string, v Viewer) (*NotesView, error) {
	if !v.CanOpenPlayerScreen() {
		return nil, errNotPlayer
	}
	if s.src.PlayerNotes == nil {
		return nil, apperror.NewNotFound("notes are not available")
	}
	return s.playerNoteView(ctx, campaignID, v)
}

// SavePlayerNote stores text as plain paragraphs in the viewer's own note,
// creating it on the first save. Like the DM Screen's note it refuses a note
// holding formatting and a note that changed since version was read.
func (s *service) SavePlayerNote(ctx context.Context, campaignID string, v Viewer, text, version string) (*NotesView, error) {
	if !v.CanOpenPlayerScreen() {
		return nil, errNotPlayer
	}
	if s.src.PlayerNotes == nil {
		return nil, apperror.NewNotFound("notes are not available")
	}
	if utf8.RuneCountInString(text) > maxNoteChars {
		return nil, apperror.NewBadRequest("the note is too long for the screen; open it in the Journal")
	}
	cur, err := s.src.PlayerNotes.FindMine(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	if cur != nil && (cur.Legacy != "" || !isPlainProse(cur.Entry)) {
		return nil, apperror.NewValidation("this note has formatting, so edit it in the Journal")
	}
	entry, entryHTML := proseFromPlain(text)
	n, err := s.src.PlayerNotes.SaveMine(ctx, campaignID, v, playerNoteTitle, entry, entryHTML, version)
	if err != nil {
		return nil, err
	}
	return playerNotesView(campaignID, n), nil
}
