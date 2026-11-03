package syncapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// CraftingAPIService is crafting as the Foundry module needs it: a hero's
// crafting, the spoils tray and the end of a fight. The implementation lives
// behind internal/app so this plugin never imports the crafting plugin;
// bodies and answers are opaque here, so crafting's own decoding and checks
// stay the only ones.
//
// Like the stash calls, every call names the key holder and, optionally, the
// member it is made for, and is answered under that member's rules: a player
// works only on characters they claimed, and only the Owner or a co-DM may
// read the Director's tray or end a fight. Naming a member only ever narrows
// the key's own power.
type CraftingAPIService interface {
	Character(ctx context.Context, campaignID, keyUserID, actingUserID, characterID string) (any, error)
	StartProject(ctx context.Context, campaignID, keyUserID, actingUserID, characterID string, body json.RawMessage) (any, error)
	RollProject(ctx context.Context, campaignID, keyUserID, actingUserID, projectID string, body json.RawMessage) (any, error)
	CancelProject(ctx context.Context, campaignID, keyUserID, actingUserID, projectID string) error
	Spoils(ctx context.Context, campaignID, keyUserID, actingUserID string) (any, error)
	TakeSpoil(ctx context.Context, campaignID, keyUserID, actingUserID string, lineID int64, body json.RawMessage) (any, error)
	StudySpoil(ctx context.Context, campaignID, keyUserID, actingUserID string, lineID int64, body json.RawMessage) (any, error)
	// Experiment, BreakDown and Help are a hero's turns: trying something
	// new, breaking an item down, and lending a hand on a party member's
	// project.
	Experiment(ctx context.Context, campaignID, keyUserID, actingUserID, characterID string, body json.RawMessage) (any, error)
	BreakDown(ctx context.Context, campaignID, keyUserID, actingUserID, characterID string, body json.RawMessage) (any, error)
	Help(ctx context.Context, campaignID, keyUserID, actingUserID, projectID string, body json.RawMessage) (any, error)
	// FightEnded puts what a fight's defeated creatures drop in the tray.
	FightEnded(ctx context.Context, campaignID, keyUserID, actingUserID string, body json.RawMessage) (any, error)
}

// maxCraftingBodyBytes bounds a crafting body. The largest is a fight's
// creature list, far under this.
const maxCraftingBodyBytes = 32 << 10

// CraftingAPIHandler serves the crafting endpoints. Thin on purpose: it
// reads the body and the acting member and hands them to the service.
type CraftingAPIHandler struct {
	svc CraftingAPIService
	// addonSlug is the addon that must be enabled; given by the wiring so
	// this plugin does not name another plugin.
	addonSlug string
}

// NewCraftingAPIHandler creates the crafting API handler.
func NewCraftingAPIHandler(svc CraftingAPIService, addonSlug string) *CraftingAPIHandler {
	return &CraftingAPIHandler{svc: svc, addonSlug: addonSlug}
}

// WithCrafting mounts the crafting endpoints.
func WithCrafting(h *CraftingAPIHandler) func(*APIHandler) {
	return func(api *APIHandler) { api.crafting = h }
}

// actingRaw reads a JSON body of at most maxCraftingBodyBytes and the member
// it names. An empty body is an empty object, for calls that need no fields.
func actingRaw(c echo.Context) (json.RawMessage, string, error) {
	// Read one byte past the cap so an oversize body is refused, not truncated.
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxCraftingBodyBytes+1))
	if err != nil || len(raw) > maxCraftingBodyBytes {
		return nil, "", apperror.NewBadRequest("invalid request body")
	}
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	var acting actingBody
	if err := json.Unmarshal(raw, &acting); err != nil {
		return nil, "", apperror.NewBadRequest("invalid JSON body")
	}
	return raw, acting.ActingUserID, nil
}

func parseSpoilLineID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("lid"), 10, 64)
	if err != nil || id < 1 {
		return 0, apperror.NewNotFound("those spoils are gone")
	}
	return id, nil
}

// Character returns a hero's crafting as the acting member sees it.
// GET /api/v1/campaigns/:id/crafting/characters/:eid?actingUserId=
func (h *CraftingAPIHandler) Character(c echo.Context) error {
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Character(c.Request().Context(), c.Param("id"), uid, c.QueryParam("actingUserId"), c.Param("eid"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// StartProject starts a project for a hero.
// POST /api/v1/campaigns/:id/crafting/characters/:eid/projects
// Body: {"actingUserId"?, "recipeId", "picks"}
func (h *CraftingAPIHandler) StartProject(c echo.Context) error {
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	raw, acting, err := actingRaw(c)
	if err != nil {
		return err
	}
	out, err := h.svc.StartProject(c.Request().Context(), c.Param("id"), uid, acting, c.Param("eid"), raw)
	if err != nil {
		return err
	}
	noteSyncActor(c, acting)
	return c.JSON(http.StatusCreated, out)
}

// RollProject makes a project's roll; the dice are the server's.
// POST /api/v1/campaigns/:id/crafting/projects/:pid/roll
// Body: {"actingUserId"?, "work", "bench", "version"}
func (h *CraftingAPIHandler) RollProject(c echo.Context) error {
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	raw, acting, err := actingRaw(c)
	if err != nil {
		return err
	}
	out, err := h.svc.RollProject(c.Request().Context(), c.Param("id"), uid, acting, c.Param("pid"), raw)
	if err != nil {
		return err
	}
	noteSyncActor(c, acting)
	return c.JSON(http.StatusOK, out)
}

// Experiment spends a hero's turn trying something new.
// POST /api/v1/campaigns/:id/crafting/characters/:eid/experiment
// Body: {"actingUserId"?, "items":[{"itemId","qty"}]}
func (h *CraftingAPIHandler) Experiment(c echo.Context) error {
	return h.turn(c, c.Param("eid"), h.svc.Experiment)
}

// BreakDown breaks a hero's copy of a crafted item down.
// POST /api/v1/campaigns/:id/crafting/characters/:eid/break-down
// Body: {"actingUserId"?, "recipeId", "copy"}
func (h *CraftingAPIHandler) BreakDown(c echo.Context) error {
	return h.turn(c, c.Param("eid"), h.svc.BreakDown)
}

// Help lends a hero's hand on a party member's project; the dice are the
// server's.
// POST /api/v1/campaigns/:id/crafting/projects/:pid/help
// Body: {"actingUserId"?, "characterId", "version"}
func (h *CraftingAPIHandler) Help(c echo.Context) error {
	return h.turn(c, c.Param("pid"), h.svc.Help)
}

func (h *CraftingAPIHandler) turn(c echo.Context, id string, fn func(ctx context.Context, campaignID, keyUserID, actingUserID, id string, body json.RawMessage) (any, error)) error {
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	raw, acting, err := actingRaw(c)
	if err != nil {
		return err
	}
	out, err := fn(c.Request().Context(), c.Param("id"), uid, acting, id, raw)
	if err != nil {
		return err
	}
	noteSyncActor(c, acting)
	return c.JSON(http.StatusOK, out)
}

// CancelProject sets a project aside.
// DELETE /api/v1/campaigns/:id/crafting/projects/:pid?actingUserId=
func (h *CraftingAPIHandler) CancelProject(c echo.Context) error {
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	acting := c.QueryParam("actingUserId")
	if err := h.svc.CancelProject(c.Request().Context(), c.Param("id"), uid, acting, c.Param("pid")); err != nil {
		return err
	}
	noteSyncActor(c, acting)
	return c.NoContent(http.StatusNoContent)
}

// Spoils returns the Director's tray and drop lists.
// GET /api/v1/campaigns/:id/crafting/spoils?actingUserId=
func (h *CraftingAPIHandler) Spoils(c echo.Context) error {
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Spoils(c.Request().Context(), c.Param("id"), uid, c.QueryParam("actingUserId"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// TakeSpoil takes or harvests a tray line for a hero.
// POST /api/v1/campaigns/:id/crafting/spoil-lines/:lid/take
// Body: {"actingUserId"?, "characterId"}
func (h *CraftingAPIHandler) TakeSpoil(c echo.Context) error {
	return h.line(c, h.svc.TakeSpoil)
}

// StudySpoil studies a find that needs it, for a hero.
// POST /api/v1/campaigns/:id/crafting/spoil-lines/:lid/study
// Body: {"actingUserId"?, "characterId"}
func (h *CraftingAPIHandler) StudySpoil(c echo.Context) error {
	return h.line(c, h.svc.StudySpoil)
}

func (h *CraftingAPIHandler) line(c echo.Context, fn func(ctx context.Context, campaignID, keyUserID, actingUserID string, lineID int64, body json.RawMessage) (any, error)) error {
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	id, err := parseSpoilLineID(c)
	if err != nil {
		return err
	}
	raw, acting, err := actingRaw(c)
	if err != nil {
		return err
	}
	out, err := fn(c.Request().Context(), c.Param("id"), uid, acting, id, raw)
	if err != nil {
		return err
	}
	noteSyncActor(c, acting)
	return c.JSON(http.StatusOK, out)
}

// FightEnded puts what a fight's defeated creatures drop in the tray, once
// the GM has confirmed it.
// POST /api/v1/campaigns/:id/crafting/fights
// Body: {"actingUserId"?, "name", "creatures":[{"pageId","name","down","copies"}]}
func (h *CraftingAPIHandler) FightEnded(c echo.Context) error {
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	raw, acting, err := actingRaw(c)
	if err != nil {
		return err
	}
	out, err := h.svc.FightEnded(c.Request().Context(), c.Param("id"), uid, acting, raw)
	if err != nil {
		return err
	}
	noteSyncActor(c, acting)
	return c.JSON(http.StatusOK, out)
}
