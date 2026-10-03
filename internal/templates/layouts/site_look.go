package layouts

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/sitelook"
)

const keySiteLook ctxKey = "layout_site_look"

// SetSiteLook stores the saved site look in the context. The zero value (never
// saved) is valid and changes nothing about any page.
func SetSiteLook(ctx context.Context, s sitelook.Settings) context.Context {
	return context.WithValue(ctx, keySiteLook, s)
}

// GetSiteLook returns the saved site look, or the zero value.
func GetSiteLook(ctx context.Context) sitelook.Settings {
	s, _ := ctx.Value(keySiteLook).(sitelook.Settings)
	return s
}

// ApplySiteLook lends the site's look to a page outside a campaign by filling
// the same context values a campaign's own settings would: accent colour, a
// gradient top bar and the heading face. Doing it through those values keeps
// one rendering path (the top bar, the accent shades and the heading CSS), and
// a campaign page never calls this, so campaigns keep their look untouched.
// Every colour comes from the fixed sitelook table, never from a request.
func ApplySiteLook(ctx context.Context) context.Context {
	s := GetSiteLook(ctx)
	look, ok := s.ActiveLook()
	if !ok || InCampaign(ctx) {
		return ctx
	}
	ctx = SetAccentColor(ctx, look.Accent)
	mode := "gradient"
	if s.Move {
		// The top bar's own moving background, on the shared rest clock.
		mode = "moving"
	}
	ctx = SetTopbarStyle(ctx, &TopbarStyleData{
		Mode:         mode,
		GradientFrom: look.HeaderFrom,
		GradientTo:   look.HeaderTo,
		GradientDir:  "to-r",
	})
	if look.HeadingFont != "" {
		ctx = SetAppearance(ctx, &AppearanceData{HeadingFont: look.HeadingFont})
	}
	return ctx
}

// SiteName is the site name for titles and brands: the saved one or
// "Chronicle".
func SiteName(ctx context.Context) string {
	return GetSiteLook(ctx).DisplayName()
}

// SiteTitle is the browser tab title: the page's name, then the site name.
func SiteTitle(ctx context.Context, title string) string {
	return title + " | " + SiteName(ctx)
}

// SiteFavicon returns the logo's URL and image type when the admin has chosen
// to use it as the tab icon, and ok false otherwise (the shipped icon stays).
func SiteFavicon(ctx context.Context) (href, mime string, ok bool) {
	s := GetSiteLook(ctx)
	if !s.LogoAsFavicon || s.Logo == "" {
		return "", "", false
	}
	switch strings.ToLower(path.Ext(s.Logo)) {
	case ".png":
		mime = "image/png"
	case ".jpg", ".jpeg":
		mime = "image/jpeg"
	case ".webp":
		mime = "image/webp"
	default:
		return "", "", false
	}
	return MediaURL(ctx, s.Logo), mime, true
}

// SiteWelcome is the optional welcome line for the sign-in card.
func SiteWelcome(ctx context.Context) string {
	return GetSiteLook(ctx).Welcome
}

// siteAuthPlain reports whether the sign-in pages keep the plain page colour:
// the default, and the fallback whenever a chosen background is unusable.
func siteAuthPlain(ctx context.Context) bool {
	return siteAuthGradient(ctx) == "" && siteAuthPicture(ctx) == ""
}

// siteAuthGradient is the inline gradient for the look's-colours background,
// or "". Both colours come from the fixed sitelook table.
func siteAuthGradient(ctx context.Context) string {
	s := GetSiteLook(ctx)
	if s.Background != sitelook.BackgroundLook {
		return ""
	}
	look, ok := s.ActiveLook()
	if !ok {
		return ""
	}
	return fmt.Sprintf("background:linear-gradient(135deg, %s, %s);", look.HeaderFrom, look.HeaderTo)
}

// siteAuthPicture is the URL of the sign-in picture, or "". It is drawn as an
// <img>, not a CSS background: a signed media URL carries "&", which templ
// would double-escape inside a style value and so break the signature. The
// stored name has already passed sitelook.PictureName.
func siteAuthPicture(ctx context.Context) string {
	s := GetSiteLook(ctx)
	if s.Background != sitelook.BackgroundPicture || s.Picture == "" {
		return ""
	}
	return MediaURL(ctx, s.Picture)
}

// siteAuthMovingStyle sizes the drifting strip behind the sign-in panel three
// panels wide and paints the look's two colours repeating once per panel
// width, so the header-motion widget sliding it by a third loops seamlessly.
// Both colours come from the fixed sitelook table.
func siteAuthMovingStyle(ctx context.Context) string {
	s := GetSiteLook(ctx)
	look, ok := s.ActiveLook()
	if !ok || !s.Move || s.Background != sitelook.BackgroundLook {
		return ""
	}
	return fmt.Sprintf("position:absolute; left:0; top:0; width:300%%; height:100%%; will-change:transform; "+
		"background:repeating-linear-gradient(90deg, %s 0%%, %s 16.6667%%, %s 33.3333%%);",
		look.HeaderFrom, look.HeaderTo, look.HeaderFrom)
}

// siteAuthPictureMoves reports whether the sign-in picture pans slowly.
func siteAuthPictureMoves(ctx context.Context) bool {
	return siteAuthPicture(ctx) != "" && GetSiteLook(ctx).Move
}

// HeadingFontStack is the CSS font stack for a campaign heading face id, or ""
// for none, so the Site look preview can show a look's heading font.
func HeadingFontStack(id string) string {
	if _, ok := czHeadingFaces[id]; !ok {
		return ""
	}
	return czFontStacks[id]
}

// SiteLookFontsCSS is @font-face rules for the heading faces the looks use,
// served from Chronicle, for the Site look preview.
func SiteLookFontsCSS() string {
	var b strings.Builder
	for _, l := range sitelook.Looks {
		if l.HeadingFont != "" {
			b.WriteString(fontFaceCSS(l.HeadingFont))
		}
	}
	return b.String()
}
