package bestiary

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// RegisterRoutes sets up all bestiary-related routes.
// Bestiary routes are instance-scoped (not campaign-scoped) and require
// authentication. All endpoints live under /bestiary/*.
func RegisterRoutes(e *echo.Echo, h *Handler, authSvc auth.AuthService) {
	// All bestiary routes require authentication.
	bg := e.Group("/bestiary", auth.RequireAuth(authSvc))

	// Browse & read (any authenticated user).
	// Rate limited: 200/min for browse, 100/min for search.
	bg.GET("", h.Browse, UserRateLimit(200, time.Minute))
	bg.GET("/my-creations", h.MyCreations)
	bg.GET("/search", h.Search, UserRateLimit(100, time.Minute))
	bg.GET("/trending", h.Trending, UserRateLimit(200, time.Minute))
	bg.GET("/newest", h.Newest, UserRateLimit(200, time.Minute))
	bg.GET("/top-rated", h.TopRated, UserRateLimit(200, time.Minute))
	bg.GET("/most-imported", h.MostImported, UserRateLimit(200, time.Minute))
	bg.GET("/favorites", h.ListFavorites)
	bg.GET("/creators/:userId", h.CreatorProfile)
	bg.GET("/:slug", h.Show)
	bg.GET("/:slug/statblock", h.GetStatblock)
	bg.GET("/:slug/reviews", h.ListReviews)

	// Publish & manage (rate limited per design doc).
	bg.POST("", h.Create, UserRateLimit(10, time.Hour))
	bg.PUT("/:id", h.Update, UserRateLimit(20, time.Hour))
	bg.DELETE("/:id", h.Delete)
	bg.PATCH("/:id/visibility", h.ChangeVisibility)

	// Rating & favorites.
	bg.POST("/:id/rate", h.RatePublication, UserRateLimit(30, time.Hour))
	bg.DELETE("/:id/rate", h.RemoveRating)
	bg.POST("/:id/favorite", h.AddFavorite)
	bg.DELETE("/:id/favorite", h.RemoveFavorite)

	// Import, fork & flag.
	bg.POST("/:id/import/:campaignId", h.ImportToCampaign, UserRateLimit(50, time.Hour))
	bg.POST("/:id/fork/:campaignId", h.ForkToCampaign, UserRateLimit(50, time.Hour))
	bg.POST("/:id/flag", h.FlagPublication, UserRateLimit(10, time.Hour))
}

// RegisterAdminRoutes sets up admin/moderation routes for the bestiary.
// These require site admin privileges and live under /admin/bestiary/*.
func RegisterAdminRoutes(e *echo.Echo, h *Handler, authSvc auth.AuthService) {
	ag := e.Group("/admin/bestiary",
		auth.RequireAuth(authSvc),
		auth.RequireSiteAdmin(),
	)

	ag.GET("/flagged", h.AdminFlagged)
	ag.GET("/stats", h.AdminStats)
	ag.POST("/:id/moderate", h.AdminModerate)
	ag.GET("/:id/moderation-log", h.AdminModerationLog)
}
