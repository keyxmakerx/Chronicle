package layouts

import (
	"context"

	"github.com/a-h/templ"
)

// AppAudience is who is looking at an app page. It picks the sentence an
// empty app opens with, so a player is never told to make something only the
// owner can.
type AppAudience int

const (
	AudiencePlayer AppAudience = iota
	AudienceCoDM
	AudienceOwner
)

// AppAudienceOf reads who is looking from the request context. An owner or
// co-DM using "view as player" is a player.
func AppAudienceOf(ctx context.Context) AppAudience {
	switch {
	case IsViewingAsPlayer(ctx):
		return AudiencePlayer
	case IsOwner(ctx):
		return AudienceOwner
	case IsDmGranted(ctx):
		return AudienceCoDM
	}
	return AudiencePlayer
}

// AppIntros are the three sentences an app writes for its empty page. Each
// says what will show there and who adds it; none offers an action the
// reader can't take. Use GMTerm for the game master's name.
type AppIntros struct {
	Player, CoDM, Owner string
}

// For picks the sentence for an audience.
func (i AppIntros) For(a AppAudience) string {
	switch a {
	case AudienceOwner:
		return i.Owner
	case AudienceCoDM:
		return i.CoDM
	}
	return i.Player
}

// AppAction is an app's main action, already permission-checked by the
// caller. Href makes it a link; without one it is a button carrying Attrs
// (for example the onclick that opens a create dialog).
type AppAction struct {
	Label string
	Icon  string
	Href  string
	Attrs templ.Attributes
}

// AppOpeningData describes one app page's opening.
type AppOpeningData struct {
	// Name is the app's name, drawn as the page's title.
	Name string
	// Empty is true when the app has nothing to show this viewer. The opening
	// then draws large: the name and the sentence for this audience.
	Empty  bool
	Intros AppIntros
	// Action is the one thing this viewer can add here, or nil.
	Action *AppAction
}
