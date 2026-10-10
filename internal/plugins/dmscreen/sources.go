package dmscreen

import (
	"context"
	"time"

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

// WorldSource reads the default calendar's current date and today's weather.
// It returns nil, nil when there is no calendar the viewer can see.
type WorldSource interface {
	World(ctx context.Context, campaignID string, v Viewer) (*WorldView, error)
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
	Fields     map[string]any
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
	World    WorldSource
	Nights   NightSource
	Foundry  FoundrySource
	Presence PresenceSource
	Party    PartySource
	Hidden   HiddenSource
	System   SystemSource
}
