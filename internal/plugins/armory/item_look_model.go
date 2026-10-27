package armory

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ItemLook is how an item shows its magic: one look from a fixed gallery,
// optionally recoloured, with its sound swapped and words that float up when
// it is used. Every value comes from a fixed list (or is short plain text), so
// nothing saved here can run code; the item_look widget draws it. The lists
// are mirrored in static/js/widgets/item_look_fx.js, and
// test/js/item_look.test.mjs fails when the two drift.
type ItemLook struct {
	// Look names a gallery look. "" means the item has no look of its own,
	// which is allowed only when it is made from a page that has one.
	Look string `json:"look"`
	// Tint replaces the look's own colour; "" keeps it.
	Tint string `json:"tint,omitempty"`
	// Sound replaces the look's own sound; "" keeps it, "none" is silent.
	Sound string `json:"sound,omitempty"`
	// Words float up from the target when the item is used.
	Words string `json:"words,omitempty"`
	// Spots places gallery looks on named spots of the stage (the item, its
	// trail, the point of impact, the target's body, the ground), each
	// playing at its own moment. Empty spots stay quiet.
	Spots map[string]ItemLookSpot `json:"spots,omitempty"`
	// MadeFrom is the page (a Remnant, say) the item was made from. The item
	// carries that page's look: as its look when it has none of its own,
	// added on top when it has. One hop only, so a chain of pages never loops.
	MadeFrom string `json:"madeFrom,omitempty"`
}

// ItemLookSpot is one gallery look placed on a spot, optionally recoloured.
type ItemLookSpot struct {
	Look string `json:"look"`
	Tint string `json:"tint,omitempty"`
}

const (
	// maxItemLookBytes caps the request body; a valid look is well under it.
	maxItemLookBytes = 1024
	// maxItemLookWords is in characters, matching the widget's input.
	maxItemLookWords = 24
)

// entityIDPattern is the shape of an entity id; anything else in MadeFrom
// is refused before it reaches a lookup.
var entityIDPattern = regexp.MustCompile(`^[0-9A-Za-z-]{1,36}$`)

var (
	itemLooks  = setOf("ember", "frost", "storm", "bloom", "dawn", "rot", "venom", "glass", "quiet")
	itemTints  = setOf("#ff7a2f", "#ffd66b", "#8fe07a", "#7cf5c8", "#8fd8ff", "#a99bff", "#f59ac0", "#e8dcc0")
	itemSounds = setOf("crackle", "ping", "zap", "chime", "bell", "thrum", "bubble", "hiss", "none")
	itemSpots  = setOf("item", "trail", "impact", "body", "ground")
)

// NormalizeItemLook validates a saved look and returns its canonical JSON.
// A look of "" with no MadeFrom clears the item, reported as cleared with a
// nil document. Whether MadeFrom names a real page is the service's check.
func NormalizeItemLook(raw []byte) (doc json.RawMessage, cleared bool, err error) {
	if len(raw) > maxItemLookBytes {
		return nil, false, apperror.NewBadRequest("look is too large")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var l ItemLook
	if err := dec.Decode(&l); err != nil {
		return nil, false, apperror.NewBadRequest("invalid look")
	}
	if dec.More() {
		return nil, false, apperror.NewBadRequest("invalid look")
	}
	if l.MadeFrom != "" && !entityIDPattern.MatchString(l.MadeFrom) {
		return nil, false, apperror.NewBadRequest("invalid page")
	}
	if l.Look == "" && l.MadeFrom == "" {
		return nil, true, nil
	}
	if l.Look == "" && l.Tint != "" {
		// A colour recolours the item's own look, so it needs one.
		return nil, false, apperror.NewBadRequest("a colour needs a look")
	}
	if l.Look != "" && !itemLooks[l.Look] {
		return nil, false, apperror.NewBadRequest("unknown look")
	}
	if l.Tint != "" && !itemTints[l.Tint] {
		return nil, false, apperror.NewBadRequest("unknown colour")
	}
	if l.Sound != "" && !itemSounds[l.Sound] {
		return nil, false, apperror.NewBadRequest("unknown sound")
	}
	l.Words = strings.TrimSpace(l.Words)
	if utf8.RuneCountInString(l.Words) > maxItemLookWords {
		return nil, false, apperror.NewBadRequest("words are too long")
	}
	if strings.IndexFunc(l.Words, unicode.IsControl) >= 0 {
		return nil, false, apperror.NewBadRequest("invalid words")
	}
	for spot, p := range l.Spots {
		if !itemSpots[spot] {
			return nil, false, apperror.NewBadRequest("unknown spot")
		}
		if !itemLooks[p.Look] {
			return nil, false, apperror.NewBadRequest("unknown look")
		}
		if p.Tint != "" && !itemTints[p.Tint] {
			return nil, false, apperror.NewBadRequest("unknown colour")
		}
	}
	if len(l.Spots) == 0 {
		l.Spots = nil
	}
	out, err := json.Marshal(l)
	if err != nil {
		return nil, false, apperror.NewInternal(err)
	}
	return out, false, nil
}
