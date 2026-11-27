package smtp

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/sitelook"
)

// SiteNameSource gives the site's name as set under Site admin > Site look.
// App wiring supplies it from the settings service.
type SiteNameSource interface {
	SiteName(ctx context.Context) string
}

// ConfigureSiteName wires the site name in as the default sender name.
// Optional: without it the default stays "Chronicle".
func ConfigureSiteName(svc SMTPService, src SiteNameSource) {
	if s, ok := svc.(*smtpService); ok {
		s.siteNames = src
	}
}

// siteName is the Site look name, or "Chronicle" when none is wired or set.
func (s *smtpService) siteName(ctx context.Context) string {
	if s.siteNames != nil {
		if n := s.siteNames.SiteName(ctx); n != "" {
			return n
		}
	}
	return sitelook.DefaultName
}

// fromName is the sender name on outgoing mail. "Chronicle" is the stored
// default (the baseline row and an empty box both save it), so it counts as
// not set and follows the site name; any other saved name is the admin's.
func (s *smtpService) fromName(ctx context.Context, saved string) string {
	if saved == "" || saved == sitelook.DefaultName {
		return s.siteName(ctx)
	}
	return saved
}
