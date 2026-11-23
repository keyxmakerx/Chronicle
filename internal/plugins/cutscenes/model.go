package cutscenes

import (
	"time"
)

// The kinds a scene may use. The browser engine (static/js/cutscene_engine.js)
// draws each of these; a Go test greps the engine for every one so the two
// lists cannot drift apart. The engine draws nothing for a kind it does not
// know, so an older client never breaks on a newer scene.
const (
	BackgroundFallAsleep  = "fall-asleep"
	BackgroundBlack       = "black"
	BackgroundFadeToBlack = "fade-to-black"
	BackgroundNone        = "none"

	OverlayEye    = "eye"
	OverlayEmbers = "embers"

	StyleWhisper = "whisper"
	StyleTitle   = "title"
	StylePlain   = "plain"

	EndingTap  = "tap"
	EndingAuto = "auto"
)

// Kind is one choice a layer offers, with the plain words the editor and the
// AI use for it.
type Kind struct {
	Key   string
	Name  string // what the editor's picker says
	Looks string // what it looks like on screen, for the AI and the editor's hint
}

// BackgroundKinds, OverlayKinds and TextStyles are every choice in the order
// the editor offers them; the first background and style are the defaults.
var (
	BackgroundKinds = []Kind{
		{BackgroundFallAsleep, "Falling asleep", "darkness creeps in from the edges in soft, blurred patches until the screen is black, like eyes closing"},
		{BackgroundFadeToBlack, "Fade to black", "the whole screen dims evenly to black"},
		{BackgroundBlack, "Black", "the screen is black at once"},
		{BackgroundNone, "Nothing", "no background of its own; only the layers on top and the words are drawn"},
	}
	OverlayKinds = []Kind{
		{OverlayEye, "An eye opens", "a single eye opens in the middle of the screen, burning with light in its colour"},
		{OverlayEmbers, "Embers", "glowing sparks drift up from the bottom of the screen in its colour"},
	}
	TextStyles = []Kind{
		{StyleWhisper, "Whisper", "small, spaced italic letters that fade in softly"},
		{StyleTitle, "Title", "large display letters, like a chapter heading"},
		{StylePlain, "Plain", "ordinary reading text"},
	}
	EndingKinds = []Kind{
		{EndingTap, "When they tap", "a quiet Close appears once the last layer has finished; a tap, click or Esc closes it"},
		{EndingAuto, "By itself", "it closes on its own a few seconds after the last layer has finished"},
	}
)

// Look is one way a kind can be drawn. The ids and names mirror the engine's
// ChronicleCutscene.looks table, in its order, the first being the one the
// engine plays when a scene names none; a Go test reads the engine to keep
// them equal.
type Look struct {
	Key   string
	Name  string
	Looks string
}

// Looks lists the looks of each kind that has them. A look on any other kind,
// or one not listed here, is dropped, and the engine plays the default.
var Looks = map[string][]Look{
	BackgroundFallAsleep: {
		{"drift", "The page drifts off", "the screen itself blurs, loses its colour and sinks while darkness closes in like tunnel vision and soft dark spots float across it"},
		{"eyelids", "Heavy eyelids", "soft dark patches breathe in from the edges while eyelids droop and flutter open twice before they give in"},
		{"ink", "Ink in water", "darkness bleeds in from the edges like ink spreading through water, in curling soft tendrils that meet in the middle"},
	},
	OverlayEye: {
		{"gather", "Sparks gather", "sparks drift out of the dark into a ring of fire; lines of light draw the eye around it and the pupil splits open"},
		{"almond", "Lids part", "a thin seam of light cracks, hesitates, then the lids part on a burning iris with a slit pupil"},
		{"rift", "Burning seam", "a line of fire burns across the dark and tears open into a ragged eye with a round pupil that narrows onto you"},
	},
}

// lookOf returns the look's entry for kind, or false when kind has no such
// look.
func lookOf(kind, key string) (Look, bool) {
	for _, l := range Looks[kind] {
		if l.Key == key {
			return l, true
		}
	}
	return Look{}, false
}

// Limits on a scene. Numbers outside them are clamped, never refused; counts
// and text lengths over them are refused with a plain sentence.
const (
	MaxNameChars   = 120
	MaxOverlays    = 8
	MaxTextLines   = 12
	MaxSayChars    = 280
	MinSeconds     = 0.5
	MaxSeconds     = 60.0
	MaxAt          = 120.0
	MaxAfter       = 60.0
	DefaultAfter   = 3.0
	DefaultSeconds = 5.0
	// MaxBodyBytes caps a scene sent as JSON or a form; a full scene is a few
	// kilobytes.
	MaxBodyBytes = 64 << 10
)

// Default colours, used when a layer comes without one.
const (
	DefaultEyeColor    = "#3dff7a"
	DefaultEmbersColor = "#ff9a3c"
)

// Scene is a cut scene, in the shape the engine reads and the database keeps
// (one JSON column per layer).
type Scene struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Background Background `json:"background"`
	Overlays   []Overlay  `json:"overlays"`
	Text       []TextLine `json:"text"`
	Ending     Ending     `json:"ending"`
	UpdatedAt  time.Time  `json:"-"`
}

// Params are a layer's settings beyond kind and timing: an overlay's colour,
// and the look of a kind that has several (falling asleep, the eye). An
// unknown param is dropped when the layer is read.
type Params struct {
	Color string `json:"color,omitempty"`
	Look  string `json:"look,omitempty"`
}

// Background is the bottom layer: how the screen goes dark.
type Background struct {
	Kind    string  `json:"kind"`
	Seconds float64 `json:"seconds"`
	Params  Params  `json:"params"`
}

// Overlay is one animation drawn on top of the background, in list order.
type Overlay struct {
	Kind    string  `json:"kind"`
	At      float64 `json:"at"`
	Seconds float64 `json:"seconds"`
	Params  Params  `json:"params"`
}

// TextLine is one line of words. Say is plain text, never HTML.
type TextLine struct {
	Say     string  `json:"say"`
	At      float64 `json:"at"`
	Seconds float64 `json:"seconds"`
	Style   string  `json:"style"`
}

// Ending says how the scene closes. After (auto only) is seconds after the
// last layer has finished.
type Ending struct {
	Kind  string   `json:"kind"`
	After *float64 `json:"after,omitempty"`
}

// Rewatch windows the Show sheet offers, by their form value.
var RewatchChoices = []struct {
	Key   string
	Name  string
	Hours int
}{
	{"1h", "1 hour", 1},
	{"12h", "Tonight (12 hours)", 12},
	{"72h", "3 days", 72},
}

// DefaultRewatch is the Show sheet's preselected window.
const DefaultRewatch = "12h"

// MaxViewers caps how many people one showing names.
const MaxViewers = 100

// LiveFor is how long after it starts a showing that was not stopped still
// counts as playing: longer than the longest scene the limits allow.
const LiveFor = 5 * time.Minute

// Showing is one play of a scene to named people.
type Showing struct {
	ID           string
	CampaignID   string
	SceneID      string // "" once the scene is deleted
	Scene        Scene  // the scene as it was shown
	ShownBy      string
	ShownAt      time.Time
	RewatchUntil time.Time
	StoppedAt    *time.Time
	Viewers      []ShowingViewer
}

// ShowingViewer is one person a showing was for.
type ShowingViewer struct {
	UserID    string
	Name      string // filled by the service from the member list
	WatchedAt *time.Time
}

// Live reports whether the showing may still be on someone's screen.
func (s Showing) Live(now time.Time) bool {
	return s.StoppedAt == nil && now.Sub(s.ShownAt) < LiveFor
}

// Viewer is the person asking, as the service needs them. Director is the
// campaign owner or a member with DM access (CanAuthorDmOnly).
type Viewer struct {
	UserID   string
	Director bool
}

// Member is one person in the campaign, from the member list.
type Member struct {
	UserID   string
	Name     string
	Director bool // the owner or a co-DM
}

// Place says where a member is now. Both false is away.
type Place struct {
	Chronicle bool
	Foundry   bool
}

// Words is how the Show sheet says a place.
func (p Place) Words() string {
	switch {
	case p.Chronicle && p.Foundry:
		return "Both"
	case p.Chronicle:
		return "In Chronicle"
	case p.Foundry:
		return "In Foundry"
	}
	return "Away"
}

// Here is true when the member has Chronicle or Foundry open.
func (p Place) Here() bool { return p.Chronicle || p.Foundry }

// AudienceRow is one person on the Show sheet.
type AudienceRow struct {
	Member
	Place Place
	You   bool
}

// ShowInput is what the Show sheet posts.
type ShowInput struct {
	To      []string
	Rewatch string // a RewatchChoices key; "" is the default
}

// MineItem is one of the caller's own showings still inside its window, in
// the shape GET /cutscenes/mine sends.
type MineItem struct {
	ShowingID    string    `json:"showingId"`
	Name         string    `json:"name"`
	ShownAt      time.Time `json:"shownAt"`
	RewatchUntil time.Time `json:"rewatchUntil"`
	Watched      bool      `json:"watched"`
}

// ShowingRead is one showing as a viewer reads it to play it.
type ShowingRead struct {
	ShowingID    string    `json:"showingId"`
	RewatchUntil time.Time `json:"rewatchUntil"`
	Scene        Scene     `json:"scene"`
}

// MemberShowing is one showing as the sync API hands it to the Foundry GM
// client, with the scene to play.
type MemberShowing struct {
	ShowingID    string    `json:"showingId"`
	Name         string    `json:"name"`
	ShownAt      time.Time `json:"shownAt"`
	RewatchUntil time.Time `json:"rewatchUntil"`
	Scene        Scene     `json:"scene"`
}

// ShowingReport is one showing on the Director's recent list.
type ShowingReport struct {
	ShowingID    string         `json:"showingId"`
	SceneID      string         `json:"sceneId,omitempty"`
	Name         string         `json:"name"`
	ShownAt      time.Time      `json:"shownAt"`
	RewatchUntil time.Time      `json:"rewatchUntil"`
	Stopped      bool           `json:"stopped"`
	Live         bool           `json:"live"`
	Viewers      []ReportViewer `json:"viewers"`
}

// ReportViewer is one person on a showing report.
type ReportViewer struct {
	UserID  string `json:"userId"`
	Name    string `json:"name"`
	Watched bool   `json:"watched"`
}
