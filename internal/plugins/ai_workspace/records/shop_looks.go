package records

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// ShopLooksAPI is the shop room and storefront services as the shop-room
// and storefront kinds need them, adapted in app wiring. The saves run the
// armory's own normalizers, so a value the Arrange panel or the storefront
// card would refuse is refused here too; check runs them without saving.
type ShopLooksAPI interface {
	// Room is the shop's saved room layout, or nil while nobody arranged it.
	Room(ctx context.Context, campaignID, shopID string, a Actor) (json.RawMessage, error)
	SaveRoom(ctx context.Context, campaignID, shopID string, a Actor, layout json.RawMessage, check bool) error
	// Front is the shop's storefront, the default one with saved false
	// while it has none.
	Front(ctx context.Context, campaignID, shopID string, a Actor) (front json.RawMessage, saved bool, err error)
	SaveFront(ctx context.Context, campaignID, shopID string, a Actor, front json.RawMessage, check bool) error
	// CounterStarter is the counter theme a room of this type is drawn with
	// until one is saved: the base of an edit when the room has none.
	CounterStarter(roomType string) json.RawMessage
}

// shopLook is what the shop-room and storefront kinds share: the shop page
// by name, and the owner's rule, since only the owner arranges a shop.
type shopLook struct {
	Svc      ShopLooksAPI
	Entities EntityLookup
	Owners   CampaignOwnerAPI
}

func (s shopLook) shop(ctx context.Context, campaignID string, a Actor, r Record, what string) (string, string, string) {
	if a.Role < permissions.RoleOwner || s.Owners == nil {
		return "", "", "only the campaign's owner can change a shop's " + what
	}
	if ok, err := s.Owners.IsOwner(ctx, campaignID, a.UserID); err != nil || !ok {
		return "", "", "only the campaign's owner can change a shop's " + what
	}
	name := shopName(r)
	if name == "" {
		return "", "", "name the shop's page with `shop`"
	}
	if r.Action == ActionDelete {
		return "", "", "a shop's " + what + " can't be removed by a block; change what you want instead"
	}
	if r.Made[entities.Slugify(name)] {
		return "", "", quote(name) + " is made in this paste; set its " + what + " in a later paste, once the page exists"
	}
	e, err := s.Entities.GetBySlug(ctx, campaignID, entities.Slugify(name))
	if err != nil || e == nil || e.CampaignID != campaignID {
		return "", "", "there is no page called " + quote(name)
	}
	if e.TypeSlug != "shop" {
		return "", "", quote(e.Name) + " is not a shop page"
	}
	return e.ID, e.Name, ""
}

func shopName(r Record) string {
	if s := strings.TrimSpace(r.Str("shop")); s != "" {
		return s
	}
	return strings.TrimSpace(r.Name)
}

// pickWord reads one word of a fixed list by its key or one of its other
// words; words maps each word to its key.
func pickWord(key, s string, words map[string]string) (string, error) {
	w := strings.ToLower(strings.TrimSpace(s))
	if k, ok := words[w]; ok {
		return k, nil
	}
	keys := map[string]bool{}
	for _, k := range words {
		if k != "" {
			keys[k] = true
		}
	}
	list := make([]string, 0, len(keys))
	for k := range keys {
		list = append(list, k)
	}
	sort.Strings(list)
	return "", badRequestf("%s: %q is not one of %s", key, s, strings.Join(list, ", "))
}

// wordsOf is a fixed list whose keys are their own words.
func wordsOf(keys ...string) map[string]string {
	m := make(map[string]string, len(keys))
	for _, k := range keys {
		m[k] = k
	}
	return m
}

func with(m map[string]string, extra ...string) map[string]string {
	for i := 0; i+1 < len(extra); i += 2 {
		m[extra[i]] = extra[i+1]
	}
	return m
}

// lines reads a list of short lines; `none` or an empty list is no lines.
func lines(key string, v any) ([]string, error) {
	if s, ok := v.(string); ok && isNone(s) {
		return []string{}, nil
	}
	var list []any
	switch x := v.(type) {
	case nil:
		return []string{}, nil
	case []any:
		list = x
	default:
		list = []any{x}
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		switch item.(type) {
		case string, int, float64, bool:
		default:
			return nil, badRequestf("%s: write each line as plain words", key)
		}
		if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

// lookError names which block's part the armory's normalizer refused.
func lookError(what string, err error) string {
	return what + ": " + planError(err)
}

// ---- shop-room ----

// The room's fixed lists, as the shop room's Arrange panel offers them; the
// armory's normalizer is the authority and refuses anything else.
var (
	roomTypes = with(wordsOf("forge", "apothecary", "jeweler", "market", "library", "general"),
		"smithy", "forge", "blacksmith", "forge", "alchemist", "apothecary", "jeweller", "jeweler",
		"books", "library", "bookshop", "library", "general store", "general")
	roomSettings    = wordsOf("room", "tower", "tree", "burrow", "cave")
	roomSizes       = with(wordsOf("s", "m", "l"), "small", "s", "medium", "m", "large", "l")
	roomFurnitures  = wordsOf("sparse", "normal", "packed")
	roomDecorations = wordsOf("none", "some", "lots")
	roomPalettes    = wordsOf("oak", "ember", "moss", "gilt", "slate", "dusk")
	roomLooks       = with(wordsOf("lit", "paper", "counter"), "lit room", "lit", "pop-up book", "paper", "at the counter", "counter")
	roomMoods       = with(wordsOf("cozy", "eldritch", "candy", "haunted", "festive", "underwater", "fey", "desert", "dwarven"), "none", "")
	roomManners     = with(wordsOf("warm", "gruff", "shrewd", "nervous", "grand"), "none", "")
	roomMoments     = []string{"hello", "look", "buy", "poor", "bye", "refuse"}
)

// emptyRoom is the room a shop has before anyone arranges it, as the shop
// room widget starts it; its furniture is laid out when it first opens.
const emptyRoom = `{"version":1,"roomType":"general","setting":"room","size":"m","furniture":"normal","decorations":"some","palette":"oak","look":"lit","mood":"","effects":null,"seeds":{"room":1,"goods":1,"deco":1},"pieces":[],"decor":[],"items":{},"portrait":null,"lines":[]}`

// ShopRoomKind dresses a shop's room: what kind of shop it is, its look and
// mood, and what the keeper says. Pieces are never moved: a change to what
// the room is marks it to lay its furniture out again when it next opens,
// around the pieces moved by hand, as rerolling it in Arrange does.
type ShopRoomKind struct{ shopLook }

// NewShopRoomKind is the shop-room kind over the shop services.
func NewShopRoomKind(svc ShopLooksAPI, e EntityLookup, owners CampaignOwnerAPI) ShopRoomKind {
	return ShopRoomKind{shopLook{Svc: svc, Entities: e, Owners: owners}}
}

func (ShopRoomKind) Name() string  { return "shop-room" }
func (ShopRoomKind) Label() string { return "Shop room" }
func (ShopRoomKind) Doc() string {
	return "The room players step into from a shop page's Shop block, matched by `shop` (the shop's page name). Keys, each optional: " +
		"`type` (forge, apothecary, jeweler, market, library, general), `setting` (room, tower, tree, burrow, cave), `size` (small, medium, large), " +
		"`furniture` (sparse, normal, packed), `decorations` (none, some, lots), `palette` (oak, ember, moss, gilt, slate, dusk), " +
		"`look` (lit, the lit room; paper, a pop-up book; counter, standing at the counter), `mood` (cozy, eldritch, candy, haunted, festive, underwater, fey, desert, dwarven, or none), " +
		"`manner` (the keeper's manner, which gives them lines for every moment: warm, gruff, shrewd, nervous, grand, or none), " +
		"`says` (the keeper's own lines for a moment of the visit, up to 5 each, which win over the manner's: `hello` on arriving, `look` when something goes on the pile, `buy` after a purchase, `poor` when the pile outgrows the purse, `bye` on leaving, `refuse` for a shop that won't serve the party; `none` gives a moment back to the manner) " +
		"`chatter` (up to 10 lines the keeper says when pressed) and `counter` (the theme of the counter look, below). " +
		"A key you leave out keeps what the room has; `says` changes only the moments it names. Changing `type`, `setting`, `size`, `furniture` or `mood` lays the furniture out again the next time the room opens, keeping every piece the owner moved by hand; the goods, prices and stock are `shop-stock`. A room can't be removed by a block.\n\n" +
		counterDoc + "\n\n" +
		"```\n---\nkind: shop-room\nshop: The Rusty Anchor\ntype: general\nmood: cozy\nmanner: gruff\nsays:\n  hello: [\"Wipe your boots.\", \"We're open. Barely.\"]\n  poor: [\"Coin first, then rope.\"]\nchatter: [\"Storm's coming. Always is.\"]\ncounter:\n  start: smithy\n  display: pegboard\n  accent: \"#ff9a50\"\n  add: [sparks]\n---\n```"
}

// Title names the review row: "Room of The Rusty Anchor".
func (ShopRoomKind) Title(r Record) string { return "Room of " + shopName(r) }

func (k ShopRoomKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	p, _, _ := k.plan(ctx, campaignID, a, r)
	return p
}

func (k ShopRoomKind) plan(ctx context.Context, campaignID string, a Actor, r Record) (Plan, string, json.RawMessage) {
	id, name, e := k.shop(ctx, campaignID, a, r, "room")
	if e != "" {
		return Plan{Error: e}, "", nil
	}
	raw, err := k.Svc.Room(ctx, campaignID, id, a)
	if err != nil {
		return Plan{Error: planError(err)}, "", nil
	}
	fresh := raw == nil
	if fresh {
		raw = json.RawMessage(emptyRoom)
	}
	var room map[string]any
	if err := json.Unmarshal(raw, &room); err != nil || room == nil {
		return Plan{Error: "the room's saved layout can't be read; arrange it once in the shop"}, "", nil
	}
	var said []string
	shape, mood := fresh, false
	for _, f := range []struct {
		key, field string
		words      map[string]string
		shape      bool
	}{
		{"type", "roomType", roomTypes, true}, {"setting", "setting", roomSettings, true}, {"size", "size", roomSizes, true},
		{"furniture", "furniture", roomFurnitures, true}, {"decorations", "decorations", roomDecorations, false},
		{"palette", "palette", roomPalettes, false}, {"look", "look", roomLooks, false}, {"mood", "mood", roomMoods, false},
		{"manner", "manner", roomManners, false},
	} {
		if !r.Has(f.key) {
			continue
		}
		v, err := pickWord(f.key, r.Str(f.key), f.words)
		if err != nil {
			return Plan{Error: planError(err)}, "", nil
		}
		// The server leaves an empty mood or manner out of the saved room, so a
		// missing field reads as "" and "none" on it is no change.
		if cur, _ := room[f.field].(string); cur == v {
			continue
		}
		room[f.field] = v
		shape = shape || f.shape
		if f.key == "mood" {
			mood = true
			// The new mood's own atmosphere, as picking it in Arrange gives.
			room["effects"] = nil
		}
		word := v
		if word == "" {
			word = "none"
		}
		said = append(said, f.key+" "+word)
	}
	if r.Has("says") {
		said = append(said, "")
		m, _ := r.Fields["says"].(map[string]any)
		if m == nil {
			return Plan{Error: "says: write each moment with its lines, like `hello: [\"Welcome.\"]`"}, "", nil
		}
		moments, _ := room["moments"].(map[string]any)
		if moments == nil {
			moments = map[string]any{}
		}
		var named []string
		for moment, v := range m {
			moment = strings.ToLower(strings.TrimSpace(moment))
			if !oneOf(roomMoments, moment) {
				return Plan{Error: fmt.Sprintf("says: %q is not a moment; the moments are %s", moment, strings.Join(roomMoments, ", "))}, "", nil
			}
			ls, err := lines("says."+moment, v)
			if err != nil {
				return Plan{Error: planError(err)}, "", nil
			}
			was, had := moments[moment]
			if len(ls) == 0 {
				if had {
					delete(moments, moment)
					named = append(named, moment+" back to the manner")
				}
				continue
			}
			if sameLines(was, ls) {
				continue
			}
			moments[moment] = ls
			named = append(named, fmt.Sprintf("%s %s", moment, plural(len(ls), "line", "lines")))
		}
		sort.Strings(named)
		room["moments"] = moments
		if len(named) > 0 {
			said[len(said)-1] = "the keeper says: " + strings.Join(named, ", ")
		} else {
			said = said[:len(said)-1]
		}
	}
	if r.Has("chatter") {
		ls, err := lines("chatter", r.Fields["chatter"])
		if err != nil {
			return Plan{Error: planError(err)}, "", nil
		}
		if !sameLines(room["lines"], ls) {
			room["lines"] = ls
			said = append(said, plural(len(ls), "line", "lines")+" of chatter")
		}
	}
	if r.Has("counter") {
		note, err := k.counter(room, r.Fields["counter"])
		if err != nil {
			return Plan{Error: planError(err)}, "", nil
		}
		if note != "" {
			said = append(said, note)
		}
	}
	switch {
	case shape:
		room["furnish"] = "room"
	case mood && room["furnish"] != "room":
		room["furnish"] = "mood"
	}
	out, err := json.Marshal(room)
	if err != nil {
		return Plan{Error: planError(err)}, "", nil
	}
	if err := k.Svc.SaveRoom(ctx, campaignID, id, a, out, true); err != nil {
		return Plan{Error: lookError("the room", err)}, "", nil
	}
	if len(said) == 0 {
		return Plan{Summary: "changes nothing in " + name + "'s room"}, id, out
	}
	sum := name + "'s room: " + strings.Join(said, "; ")
	if fresh {
		sum = "sets up " + sum
	}
	p := Plan{Summary: sum}
	if (shape && !fresh) || mood {
		p.Warnings = append(p.Warnings, "The furniture is laid out again the next time the room opens; pieces moved by hand stay where they are.")
	}
	return p, id, out
}

func (k ShopRoomKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	p, id, layout := k.plan(ctx, campaignID, a, r)
	if p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	return k.Svc.SaveRoom(ctx, campaignID, id, a, layout, false)
}

// Export is empty: the `stock` lookup prints a shop's room as a block.
func (ShopRoomKind) Export(context.Context, string, Actor) (string, error) { return "", nil }

// roomBlock writes a saved room in the block's own keys, so the AI can send
// it back changed.
func roomBlock(shop string, raw json.RawMessage) string {
	var room struct {
		RoomType, Setting, Size, Furniture, Decorations, Palette, Look, Mood, Manner string
		Moments                                                                      map[string][]string
		Lines                                                                        []string
		Counter                                                                      json.RawMessage
	}
	if json.Unmarshal(raw, &room) != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("---\nkind: shop-room\naction: update\nshop: " + quote(shop) + "\n")
	for _, kv := range [][2]string{{"type", room.RoomType}, {"setting", room.Setting}, {"size", map[string]string{"s": "small", "m": "medium", "l": "large"}[room.Size]},
		{"furniture", room.Furniture}, {"decorations", room.Decorations}, {"palette", room.Palette}, {"look", room.Look}, {"mood", room.Mood}, {"manner", room.Manner}} {
		if kv[1] != "" {
			b.WriteString(kv[0] + ": " + kv[1] + "\n")
		}
	}
	if len(room.Moments) > 0 {
		b.WriteString("says:\n")
		for _, m := range roomMoments {
			if ls := room.Moments[m]; len(ls) > 0 {
				b.WriteString("  " + m + ": " + quotedList(ls) + "\n")
			}
		}
	}
	if len(room.Lines) > 0 {
		b.WriteString("chatter: " + quotedList(room.Lines) + "\n")
	}
	b.WriteString(counterBlock(room.Counter))
	b.WriteString("---\n")
	return b.String()
}

func quotedList(ls []string) string {
	q := make([]string, len(ls))
	for i, l := range ls {
		q[i] = quote(l)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func oneOf(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ---- storefront ----

var (
	frontStyles  = wordsOf("timber", "stone", "plaster", "painted")
	frontAwnings = wordsOf("none", "plain", "stripe")
	frontSigns   = with(wordsOf("hang", "board", "banner"), "hanging", "hang", "hanging sign", "hang", "fascia", "board")
	frontWares   = wordsOf("bottle", "sword", "scroll", "gem", "bread", "shield", "hammer", "herb")
	frontIcons   = with(wordsOf("bottle", "sword", "scroll", "gem", "bread", "shield", "hammer", "herb"), "none", "")
	frontClosed  = wordsOf("shutters", "curtains", "boards", "board", "dark")
)

// StorefrontKind dresses how a shop's front looks on the Shops street.
type StorefrontKind struct{ shopLook }

// NewStorefrontKind is the storefront kind over the shop services.
func NewStorefrontKind(svc ShopLooksAPI, e EntityLookup, owners CampaignOwnerAPI) StorefrontKind {
	return StorefrontKind{shopLook{Svc: svc, Entities: e, Owners: owners}}
}

func (StorefrontKind) Name() string  { return "storefront" }
func (StorefrontKind) Label() string { return "Storefront" }
func (StorefrontKind) Doc() string {
	return "How a shop's front looks on the Shops street, matched by `shop` (the shop's page name). Keys, each optional: " +
		"`style` (timber, stone, plaster, painted), `colour` (the walls, as `#rrggbb`), `trim` (`#rrggbb` or none), " +
		"`awning` (none, plain, stripe) and `awning_colour`, `sign` (hang, a swinging board; board, over the door; banner), " +
		"`sign_words` (at most 24 characters in place of the shop's name; none for the name), `sign_picture` (bottle, sword, scroll, gem, bread, shield, hammer, herb, or none), " +
		"`wares` (up to 3 of bottle, sword, scroll, gem, bread, shield, hammer, herb, shown in the windows), `lamp` (true or false), " +
		"`closed` (the look while the shop is shut: shutters, curtains, boards, board, dark) with `shutter_colour`, `curtain_colour` and `closed_note` (at most 26 characters on the board). " +
		"A key you leave out keeps what the front has; a shop that was never dressed starts from the street's default front. A front can't be removed by a block.\n\n" +
		"```\n---\nkind: storefront\nshop: The Rusty Anchor\nstyle: timber\ncolour: \"#6b4f3a\"\nawning: stripe\nawning_colour: \"#2f5d50\"\nsign: hang\nsign_picture: hammer\nwares: [hammer, shield]\nclosed: board\nclosed_note: Back at dusk\n---\n```"
}

// Title names the review row: "Storefront of The Rusty Anchor".
func (StorefrontKind) Title(r Record) string { return "Storefront of " + shopName(r) }

func (k StorefrontKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	p, _, _ := k.plan(ctx, campaignID, a, r)
	return p
}

func (k StorefrontKind) plan(ctx context.Context, campaignID string, a Actor, r Record) (Plan, string, json.RawMessage) {
	id, name, e := k.shop(ctx, campaignID, a, r, "storefront")
	if e != "" {
		return Plan{Error: e}, "", nil
	}
	raw, saved, err := k.Svc.Front(ctx, campaignID, id, a)
	if err != nil {
		return Plan{Error: planError(err)}, "", nil
	}
	var front map[string]any
	if err := json.Unmarshal(raw, &front); err != nil || front == nil {
		return Plan{Error: "the storefront can't be read; dress it once on the Shops street"}, "", nil
	}
	var said []string
	set := func(key, field string, v any) {
		if front[field] == v {
			return
		}
		front[field] = v
		w := fmt.Sprint(v)
		if w == "" {
			w = "none"
		}
		said = append(said, strings.ReplaceAll(key, "_", " ")+" "+w)
	}
	for _, f := range []struct {
		key, field string
		words      map[string]string
	}{
		{"style", "style", frontStyles}, {"awning", "awning", frontAwnings}, {"sign", "sign", frontSigns},
		{"sign_picture", "icon", frontIcons}, {"closed", "closed", frontClosed},
	} {
		if r.Has(f.key) {
			v, err := pickWord(f.key, r.Str(f.key), f.words)
			if err != nil {
				return Plan{Error: planError(err)}, "", nil
			}
			set(f.key, f.field, v)
		}
	}
	colours := []struct{ key, field string }{
		{"colour", "colour"}, {"trim", "trim"}, {"awning_colour", "awningColour"}, {"shutter_colour", "shutterColour"}, {"curtain_colour", "curtainColour"},
	}
	for _, c := range colours {
		key := c.key
		if !r.Has(key) {
			key = strings.Replace(c.key, "colour", "color", 1)
			if !r.Has(key) {
				continue
			}
		}
		v := strings.ToLower(strings.TrimSpace(r.Str(key)))
		if isNone(v) && c.key != "colour" {
			v = ""
		} else if !hexColor(v) {
			return Plan{Error: fmt.Sprintf("%s: write a colour as #rrggbb, like #6b4f3a", key)}, "", nil
		}
		set(c.key, c.field, v)
	}
	for _, t := range []struct{ key, field string }{{"sign_words", "words"}, {"closed_note", "closedNote"}} {
		if r.Has(t.key) {
			v := strings.TrimSpace(r.Str(t.key))
			if isNone(v) {
				v = ""
			}
			set(t.key, t.field, v)
		}
	}
	if r.Has("wares") {
		var ws []any
		for _, w := range r.List("wares") {
			if isNone(fmt.Sprint(w)) {
				continue
			}
			v, err := pickWord("wares", fmt.Sprint(w), frontWares)
			if err != nil {
				return Plan{Error: planError(err)}, "", nil
			}
			ws = append(ws, v)
		}
		if len(ws) > 3 {
			return Plan{Error: "wares: the windows show at most 3"}, "", nil
		}
		if ws == nil {
			ws = []any{}
		}
		if !sameLines(front["wares"], strs(ws)) {
			front["wares"] = ws
			said = append(said, "wares "+joinAnd(anyWords(ws)))
		}
	}
	if r.Has("lamp") {
		on, ok := r.Bool("lamp")
		if !ok {
			return Plan{Error: "lamp: true or false"}, "", nil
		}
		set("lamp", "lamp", on)
	}
	out, err := json.Marshal(front)
	if err != nil {
		return Plan{Error: planError(err)}, "", nil
	}
	if err := k.Svc.SaveFront(ctx, campaignID, id, a, out, true); err != nil {
		return Plan{Error: lookError("the storefront", err)}, "", nil
	}
	if len(said) == 0 {
		return Plan{Summary: "changes nothing on " + name + "'s storefront"}, id, out
	}
	sum := name + "'s storefront: " + strings.Join(said, "; ")
	if !saved {
		sum = "dresses " + sum + " (the rest stays the street's default)"
	}
	return Plan{Summary: sum}, id, out
}

func (k StorefrontKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	p, id, front := k.plan(ctx, campaignID, a, r)
	if p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	return k.Svc.SaveFront(ctx, campaignID, id, a, front, false)
}

// Export is empty: the `stock` lookup prints a shop's storefront as a block.
func (StorefrontKind) Export(context.Context, string, Actor) (string, error) { return "", nil }

func hexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

// frontBlock writes a storefront in the block's own keys.
func frontBlock(shop string, raw json.RawMessage) string {
	var f struct {
		Style, Colour, Trim, Awning, AwningColour, Sign, Words, Icon, Closed, ShutterColour, CurtainColour, ClosedNote string
		Wares                                                                                                          []string
		Lamp                                                                                                           bool
	}
	if json.Unmarshal(raw, &f) != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("---\nkind: storefront\nshop: " + quote(shop) + "\n")
	for _, kv := range [][2]string{{"style", f.Style}, {"colour", f.Colour}, {"trim", f.Trim}, {"awning", f.Awning}, {"awning_colour", f.AwningColour},
		{"sign", f.Sign}, {"sign_words", f.Words}, {"sign_picture", f.Icon}, {"closed", f.Closed}, {"shutter_colour", f.ShutterColour},
		{"curtain_colour", f.CurtainColour}, {"closed_note", f.ClosedNote}} {
		if kv[1] != "" {
			b.WriteString(kv[0] + ": " + quote(kv[1]) + "\n")
		}
	}
	b.WriteString("wares: [" + strings.Join(f.Wares, ", ") + "]\n")
	fmt.Fprintf(&b, "lamp: %v\n---\n", f.Lamp)
	return b.String()
}

func strs(vs []any) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = fmt.Sprint(v)
	}
	return out
}

func anyWords(vs []any) []string {
	if len(vs) == 0 {
		return []string{"none"}
	}
	return strs(vs)
}

// sameLines compares stored lines (decoded JSON) with written ones.
func sameLines(stored any, ls []string) bool {
	list, _ := stored.([]any)
	if len(list) != len(ls) {
		return len(list) == 0 && len(ls) == 0
	}
	for i, v := range list {
		if fmt.Sprint(v) != ls[i] {
			return false
		}
	}
	return true
}
