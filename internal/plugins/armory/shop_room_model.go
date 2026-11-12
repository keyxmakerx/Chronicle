package armory

import "encoding/json"

// maxShopRoomBytes caps the request body. A full 80-piece, 500-item layout is
// well under this; the cap only stops a client from parking arbitrary data in
// the row.
const maxShopRoomBytes = 64 << 10

// ShopRoomLayout is the typed form of a saved shop room. Layouts are decoded
// into this and re-marshalled, never stored as the client sent them, so the
// stored document can only contain fields and values validated here.
type ShopRoomLayout struct {
	Version     int    `json:"version"`
	RoomType    string `json:"roomType"`
	Setting     string `json:"setting"`
	Size        string `json:"size"`
	Furniture   string `json:"furniture"`
	Decorations string `json:"decorations"`
	Palette     string `json:"palette"`
	// Look is how the room is drawn: "lit" (the isometric room), "paper" (a
	// pop-up book) or "counter" (standing at the counter, able to turn round to
	// the shop floor). Empty, from layouts saved before there was a choice, is lit.
	Look string `json:"look"`
	// Mood themes the room's furniture and atmosphere. Empty, from layouts
	// saved before moods existed, is no mood.
	Mood string `json:"mood"`
	// Effects tunes the atmosphere. Nil, from older layouts, means the widget's
	// defaults; a pointer keeps "never set" distinct from "all zero".
	Effects  *ShopRoomEffects            `json:"effects"`
	Seeds    ShopRoomSeeds               `json:"seeds"`
	Pieces   []ShopRoomPiece             `json:"pieces"`
	Decor    []ShopRoomDecor             `json:"decor"`
	Items    map[string]ShopRoomItemLook `json:"items"`
	Portrait *ShopRoomPortrait           `json:"portrait"`
	Lines    []string                    `json:"lines"`
	// Manner is the keeper's manner, which supplies starting lines for each
	// moment of a visit; empty means none. Moments holds the GM's own lines
	// per moment (keys in shopRoomMoments), which win over the manner's.
	Manner  string              `json:"manner,omitempty"`
	Moments map[string][]string `json:"moments,omitempty"`
	// KeeperSpot is where the paper look's keeper stands behind the counter:
	// "left", "middle" or "right". Empty is the middle, as before there was a choice.
	KeeperSpot string `json:"keeperSpot,omitempty"`
	// Counter is the counter look's theme. Nil means the starter for the
	// shop's type, so a shop that never opened the builder is drawn as before.
	Counter *ShopRoomCounter `json:"counter,omitempty"`
	// Rolls is what players may roll at this shop's counter and how hard the
	// owner makes it. Nil means both rolls are off. Nothing a roll can reveal
	// lives here: the layout is read by every player who can see the shop.
	Rolls *ShopRoomRolls `json:"rolls,omitempty"`
	// Furnish asks the widget to lay out the furniture when the room opens,
	// because something other than Arrange changed what the room is (the AI
	// connection): "room" rebuilds it around pieces moved by hand, as
	// rerolling the room does, and "mood" swaps the mood's pieces only. The
	// widget never sends it, so the owner's next save in Arrange clears it.
	Furnish string `json:"furnish,omitempty"`
}

// ShopRoomEffects is the room's atmosphere. The int fields are percentages of
// the default look (100 = unchanged), so a GM can dim or boost each one.
type ShopRoomEffects struct {
	Shadows  int  `json:"shadows"`
	Warmth   int  `json:"warmth"`
	Window   int  `json:"window"`
	Haze     int  `json:"haze"`
	Vignette int  `json:"vignette"`
	Dust     bool `json:"dust"`
	Flicker  bool `json:"flicker"`
	Embers   bool `json:"embers"`
	Smoke    bool `json:"smoke"`
	// Weather and Time are "" for none/default.
	Weather string `json:"weather"`
	Time    string `json:"time"`
}

// UnmarshalJSON starts from the default look, so a field the body leaves out
// keeps its default instead of becoming zero (no shadows, no dust).
func (e *ShopRoomEffects) UnmarshalJSON(b []byte) error {
	type plain ShopRoomEffects
	v := plain{Shadows: 100, Warmth: 100, Window: 100, Haze: 100, Vignette: 100, Dust: true, Flicker: true, Embers: true}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*e = ShopRoomEffects(v)
	return nil
}

// ShopRoomSeeds drive the widget's deterministic procedural layout, so a
// room looks the same for every viewer.
type ShopRoomSeeds struct {
	Room  int `json:"room"`
	Goods int `json:"goods"`
	Deco  int `json:"deco"`
}

// ShopRoomPiece is one placed furniture piece. Coordinates are in room tiles.
type ShopRoomPiece struct {
	ID     int     `json:"id"`
	Kind   string  `json:"kind"`
	Wall   string  `json:"wall"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Off    float64 `json:"off"`
	Len    float64 `json:"len"`
	W      float64 `json:"w"`
	D      float64 `json:"d"`
	Pinned bool    `json:"pinned"`
}

// ShopRoomDecor is a decoration the GM placed by hand: an icon standing on
// one of a furniture piece's spots (Spot indexes that piece's spots in
// drawing order). It is dropped when its piece is gone.
type ShopRoomDecor struct {
	Piece int    `json:"piece"`
	Spot  int    `json:"spot"`
	Icon  string `json:"icon"`
}

// ShopRoomItemLook is how one shop good (a "sells" relation) is drawn. Both
// fields may be empty, meaning the widget picks a default.
type ShopRoomItemLook struct {
	Icon  string `json:"icon"`
	Color string `json:"color"`
}

// ShopRoomPortrait positions the shopkeeper portrait, as percentages.
type ShopRoomPortrait struct {
	Left float64 `json:"left"`
	Top  float64 `json:"top"`
}

// ShopRoomCounter is the counter look's room as three layers: the skeleton
// (back wall, opening, how goods are shown, the counter), the theme (wall
// material, colours, light, outside) and the props. The widget draws every
// part from these values, so each enum must match CM_OPT and CM_PROPS in
// static/js/widgets/shop_room_counter.js.
type ShopRoomCounter struct {
	Name     string                  `json:"name"`
	Version  int                     `json:"version"`
	Skeleton ShopRoomCounterSkeleton `json:"skeleton"`
	Theme    ShopRoomCounterTheme    `json:"theme"`
	Props    []string                `json:"props"`
}

// ShopRoomCounterSkeleton is the room's shape, before any paint.
type ShopRoomCounterSkeleton struct {
	Shape   string `json:"shape"`
	Opening string `json:"opening"`
	Display string `json:"display"`
	Counter string `json:"counter"`
}

// ShopRoomCounterTheme is the paint and the light. Colours are #rrggbb;
// Warmth runs 0 (cool) to 1 (warm).
type ShopRoomCounterTheme struct {
	Wall    string  `json:"wall"`
	Wood    string  `json:"wood"`
	Cloth   string  `json:"cloth"`
	Accent  string  `json:"accent"`
	Trim    string  `json:"trim"`
	Light   string  `json:"light"`
	Warmth  float64 `json:"warmth"`
	Outside string  `json:"outside"`
}

// ShopRoomRolls is the owner's settings for the counter rolls. Every number is
// a default the owner may change. Great, Middling and Poor are percentages of
// a good's price: a great haggle takes Great off, a middling one Middling, a
// poor one adds Poor.
type ShopRoomRolls struct {
	Haggle bool `json:"haggle"`
	Look   bool `json:"look"`
	// Stubborn is how hard the keeper is to move: "easy", "usual" or "hard".
	Stubborn string `json:"stubborn"`
	// HaggleStat and LookStat are sheet field keys; empty is the game
	// system's own choice.
	HaggleStat string `json:"haggleStat"`
	LookStat   string `json:"lookStat"`
	Great      int    `json:"great"`
	Middling   int    `json:"middling"`
	Poor       int    `json:"poor"`
}

// Defaults for the counter rolls, used for any number a saved layout leaves out.
const (
	defaultRollStubborn = "usual"
	defaultRollGreat    = 20
	defaultRollMiddling = 10
	defaultRollPoor     = 10
)

// UnmarshalJSON starts from the defaults, so a body that leaves out a number
// keeps its default instead of becoming a free haggle (0%) or an unreadable
// keeper; the two switches still default to off.
func (r *ShopRoomRolls) UnmarshalJSON(b []byte) error {
	type plain ShopRoomRolls
	v := plain{Stubborn: defaultRollStubborn, Great: defaultRollGreat, Middling: defaultRollMiddling, Poor: defaultRollPoor}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = ShopRoomRolls(v)
	return nil
}
