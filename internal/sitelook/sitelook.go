// Package sitelook holds the site-wide look of the pages outside a campaign:
// the site name, logo, which of the eight looks to borrow colours from, and
// the sign-in page's background. It is a leaf package (no plugin imports) so
// the settings service, the admin plugin, the campaigns plugin and the layouts
// can all share one definition of what is allowed.
package sitelook

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Limits shown on the admin page and enforced on save and upload.
const (
	DefaultName = "Chronicle"
	NameMax     = 40
	WelcomeMax  = 80

	// LogoMaxBytes and PictureMaxBytes cap the two uploads.
	LogoMaxBytes    = 1 << 20
	PictureMaxBytes = 3 << 20
)

// Sign-in background choices. A string enum rather than a bool so a further
// choice can be added later without reshaping the stored value.
const (
	BackgroundPlain   = "plain"   // The page colour, as before any look was set.
	BackgroundLook    = "look"    // A gradient from the chosen look's header colours.
	BackgroundPicture = "picture" // An uploaded wide picture.
)

// Backgrounds lists the allowed sign-in backgrounds; the first is the default.
var Backgrounds = []string{BackgroundPlain, BackgroundLook, BackgroundPicture}

// Look is the part of a campaign look that pages outside a campaign borrow:
// the accent, the two header colours and the heading face. The colours are
// the only ones that ever reach inline CSS, so they are fixed here rather than
// accepted from a request. IDs match campaigns.AppearanceLooks (a test pins it).
type Look struct {
	ID          string
	Name        string
	Accent      string
	HeaderFrom  string
	HeaderTo    string
	HeadingFont string // A campaign heading-face id; "" follows the body text.
}

// Looks are the eight campaign looks in the order the Customize page lists
// them. Values follow the signed Site look mockup and customize_look.js.
var Looks = []Look{
	{"classic", "Classic", "#6366f1", "#0f172a", "#1e2a5a", ""},
	{"parchment", "Parchment", "#9a4a26", "#3b2a1c", "#4a1512", "imfell"},
	{"midnight", "Midnight", "#5b6be0", "#0f172a", "#1e2a5a", "cormorant"},
	{"forest", "Forest", "#2f7d4f", "#14352a", "#2f4a2c", "marcellus"},
	{"ember", "Ember", "#c2410c", "#1f2937", "#4a1512", "cinzel"},
	{"frost", "Frost", "#0e7490", "#0f172a", "#1e3a4a", "josefin"},
	{"arcane", "Arcane", "#7c3aed", "#0f172a", "#3b1d5e", "fraunces"},
	{"starship", "Starship", "#0f766e", "#0f172a", "#1f2937", "chakra"},
}

// Find returns the look with the given id.
func Find(id string) (Look, bool) {
	for _, l := range Looks {
		if l.ID == id {
			return l, true
		}
	}
	return Look{}, false
}

// Settings is the saved site look. The zero value means "never saved": every
// page renders exactly as it did before this feature existed.
type Settings struct {
	// Configured is true once an admin has saved; it separates "Chronicle's
	// own mark" (never saved) from a saved default name with the letter mark.
	Configured bool

	Name          string // "" is DefaultName.
	Logo          string // Stored media filename, "" for the letter mark.
	LogoAsFavicon bool   // Use the logo as the browser tab icon.

	Look string // A Looks id, or "" to leave pages outside a campaign as they are.

	Background string // One of Backgrounds.
	Picture    string // Stored media filename, used when Background is "picture".
	Welcome    string // Optional line on the sign-in card.

	// Move lets the sign-in background and the top bar outside a campaign
	// drift slowly. Off unless ticked, and meaningless over a plain background.
	Move bool
}

// DisplayName is the name to show: the saved one or "Chronicle".
func (s Settings) DisplayName() string {
	if s.Name == "" {
		return DefaultName
	}
	return s.Name
}

// Initial is the first letter of the display name, upper-cased, for the
// letter mark.
func (s Settings) Initial() string {
	r, _ := utf8.DecodeRuneInString(s.DisplayName())
	return string(unicode.ToUpper(r))
}

// ActiveLook returns the chosen look, if one is set.
func (s Settings) ActiveLook() (Look, bool) {
	return Find(s.Look)
}

// hasControl reports whether v contains a control character, which would let
// a name or line break the page or the tab title.
func hasControl(v string) bool {
	for _, r := range v {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// allowedPictureExt are the extensions a stored site picture may have. SVG is
// deliberately absent: an SVG can carry script.
var allowedPictureExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}

// PictureName accepts a stored media filename ("2026/09/<id>.png") or empty.
// It applies the same traversal rules as campaign pictures, and also limits
// the characters and the extension: the name ends up inside CSS url() and an
// <img>/<link>, so nothing but plain path characters may pass.
func PictureName(label, v string) (string, error) {
	if v == "" {
		return "", nil
	}
	bad := len(v) > 255 || strings.ContainsRune(v, '\\')
	for _, part := range strings.Split(v, "/") {
		if part == "" || strings.HasPrefix(part, ".") {
			bad = true
		}
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '/' || r == '.' || r == '-' || r == '_') {
			bad = true
		}
	}
	dot := strings.LastIndexByte(v, '.')
	if dot < 0 || !allowedPictureExt[strings.ToLower(v[dot:])] {
		bad = true
	}
	if bad {
		return "", apperror.NewBadRequest(fmt.Sprintf("invalid %s picture", label))
	}
	return v, nil
}

// Validate checks every field against its allowed values and returns the
// normalised settings to store: trimmed text, defaults stored as empty.
func Validate(in Settings) (Settings, error) {
	out := Settings{Configured: true, LogoAsFavicon: in.LogoAsFavicon}

	name := strings.TrimSpace(in.Name)
	if utf8.RuneCountInString(name) > NameMax {
		return Settings{}, apperror.NewBadRequest(fmt.Sprintf("the site name must be %d characters or fewer", NameMax))
	}
	if hasControl(name) {
		return Settings{}, apperror.NewBadRequest("the site name can't contain control characters")
	}
	if name != DefaultName {
		out.Name = name
	}

	welcome := strings.TrimSpace(in.Welcome)
	if utf8.RuneCountInString(welcome) > WelcomeMax {
		return Settings{}, apperror.NewBadRequest(fmt.Sprintf("the welcome line must be %d characters or fewer", WelcomeMax))
	}
	if hasControl(welcome) {
		return Settings{}, apperror.NewBadRequest("the welcome line can't contain control characters")
	}
	out.Welcome = welcome

	var err error
	if out.Logo, err = PictureName("logo", in.Logo); err != nil {
		return Settings{}, err
	}
	if out.Picture, err = PictureName("sign-in", in.Picture); err != nil {
		return Settings{}, err
	}

	if in.Look != "" {
		if _, ok := Find(in.Look); !ok {
			return Settings{}, apperror.NewBadRequest("invalid look")
		}
	}
	out.Look = in.Look

	switch in.Background {
	case "", BackgroundPlain:
		out.Background = BackgroundPlain
	case BackgroundLook:
		if out.Look == "" {
			return Settings{}, apperror.NewBadRequest("choose a look to use its colours behind sign-in")
		}
		out.Background = BackgroundLook
	case BackgroundPicture:
		if out.Picture == "" {
			return Settings{}, apperror.NewBadRequest("upload a picture to use one behind sign-in")
		}
		out.Background = BackgroundPicture
	default:
		return Settings{}, apperror.NewBadRequest("invalid sign-in background")
	}
	// Moving needs something to move: a plain background has nothing, so the
	// choice is dropped rather than stored dormant.
	out.Move = in.Move && out.Background != BackgroundPlain
	// A picture the background doesn't use is dropped, so nothing stale is kept.
	if out.Background != BackgroundPicture {
		out.Picture = ""
	}
	return out, nil
}
