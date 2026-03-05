package posts

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler handles HTTP requests for entity post operations. Handlers are
// thin: bind request, call service, render response. No business logic.
type Handler struct {
	service PostService
}

// NewHandler creates a new post handler backed by the given service.
func NewHandler(service PostService) *Handler {
	return &Handler{service: service}
}

// ListPosts returns all posts for an entity as JSON.
// GET /campaigns/:id/entities/:eid/posts
func (h *Handler) ListPosts(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	// Scribes and above see DM-only posts.
	includeDMOnly := cc.MemberRole >= campaigns.RoleScribe

	posts, err := h.service.ListByEntity(c.Request().Context(), entityID, includeDMOnly)
	if err != nil {
		return err
	}

	if posts == nil {
		posts = []Post{}
	}

	return c.JSON(http.StatusOK, posts)
}

// CreatePost creates a new post attached to an entity.
// POST /campaigns/:id/entities/:eid/posts
func (h *Handler) CreatePost(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	var req CreatePostRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	userID := auth.GetUserID(c)

	post, err := h.service.Create(c.Request().Context(), cc.Campaign.ID, entityID, userID, req.Name, req)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, post)
}

// UpdatePost updates a post's name, content, or visibility.
// PUT /campaigns/:id/entities/:eid/posts/:pid
func (h *Handler) UpdatePost(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	postID := c.Param("pid")
	if postID == "" {
		return apperror.NewBadRequest("post ID is required")
	}

	// Verify post belongs to this campaign.
	existing, err := h.service.GetByID(c.Request().Context(), postID)
	if err != nil {
		return err
	}
	if existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("post not found")
	}

	var req UpdatePostRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	post, err := h.service.Update(c.Request().Context(), postID, req)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, post)
}

// DeletePost removes a post.
// DELETE /campaigns/:id/entities/:eid/posts/:pid
func (h *Handler) DeletePost(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	postID := c.Param("pid")
	if postID == "" {
		return apperror.NewBadRequest("post ID is required")
	}

	// Verify post belongs to this campaign.
	existing, err := h.service.GetByID(c.Request().Context(), postID)
	if err != nil {
		return err
	}
	if existing.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("post not found")
	}

	if err := h.service.Delete(c.Request().Context(), postID); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ReorderPosts updates the sort order for posts within an entity.
// PUT /campaigns/:id/entities/:eid/posts/reorder
func (h *Handler) ReorderPosts(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	var req ReorderPostsRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	if err := h.service.Reorder(c.Request().Context(), entityID, req.PostIDs); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
