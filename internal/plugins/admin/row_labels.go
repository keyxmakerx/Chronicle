package admin

import (
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// userRoleLabel is the role chip text; disabled outranks admin, as in the table.
func userRoleLabel(u auth.User) string {
	switch {
	case u.IsDisabled:
		return "Disabled"
	case u.IsAdmin:
		return "Admin"
	}
	return "User"
}

// userRoleChipClass is the chip colour for userRoleLabel.
func userRoleChipClass(u auth.User) string {
	switch {
	case u.IsDisabled:
		return "bg-red-50 dark:bg-red-900/30 text-red-600 dark:text-red-400"
	case u.IsAdmin:
		return "bg-accent/10 text-accent"
	}
	return "bg-surface-alt text-fg-secondary"
}

// campaignVisibilityLabel is the card chip: whether the campaign is public.
func campaignVisibilityLabel(c campaigns.Campaign) string {
	if c.IsPublic {
		return "Public"
	}
	return "Private"
}
