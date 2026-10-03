package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// StashAPIService is the stash feature as the sync API needs it. The
// implementation lives behind internal/app so this plugin never touches
// another plugin's types or repository; payloads are opaque to the handler
// and serialised as given (camelCase JSON shapes are owned by the service).
//
// Every call names the key holder and, optionally, the member the call is made
// for. The service answers under that member's rules: a narrowing of the key's
// own power, never a widening.
type StashAPIService interface {
	View(ctx context.Context, campaignID, keyUserID, actingUserID, characterID string) (any, error)
	Move(ctx context.Context, campaignID, keyUserID string, req StashMoveRequest) (any, error)
	History(ctx context.Context, campaignID, keyUserID, actingUserID, characterID, stashID string) (any, error)
	Requests(ctx context.Context, campaignID, keyUserID, actingUserID string) (any, error)
	Approve(ctx context.Context, campaignID, keyUserID, actingUserID string, moveID int64) (any, error)
	Decline(ctx context.Context, campaignID, keyUserID, actingUserID string, moveID int64) (any, error)
	Downtime(ctx context.Context, campaignID string) (any, error)
	SetDowntime(ctx context.Context, campaignID, keyUserID, actingUserID string, open bool) (any, error)
}

// StashEndpoint is one side of a move on the wire.
type StashEndpoint struct {
	Kind string
	ID   string
}

// StashMoveRequest is a decoded POST /stashes/moves body.
type StashMoveRequest struct {
	ActingUserID string
	Kind         string
	ItemID       string
	Quantity     int
	Amount       string
	From         StashEndpoint
	To           StashEndpoint
}

// StashAPIHandler serves the stash endpoints. Thin on purpose: it decodes the
// request and hands it, with the key holder and the named member, to the
// service, which owns every rule.
type StashAPIHandler struct {
	svc StashAPIService
	// addonSlug is the addon that must be enabled; given by the wiring so this
	// plugin does not name another plugin.
	addonSlug string
}

// NewStashAPIHandler creates the stash API handler.
func NewStashAPIHandler(svc StashAPIService, addonSlug string) *StashAPIHandler {
	return &StashAPIHandler{svc: svc, addonSlug: addonSlug}
}

// flexID decodes an id sent as a JSON string or number: stash ids are
// numeric in the database, and a client should not have to quote them.
type flexID string

func (f *flexID) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexID(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexID(n.String())
	return nil
}

type stashEndpointBody struct {
	Kind string `json:"kind"`
	ID   flexID `json:"id"`
}

type stashMoveBody struct {
	ActingUserID string            `json:"actingUserId"`
	Kind         string            `json:"kind"`
	ItemID       string            `json:"itemId"`
	Quantity     int               `json:"quantity"`
	Amount       json.Number       `json:"amount"`
	From         stashEndpointBody `json:"from"`
	To           stashEndpointBody `json:"to"`
}

// actingBody is the optional body of the answer endpoints.
type actingBody struct {
	ActingUserID string `json:"actingUserId"`
}

func keyUser(c echo.Context) (string, error) {
	key := GetAPIKey(c)
	if key == nil {
		return "", apperror.NewUnauthorized("api key required")
	}
	return key.UserID, nil
}

// decodeOptional reads a JSON body that may be absent.
func decodeOptional(c echo.Context, v any) error {
	err := json.NewDecoder(c.Request().Body).Decode(v)
	if err != nil && !errors.Is(err, io.EOF) {
		return apperror.NewBadRequest("invalid request body")
	}
	return nil
}

func parseMoveID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("moveId"), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperror.NewNotFound("request not found")
	}
	return id, nil
}

// View returns what the acting member sees for a character.
// GET /api/v1/campaigns/:id/stashes/view?characterId=&actingUserId=
func (h *StashAPIHandler) View(c echo.Context) error {
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	characterID := c.QueryParam("characterId")
	if characterID == "" {
		return apperror.NewBadRequest("characterId is required")
	}
	out, err := h.svc.View(c.Request().Context(), c.Param("id"), uid, c.QueryParam("actingUserId"), characterID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Move applies or queues a move as the acting member.
// POST /api/v1/campaigns/:id/stashes/moves
func (h *StashAPIHandler) Move(c echo.Context) error {
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	var body stashMoveBody
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	out, err := h.svc.Move(c.Request().Context(), c.Param("id"), uid, StashMoveRequest{
		ActingUserID: body.ActingUserID, Kind: body.Kind, ItemID: body.ItemID,
		Quantity: body.Quantity, Amount: body.Amount.String(),
		From: StashEndpoint{Kind: body.From.Kind, ID: string(body.From.ID)},
		To:   StashEndpoint{Kind: body.To.Kind, ID: string(body.To.ID)},
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// History lists the moves of a character or a stash.
// GET /api/v1/campaigns/:id/stashes/history?characterId=|stashId=&actingUserId=
func (h *StashAPIHandler) History(c echo.Context) error {
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	out, err := h.svc.History(c.Request().Context(), c.Param("id"), uid,
		c.QueryParam("actingUserId"), c.QueryParam("characterId"), c.QueryParam("stashId"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Requests lists the pending requests; the acting member must be an approver.
// GET /api/v1/campaigns/:id/stashes/requests?actingUserId=
func (h *StashAPIHandler) Requests(c echo.Context) error {
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Requests(c.Request().Context(), c.Param("id"), uid, c.QueryParam("actingUserId"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Approve answers a pending request with a yes.
// POST /api/v1/campaigns/:id/stashes/requests/:moveId/approve
func (h *StashAPIHandler) Approve(c echo.Context) error {
	return h.answer(c, h.svc.Approve)
}

// Decline answers a pending request with a no.
// POST /api/v1/campaigns/:id/stashes/requests/:moveId/decline
func (h *StashAPIHandler) Decline(c echo.Context) error {
	return h.answer(c, h.svc.Decline)
}

func (h *StashAPIHandler) answer(c echo.Context, fn func(ctx context.Context, campaignID, keyUserID, actingUserID string, moveID int64) (any, error)) error {
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	id, err := parseMoveID(c)
	if err != nil {
		return err
	}
	var body actingBody
	if err := decodeOptional(c, &body); err != nil {
		return err
	}
	out, err := fn(c.Request().Context(), c.Param("id"), uid, body.ActingUserID, id)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Downtime reports whether downtime is open.
// GET /api/v1/campaigns/:id/stashes/downtime
func (h *StashAPIHandler) Downtime(c echo.Context) error {
	out, err := h.svc.Downtime(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// SetDowntime opens or closes downtime; only an approver may.
// PUT /api/v1/campaigns/:id/stashes/downtime  {open, actingUserId}
func (h *StashAPIHandler) SetDowntime(c echo.Context) error {
	uid, err := keyUser(c)
	if err != nil {
		return err
	}
	var body struct {
		Open         *bool  `json:"open"`
		ActingUserID string `json:"actingUserId"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if body.Open == nil {
		return apperror.NewBadRequest("open is required")
	}
	out, err := h.svc.SetDowntime(c.Request().Context(), c.Param("id"), uid, body.ActingUserID, *body.Open)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
