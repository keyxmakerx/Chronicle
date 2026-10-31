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

// ShopFront is how a shop's storefront is drawn on the Shops street. Every
// value is from a fixed list, a #rrggbb colour or short plain text, so nothing
// saved here can run code. The lists are mirrored in
// static/js/widgets/shop_storefront.js; test/js/shop_storefront.test.mjs
// fails when the two drift.
type ShopFront struct {
	Style  string `json:"style"`
	Colour string `json:"colour"`
	Trim   string `json:"trim,omitempty"`
	// Awning is "none", "plain" or "stripe"; AwningColour colours it.
	Awning       string `json:"awning"`
	AwningColour string `json:"awningColour,omitempty"`
	// Sign is how the name hangs: a swinging board, a fascia board or a banner.
	Sign string `json:"sign"`
	// Words replaces the shop's name on the sign; "" uses the name.
	Words string `json:"words,omitempty"`
	// Icon is the picture on a hanging sign; "" shows none.
	Icon string `json:"icon,omitempty"`
	// Wares are up to three things shown in the windows.
	Wares []string `json:"wares"`
	Lamp  bool     `json:"lamp"`
	// Closed is the look the front takes when the shop is shut.
	Closed        string `json:"closed"`
	ShutterColour string `json:"shutterColour,omitempty"`
	CurtainColour string `json:"curtainColour,omitempty"`
	// ClosedNote is written on the board when Closed is "board".
	ClosedNote string `json:"closedNote,omitempty"`
}

const (
	// maxShopFrontBytes caps the request body; a valid front is well under it.
	maxShopFrontBytes = 2048
	maxShopFrontWords = 24
	maxShopFrontNote  = 26
	maxShopFrontWares = 3
)

var (
	shopFrontStyles  = setOf("timber", "stone", "plaster", "painted")
	shopFrontAwnings = setOf("none", "plain", "stripe")
	shopFrontSigns   = setOf("hang", "board", "banner")
	shopFrontWares   = setOf("bottle", "sword", "scroll", "gem", "bread", "shield", "hammer", "herb")
	shopFrontIcons   = setOf("", "bottle", "sword", "scroll", "gem", "bread", "shield", "hammer", "herb")
	shopFrontClosed  = setOf("shutters", "curtains", "boards", "board", "dark")
	hexColour        = regexp.MustCompile(`^#[0-9a-f]{6}$`)
)

// DefaultShopFront is what a shop with no saved storefront shows.
func DefaultShopFront() ShopFront {
	return ShopFront{Style: "timber", Colour: "#d7b98f", Trim: "#7a5a2a", Awning: "stripe", AwningColour: "#2f5d50",
		Sign: "hang", Icon: "bread", Wares: []string{"bread", "scroll"}, Lamp: true, Closed: "shutters", ShutterColour: "#4f6b55"}
}

// NormalizeShopFront validates a storefront and returns its canonical JSON.
func NormalizeShopFront(raw []byte) (json.RawMessage, error) {
	if len(raw) > maxShopFrontBytes {
		return nil, apperror.NewBadRequest("storefront is too large")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var f ShopFront
	if err := dec.Decode(&f); err != nil || dec.More() {
		return nil, apperror.NewBadRequest("invalid storefront")
	}
	if err := f.validate(); err != nil {
		return nil, err
	}
	out, err := json.Marshal(f)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

func (f *ShopFront) validate() error {
	bad := apperror.NewBadRequest
	switch {
	case !shopFrontStyles[f.Style]:
		return bad("unknown building style")
	case !shopFrontAwnings[f.Awning]:
		return bad("unknown awning")
	case !shopFrontSigns[f.Sign]:
		return bad("unknown sign")
	case !shopFrontIcons[f.Icon]:
		return bad("unknown sign picture")
	case !shopFrontClosed[f.Closed]:
		return bad("unknown closed look")
	}
	f.Colour = strings.ToLower(f.Colour)
	if !hexColour.MatchString(f.Colour) {
		return bad("invalid colour")
	}
	for _, c := range []*string{&f.Trim, &f.AwningColour, &f.ShutterColour, &f.CurtainColour} {
		*c = strings.ToLower(*c)
		if *c != "" && !hexColour.MatchString(*c) {
			return bad("invalid colour")
		}
	}
	if len(f.Wares) > maxShopFrontWares {
		return bad("too many wares")
	}
	if f.Wares == nil {
		f.Wares = []string{}
	}
	for _, w := range f.Wares {
		if !shopFrontWares[w] {
			return bad("unknown ware")
		}
	}
	var err error
	if f.Words, err = plainText(f.Words, maxShopFrontWords); err != nil {
		return err
	}
	if f.ClosedNote, err = plainText(f.ClosedNote, maxShopFrontNote); err != nil {
		return err
	}
	return nil
}

// plainText trims s and refuses control characters or anything over max
// characters; the widget escapes it, this keeps the stored form tidy.
func plainText(s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > max {
		return "", apperror.NewBadRequest("text is too long")
	}
	if strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", apperror.NewBadRequest("invalid text")
	}
	return s, nil
}
