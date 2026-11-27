package auth

import (
	"encoding/json"
	"slices"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

// ViewPrefs are one person's own viewing choices. They are per account and
// per viewer only: nothing here is ever read for another member's page or
// written to a campaign's settings, so a player's choice can never change
// what the owner or anyone else sees. Every field always holds one of its
// allowed values; the first value of each list is the default.
type ViewPrefs struct {
	Theme    string `json:"theme"`    // device | light | dark
	Motion   string `json:"motion"`   // owner | calm | off
	TextSize string `json:"textSize"` // standard | larger | largest
	Contrast string `json:"contrast"` // standard | high
	// BrushHidden lists the campaigns where this person put the header's
	// Customize brush away. The choice is per campaign: hiding it in one
	// leaves it shown in the rest. Newest last; never nil.
	BrushHidden []string `json:"brushHidden"`
	// StoreHidden lists the campaigns where this person put the top bar's
	// Store icon away, kept exactly like BrushHidden. Newest last; never nil.
	StoreHidden []string `json:"storeHidden"`
}

// maxBrushHidden bounds the list so a stored row cannot grow without limit;
// past it the oldest choice is forgotten (that campaign shows the brush again).
const maxBrushHidden = 200

// maxStoreHidden bounds the Store list the same way and for the same reason.
const maxStoreHidden = 200

// HidesBrushIn reports whether the brush is put away in this campaign.
func (p ViewPrefs) HidesBrushIn(campaignID string) bool {
	return slices.Contains(p.BrushHidden, campaignID)
}

// HidesStoreIn reports whether the Store icon is put away in this campaign.
func (p ViewPrefs) HidesStoreIn(campaignID string) bool {
	return slices.Contains(p.StoreHidden, campaignID)
}

// validBrushCampaignID accepts the shape of a campaign id (a UUID), so only
// vetted text is ever stored or echoed back.
func validBrushCampaignID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		ok := r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_'
		if !ok {
			return false
		}
	}
	return true
}

// WithBrushHidden returns the prefs with the brush put away in (hidden) or
// brought back in (!hidden) one campaign, leaving every other campaign as it
// was.
func (p ViewPrefs) WithBrushHidden(campaignID string, hidden bool) (ViewPrefs, error) {
	next, err := withCampaignHidden(p.BrushHidden, campaignID, hidden, maxBrushHidden)
	if err != nil {
		return p, err
	}
	p.BrushHidden = next
	return p, nil
}

// WithStoreHidden returns the prefs with the Store icon put away in (hidden)
// or brought back in (!hidden) one campaign, leaving every other campaign as
// it was.
func (p ViewPrefs) WithStoreHidden(campaignID string, hidden bool) (ViewPrefs, error) {
	next, err := withCampaignHidden(p.StoreHidden, campaignID, hidden, maxStoreHidden)
	if err != nil {
		return p, err
	}
	p.StoreHidden = next
	return p, nil
}

// withCampaignHidden adds (hidden) or removes one campaign from a per-campaign
// "put away" list, newest last and capped at limit, always into a new slice so
// the caller's stored list is never changed under it.
func withCampaignHidden(list []string, campaignID string, hidden bool, limit int) ([]string, error) {
	if !validBrushCampaignID(campaignID) {
		return nil, apperror.NewBadRequest("invalid campaign")
	}
	next := make([]string, 0, len(list)+1)
	for _, id := range list {
		if id != campaignID {
			next = append(next, id)
		}
	}
	if hidden {
		next = append(next, campaignID)
		if len(next) > limit {
			next = next[len(next)-limit:]
		}
	}
	return next, nil
}

// Allowed values per choice. The default is first so an empty or cleared
// value maps to allowed[0]. The wire values are also what the layout writes
// into <html> attributes and what static/js/theme.js sends, so renaming one
// is a cross-layer change.
var (
	viewThemes    = []string{"device", "light", "dark"}
	viewMotions   = []string{"owner", "calm", "off"}
	viewTextSizes = []string{"standard", "larger", "largest"}
	viewContrasts = []string{"standard", "high"}
)

// DefaultViewPrefs is the look a person gets before choosing anything.
func DefaultViewPrefs() ViewPrefs {
	return ViewPrefs{Theme: viewThemes[0], Motion: viewMotions[0], TextSize: viewTextSizes[0], Contrast: viewContrasts[0], BrushHidden: []string{}, StoreHidden: []string{}}
}

// UpdateViewPrefsInput is a PARTIAL update: each choice saves on its own as
// the person taps it, so an absent key preserves the stored choice, an
// explicit null or "" resets that one choice to its default, and a value
// replaces it. Echoing the others back would race a second device.
type UpdateViewPrefsInput struct {
	Theme    patch.Field[string] `json:"theme"`
	Motion   patch.Field[string] `json:"motion"`
	TextSize patch.Field[string] `json:"textSize"`
	Contrast patch.Field[string] `json:"contrast"`
}

// merge resolves one field over its stored value: absent keeps cur, null or
// "" resets to the default, anything else must be in the allowed list.
func mergeViewChoice(name string, f patch.Field[string], cur string, allowed []string) (string, error) {
	if !f.Present() {
		return cur, nil
	}
	v, ok := f.Get()
	if !ok || v == "" {
		return allowed[0], nil
	}
	for _, a := range allowed {
		if v == a {
			return v, nil
		}
	}
	return "", apperror.NewBadRequest("invalid " + name + " choice")
}

// ApplyTo returns cur with the input merged in, refusing any value outside
// the allowed lists so nothing unvetted reaches the database or an <html>
// attribute.
func (in UpdateViewPrefsInput) ApplyTo(cur ViewPrefs) (ViewPrefs, error) {
	var err error
	out := cur
	if out.Theme, err = mergeViewChoice("theme", in.Theme, cur.Theme, viewThemes); err != nil {
		return cur, err
	}
	if out.Motion, err = mergeViewChoice("motion", in.Motion, cur.Motion, viewMotions); err != nil {
		return cur, err
	}
	if out.TextSize, err = mergeViewChoice("text size", in.TextSize, cur.TextSize, viewTextSizes); err != nil {
		return cur, err
	}
	if out.Contrast, err = mergeViewChoice("contrast", in.Contrast, cur.Contrast, viewContrasts); err != nil {
		return cur, err
	}
	return out, nil
}

// ParseViewPrefs reads the stored JSON column. Missing, malformed or
// out-of-list values fall back to the default for that one choice, so a
// hand-edited row or a future rename degrades to the default look rather
// than failing the page.
func ParseViewPrefs(raw []byte) ViewPrefs {
	out := DefaultViewPrefs()
	if len(raw) == 0 {
		return out
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(raw, &stored); err != nil {
		return out
	}
	str := func(key string) string {
		var v string
		_ = json.Unmarshal(stored[key], &v)
		return v
	}
	pick := func(v string, allowed []string, def string) string {
		for _, a := range allowed {
			if v == a {
				return v
			}
		}
		return def
	}
	out.Theme = pick(str("theme"), viewThemes, out.Theme)
	out.Motion = pick(str("motion"), viewMotions, out.Motion)
	out.TextSize = pick(str("textSize"), viewTextSizes, out.TextSize)
	out.Contrast = pick(str("contrast"), viewContrasts, out.Contrast)
	// Rows from before the brush choice was per campaign carry "brush":
	// "hidden" for the whole account. It is not read: the brush is shown
	// again everywhere and the person hides it where they want it gone.
	out.BrushHidden = parseCampaignHidden(stored["brushHidden"], maxBrushHidden)
	out.StoreHidden = parseCampaignHidden(stored["storeHidden"], maxStoreHidden)
	return out
}

// parseCampaignHidden reads one stored per-campaign "put away" list,
// dropping duplicates and anything not shaped like a campaign id, and keeping
// only the newest limit. A missing or wrongly typed list reads as empty.
func parseCampaignHidden(raw json.RawMessage, limit int) []string {
	var hidden []string
	_ = json.Unmarshal(raw, &hidden)
	out := []string{}
	seen := map[string]bool{}
	for _, id := range hidden {
		if validBrushCampaignID(id) && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}
