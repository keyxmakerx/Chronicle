package auth

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/sitelook"
)

// SiteNameSource gives the site's name as set under Site admin > Site look,
// for the account emails this plugin sends. App wiring supplies it from the
// settings service.
type SiteNameSource interface {
	SiteName(ctx context.Context) string
}

// ConfigureSiteName wires the site name into the auth service's emails.
// Optional: without it they say "Chronicle".
func ConfigureSiteName(svc AuthService, src SiteNameSource) {
	if s, ok := svc.(*authService); ok {
		s.siteNames = src
	}
}

// siteName is the name account emails use: the Site look name, or
// "Chronicle" when none is wired or set.
func (s *authService) siteName(ctx context.Context) string {
	if s.siteNames == nil {
		return sitelook.DefaultName
	}
	if n := s.siteNames.SiteName(ctx); n != "" {
		return n
	}
	return sitelook.DefaultName
}
