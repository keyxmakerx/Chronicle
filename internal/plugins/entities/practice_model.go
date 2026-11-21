package entities

import "time"

// practice_model.go holds the shapes of practice heroes: a player's saved
// hero-creator results, kept in their own table so no hero reader can see one.

// MaxPracticeHeroes is how many practice heroes one player keeps in one
// campaign.
const MaxPracticeHeroes = 3

// maxPracticePicksBytes bounds a practice hero's stored picks. A real hero
// has a dozen short steps; the bound only stops a hand-made body.
const maxPracticePicksBytes = 64 * 1024

// Words the creator and the pages use for a practice pick the rules keep from
// the player.
const (
	// HeroChoiceStateNotOffered marks an entry the campaign's rules keep from
	// this player, shown in a practice plan so they can still try it.
	HeroChoiceStateNotOffered = "not_offered"

	// PracticeAskWaiting and PracticeAskDeclined are the two answers an ask
	// can be in; an empty status means the maker has not asked.
	PracticeAskWaiting  = "waiting"
	PracticeAskDeclined = "declined"
)

// PracticeHero is one saved practice hero.
type PracticeHero struct {
	ID         string
	CampaignID string
	// UserID is the maker; only they and the Directors may open it.
	UserID string
	Name   string
	// Picks is the creator's map exactly as validated against the practice
	// plan when it was saved.
	Picks map[string]HeroPick
	// Level is carried from a copied hero that had a readable level.
	Level *int
	// CopiedFromName is the hero this one started as a copy of, as named then.
	CopiedFromName string
	AskStatus      string
	AskNote        string
	AskedAt        *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// PracticeActor is who is acting on a practice hero. Director is the owner or
// a co-Director (campaigns.CampaignContext.CanAuthorDmOnly), decided by the
// caller so the service never sees a request type.
type PracticeActor struct {
	UserID   string
	Director bool
}

// PracticeInput is what the creator sends to save a practice hero.
type PracticeInput struct {
	Name  string
	Picks map[string]HeroPick
	// CopiedFrom and Level are set by the server when the hero was started as
	// a copy of the player's own; a client cannot choose them.
	CopiedFrom string
	Level      *int
}

// PracticeRow is a practice hero worded for a page: its summary line and the
// maker's name, for the shelf and the Directors' list.
type PracticeRow struct {
	Hero    PracticeHero
	Summary string
	Maker   string
}

// PracticePickRow is one line of the practice page: "Class: Fury" and the
// plain note under it when the rules keep the entry from this player.
type PracticePickRow struct {
	Label string
	Value string
	// Options are what was bought from the entry, in the order picked.
	Options string
	Note    string
}

// PracticeShelfView is the player's Practice shelf on the Characters page.
type PracticeShelfView struct {
	CampaignID string
	CSRFToken  string
	Rows       []PracticeShelfRow
	// Max is how many the player may keep; HeroName is their current hero's
	// name, empty when they have none, for "Start as a copy of <name>".
	Max      int
	HeroName string
	// Tryout is where a try-out kept on this device shows as the shelf's last
	// row; nil when the shelf cannot take one.
	Tryout *TryoutView
}

// Full reports whether the player holds the most they may.
func (v *PracticeShelfView) Full() bool { return len(v.Rows) >= v.Max }

// PracticeShelfRow is one practice hero on the shelf.
type PracticeShelfRow struct {
	ID   string
	Name string
	// Line is the muted line under the name: what it is built as, or whose
	// copy it is.
	Line string
}

// PracticeDirectorView is the Directors' list of every practice hero.
type PracticeDirectorView struct {
	CampaignID string
	CSRFToken  string
	Rows       []PracticeDirectorRow
}

// PracticeDirectorRow is one practice hero in the Directors' list.
type PracticeDirectorRow struct {
	ID      string
	Name    string
	Summary string
	Maker   string
	// Waiting means the maker asked to play it. OldHero is the maker's
	// current hero, which a yes sends back to Free to pick; empty when they
	// have none.
	Waiting bool
	OldHero string
}

// PracticeCreatorView is what the creator page needs in practice mode.
type PracticeCreatorView struct {
	CampaignID string
	CSRFToken  string
	PlanURL    string
	CreateURL  string
	// UpdateURL is set when an existing practice hero is being changed.
	UpdateURL string
	Editing   bool
	// Preload is the JSON the creator starts from for a copy or an edit.
	Preload string
}

// PracticePageView is a practice hero's own page.
type PracticePageView struct {
	CampaignID string
	CSRFToken  string
	ID         string
	Name       string
	Summary    string
	Rows       []PracticePickRow
	// Mine is true for the maker; a Director viewing another's sees Maker.
	Mine  bool
	Maker string
	// Status and Note are the ask: "", "waiting" or "declined", and the
	// Director's note when they declined.
	Status   string
	Note     string
	NeedsYes bool
	Copied   string
	Notice   string
}

// TryoutCreatorView is what the creator page needs in try-out mode.
type TryoutCreatorView struct {
	CampaignID string
	UserID     string
}

// TryoutView is the Characters page's mount for a try-out kept in the
// viewer's browser. It carries who the viewer is and nothing about the
// try-out itself, which the server never sees until it is sent.
type TryoutView struct {
	CampaignID string
	UserID     string
	CSRFToken  string
	// Visitor is a look-around visitor, who gets the Practice section of their
	// own. Otherwise this is a player, who is offered the hero they built
	// while looking around.
	Visitor bool
	// HasHero says a player already has a hero, so the offer is to keep the
	// try-out as a practice hero instead.
	HasHero bool
}

// Base is the campaign path the mount's script builds its addresses from.
func (v *TryoutView) Base() string { return "/campaigns/" + v.CampaignID }
