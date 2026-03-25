package bestiary

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// Handler processes HTTP requests for the community bestiary plugin.
type Handler struct {
	svc BestiaryService
}

// NewHandler creates a new bestiary Handler.
func NewHandler(svc BestiaryService) *Handler {
	return &Handler{svc: svc}
}

// Browse lists published bestiary publications, paginated.
// GET /bestiary
func (h *Handler) Browse(c echo.Context) error {
	page, perPage := parsePagination(c)
	ctx := c.Request().Context()

	result, err := h.svc.ListPublished(ctx, page, perPage)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, result)
}

// Show returns a single publication by slug.
// GET /bestiary/:slug
func (h *Handler) Show(c echo.Context) error {
	slug := c.Param("slug")
	ctx := c.Request().Context()

	pub, err := h.svc.GetBySlug(ctx, slug)
	if err != nil {
		return err
	}

	// Only the creator can view draft/archived/flagged publications.
	userID := auth.GetUserID(c)
	if pub.Visibility != VisibilityPublished && pub.Visibility != VisibilityUnlisted {
		if pub.CreatorID != userID {
			return apperror.NewNotFound("publication not found")
		}
	}

	return c.JSON(http.StatusOK, pub)
}

// GetStatblock returns the raw statblock JSON for a publication.
// GET /bestiary/:slug/statblock
func (h *Handler) GetStatblock(c echo.Context) error {
	slug := c.Param("slug")
	ctx := c.Request().Context()

	pub, err := h.svc.GetBySlug(ctx, slug)
	if err != nil {
		return err
	}

	// Only published/unlisted are publicly accessible.
	userID := auth.GetUserID(c)
	if pub.Visibility != VisibilityPublished && pub.Visibility != VisibilityUnlisted {
		if pub.CreatorID != userID {
			return apperror.NewNotFound("publication not found")
		}
	}

	return c.JSONBlob(http.StatusOK, pub.StatblockJSON)
}

// Create publishes a new creature to the bestiary.
// POST /bestiary
func (h *Handler) Create(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}

	var req CreatePublicationInput
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	pub, err := h.svc.Publish(c.Request().Context(), userID, req)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, pub)
}

// Update modifies an existing publication.
// PUT /bestiary/:id
func (h *Handler) Update(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}
	pubID := c.Param("id")

	var req UpdatePublicationInput
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	pub, err := h.svc.Update(c.Request().Context(), userID, pubID, req)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, pub)
}

// Delete archives a publication (soft-delete).
// DELETE /bestiary/:id
func (h *Handler) Delete(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}
	pubID := c.Param("id")

	if err := h.svc.Archive(c.Request().Context(), userID, pubID); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// ChangeVisibility updates the visibility of a publication.
// PATCH /bestiary/:id/visibility
func (h *Handler) ChangeVisibility(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}
	pubID := c.Param("id")

	var req ChangeVisibilityInput
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.svc.ChangeVisibility(c.Request().Context(), userID, pubID, req.Visibility); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{
		"id":         pubID,
		"visibility": req.Visibility,
	})
}

// MyCreations lists the current user's publications across all visibility states.
// GET /bestiary/my-creations
func (h *Handler) MyCreations(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}

	page, perPage := parsePagination(c)
	result, err := h.svc.ListMyCreations(c.Request().Context(), userID, page, perPage)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, result)
}

// Search performs a filtered search across published publications.
// GET /bestiary/search
func (h *Handler) Search(c echo.Context) error {
	page, perPage := parsePagination(c)

	filters := SearchFilters{
		Query:        c.QueryParam("q"),
		Organization: c.QueryParam("organization"),
		Role:         c.QueryParam("role"),
		Tags:         c.QueryParam("tags"),
		CreatorID:    c.QueryParam("creator_id"),
		SystemID:     c.QueryParam("system_id"),
		Sort:         c.QueryParam("sort"),
		Page:         page,
		PerPage:      perPage,
	}

	if v := c.QueryParam("level_min"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filters.LevelMin = &n
		}
	}
	if v := c.QueryParam("level_max"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filters.LevelMax = &n
		}
	}

	result, err := h.svc.Search(c.Request().Context(), filters)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, result)
}

// Trending returns publications sorted by trending score.
// GET /bestiary/trending
func (h *Handler) Trending(c echo.Context) error {
	page, perPage := parsePagination(c)

	// Trending uses the search endpoint with sort=trending and no filters.
	result, err := h.svc.Search(c.Request().Context(), SearchFilters{
		Sort:    "trending",
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, result)
}

// Newest returns the most recently published publications.
// GET /bestiary/newest
func (h *Handler) Newest(c echo.Context) error {
	page, perPage := parsePagination(c)

	result, err := h.svc.ListNewest(c.Request().Context(), page, perPage)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, result)
}

// TopRated returns publications with the highest average rating.
// GET /bestiary/top-rated
func (h *Handler) TopRated(c echo.Context) error {
	page, perPage := parsePagination(c)

	result, err := h.svc.ListTopRated(c.Request().Context(), page, perPage)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, result)
}

// MostImported returns publications with the most downloads.
// GET /bestiary/most-imported
func (h *Handler) MostImported(c echo.Context) error {
	page, perPage := parsePagination(c)

	result, err := h.svc.ListMostImported(c.Request().Context(), page, perPage)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, result)
}

// CreatorProfile returns a creator's public profile with stats.
// GET /bestiary/creators/:userId
func (h *Handler) CreatorProfile(c echo.Context) error {
	userID := c.Param("userId")

	profile, err := h.svc.GetCreatorProfile(c.Request().Context(), userID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, profile)
}

// --- Admin moderation handlers ---

// AdminFlagged lists flagged publications for moderation review.
// GET /admin/bestiary/flagged
func (h *Handler) AdminFlagged(c echo.Context) error {
	page, perPage := parsePagination(c)

	result, err := h.svc.ListFlagged(c.Request().Context(), page, perPage)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, result)
}

// AdminStats returns aggregate bestiary statistics.
// GET /admin/bestiary/stats
func (h *Handler) AdminStats(c echo.Context) error {
	stats, err := h.svc.GetStats(c.Request().Context())
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, stats)
}

// AdminModerate performs a moderation action on a publication.
// POST /admin/bestiary/:id/moderate
func (h *Handler) AdminModerate(c echo.Context) error {
	moderatorID := auth.GetUserID(c)
	if moderatorID == "" {
		return apperror.NewUnauthorized("authentication required")
	}
	pubID := c.Param("id")

	var req ModerateInput
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.svc.Moderate(c.Request().Context(), moderatorID, pubID, req.Action, req.Reason); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{
		"publication_id": pubID,
		"action":         req.Action,
		"status":         "completed",
	})
}

// AdminModerationLog returns the moderation audit trail for a publication.
// GET /admin/bestiary/:id/moderation-log
func (h *Handler) AdminModerationLog(c echo.Context) error {
	pubID := c.Param("id")

	entries, err := h.svc.GetModerationLog(c.Request().Context(), pubID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, entries)
}

// --- Import, Fork & Flag handlers ---

// ImportToCampaign imports a publication's creature into a campaign.
// POST /bestiary/:id/import/:campaignId
func (h *Handler) ImportToCampaign(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}
	pubID := c.Param("id")
	campaignID := c.Param("campaignId")

	result, err := h.svc.Import(c.Request().Context(), userID, pubID, campaignID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, result)
}

// ForkToCampaign imports a publication as an editable copy into a campaign.
// POST /bestiary/:id/fork/:campaignId
func (h *Handler) ForkToCampaign(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}
	pubID := c.Param("id")
	campaignID := c.Param("campaignId")

	result, err := h.svc.Fork(c.Request().Context(), userID, pubID, campaignID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, result)
}

// FlagPublication flags a publication for moderation.
// POST /bestiary/:id/flag
func (h *Handler) FlagPublication(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}
	pubID := c.Param("id")

	var req FlagInput
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.svc.Flag(c.Request().Context(), userID, pubID, req.Reason); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{
		"publication_id": pubID,
		"status":         "flagged",
	})
}

// --- Rating & Favorite handlers ---

// RatePublication creates or updates a rating on a publication.
// POST /bestiary/:id/rate
func (h *Handler) RatePublication(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}
	pubID := c.Param("id")

	var req struct {
		Rating     int     `json:"rating"`
		ReviewText *string `json:"review_text,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}

	if err := h.svc.Rate(c.Request().Context(), userID, pubID, req.Rating, req.ReviewText); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{
		"publication_id": pubID,
		"rating":         req.Rating,
	})
}

// RemoveRating removes a user's rating on a publication.
// DELETE /bestiary/:id/rate
func (h *Handler) RemoveRating(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}
	pubID := c.Param("id")

	if err := h.svc.RemoveRating(c.Request().Context(), userID, pubID); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// ListReviews returns paginated reviews for a publication.
// GET /bestiary/:slug/reviews
func (h *Handler) ListReviews(c echo.Context) error {
	slug := c.Param("slug")
	ctx := c.Request().Context()

	// Resolve slug to ID.
	pub, err := h.svc.GetBySlug(ctx, slug)
	if err != nil {
		return err
	}

	page, perPage := parsePagination(c)
	result, err := h.svc.ListReviews(ctx, pub.ID, page, perPage)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, result)
}

// AddFavorite toggles a favorite on a publication.
// POST /bestiary/:id/favorite
func (h *Handler) AddFavorite(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}
	pubID := c.Param("id")

	favorited, err := h.svc.ToggleFavorite(c.Request().Context(), userID, pubID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{
		"publication_id": pubID,
		"favorited":      favorited,
	})
}

// RemoveFavorite removes a favorite from a publication.
// DELETE /bestiary/:id/favorite
func (h *Handler) RemoveFavorite(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}
	pubID := c.Param("id")

	if err := h.svc.RemoveFavorite(c.Request().Context(), userID, pubID); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// ListFavorites returns the current user's favorited publications.
// GET /bestiary/favorites
func (h *Handler) ListFavorites(c echo.Context) error {
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}

	page, perPage := parsePagination(c)
	result, err := h.svc.ListFavorites(c.Request().Context(), userID, page, perPage)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, result)
}

// --- Helpers ---

// parsePagination extracts page and per_page query parameters with safe defaults.
func parsePagination(c echo.Context) (int, int) {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = defaultPerPage
	}
	return page, perPage
}
