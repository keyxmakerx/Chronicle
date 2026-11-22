package systems

import "time"

// Card looks are the border a campaign's owner gives a card in the hero
// creator. A package entry has no row of its own to write the choice into, so
// it is kept in campaign_card_looks; a campaign's own entry keeps it in its
// own card_border / card_colour properties (CardLookService.SetCardLook), so
// nothing shadows an entry's truth.

const (
	// MaxCardLookKeyLen is the entry_key column's width. Package slugs are not
	// length-limited by the package format, so a longer key is refused rather
	// than truncated into another entry's key.
	MaxCardLookKeyLen = 150
	// MaxCardLookChanges bounds the items one Customizer save may carry.
	MaxCardLookChanges = 500
)

// StoredLook is one campaign_card_looks row as the choice list reads it.
// Border "" means "no border"; Colour "" means "the border's own colour".
type StoredLook struct {
	Border string
	Colour string
}

// CardLookRow is a full row, for the repository.
type CardLookRow struct {
	CampaignID string
	SystemID   string
	FieldKey   string
	EntryKey   string
	StoredLook
	UpdatedBy string
	UpdatedAt time.Time
}

// CardLookChange is one owner's choice for one card, the unit of the
// Customizer's herolooks section. Source is ChoiceSourcePackage or
// ChoiceSourceCampaign; Key is Choice.Key. Reset (package entries only) drops
// the choice so the game system's own look applies again.
type CardLookChange struct {
	FieldKey string
	Source   string
	Key      string
	Border   string
	Colour   string
	Reset    bool
}
