package dmscreen

import (
	"context"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// DowntimeSource reads the armory's downtime switch and how many stash moves
// wait for approval. ok is false when the campaign has no armory.
type DowntimeSource interface {
	Downtime(ctx context.Context, campaignID string, v Viewer) (open bool, pending int, ok bool, err error)

	// SetDowntime opens or closes downtime; opening applies the requests
	// waiting on it. The armory enforces that only the owner may switch.
	SetDowntime(ctx context.Context, campaignID string, v Viewer, open bool) (applied, failed int, err error)
}

// Request is one request waiting on the owner, in the armory's wording.
type Request struct {
	Kind      string
	ID        int64
	Text      string
	CreatedAt time.Time
}

// RequestSource lists every request waiting on the viewer, in any order; the
// service sorts oldest first and trims. ok is false when the campaign has no
// armory.
type RequestSource interface {
	WaitingRequests(ctx context.Context, campaignID string, v Viewer) (reqs []Request, ok bool, err error)
}

// ScreenNote is the stored note behind the Notes tab.
type ScreenNote struct {
	ID    string
	Title string
	// Entry is the note's ProseMirror JSON as stored.
	Entry string
	// Legacy is the text of an old block-style note with no Entry; such a note
	// is shown read-only.
	Legacy string
}

// ErrNoteChanged is returned by NotesSource.Save when the note's body no longer
// matches the version the caller loaded.
var ErrNoteChanged = apperror.NewConflict("changed elsewhere")

// NotesSource keeps the screen's note in the notes widget. nightKey names the
// game night the note is kept with (NightView.Key); empty means the campaign's
// standing note. The note's id is derived from the campaign and key, so
// nothing here owns a table. Find returns nil, nil before the first save.
// Save creates the note (shared with the GM side, titled title) or, when it
// exists, replaces only its body; it never retitles. version is the body
// version the caller loaded ("" for no note yet) and a mismatch returns
// ErrNoteChanged.
type NotesSource interface {
	Find(ctx context.Context, campaignID, nightKey string, v Viewer) (*ScreenNote, error)
	Save(ctx context.Context, campaignID, nightKey string, v Viewer, title, entry, entryHTML, version string) (*ScreenNote, error)
}

// WorldSource reads the default calendar's current date and today's weather.
// It returns nil, nil when there is no calendar the viewer can see. It sets
// WorldView.CanStep only for what the calendar allows (not a real-time
// calendar); the service adds the role check.
type WorldSource interface {
	World(ctx context.Context, campaignID string, v Viewer) (*WorldView, error)

	// Advance moves the default calendar forward by hours and whole days of
	// its own length. The calendar refuses a real-time calendar.
	Advance(ctx context.Context, campaignID string, v Viewer, hours, days int) error
}

// NightSource returns the next game night that hasn't happened, or nil.
type NightSource interface {
	NextNight(ctx context.Context, campaignID string, v Viewer) (*NightView, error)
}

// FoundrySource reports whether the campaign's Foundry world is connected.
type FoundrySource interface {
	FoundryPresence(campaignID string) (lastSeen *time.Time, connected bool)
}

// PlayerPresence is one player of the campaign and whether they are here now.
type PlayerPresence struct {
	UserID string
	Name   string
	Here   bool
}

// PresenceSource lists the campaign's players (members with the Player role,
// not the owner, co-DMs or scribes) with whether each is here. A campaign with
// no players returns an empty list.
type PresenceSource interface {
	Players(ctx context.Context, campaignID string) ([]PlayerPresence, error)
}

// Hero is a claimed player character with its raw sheet fields.
type Hero struct {
	ID           string
	Name         string
	PlayerName   string
	PlayerUserID string
	Fields       map[string]any
}

// PartySource lists the campaign's player characters.
type PartySource interface {
	Heroes(ctx context.Context, campaignID string, v Viewer) ([]Hero, error)
}

// Hidden is a character entity players can't see.
type Hidden struct {
	ID       string
	Name     string
	TypeName string
}

// HiddenSource lists hidden characters and reveals one, returning its name.
// HiddenCharacters returns at most limit of the newest and reports whether
// more exist beyond them, so the panel can point at the full list.
// Reveal must only ever make an entity visible, never hide it, and must refuse an entity
// outside campaignID.
type HiddenSource interface {
	HiddenCharacters(ctx context.Context, campaignID string, v Viewer, limit int) (hidden []Hidden, more bool, err error)
	Reveal(ctx context.Context, entityID, campaignID string) (name string, err error)
}

// SystemSource returns the game system enabled for the campaign, or nil.
type SystemSource interface {
	EnabledSystem(ctx context.Context, campaignID string) systems.System
}

// Sources groups the screen's inputs. Any of them may be nil, which leaves
// that section out.
type Sources struct {
	Downtime DowntimeSource
	Requests RequestSource
	Notes    NotesSource
	World    WorldSource
	Nights   NightSource
	Foundry  FoundrySource
	Presence PresenceSource
	Party    PartySource
	Hidden   HiddenSource
	System   SystemSource
}
