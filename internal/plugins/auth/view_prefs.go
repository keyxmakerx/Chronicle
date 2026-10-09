package auth

import (
	"encoding/json"

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
	return ViewPrefs{Theme: viewThemes[0], Motion: viewMotions[0], TextSize: viewTextSizes[0], Contrast: viewContrasts[0]}
}

// UpdateViewPrefsInput is a PARTIAL update: each choice saves on its own as
// the person taps it, so an absent key preserves the stored choice, an
// explicit null or "" resets that one choice to its default, and a value
// replaces it. Echoing the other three back would race a second device.
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
	var stored map[string]string
	if err := json.Unmarshal(raw, &stored); err != nil {
		return out
	}
	pick := func(v string, allowed []string, def string) string {
		for _, a := range allowed {
			if v == a {
				return v
			}
		}
		return def
	}
	out.Theme = pick(stored["theme"], viewThemes, out.Theme)
	out.Motion = pick(stored["motion"], viewMotions, out.Motion)
	out.TextSize = pick(stored["textSize"], viewTextSizes, out.TextSize)
	out.Contrast = pick(stored["contrast"], viewContrasts, out.Contrast)
	return out
}
