package decks

import (
	"context"
	"time"
)

// Heroes is owned by this plugin and implemented in internal/app over the
// entities service, so decks never imports another plugin.
type Heroes interface {
	// Owned lists the pages of the campaign the user is the claimed owner of.
	Owned(ctx context.Context, campaignID, userID string) ([]Hero, error)
	// InCampaign finds a live page of this campaign by id; ok is false when
	// it is missing, trashed or in another campaign.
	InCampaign(ctx context.Context, campaignID, entityID string) (hero Hero, ok bool, err error)
	// Viewable says whether the viewer may open the page by the page's own
	// visibility rules, so a hidden hero is never named to a player.
	Viewable(ctx context.Context, campaignID, entityID string, v Viewer) (bool, error)
	// Party lists the campaign's hero pages (the claimed pages, the party) as
	// the viewer may see them, for a Director's hero picker.
	Party(ctx context.Context, campaignID string, v Viewer) ([]Hero, error)
}

// Members lists the campaign's members, implemented in internal/app over the
// campaigns service. It checks the players a Director chooses and drops a
// grant made before the member's current stay.
type Members interface {
	Members(ctx context.Context, campaignID string) ([]Member, error)
}

// Member is one person in the campaign.
type Member struct {
	UserID string
	// Name is how the person is shown ("" when they have none).
	Name string
	// Role is the membership role (1 player, 2 scribe, 3 owner).
	Role int
	// Director is the owner or a co-Director.
	Director bool
	// JoinedAt is when the member joined; a grant made before it is from an
	// earlier stay.
	JoinedAt time.Time
}

// Pictures decides whether a media file may be a card's picture. It is owned
// here and implemented in internal/app over the media service, so decks never
// imports the media plugin.
type Pictures interface {
	// Usable is true only for an image of this campaign that is neither a page
	// file nor in the Trash. A file that is missing, foreign or unusable all
	// answer false with no error, so a refusal cannot tell them apart.
	Usable(ctx context.Context, campaignID, mediaID string) (bool, error)
}

// Tables reads the campaign's rolling tables so a deck can be made from one.
// It is owned here and implemented in internal/app over the rolling tables
// service, and bound only while that plugin is healthy, so decks never imports
// it. Every method answers as a Director would see it: a Director-only table is
// listed too, because the workshop is the Director's.
type Tables interface {
	// List is the campaign's tables, in their own order.
	List(ctx context.Context, campaignID string) ([]TableSummary, error)
	// Lines reads one table's lines; ok is false when there is no such table.
	Lines(ctx context.Context, campaignID, tableID string) (lines []TableLine, name string, ok bool, err error)
}

// TableSummary is one rolling table in a list.
type TableSummary struct {
	ID    string
	Name  string
	Lines int
}

// TableLine is one line of a rolling table.
type TableLine struct {
	Name   string
	Brief  string
	Weight float64
	// HasDays says the line only comes up on certain days.
	HasDays bool
}
