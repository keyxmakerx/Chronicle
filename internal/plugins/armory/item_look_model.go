package armory

import (
	"bytes"
	"encoding/json"
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
	// Look names a gallery look. "" means the item has no look.
	Look string `json:"look"`
	// Tint replaces the look's own colour; "" keeps it.
	Tint string `json:"tint,omitempty"`
	// Sound replaces the look's own sound; "" keeps it, "none" is silent.
	Sound string `json:"sound,omitempty"`
	// Words float up from the target when the item is used.
	Words string `json:"words,omitempty"`
}

const (
	// maxItemLookBytes caps the request body; a valid look is well under it.
	maxItemLookBytes = 1024
	// maxItemLookWords is in characters, matching the widget's input.
	maxItemLookWords = 24
)

var (
	itemLooks  = setOf("ember", "frost", "storm", "bloom", "dawn", "rot", "venom", "glass", "quiet")
	itemTints  = setOf("#ff7a2f", "#ffd66b", "#8fe07a", "#7cf5c8", "#8fd8ff", "#a99bff", "#f59ac0", "#e8dcc0")
	itemSounds = setOf("crackle", "ping", "zap", "chime", "bell", "thrum", "bubble", "hiss", "none")
)

// NormalizeItemLook validates a saved look and returns its canonical JSON.
// A look of "" clears the item, reported as cleared with a nil document.
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
	if l.Look == "" {
		return nil, true, nil
	}
	if !itemLooks[l.Look] {
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
	out, err := json.Marshal(l)
	if err != nil {
		return nil, false, apperror.NewInternal(err)
	}
	return out, false, nil
}
