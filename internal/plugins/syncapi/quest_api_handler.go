package syncapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// questBodyLimit bounds a quest write. It matches the quest plugin's own cap,
// which refuses anything larger again.
const questBodyLimit = 256 << 10

// QuestAPIService is the quests feature as the sync API needs it, implemented
// in internal/app so this plugin never imports the quests or armory plugins.
// Payloads are opaque here and serialised as the service returns them.
//
// players asks for what a plain player may see (the Foundry module shows that
// one copy to every player at the table); otherwise the caller's own view.
type QuestAPIService interface {
	Homes(ctx context.Context, campaignID, userID string, players bool) (any, error)
	Boards(ctx context.Context, campaignID, userID string, home QuestHome, players bool) (any, error)
	Quest(ctx context.Context, campaignID, userID, entityID string, players bool) (any, error)
	PutQuest(ctx context.Context, campaignID, userID, entityID string, body []byte) (any, error)
	Party(ctx context.Context, campaignID, userID string) (any, error)
	Pay(ctx context.Context, campaignID, userID string, in QuestPay) (any, error)
	Give(ctx context.Context, campaignID, userID string, in QuestGive) (any, error)
}

// QuestHome names where boards live: Kind "category" (an entity type id) or
// "page" (an entity id).
type QuestHome struct {
	Kind string
	ID   string
}

// QuestPay is coins for one character's sheet, in the sheet's main unit.
type QuestPay struct {
	CharacterID string  `json:"characterId"`
	Amount      float64 `json:"amount"`
	Reason      string  `json:"reason"`
}

// QuestGive is one reward item for one character.
type QuestGive struct {
	CharacterID string `json:"characterId"`
	ItemID      string `json:"itemId"`
}

// QuestAPIHandler serves the quest endpoints. Every route is for the campaign
// owner or a co-DM: players have no key of their own, and the GM's client
// passes on only the players view. The services still apply their own rules.
type QuestAPIHandler struct {
	svc         QuestAPIService
	campaignSvc campaigns.CampaignService
	// rewardAddon gates paying and giving; named by the wiring so this plugin
	// does not name another plugin.
	rewardAddon string
}

// NewQuestAPIHandler creates the quest API handler.
func NewQuestAPIHandler(svc QuestAPIService, campaignSvc campaigns.CampaignService, rewardAddon string) *QuestAPIHandler {
	return &QuestAPIHandler{svc: svc, campaignSvc: campaignSvc, rewardAddon: rewardAddon}
}

// WithQuests mounts the quest endpoints.
func WithQuests(h *QuestAPIHandler) func(*APIHandler) {
	return func(api *APIHandler) { api.quests = h }
}

// dmUser returns the caller when they are the campaign owner or a co-DM, and
// fails closed on any lookup error.
func (h *QuestAPIHandler) dmUser(c echo.Context) (string, error) {
	key := GetAPIKey(c)
	if key == nil {
		return "", apperror.NewUnauthorized("api key required")
	}
	ctx := c.Request().Context()
	if member, err := h.campaignSvc.GetMember(ctx, key.CampaignID, key.UserID); err == nil && member.Role >= campaigns.RoleOwner {
		return key.UserID, nil
	}
	if granted, err := h.campaignSvc.IsUserDmGranted(ctx, key.CampaignID, key.UserID); err == nil && granted {
		return key.UserID, nil
	}
	return "", apperror.NewForbidden("quests over the API need owner or co-DM access")
}

func playersView(c echo.Context) bool { return c.QueryParam("audience") == "players" }

func (h *QuestAPIHandler) answer(c echo.Context, out any, err error) error {
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Homes lists the places with boards.
// GET /api/v1/campaigns/:id/quests/homes[?audience=players]
func (h *QuestAPIHandler) Homes(c echo.Context) error {
	uid, err := h.dmUser(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Homes(c.Request().Context(), c.Param("id"), uid, playersView(c))
	return h.answer(c, out, err)
}

// Boards returns one home's boards.
// GET /api/v1/campaigns/:id/quests/boards?category=<typeId>|page=<entityId>[&audience=players]
func (h *QuestAPIHandler) Boards(c echo.Context) error {
	uid, err := h.dmUser(c)
	if err != nil {
		return err
	}
	cat, page := strings.TrimSpace(c.QueryParam("category")), strings.TrimSpace(c.QueryParam("page"))
	var home QuestHome
	switch {
	case cat != "" && page == "":
		home = QuestHome{Kind: "category", ID: cat}
	case page != "" && cat == "":
		home = QuestHome{Kind: "page", ID: page}
	default:
		return apperror.NewBadRequest("name exactly one of category or page")
	}
	out, err := h.svc.Boards(c.Request().Context(), c.Param("id"), uid, home, playersView(c))
	return h.answer(c, out, err)
}

// GetQuest returns one quest sheet.
// GET /api/v1/campaigns/:id/quests/:entityID[?audience=players]
func (h *QuestAPIHandler) GetQuest(c echo.Context) error {
	uid, err := h.dmUser(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Quest(c.Request().Context(), c.Param("id"), uid, c.Param("entityID"), playersView(c))
	return h.answer(c, out, err)
}

// PutQuest saves a partial quest change; the body is the quest plugin's own
// patch (version required, absent fields kept).
// PUT /api/v1/campaigns/:id/quests/:entityID
func (h *QuestAPIHandler) PutQuest(c echo.Context) error {
	uid, err := h.dmUser(c)
	if err != nil {
		return err
	}
	raw, rerr := io.ReadAll(io.LimitReader(c.Request().Body, questBodyLimit+1))
	if rerr != nil || len(raw) == 0 || len(raw) > questBodyLimit {
		return apperror.NewBadRequest("request body is missing or too large")
	}
	out, err := h.svc.PutQuest(c.Request().Context(), c.Param("id"), uid, c.Param("entityID"), raw)
	return h.answer(c, out, err)
}

// Party lists the characters rewards can go to.
// GET /api/v1/campaigns/:id/quests/party
func (h *QuestAPIHandler) Party(c echo.Context) error {
	uid, err := h.dmUser(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Party(c.Request().Context(), c.Param("id"), uid)
	return h.answer(c, out, err)
}

// Pay adds coins to one character's sheet.
// POST /api/v1/campaigns/:id/quests/pay {characterId, amount, reason}
func (h *QuestAPIHandler) Pay(c echo.Context) error {
	uid, err := h.dmUser(c)
	if err != nil {
		return err
	}
	var in QuestPay
	if err := json.NewDecoder(io.LimitReader(c.Request().Body, 4<<10)).Decode(&in); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	out, err := h.svc.Pay(c.Request().Context(), c.Param("id"), uid, in)
	return h.answer(c, out, err)
}

// Give hands one reward item to one character.
// POST /api/v1/campaigns/:id/quests/give {characterId, itemId}
func (h *QuestAPIHandler) Give(c echo.Context) error {
	uid, err := h.dmUser(c)
	if err != nil {
		return err
	}
	var in QuestGive
	if err := json.NewDecoder(io.LimitReader(c.Request().Body, 4<<10)).Decode(&in); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	out, err := h.svc.Give(c.Request().Context(), c.Param("id"), uid, in)
	return h.answer(c, out, err)
}
