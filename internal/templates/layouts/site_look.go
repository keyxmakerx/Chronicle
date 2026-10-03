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
	ctx = SetTopbarStyle(ctx, &TopbarStyleData{
		Mode:         "gradient",
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
