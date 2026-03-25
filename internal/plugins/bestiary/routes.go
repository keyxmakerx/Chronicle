package bestiary

import (
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
	bg.GET("", h.Browse)
	bg.GET("/my-creations", h.MyCreations)
	bg.GET("/search", h.Search)
	bg.GET("/trending", h.Trending)
	bg.GET("/newest", h.Newest)
	bg.GET("/top-rated", h.TopRated)
	bg.GET("/most-imported", h.MostImported)
	bg.GET("/favorites", h.ListFavorites)
	bg.GET("/creators/:userId", h.CreatorProfile)
	bg.GET("/:slug", h.Show)
	bg.GET("/:slug/statblock", h.GetStatblock)
	bg.GET("/:slug/reviews", h.ListReviews)

	// Publish & manage (any authenticated user; ownership checked in service).
	bg.POST("", h.Create)
	bg.PUT("/:id", h.Update)
	bg.DELETE("/:id", h.Delete)
	bg.PATCH("/:id/visibility", h.ChangeVisibility)

	// Rating & favorites.
	bg.POST("/:id/rate", h.RatePublication)
	bg.DELETE("/:id/rate", h.RemoveRating)
	bg.POST("/:id/favorite", h.AddFavorite)
	bg.DELETE("/:id/favorite", h.RemoveFavorite)

	// Import, fork & flag.
	bg.POST("/:id/import/:campaignId", h.ImportToCampaign)
	bg.POST("/:id/fork/:campaignId", h.ForkToCampaign)
	bg.POST("/:id/flag", h.FlagPublication)
}
