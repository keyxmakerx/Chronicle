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
