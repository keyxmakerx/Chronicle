package layouts

import (
	"context"

	"github.com/a-h/templ"
)

// ViewPrefsData is the signed-in person's own look as the layout needs it.
// It is a separate type from the auth plugin's so the layout package does not
// import a plugin; the values are the same wire strings (see
// internal/plugins/auth/view_prefs.go).
type ViewPrefsData struct {
	Theme    string // device | light | dark
	Motion   string // owner | calm
	TextSize string // standard | larger | largest
	Contrast string // standard | high
}

const keyViewPrefs ctxKey = "layout_view_prefs"

// SetViewPrefs stores the viewer's own choices. Never call it for anyone but
// the person the page is rendered for: these attributes are per viewer.
func SetViewPrefs(ctx context.Context, p *ViewPrefsData) context.Context {
	return context.WithValue(ctx, keyViewPrefs, p)
}

// GetViewPrefs returns the viewer's choices, or nil for a signed-out visitor.
func GetViewPrefs(ctx context.Context) *ViewPrefsData {
	p, _ := ctx.Value(keyViewPrefs).(*ViewPrefsData)
	return p
}

// ViewPrefAttrs are the <html> attributes carrying the viewer's own choices.
// data-view-theme is always present for a signed-in person, even at the
// default, so the first-paint script and theme.js can tell "the account
// decides" from "a visitor, use this browser's saved theme". The other three
// appear only when they differ from the default, so the stylesheet's plain
// look is untouched for anyone who chose nothing. Values are re-checked
// against the allowed lists here, so a bad stored value can never become an
// attribute.
func ViewPrefAttrs(ctx context.Context) templ.Attributes {
	attrs := templ.Attributes{}
	p := GetViewPrefs(ctx)
	if p == nil {
		return attrs
	}
	attrs["data-view-theme"] = oneOf(p.Theme, "device", "light", "dark")
	if oneOf(p.Motion, "owner", "calm") == "calm" {
		attrs["data-view-motion"] = "calm"
	}
	if t := oneOf(p.TextSize, "standard", "larger", "largest"); t != "standard" {
		attrs["data-view-text"] = t
	}
	if oneOf(p.Contrast, "standard", "high") == "high" {
		attrs["data-view-contrast"] = "high"
	}
	return attrs
}

// oneOf returns v when it is in allowed, else the first (default) value.
func oneOf(v string, allowed ...string) string {
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	return allowed[0]
}
