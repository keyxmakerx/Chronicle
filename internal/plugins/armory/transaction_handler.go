// transaction_handler.go provides HTTP endpoints for shop transactions.
// Thin handlers: bind request, call service, return JSON. No business logic.
package armory

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TransactionHandler serves transaction REST endpoints.
type TransactionHandler struct {
	svc TransactionService
}

// NewTransactionHandler creates a new transaction handler.
func NewTransactionHandler(svc TransactionService) *TransactionHandler {
	return &TransactionHandler{svc: svc}
}

// Purchase handles POST /campaigns/:id/armory/purchase.
// Validates stock, creates transaction, decrements shop inventory.
func (h *TransactionHandler) Purchase(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var input CreateTransactionInput
	if err := json.NewDecoder(c.Request().Body).Decode(&input); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	userID := auth.GetUserID(c)
	result, err := h.svc.Purchase(c.Request().Context(), cc.Campaign.ID, userID, int(cc.MemberRole), input)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, result)
}

// CreateTransaction handles POST /campaigns/:id/armory/transactions.
// Records a manual transaction (gift, transfer, restock).
func (h *TransactionHandler) CreateTransaction(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var input CreateTransactionInput
	if err := json.NewDecoder(c.Request().Body).Decode(&input); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	userID := auth.GetUserID(c)
	tx, err := h.svc.CreateTransaction(c.Request().Context(), cc.Campaign.ID, userID, input)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, tx)
}

// ListTransactions handles GET /campaigns/:id/armory/transactions.
// Returns paginated transactions with optional filters.
func (h *TransactionHandler) ListTransactions(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	opts := DefaultTransactionListOptions()
	if p := c.QueryParam("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			opts.Page = n
		}
	}
	if pp := c.QueryParam("per_page"); pp != "" {
		if n, err := strconv.Atoi(pp); err == nil && n > 0 && n <= 100 {
			opts.PerPage = n
		}
	}
	if sid := c.QueryParam("shop"); sid != "" {
		opts.ShopEntityID = sid
	}
	if bid := c.QueryParam("buyer"); bid != "" {
		opts.BuyerEntityID = bid
	}
	if iid := c.QueryParam("item"); iid != "" {
		opts.ItemEntityID = iid
	}
	if tt := c.QueryParam("type"); tt != "" {
		opts.TransactionType = tt
	}

	txs, total, err := h.svc.ListTransactions(c.Request().Context(), cc.Campaign.ID, opts)
	if err != nil {
		return apperror.NewInternal(err)
	}

	// Return empty array, not null.
	if txs == nil {
		txs = []Transaction{}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"data":  txs,
		"total": total,
		"page":  opts.Page,
	})
}

// ListShopTransactions handles GET /campaigns/:id/armory/shops/:eid/transactions.
// Returns transactions for a specific shop entity.
func (h *TransactionHandler) ListShopTransactions(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	opts := DefaultTransactionListOptions()
	if p := c.QueryParam("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			opts.Page = n
		}
	}

	txs, total, err := h.svc.ListShopTransactions(c.Request().Context(), entityID, opts)
	if err != nil {
		return apperror.NewInternal(err)
	}

	if txs == nil {
		txs = []Transaction{}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"data":  txs,
		"total": total,
		"page":  opts.Page,
	})
}
