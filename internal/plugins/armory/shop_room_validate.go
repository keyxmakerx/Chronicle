package armory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	maxShopRoomPieces = 80
	maxShopRoomDecor  = 120
	maxShopRoomSpot   = 63
	maxShopRoomItems  = 500
	maxShopRoomLines  = 10
	maxShopRoomLine   = 200
	// Lines per moment of a visit; a handful is enough for variety.
	maxShopRoomMomentLines = 5
	maxShopRoomSeed        = 1_000_000_000
	maxShopRoomPieceID     = 100_000
	maxShopRoomRelID       = 2_147_483_647
	shopRoomCoordMin       = -1.0
	shopRoomCoordMax       = 20.0
	// Effect levels are percentages of the default look; 250 leaves room to
	// exaggerate without letting a client store an absurd value.
	maxShopRoomEffect = 250
	// The counter theme's name is a label, and its props are a short list of
	// known ids, so both caps are far above anything a builder writes.
	maxShopRoomCounterName  = 60
	maxShopRoomCounterProps = 24
	// A haggle's best and middling cut and a failed haggle's markup. A cut
	// cannot reach 100% (a free good), and a failure may at most triple a price.
	maxShopRollCut  = 90
	maxShopRollPoor = 200
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
	shopRoomLooks       = setOf("lit", "paper", "counter", "")
	shopRoomMoods       = setOf("", "cozy", "eldritch", "candy", "haunted", "festive", "underwater", "fey", "desert", "dwarven")
	shopRoomWeathers    = setOf("", "rain", "snow")
	shopRoomTimes       = setOf("", "dawn", "dusk", "night")
	// Must match MANNERS and MOMENTS in static/js/widgets/shop_room_keeper.js.
	shopRoomManners     = setOf("", "warm", "gruff", "shrewd", "nervous", "grand")
	shopRoomKeeperSpots = setOf("", "left", "middle", "right")
	shopRoomMoments     = setOf("hello", "look", "buy", "poor", "bye", "refuse")
	shopRoomFurnish     = setOf("", "room", "mood")
	shopRollStubborn    = setOf("easy", "usual", "hard")
	shopRoomKinds       = setOf("shelf", "rack", "cabinet", "bookcase", "forge", "window", "herbs", "counter", "table", "barrel", "crate", "anvil", "glass", "stall", "rug", "pedestal", "lamp", "sack",
		"tentacle", "monolith", "circle", "cane", "gumdrop", "lolly", "ghost", "coffin", "gift", "pine", "coral", "kelp", "mushroom", "bloom", "urn", "palm", "pillar", "brazier")
	shopRoomWalls   = setOf("Y", "X", "")
	shopRoomColors  = setOf("steel", "iron", "gold", "silver", "red", "green", "teal", "blue", "violet", "leather", "paper", "cloth", "wood", "dark", "bone", "honey", "")
	shopRoomIconSet = func() map[string]bool {
		m := setOf(shopRoomIcons...)
		m[""] = true
		return m
	}()
)

// The counter look's layers; must match CM_OPT and CM_PROPS in
// static/js/widgets/shop_room_counter.js once its parts packs
// (shop_room_parts_*.js) have loaded.
var (
	shopRoomCounterShapes = setOf("square", "round", "open-air", "alcove",
		"cavern", "hold", "hollow", "tunnel", "glade", "void", "ribs", "taproom", "parlour", "study",
		"barn", "chapel", "cabin", "ice-cave", "reef", "tent", "gondola", "hut", "works", "colonnade",
		"conservatory")
	shopRoomCounterOpenings = setOf("none", "window-left", "window-right", "window-centre", "round-window", "doorway",
		"porthole", "grate", "knothole", "lava-fall", "planet", "lone-door", "rift", "portal", "hearth", "bay-window",
		"harbour-window", "stable-door", "rose-window", "frost-window", "aurora", "sea-arch", "tent-flap", "cloud-sea", "bog-window", "clock-face",
		"dragon-eye", "sun-disc", "tall-mirror", "garden-door")
	shopRoomCounterDisplays = setOf("shelves", "pegboard", "drawers", "pigeonholes", "domes",
		"roots", "adrift", "orbits", "columns", "tendrils", "circles", "casks", "bolts", "scroll-rack", "hutches",
		"niches", "gear-wall", "ice-ledges", "shells", "carpet-wall", "cargo-nets", "crooked-shelves", "cogs", "hoard", "clouds",
		"curio", "plant-stand")
	shopRoomCounterCounters = setOf("straight", "curved", "stall-table",
		"log", "slab", "adrift", "bar", "cutting", "plan-chest", "feed-stall", "altar", "woodpile", "ice-block",
		"wreck", "carpet", "gunwale", "driftwood", "engine", "coin-drift", "marble-slab", "sheeted", "potting-bench")
	shopRoomCounterWalls = setOf("plaster", "stone", "timber", "brick", "canvas", "night-sky",
		"rock", "hull", "bark", "glade", "basalt", "nebula", "white", "obsidian", "runestone", "daub",
		"damask", "chart", "barnboard", "ashlar", "logs", "glacier", "deepwater", "tent-stripes", "open-sky", "wattle",
		"brass-plate", "lair", "marble", "faded-paper", "glazing")
	shopRoomCounterLights = setOf("lamp", "forge", "candles", "lanterns", "moonlight",
		"crystals", "brazier", "wisps", "star", "halo", "witchfire", "orb", "firelight", "votives", "frostfire",
		"jellyfish", "pierced-lamp", "cauldron", "filament", "sunlight", "candelabra")
	// "live" is the campaign's own sky, read from its calendar where the room is drawn.
	shopRoomCounterOutsides = setOf("day", "dusk", "night", "rain", "live")
	// What stands outside; "town" is the room's own and is saved as no scene.
	shopRoomCounterScenes = setOf("town", "forest", "mountains", "coast", "desert", "swamp", "snowfield", "fields",
		"jungle", "volcano", "ruins", "spires", "harbour", "underdark", "astral", "sky-islands", "castle", "graveyard")
	// Must match CM_PIC in static/js/widgets/shop_room_counter.js.
	shopRoomPictureOn    = setOf("wall", "outside")
	shopRoomPictureFit   = setOf("fill", "fit", "repeat")
	shopRoomCounterProps = setOf("anvil", "alembic", "jars", "crystal", "herbs", "floating-books", "orrery", "ladder", "banner", "sconce", "sparks", "lanterns", "rain", "steam-cart", "motes",
		"fireflies", "drips", "cobwebs", "nets", "chains", "mushrooms", "tentacle", "petals", "butterflies", "embers",
		"smoke", "comets", "floating-stones", "eyes", "glyphs", "magic-circle", "dew", "beads", "rune-chase", "arcs",
		"bubbles", "ghostlights", "crumbs", "dust", "tankards", "keg", "antlers", "mannequin", "ribbons", "shears",
		"globe", "charts", "inkwell", "birdcages", "perch", "feathers", "haybale", "critters", "censer", "casket",
		"stove", "pelts", "sacks", "snowfall", "ice-swan", "fish", "kelp", "caustics", "crab", "glass-lamps",
		"hookah", "cobra", "pennants", "propeller", "grapnel", "toad", "bottle-tree", "gears", "steam-puffs", "automaton",
		"gauge", "glints", "dragon-tail", "doves", "laurel", "lyre", "ghost", "portrait", "phonograph", "hanging-baskets",
		"palm", "watering-can")
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

// NormalizeShopRoomCounter validates one counter theme on its own and returns
// its canonical JSON. The saved-designs library keeps counter themes as
// designs of kind room-theme, and this is its authority for them: the same
// rules a room's `counter` field is held to, so a design can never hold a part
// the counter look would refuse.
func NormalizeShopRoomCounter(raw []byte) (json.RawMessage, error) {
	c, err := decodeShopRoomCounter(raw)
	if err != nil {
		return nil, err
	}
	return marshalShopRoomCounter(c)
}

// NormalizeShopRoomCounterDesign is NormalizeShopRoomCounter for a saved
// design. It drops the owner's picture: a picture is one campaign's file and
// a design travels to other campaigns and the server gallery, where the id
// would point at a file nobody there may open.
func NormalizeShopRoomCounterDesign(raw []byte) (json.RawMessage, error) {
	c, err := decodeShopRoomCounter(raw)
	if err != nil {
		return nil, err
	}
	c.Theme.Picture = nil
	return marshalShopRoomCounter(c)
}

func marshalShopRoomCounter(c *ShopRoomCounter) (json.RawMessage, error) {
	out, err := json.Marshal(c)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

func decodeShopRoomCounter(raw []byte) (*ShopRoomCounter, error) {
	if len(raw) > maxShopRoomBytes {
		return nil, apperror.NewBadRequest("counter theme is too large")
	}
	var c ShopRoomCounter
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&c); err != nil {
		return nil, apperror.NewBadRequest("counter theme is not valid JSON of the expected shape")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, apperror.NewBadRequest("counter theme has trailing data")
	}
	if err := c.validate(); err != nil {
		return nil, apperror.NewBadRequest(err.Error())
	}
	return &c, nil
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
		{"look", l.Look, shopRoomLooks},
		{"mood", l.Mood, shopRoomMoods},
		{"manner", l.Manner, shopRoomManners},
		{"keeperSpot", l.KeeperSpot, shopRoomKeeperSpots},
		{"furnish", l.Furnish, shopRoomFurnish},
	}
	for _, e := range enums {
		if !e.set[e.val] {
			return fmt.Errorf("invalid %s", e.name)
		}
	}
	if ef := l.Effects; ef != nil {
		levels := []struct {
			name string
			v    int
		}{{"shadows", ef.Shadows}, {"warmth", ef.Warmth}, {"window", ef.Window}, {"haze", ef.Haze}, {"vignette", ef.Vignette}}
		for _, lv := range levels {
			if lv.v < 0 || lv.v > maxShopRoomEffect {
				return fmt.Errorf("invalid effects %s", lv.name)
			}
		}
		if !shopRoomWeathers[ef.Weather] {
			return fmt.Errorf("invalid effects weather")
		}
		if !shopRoomTimes[ef.Time] {
			return fmt.Errorf("invalid effects time")
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

	if c := l.Counter; c != nil {
		if err := c.validate(); err != nil {
			return err
		}
	}

	if r := l.Rolls; r != nil {
		if err := r.validate(); err != nil {
			return err
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

	// Moments with no lines left after trimming are dropped, so an emptied
	// moment falls back to the manner instead of silencing the keeper.
	var moments map[string][]string
	for k, ls := range l.Moments {
		if !shopRoomMoments[k] {
			return fmt.Errorf("invalid moment")
		}
		kept := make([]string, 0, len(ls))
		for _, s := range ls {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if utf8.RuneCountInString(s) > maxShopRoomLine {
				return fmt.Errorf("line is too long")
			}
			kept = append(kept, s)
		}
		if len(kept) > maxShopRoomMomentLines {
			return fmt.Errorf("too many lines for a moment")
		}
		if len(kept) > 0 {
			if moments == nil {
				moments = make(map[string][]string, len(l.Moments))
			}
			moments[k] = kept
		}
	}
	l.Moments = moments
	return nil
}

// validate checks the counter theme and rewrites it canonical: trimmed name,
// lowercase colours, warmth clamped to 0..1 and each prop once, in the order
// given. Unknown values are rejected like every other enum here; the widget
// always writes a complete, normalised theme.
func (c *ShopRoomCounter) validate() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported counter version")
	}
	c.Name = strings.TrimSpace(c.Name)
	if utf8.RuneCountInString(c.Name) > maxShopRoomCounterName {
		return fmt.Errorf("counter name is too long")
	}
	enums := []struct {
		name, val string
		set       map[string]bool
	}{
		{"counter shape", c.Skeleton.Shape, shopRoomCounterShapes},
		{"counter opening", c.Skeleton.Opening, shopRoomCounterOpenings},
		{"counter display", c.Skeleton.Display, shopRoomCounterDisplays},
		{"counter counter", c.Skeleton.Counter, shopRoomCounterCounters},
		{"counter wall", c.Theme.Wall, shopRoomCounterWalls},
		{"counter light", c.Theme.Light, shopRoomCounterLights},
		{"counter outside", c.Theme.Outside, shopRoomCounterOutsides},
	}
	for _, e := range enums {
		if !e.set[e.val] {
			return fmt.Errorf("invalid %s", e.name)
		}
	}
	for name, col := range map[string]*string{"wood": &c.Theme.Wood, "cloth": &c.Theme.Cloth, "accent": &c.Theme.Accent, "trim": &c.Theme.Trim} {
		if !validHexColor(*col) {
			return fmt.Errorf("invalid counter %s colour", name)
		}
		*col = strings.ToLower(*col)
	}
	if c.Theme.Scene == "town" {
		c.Theme.Scene = ""
	}
	if c.Theme.Scene != "" && !shopRoomCounterScenes[c.Theme.Scene] {
		return fmt.Errorf("invalid counter scene")
	}
	if p := c.Theme.Picture; p != nil {
		p.ID = strings.ToLower(p.ID)
		if !shopRoomPictureIDRe.MatchString(p.ID) {
			return fmt.Errorf("invalid counter picture")
		}
		if !shopRoomPictureOn[p.On] || !shopRoomPictureFit[p.Fit] {
			return fmt.Errorf("invalid counter picture placement")
		}
	}
	// NaN fails both comparisons, so it lands on the lower bound.
	switch w := c.Theme.Warmth; {
	case w > 1:
		c.Theme.Warmth = 1
	case !(w >= 0):
		c.Theme.Warmth = 0
	}
	if len(c.Props) > maxShopRoomCounterProps {
		return fmt.Errorf("too many counter props")
	}
	props := make([]string, 0, len(c.Props))
	seen := make(map[string]bool, len(c.Props))
	for _, p := range c.Props {
		if !shopRoomCounterProps[p] {
			return fmt.Errorf("invalid counter prop")
		}
		if !seen[p] {
			seen[p] = true
			props = append(props, p)
		}
	}
	c.Props = props
	return nil
}

// shopRollStatRe is a sheet field key, the same slug a game system names its stats with.
// A media file id is a UUID; the room stores the id alone, never a URL.
var shopRoomPictureIDRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var shopRollStatRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

// validate checks the counter roll settings. Stat keys are never looked up
// here: a key the system does not list is allowed, since the sheet decides
// whether it holds a number.
func (r *ShopRoomRolls) validate() error {
	if !shopRollStubborn[r.Stubborn] {
		return fmt.Errorf("invalid rolls stubborn")
	}
	for name, k := range map[string]string{"haggleStat": r.HaggleStat, "lookStat": r.LookStat} {
		if k != "" && !shopRollStatRe.MatchString(k) {
			return fmt.Errorf("invalid rolls %s", name)
		}
	}
	for name, v := range map[string]int{"great": r.Great, "middling": r.Middling} {
		if v < 0 || v > maxShopRollCut {
			return fmt.Errorf("invalid rolls %s", name)
		}
	}
	if r.Poor < 0 || r.Poor > maxShopRollPoor {
		return fmt.Errorf("invalid rolls poor")
	}
	return nil
}

// validHexColor accepts exactly #rrggbb, either case.
func validHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for i := 1; i < 7; i++ {
		if !isHexDigit(s[i]) {
			return false
		}
	}
	return true
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
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
