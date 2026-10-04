package armory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	maxShopRoomPieces  = 80
	maxShopRoomDecor   = 120
	maxShopRoomSpot    = 63
	maxShopRoomItems   = 500
	maxShopRoomLines   = 10
	maxShopRoomLine    = 200
	maxShopRoomSeed    = 1_000_000_000
	maxShopRoomPieceID = 100_000
	maxShopRoomRelID   = 2_147_483_647
	shopRoomCoordMin   = -1.0
	shopRoomCoordMax   = 20.0
)

func setOf(vals ...string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return m
}

var (
	shopRoomTypes       = setOf("forge", "apothecary", "jeweler", "market", "library", "general")
	shopRoomSettings    = setOf("room", "tower", "tree", "burrow", "cave")
	shopRoomSizes       = setOf("s", "m", "l")
	shopRoomFurnitures  = setOf("sparse", "normal", "packed")
	shopRoomDecorations = setOf("none", "some", "lots")
	shopRoomPalettes    = setOf("oak", "ember", "moss", "gilt", "slate", "dusk")
	shopRoomKinds       = setOf("shelf", "rack", "cabinet", "bookcase", "forge", "window", "herbs", "counter", "table", "barrel", "crate", "anvil", "glass", "stall", "rug", "pedestal", "lamp", "sack")
	shopRoomWalls       = setOf("Y", "X", "")
	shopRoomColors      = setOf("steel", "iron", "gold", "silver", "red", "green", "teal", "blue", "violet", "leather", "paper", "cloth", "wood", "dark", "bone", "honey", "")
	shopRoomIconSet     = func() map[string]bool {
		m := setOf(shopRoomIcons...)
		m[""] = true
		return m
	}()
)

// NormalizeShopRoomLayout validates a client-supplied layout and returns the
// canonical JSON to store. Every violation is a BadRequest; nothing the client
// sent is stored verbatim.
func NormalizeShopRoomLayout(raw []byte) (json.RawMessage, error) {
	if len(raw) > maxShopRoomBytes {
		return nil, apperror.NewBadRequest("room layout is too large")
	}
	var l ShopRoomLayout
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&l); err != nil {
		return nil, apperror.NewBadRequest("room layout is not valid JSON of the expected shape")
	}
	// Trailing data after the document would be silently dropped otherwise.
	if _, err := dec.Token(); err != io.EOF {
		return nil, apperror.NewBadRequest("room layout has trailing data")
	}
	if err := l.validate(); err != nil {
		return nil, apperror.NewBadRequest(err.Error())
	}
	out, err := json.Marshal(l)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

// validate checks every field and rewrites l into its canonical form (empty
// collections instead of null, trimmed lines).
func (l *ShopRoomLayout) validate() error {
	if l.Version != 1 {
		return fmt.Errorf("unsupported layout version")
	}
	enums := []struct {
		name, val string
		set       map[string]bool
	}{
		{"roomType", l.RoomType, shopRoomTypes},
		{"setting", l.Setting, shopRoomSettings},
		{"size", l.Size, shopRoomSizes},
		{"furniture", l.Furniture, shopRoomFurnitures},
		{"decorations", l.Decorations, shopRoomDecorations},
		{"palette", l.Palette, shopRoomPalettes},
	}
	for _, e := range enums {
		if !e.set[e.val] {
			return fmt.Errorf("invalid %s", e.name)
		}
	}
	for name, v := range map[string]int{"room": l.Seeds.Room, "goods": l.Seeds.Goods, "deco": l.Seeds.Deco} {
		if v < 1 || v > maxShopRoomSeed {
			return fmt.Errorf("invalid seed %s", name)
		}
	}

	if len(l.Pieces) > maxShopRoomPieces {
		return fmt.Errorf("too many pieces")
	}
	if l.Pieces == nil {
		l.Pieces = []ShopRoomPiece{}
	}
	seen := make(map[int]bool, len(l.Pieces))
	for _, p := range l.Pieces {
		if p.ID < 1 || p.ID > maxShopRoomPieceID {
			return fmt.Errorf("invalid piece id")
		}
		if seen[p.ID] {
			return fmt.Errorf("duplicate piece id")
		}
		seen[p.ID] = true
		if !shopRoomKinds[p.Kind] {
			return fmt.Errorf("invalid piece kind")
		}
		if !shopRoomWalls[p.Wall] {
			return fmt.Errorf("invalid piece wall")
		}
		for _, v := range []float64{p.X, p.Y, p.Off, p.Len, p.W, p.D} {
			// NaN fails both comparisons' negation, so it is rejected too.
			if !(v >= shopRoomCoordMin && v <= shopRoomCoordMax) {
				return fmt.Errorf("piece coordinate out of range")
			}
		}
	}

	if len(l.Decor) > maxShopRoomDecor {
		return fmt.Errorf("too many decorations")
	}
	if l.Decor == nil {
		l.Decor = []ShopRoomDecor{}
	}
	for _, d := range l.Decor {
		if !seen[d.Piece] {
			return fmt.Errorf("decoration on a missing piece")
		}
		if d.Spot < 0 || d.Spot > maxShopRoomSpot {
			return fmt.Errorf("invalid decoration spot")
		}
		if d.Icon == "" || !shopRoomIconSet[d.Icon] {
			return fmt.Errorf("invalid decoration icon")
		}
	}

	if len(l.Items) > maxShopRoomItems {
		return fmt.Errorf("too many items")
	}
	if l.Items == nil {
		l.Items = map[string]ShopRoomItemLook{}
	}
	for k, it := range l.Items {
		if !validRelationKey(k) {
			return fmt.Errorf("invalid item key")
		}
		if !shopRoomIconSet[it.Icon] {
			return fmt.Errorf("invalid item icon")
		}
		if !shopRoomColors[it.Color] {
			return fmt.Errorf("invalid item color")
		}
	}

	if p := l.Portrait; p != nil {
		if !(p.Left >= 0 && p.Left <= 100 && p.Top >= 0 && p.Top <= 100) {
			return fmt.Errorf("portrait position out of range")
		}
	}

	lines := make([]string, 0, len(l.Lines))
	for _, s := range l.Lines {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if utf8.RuneCountInString(s) > maxShopRoomLine {
			return fmt.Errorf("line is too long")
		}
		lines = append(lines, s)
	}
	if len(lines) > maxShopRoomLines {
		return fmt.Errorf("too many lines")
	}
	l.Lines = lines
	return nil
}

// validRelationKey accepts a canonical decimal relation id: digits only, no
// sign or leading zero, 1..2147483647.
func validRelationKey(k string) bool {
	if k == "" || k[0] == '0' || len(k) > 10 {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < '0' || k[i] > '9' {
			return false
		}
	}
	n, err := strconv.ParseInt(k, 10, 64)
	return err == nil && n >= 1 && n <= maxShopRoomRelID
}
