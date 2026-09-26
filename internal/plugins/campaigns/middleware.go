package campaigns

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// campaignWriteRateLimitBudget caps each user's writes per minute through the
// campaign gates, so every campaign route gets it without wiring its own. It
// sits far above human pace (the busiest loop, the notes edit-lock
// heartbeat, runs every 2 minutes); it only blunts scripted floods.
const campaignWriteRateLimitBudget = 300

// sharedWriteRateLimiter is the one limiter behind every call to either gate,
// so the budget counts per user across all plugins, not per route group.
var sharedWriteRateLimiter = middleware.UserRateLimit(auth.GetUserID, campaignWriteRateLimitBudget, time.Minute)

// contextKeyCampaign is the Echo context key for campaign context data.
const contextKeyCampaign = "campaign_context"

// resolveCampaignContext resolves the campaign from the :id URL parameter and
// the caller's session/membership into a CampaignContext. Shared by
// RequireCampaignAccess and RequireCampaignAccessEvenIfArchived so the two
// can never drift on who counts as a member — they differ only in whether an
// archived campaign additionally blocks the write.
func resolveCampaignContext(c echo.Context, service CampaignService) (*CampaignContext, error) {
	campaignID := c.Param("id")
	if campaignID == "" {
		return nil, apperror.NewBadRequest("campaign ID is required")
	}

	session := auth.GetSession(c)
	if session == nil {
		return nil, apperror.NewUnauthorized("authentication required")
	}

	// Verify the campaign exists.
	campaign, err := service.GetByID(c.Request().Context(), campaignID)
	if err != nil {
		return nil, err
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
		cc.IsMember = true
	} else if session.IsAdmin {
		// Not a member but is a site admin — they can still access
		// the route, but with no content visibility (RoleNone).
		// Admin-specific actions route through /admin endpoints.
		cc.MemberRole = RoleNone
	} else {
		// Not a member and not an admin — deny access.
		return nil, apperror.NewForbidden("you are not a member of this campaign")
	}

	// Check if this member has been granted dm_only visibility.
	cc.IsDmGranted = hasDmGrant(campaign, session.UserID)

	return cc, nil
}

// isWriteMethod reports whether method is state-changing. GET/HEAD/OPTIONS
// (and anything else Echo might route) are treated as safe and pass through.
func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// RequireCampaignAccess returns middleware that resolves the campaign from the
// :id URL parameter and the user's membership role. The resolved CampaignContext
// is injected into the Echo context for downstream handlers.
//
// Behavior:
//   - If the user is a member → MemberRole is set from the campaign_members row
//   - If the user is NOT a member AND is a site admin → MemberRole = RoleNone,
//     IsSiteAdmin = true (admin actions go through /admin routes)
//   - If the user is NOT a member AND is NOT an admin → 403 Forbidden
//   - If the campaign is archived, POST/PUT/PATCH/DELETE → 403 Forbidden
//     (GET/HEAD/OPTIONS still pass). Checked only after the above, so a
//     non-member's 403/404 is unchanged by archive state. This is the
//     single default every route group built on this middleware inherits;
//     RequireCampaignAccessEvenIfArchived is the deliberate, short exception
//     list (see routes.go).
//
// Must be applied AFTER auth.RequireAuth.
func RequireCampaignAccess(service CampaignService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc, err := resolveCampaignContext(c, service)
			if err != nil {
				return err
			}

			if isWriteMethod(c.Request().Method) && cc.Campaign.IsArchived() {
				return apperror.NewForbidden("campaign is archived and read-only")
			}

			c.Set(contextKeyCampaign, cc)

			if isWriteMethod(c.Request().Method) {
				return sharedWriteRateLimiter(next)(c)
			}
			return next(c)
		}
	}
}

// RequireCampaignAccessEvenIfArchived is RequireCampaignAccess without the
// archive gate: same membership resolution, same 404/403 for a missing
// campaign or a non-member, but a write is never blocked for being archived.
// Reserved for the few actions that must keep working on an archived
// campaign — see routes.go for the short, commented list.
//
// Must be applied AFTER auth.RequireAuth.
func RequireCampaignAccessEvenIfArchived(service CampaignService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc, err := resolveCampaignContext(c, service)
			if err != nil {
				return err
			}

			c.Set(contextKeyCampaign, cc)

			if isWriteMethod(c.Request().Method) {
				return sharedWriteRateLimiter(next)(c)
			}
			return next(c)
		}
	}
}

// hasDmGrant checks whether a user has been granted dm_only visibility
// via the campaign's DmGrantIDs setting.
func hasDmGrant(campaign *Campaign, userID string) bool {
	settings := campaign.ParseSettings()
	for _, id := range settings.DmGrantIDs {
		if id == userID {
			return true
		}
	}
	return false
}

// AllowPublicCampaignAccess is like RequireCampaignAccess but also allows
// unauthenticated users to view public campaigns. Non-member public visitors
// (logged-out, or authenticated but not a member) get MemberRole=RoleNone —
// strictly below RolePlayer — so they see only public/everyone content and
// never Player-only or Player-role tag-granted content. RolePlayer is
// reserved for authenticated party members.
//
// Use this on view routes (/campaigns/:id, /campaigns/:id/entities, etc.).
// Mutating routes should still use RequireCampaignAccess + RequireRole.
func AllowPublicCampaignAccess(service CampaignService) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			campaignID := c.Param("id")
			if campaignID == "" {
				return apperror.NewBadRequest("campaign ID is required")
			}

			campaign, err := service.GetByID(c.Request().Context(), campaignID)
			if err != nil {
				return err
			}

			session := auth.GetSession(c)

			// Authenticated user — use normal membership logic.
			if session != nil {
				cc := &CampaignContext{
					Campaign:    campaign,
					IsSiteAdmin: session.IsAdmin,
					MemberRole:  RoleNone,
				}
				member, err := service.GetMember(c.Request().Context(), campaignID, session.UserID)
				if err == nil {
					cc.MemberRole = member.Role
					cc.IsMember = true
				} else if session.IsAdmin {
					cc.MemberRole = RoleNone
				} else if !campaign.IsPublic {
					return apperror.NewForbidden("you are not a member of this campaign")
				} else {
					// Authenticated non-member viewing a public campaign: they
					// are NOT a party member, so they get the public identity
					// (RoleNone), not RolePlayer. Their user/group grants still
					// match (the filter keys those off userID), but Player-role
					// grants do not.
					cc.MemberRole = RoleNone
				}
				// A grant is only honoured for an actual member. Without this
				// membership test, a stale id in dm_grant_ids on a PUBLIC
				// campaign would send an authenticated non-member (deliberately
				// admitted as RoleNone above) straight to RoleOwner through
				// VisibilityRole(). Site admins are unaffected — they take the
				// IsSiteAdmin branch above and never rely on a grant.
				cc.IsDmGranted = cc.IsMember && hasDmGrant(campaign, session.UserID)
				c.Set(contextKeyCampaign, cc)
				return next(c)
			}

			// Unauthenticated — only allow if campaign is public.
			if !campaign.IsPublic {
				return c.Redirect(302, "/login")
			}

			cc := &CampaignContext{
				Campaign:    campaign,
				MemberRole:  RoleNone, // Anonymous public visitor — below Player.
				IsSiteAdmin: false,
				IsAnonymous: true,
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

// RequireViewAccess gates a public-capable VIEW route on view eligibility rather
// than a role threshold: it passes when the requester is a real campaign member,
// a site admin, or the campaign is public. This is the correct gate for routes
// mounted under AllowPublicCampaignAccess — RequireRole(RolePlayer) would reject
// every anonymous/non-member RoleNone requester even on a public campaign, and
// promoting anon to RolePlayer would let Player-only content leak to the public.
// The visibility filter still strips Player-only content for RoleNone viewers,
// so they see only public/everyone data.
//
// Must be applied AFTER AllowPublicCampaignAccess (which populates the context;
// for a PRIVATE campaign it already rejects anon -> 302 /login and authenticated
// non-members -> 403 before this runs, so IsPublic is only ever reached for
// genuinely public campaigns). Mutating or Player-only routes must keep
// RequireRole.
func RequireViewAccess() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc := GetCampaignContext(c)
			if cc == nil {
				return apperror.NewInternal(
					fmt.Errorf("RequireViewAccess used without AllowPublicCampaignAccess"),
				)
			}

			if cc.IsMember || cc.IsSiteAdmin || cc.Campaign.IsPublic {
				return next(c)
			}

			return apperror.NewForbidden("insufficient permissions")
		}
	}
}

// RequireCapability gates a route on a CampaignContext capability predicate
// (e.g. (*CampaignContext).CanControlWorldState) rather than a flat MemberRole
// threshold — so a co-DM grantee passes where RequireRole(RoleOwner) would
// reject them. Must be applied after RequireCampaignAccess. denyMsg is the
// 403 message on failure.
func RequireCapability(check func(*CampaignContext) bool, denyMsg string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc := GetCampaignContext(c)
			if cc == nil {
				return apperror.NewInternal(
					fmt.Errorf("RequireCapability used without RequireCampaignAccess"),
				)
			}
			if !check(cc) {
				return apperror.NewForbidden(denyMsg)
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
