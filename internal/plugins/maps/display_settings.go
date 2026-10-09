package maps

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// This file owns the per-map display settings: the look of the map page
// (frame, pins, grid, opening view) and the one access rule stored beside them
// (who can draw). They live in one nullable JSON column so a new display knob
// is not a migration, which is why every value is validated and normalised
// here before it is written: the renderer trusts what it reads.
//
// Wire keys are snake_case to match the rest of the maps API (pin_category,
// image_id), not the camelCase default, so one map payload does not mix styles.

// FrameStyle is one of the six frames drawn around the map on the map page.
type FrameStyle struct {
	ID          string
	Label       string
	Description string
}

// FrameStyles is the closed set of frames, in gallery order. Atlas is first
// because it is the default for every campaign that has not chosen.
var FrameStyles = []FrameStyle{
	{"atlas", "Atlas", "A printed map plate: double rule, corner flourishes, grid letters and a title plate."},
	{"arcane", "Arcane", "A warded vellum: glowing runes in the margin that slowly breathe."},
	{"old", "Old", "A worn sea chart: stained parchment, a ragged edge and a wax seal."},
	{"modern", "Modern", "Clean and new: a plain rounded card, nothing in the way of the map."},
	{"futuristic", "Futuristic", "A tactical display: bracketed corners, scan lines and a live position readout."},
	{"gilded", "Gilded", "A gold picture frame for the map you want to show off."},
}

// DefaultFrame is what a campaign uses until its owner picks another.
const DefaultFrame = "atlas"

// IsValidFrame reports whether id names one of the six frames.
func IsValidFrame(id string) bool {
	for _, f := range FrameStyles {
		if f.ID == id {
			return true
		}
	}
	return false
}

// Allowed values for the enum-like settings. Kept as small sets so a stored
// value the renderer does not know can never exist.
const (
	PinStyleDrop = "drop"
	PinStyleSeal = "seal"
	PinStyleFlag = "flag"
	PinStyleDot  = "dot"

	PinSizeSmall  = "s"
	PinSizeMedium = "m"
	PinSizeLarge  = "l"

	PinLabelsAlways = "always"
	PinLabelsHover  = "hover"
	PinLabelsNever  = "never"

	GridNone   = "none"
	GridSquare = "square"
	GridHex    = "hex"

	OpenWhole = "whole"
	OpenLast  = "last"
	OpenSpot  = "spot"

	// DrawWhoOwners limits drawing to campaign owners; DrawWhoScribes (the
	// default, and the behaviour before this setting existed) also lets
	// scribes draw.
	DrawWhoOwners  = "owners"
	DrawWhoScribes = "scribes"

	// The hex layer's terrain art, shown to everyone who sees the map.
	// Detailed is the default and is not stored.
	HexArtRealistic = "real"
	HexArtDetailed  = "detailed"
	HexArtSimple    = "simple"
)

// IsValidHexArt reports whether a is one of the terrain art styles.
func IsValidHexArt(a string) bool {
	return oneOf(a, HexArtRealistic, HexArtDetailed, HexArtSimple)
}

// Bounds for the numeric settings. Out-of-range input is clamped, not
// rejected: a slider that overshoots by a pixel should not fail a save.
const (
	gridSizeMin, gridSizeMax         = 10, 300
	gridStrengthMin, gridStrengthMax = 5, 80
	openZoomMin, openZoomMax         = -3.0, 6.0
	kindLabelMaxRunes                = 30
)

// Default grid values: the resolver fills them in, and the normaliser does not
// store a value equal to them.
const (
	defaultGridSize     = 50
	defaultGridStrength = 30
)

// KindDefaults are the five pin kinds with their built-in labels and colours.
// The ids are the server's pin_category set (and the Foundry module's pin
// types), so this list is closed: a map can rename and recolour a kind but
// never add one.
var KindDefaults = []KindDisplay{
	{ID: "location", Label: "Places", Color: "#2563eb"},
	{ID: "danger", Label: "Danger", Color: "#dc2626"},
	{ID: "treasure", Label: "Treasure", Color: "#d97706"},
	{ID: "quest", Label: "Quests", Color: "#7c3aed"},
	{ID: "note", Label: "Notes", Color: "#0d9488"},
}

// DisplaySettings is the stored per-map document. Every group is optional and
// every field within a group is optional: absent means "the default".
type DisplaySettings struct {
	Frame *FrameDisplay          `json:"frame,omitempty"`
	Pins  *PinDisplay            `json:"pins,omitempty"`
	Kinds map[string]KindDisplay `json:"kinds,omitempty"`
	Grid  *GridDisplay           `json:"grid,omitempty"`
	Open  *OpenDisplay           `json:"open,omitempty"`
	Draw  *DrawDisplay           `json:"draw,omitempty"`
	Hexes *HexesDisplay          `json:"hexes,omitempty"`
}

// FrameDisplay overrides the campaign frame for one map. An empty Style means
// "follow the campaign style"; Tint is a pointer so "unset" (on, the default)
// and an explicit off are distinguishable.
type FrameDisplay struct {
	Style string `json:"style,omitempty"`
	Tint  *bool  `json:"tint,omitempty"`
}

// PinDisplay is the marker shape drawn around the Font Awesome icon, its size,
// and when the pin's name shows on the map.
type PinDisplay struct {
	Style  string `json:"style,omitempty"`
	Size   string `json:"size,omitempty"`
	Labels string `json:"labels,omitempty"`
}

// KindDisplay renames and recolours one of the five pin kinds for this map.
// ID is set only on the resolved copy, never stored: the map key is the id.
type KindDisplay struct {
	ID    string `json:"id,omitempty"`
	Label string `json:"label,omitempty"`
	Color string `json:"color,omitempty"`
}

// GridDisplay is the grid overlay, drawn in map coordinates so it scales with
// zoom. Size is measured as if the map were 1000 wide, so the same number
// looks the same on a 600px sketch and a 6000px poster.
type GridDisplay struct {
	Type     string `json:"type,omitempty"`
	Size     int    `json:"size,omitempty"`
	Strength int    `json:"strength,omitempty"`
}

// OpenDisplay is where the map opens. X and Y are percent of the image (like
// marker coordinates) and Zoom is levels relative to the whole-map view, so a
// stored spot means the same on every screen size.
type OpenDisplay struct {
	Mode string   `json:"mode,omitempty"`
	X    *float64 `json:"x,omitempty"`
	Y    *float64 `json:"y,omitempty"`
	Zoom *float64 `json:"zoom,omitempty"`
}

// DrawDisplay is the draw gate. It is enforced on the server (see
// DrawingService), not only hidden in the UI.
type DrawDisplay struct {
	Who string `json:"who,omitempty"`
}

// HexesDisplay holds the hex layer's settings. PartyWho is enforced on the
// server (HexService.MoveParty), not only hidden in the UI. "Everyone" is not
// offered: letting a player move the party would let them uncover land. Art is
// the terrain style everyone sees; owners set it in Map settings and DM grants
// from the Paint mode, so this group merges per field (see
// MergeDisplaySettings): saving one never resets the other.
type HexesDisplay struct {
	PartyWho string `json:"party_who,omitempty"`
	Art      string `json:"art,omitempty"`
}

// displayGroups is the closed set of top-level keys; anything else in an
// incoming document is dropped.
var displayGroups = []string{"frame", "pins", "kinds", "grid", "open", "draw", "hexes"}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// decodeGroup unmarshals one group's JSON into dst, turning a type mismatch
// into a validation error that names the group.
func decodeGroup(name string, raw json.RawMessage, dst any) error {
	if err := json.Unmarshal(raw, dst); err != nil {
		return apperror.NewValidation("display_settings." + name + " is not valid")
	}
	return nil
}

func normalizeFrame(raw json.RawMessage) (*FrameDisplay, error) {
	var f FrameDisplay
	if err := decodeGroup("frame", raw, &f); err != nil {
		return nil, err
	}
	f.Style = strings.TrimSpace(f.Style)
	if f.Style != "" && !IsValidFrame(f.Style) {
		return nil, apperror.NewValidation("frame style must be one of: atlas, arcane, old, modern, futuristic, gilded")
	}
	// Tint on is the default, so it is not stored; only an explicit off is.
	if f.Tint != nil && *f.Tint {
		f.Tint = nil
	}
	if f.Style == "" && f.Tint == nil {
		return nil, nil
	}
	return &f, nil
}

func normalizePins(raw json.RawMessage) (*PinDisplay, error) {
	var p PinDisplay
	if err := decodeGroup("pins", raw, &p); err != nil {
		return nil, err
	}
	if p.Style != "" && !oneOf(p.Style, PinStyleDrop, PinStyleSeal, PinStyleFlag, PinStyleDot) {
		return nil, apperror.NewValidation("pin style must be one of: drop, seal, flag, dot")
	}
	if p.Size != "" && !oneOf(p.Size, PinSizeSmall, PinSizeMedium, PinSizeLarge) {
		return nil, apperror.NewValidation("pin size must be one of: s, m, l")
	}
	if p.Labels != "" && !oneOf(p.Labels, PinLabelsAlways, PinLabelsHover, PinLabelsNever) {
		return nil, apperror.NewValidation("pin labels must be one of: always, hover, never")
	}
	// Values equal to the default are not stored: absent already means default,
	// and a group that reduces to nothing clears itself back to NULL.
	if p.Style == PinStyleDrop {
		p.Style = ""
	}
	if p.Size == PinSizeMedium {
		p.Size = ""
	}
	if p.Labels == PinLabelsHover {
		p.Labels = ""
	}
	if p == (PinDisplay{}) {
		return nil, nil
	}
	return &p, nil
}

// cleanKindLabel drops control characters and clamps the length. Labels are
// only ever rendered as text (textContent / templ escaping), so this is about
// layout, not markup safety.
func cleanKindLabel(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > kindLabelMaxRunes {
		s = strings.TrimSpace(string(r[:kindLabelMaxRunes]))
	}
	return s
}

func normalizeKinds(raw json.RawMessage) (map[string]KindDisplay, error) {
	var in map[string]KindDisplay
	if err := decodeGroup("kinds", raw, &in); err != nil {
		return nil, err
	}
	out := map[string]KindDisplay{}
	for _, def := range KindDefaults {
		k, ok := in[def.ID]
		if !ok {
			continue // Unknown kind ids are dropped: the set is closed.
		}
		k.ID = ""
		k.Label = cleanKindLabel(k.Label)
		k.Color = strings.ToLower(strings.TrimSpace(k.Color))
		if k.Color != "" && !colorPattern.MatchString(k.Color) {
			return nil, apperror.NewValidation("kind colour must be a valid hex colour (e.g., #2563eb)")
		}
		// Equal to the built-in value means "not renamed / not recoloured".
		if k.Label == def.Label {
			k.Label = ""
		}
		if k.Color == def.Color {
			k.Color = ""
		}
		if k.Label == "" && k.Color == "" {
			continue
		}
		out[def.ID] = k
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func normalizeGrid(raw json.RawMessage) (*GridDisplay, error) {
	var g GridDisplay
	if err := decodeGroup("grid", raw, &g); err != nil {
		return nil, err
	}
	if g.Type != "" && !oneOf(g.Type, GridNone, GridSquare, GridHex) {
		return nil, apperror.NewValidation("grid type must be one of: none, square, hex")
	}
	// No grid means size and strength are moot: store nothing.
	if g.Type == "" || g.Type == GridNone {
		return nil, nil
	}
	if g.Size != 0 {
		g.Size = clampInt(g.Size, gridSizeMin, gridSizeMax)
	}
	if g.Strength != 0 {
		g.Strength = clampInt(g.Strength, gridStrengthMin, gridStrengthMax)
	}
	if g.Size == defaultGridSize {
		g.Size = 0
	}
	if g.Strength == defaultGridStrength {
		g.Strength = 0
	}
	return &g, nil
}

func normalizeOpen(raw json.RawMessage) (*OpenDisplay, error) {
	var o OpenDisplay
	if err := decodeGroup("open", raw, &o); err != nil {
		return nil, err
	}
	if o.Mode != "" && !oneOf(o.Mode, OpenWhole, OpenLast, OpenSpot) {
		return nil, apperror.NewValidation("opening mode must be one of: whole, last, spot")
	}
	if o.Mode == OpenSpot && (o.X == nil || o.Y == nil || o.Zoom == nil) {
		return nil, apperror.NewValidation("choose a spot first: opening at a spot needs a centre and a zoom")
	}
	if o.X != nil {
		v := clampFloat(*o.X, 0, 100)
		o.X = &v
	}
	if o.Y != nil {
		v := clampFloat(*o.Y, 0, 100)
		o.Y = &v
	}
	if o.Zoom != nil {
		v := clampFloat(*o.Zoom, openZoomMin, openZoomMax)
		o.Zoom = &v
	}
	// A spot that is not the active mode is dead weight; keep the document
	// small and unambiguous.
	if o.Mode != OpenSpot {
		o.X, o.Y, o.Zoom = nil, nil, nil
	}
	if o.Mode == "" || o.Mode == OpenWhole {
		return nil, nil
	}
	return &o, nil
}

func normalizeDraw(raw json.RawMessage) (*DrawDisplay, error) {
	var d DrawDisplay
	if err := decodeGroup("draw", raw, &d); err != nil {
		return nil, err
	}
	if d.Who != "" && !oneOf(d.Who, DrawWhoOwners, DrawWhoScribes) {
		return nil, apperror.NewValidation("who can draw must be one of: owners, scribes")
	}
	if d.Who == "" || d.Who == DrawWhoScribes {
		return nil, nil
	}
	return &d, nil
}

func normalizeHexes(raw json.RawMessage) (*HexesDisplay, error) {
	var h HexesDisplay
	if err := decodeGroup("hexes", raw, &h); err != nil {
		return nil, err
	}
	if h.PartyWho != "" && !oneOf(h.PartyWho, PartyWhoOwners, PartyWhoScribes) {
		return nil, apperror.NewValidation("who can move the party must be one of: owners, scribes")
	}
	if h.Art != "" && !IsValidHexArt(h.Art) {
		return nil, apperror.NewValidation("terrain art must be one of: real, detailed, simple")
	}
	// Scribes and Detailed are the defaults, so they are not stored.
	if h.PartyWho == PartyWhoScribes {
		h.PartyWho = ""
	}
	if h.Art == HexArtDetailed {
		h.Art = ""
	}
	if h.PartyWho == "" && h.Art == "" {
		return nil, nil
	}
	return &h, nil
}

// applyGroup validates one incoming group and writes it into ds, or removes it
// when the group normalises to nothing (every field default). raw is never the
// literal null here: the caller handles an explicit null as "clear this group".
func applyGroup(ds *DisplaySettings, name string, raw json.RawMessage) error {
	var err error
	switch name {
	case "frame":
		ds.Frame, err = normalizeFrame(raw)
	case "pins":
		ds.Pins, err = normalizePins(raw)
	case "kinds":
		ds.Kinds, err = normalizeKinds(raw)
	case "grid":
		ds.Grid, err = normalizeGrid(raw)
	case "open":
		ds.Open, err = normalizeOpen(raw)
	case "draw":
		ds.Draw, err = normalizeDraw(raw)
	case "hexes":
		ds.Hexes, err = normalizeHexes(raw)
	}
	return err
}

func clearGroup(ds *DisplaySettings, name string) {
	switch name {
	case "frame":
		ds.Frame = nil
	case "pins":
		ds.Pins = nil
	case "kinds":
		ds.Kinds = nil
	case "grid":
		ds.Grid = nil
	case "open":
		ds.Open = nil
	case "draw":
		ds.Draw = nil
	case "hexes":
		ds.Hexes = nil
	}
}

// IsEmpty reports whether no group is set, i.e. the stored value should be NULL.
func (d *DisplaySettings) IsEmpty() bool {
	return d == nil || (d.Frame == nil && d.Pins == nil && d.Kinds == nil &&
		d.Grid == nil && d.Open == nil && d.Draw == nil && d.Hexes == nil)
}

// MergeDisplaySettings applies an incoming display_settings document to the
// stored one under the partial-update contract, at group granularity:
//
//   - a group key that is absent keeps the stored group;
//   - a group key that is an explicit null clears that group;
//   - a group key with a value replaces that group, validated and normalised.
//
// Groups, not individual fields, are the unit because the sheet edits a group
// at a time and sends only the groups the person touched; replacing a whole
// group is exact, while merging inside one would make "back to the default"
// impossible to say. The one exception is "hexes", which merges per field
// (absent keeps, null or the default clears, a value replaces): its two
// settings are written from different places by different people, so a save
// of one must never carry a stale copy of the other. Unknown top-level keys
// are dropped. The result is nil when nothing is left, so the column goes
// back to NULL.
func MergeDisplaySettings(current *DisplaySettings, incoming json.RawMessage) (*DisplaySettings, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(incoming, &doc); err != nil {
		return nil, apperror.NewValidation("display_settings must be an object")
	}
	next := DisplaySettings{}
	if current != nil {
		next = *current
	}
	for _, name := range displayGroups {
		raw, present := doc[name]
		if !present {
			continue
		}
		if strings.TrimSpace(string(raw)) == "null" {
			clearGroup(&next, name)
			continue
		}
		if name == "hexes" {
			merged, err := mergeGroupFields(next.Hexes, raw)
			if err != nil {
				return nil, apperror.NewValidation("display_settings.hexes is not valid")
			}
			raw = merged
		}
		if err := applyGroup(&next, name, raw); err != nil {
			return nil, err
		}
	}
	if next.IsEmpty() {
		return nil, nil
	}
	return &next, nil
}

// mergeGroupFields lays the fields of an incoming group over the stored one:
// a field the incoming object names replaces (null removes it), every other
// stored field is kept. The result still goes through the group's normaliser.
func mergeGroupFields(stored any, incoming json.RawMessage) (json.RawMessage, error) {
	var in map[string]json.RawMessage
	if err := json.Unmarshal(incoming, &in); err != nil {
		return nil, err
	}
	out := map[string]json.RawMessage{}
	if b, err := json.Marshal(stored); err == nil {
		var cur map[string]json.RawMessage
		_ = json.Unmarshal(b, &cur) // a nil group marshals to null and adds nothing
		for k, v := range cur {
			out[k] = v
		}
	}
	for k, v := range in {
		if strings.TrimSpace(string(v)) == "null" {
			delete(out, k)
			continue
		}
		out[k] = v
	}
	return json.Marshal(out)
}

// ParseDisplaySettings reads the stored column. A document that fails to parse
// reads as "no settings" rather than failing the page: the column is written
// only through MergeDisplaySettings, so a bad value means hand-edited data and
// the map should still open on its defaults.
func ParseDisplaySettings(raw *string) *DisplaySettings {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	var d DisplaySettings
	if err := json.Unmarshal([]byte(*raw), &d); err != nil {
		return nil
	}
	if d.IsEmpty() {
		return nil
	}
	return &d
}

// DrawWho returns the effective draw gate for the map: owners only, or the
// default of owners and scribes.
func (m *Map) DrawWho() string {
	if m != nil && m.Display != nil && m.Display.Draw != nil && m.Display.Draw.Who == DrawWhoOwners {
		return DrawWhoOwners
	}
	return DrawWhoScribes
}

// PartyWho returns who may move the party: owners only, or the default of
// owners and scribes. An owner or DM grant always may.
func (m *Map) PartyWho() string {
	if m != nil && m.Display != nil && m.Display.Hexes != nil && m.Display.Hexes.PartyWho == PartyWhoOwners {
		return PartyWhoOwners
	}
	return PartyWhoScribes
}

// HexArt returns the map's terrain art style, Detailed unless the map picked
// another.
func (m *Map) HexArt() string {
	if m != nil && m.Display != nil && m.Display.Hexes != nil && IsValidHexArt(m.Display.Hexes.Art) {
		return m.Display.Hexes.Art
	}
	return HexArtDetailed
}

// ResolvedDisplay is the display settings with every default filled in, in the
// shape the page script reads. Resolving server-side keeps the defaults in one
// place instead of repeating them in JavaScript.
type ResolvedDisplay struct {
	// Frame is the frame actually drawn: the map's own pick, else the campaign's.
	Frame string `json:"frame"`
	// FrameSource is "map" when this map overrides the campaign style, else "campaign".
	FrameSource string `json:"frame_source"`
	// CampaignFrame is the campaign style, so the sheet can label "Campaign style (Atlas)".
	CampaignFrame string        `json:"campaign_frame"`
	Tint          bool          `json:"tint"`
	PinStyle      string        `json:"pin_style"`
	PinSize       string        `json:"pin_size"`
	PinLabels     string        `json:"pin_labels"`
	Kinds         []KindDisplay `json:"kinds"`
	GridType      string        `json:"grid_type"`
	GridSize      int           `json:"grid_size"`
	GridStrength  int           `json:"grid_strength"`
	OpenMode      string        `json:"open_mode"`
	OpenX         float64       `json:"open_x"`
	OpenY         float64       `json:"open_y"`
	OpenZoom      float64       `json:"open_zoom"`
	DrawWho       string        `json:"draw_who"`
	PartyWho      string        `json:"party_who"`
}

// ResolveDisplay fills every default for a map. campaignFrame is the
// campaign-wide style; an invalid or empty value falls back to the default so a
// stale row can never leave the page without a frame.
func ResolveDisplay(m *Map, campaignFrame string) ResolvedDisplay {
	if !IsValidFrame(campaignFrame) {
		campaignFrame = DefaultFrame
	}
	r := ResolvedDisplay{
		Frame: campaignFrame, FrameSource: "campaign", CampaignFrame: campaignFrame,
		Tint: true, PinStyle: PinStyleDrop, PinSize: PinSizeMedium, PinLabels: PinLabelsHover,
		GridType: GridNone, GridSize: defaultGridSize, GridStrength: defaultGridStrength,
		OpenMode: OpenWhole, OpenX: 50, OpenY: 50, OpenZoom: 0, DrawWho: DrawWhoScribes, PartyWho: PartyWhoScribes,
	}
	kinds := make([]KindDisplay, len(KindDefaults))
	copy(kinds, KindDefaults)
	r.Kinds = kinds
	if m == nil || m.Display == nil {
		return r
	}
	d := m.Display
	if d.Frame != nil {
		if d.Frame.Style != "" && IsValidFrame(d.Frame.Style) {
			r.Frame, r.FrameSource = d.Frame.Style, "map"
		}
		if d.Frame.Tint != nil {
			r.Tint = *d.Frame.Tint
		}
	}
	if d.Pins != nil {
		if d.Pins.Style != "" {
			r.PinStyle = d.Pins.Style
		}
		if d.Pins.Size != "" {
			r.PinSize = d.Pins.Size
		}
		if d.Pins.Labels != "" {
			r.PinLabels = d.Pins.Labels
		}
	}
	for i := range r.Kinds {
		if k, ok := d.Kinds[r.Kinds[i].ID]; ok {
			if k.Label != "" {
				r.Kinds[i].Label = k.Label
			}
			if k.Color != "" {
				r.Kinds[i].Color = k.Color
			}
		}
	}
	if d.Grid != nil {
		if d.Grid.Type != "" {
			r.GridType = d.Grid.Type
		}
		if d.Grid.Size != 0 {
			r.GridSize = d.Grid.Size
		}
		if d.Grid.Strength != 0 {
			r.GridStrength = d.Grid.Strength
		}
	}
	if d.Open != nil && d.Open.Mode != "" {
		r.OpenMode = d.Open.Mode
		if d.Open.X != nil && d.Open.Y != nil && d.Open.Zoom != nil {
			r.OpenX, r.OpenY, r.OpenZoom = *d.Open.X, *d.Open.Y, *d.Open.Zoom
		} else if r.OpenMode == OpenSpot {
			// A spot with no coordinates (hand-edited data) opens on the whole map.
			r.OpenMode = OpenWhole
		}
	}
	r.DrawWho = m.DrawWho()
	r.PartyWho = m.PartyWho()
	return r
}
