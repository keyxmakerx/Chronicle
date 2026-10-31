package media

import "github.com/keyxmakerx/chronicle/internal/apperror"

// ClientUsageType is the usage type an upload form may ask for: an
// attachment (the default when blank) or a page picture. Every other type is
// stamped by the route that owns it: avatars by the account page, backdrops
// by campaign appearance and site settings, note pictures by notes, page
// files by the page's Files list. Taking one from a form field would let any
// signed-in user mint a publicly served backdrop or skip those routes' rules.
func ClientUsageType(requested string) (string, error) {
	switch requested {
	case "":
		return UsageAttachment, nil
	case UsageAttachment, UsageEntityImage:
		return requested, nil
	default:
		return "", apperror.NewBadRequest("unsupported usage type")
	}
}
