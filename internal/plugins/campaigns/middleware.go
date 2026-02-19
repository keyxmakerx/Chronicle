package campaigns

import (
	"fmt"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// contextKeyCampaign is the Echo context key for campaign context data.
const contextKeyCampaign = "campaign_context"

// RequireCampaignAccess returns middleware that resolves the campaign from the
// :id URL parameter and the user's membership role. The resolved CampaignContext
// is injected into the Echo context for downstream handlers.
//
// Behavior:
//   - If the user is a member → MemberRole is set from the campaign_members row
//   - If the user is NOT a member AND is a site admin → MemberRole = RoleNone,
//     IsSiteAdmin = true (admin actions go through /admin routes)
//   - If the user is NOT a member AND is NOT an admin → 403 Forbidden
//
// Must be applied AFTER auth.RequireAuth.
func RequireCampaignAccess(service CampaignService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			campaignID := c.Param("id")
			if campaignID == "" {
				return apperror.NewBadRequest("campaign ID is required")
			}

			session := auth.GetSession(c)
			if session == nil {
				return apperror.NewUnauthorized("authentication required")
			}

			// Verify the campaign exists.
			campaign, err := service.GetByID(c.Request().Context(), campaignID)
			if err != nil {
				return err
			}

			cc := &CampaignContext{
				Campaign:    campaign,
				IsSiteAdmin: session.IsAdmin,
				MemberRole:  RoleNone,
			}

			// Look up the user's membership.
			member, err := service.GetMember(c.Request().Context(), campaignID, session.UserID)
			if err == nil {
				// User is a member — set their actual role.
				cc.MemberRole = member.Role
			} else if session.IsAdmin {
				// Not a member but is a site admin — they can still access
				// the route, but with no content visibility (RoleNone).
				// Admin-specific actions route through /admin endpoints.
				cc.MemberRole = RoleNone
			} else {
				// Not a member and not an admin — deny access.
				return apperror.NewForbidden("you are not a member of this campaign")
			}

			c.Set(contextKeyCampaign, cc)
			return next(c)
		}
	}
}

// RequireRole returns middleware that checks the user's membership role
// meets the minimum required level. Uses MemberRole (not admin bypass) so
// that admins who joined as Player are treated as Players for content access.
//
// Must be applied AFTER RequireCampaignAccess.
func RequireRole(minRole Role) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc := GetCampaignContext(c)
			if cc == nil {
				return apperror.NewInternal(
					fmt.Errorf("RequireRole used without RequireCampaignAccess"),
				)
			}

			if cc.MemberRole < minRole {
				return apperror.NewForbidden("insufficient permissions")
			}

			return next(c)
		}
	}
}

// GetCampaignContext retrieves the campaign context from the Echo context.
// Returns nil if RequireCampaignAccess middleware was not applied.
func GetCampaignContext(c echo.Context) *CampaignContext {
	cc, ok := c.Get(contextKeyCampaign).(*CampaignContext)
	if !ok {
		return nil
	}
	return cc
}
