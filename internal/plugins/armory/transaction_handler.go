// transaction_handler.go provides HTTP endpoints for shop transactions.
// Thin handlers: bind request, call service, return JSON. No business logic.
package armory

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TransactionHandler serves transaction REST endpoints.
type TransactionHandler struct {
	svc    TransactionService
	reader TransactionReader
}

// NewTransactionHandler creates a new transaction handler. List endpoints go
// through a visibility-aware reader that fails closed until
// SetEntityVisibility wires the canonical filter.
func NewTransactionHandler(svc TransactionService) *TransactionHandler {
	return &TransactionHandler{svc: svc, reader: NewTransactionReader(svc, nil)}
}

// SetEntityVisibility injects the entity visibility filter the list endpoints
// use to hide names of entities the viewer cannot see.
func (h *TransactionHandler) SetEntityVisibility(v EntityVisibilityFilter) {
	h.reader = NewTransactionReader(h.svc, v)
}

// listError keeps domain errors (e.g. not found) intact and wraps the rest.
func listError(err error) error {
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		return err
	}
	return apperror.NewInternal(err)
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

	txs, total, err := h.reader.ListTransactions(c.Request().Context(), cc.Campaign.ID, cc.VisibilityRole(), auth.GetUserID(c), opts)
	if err != nil {
		return listError(err)
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

	// Scope to the caller's campaign so a shop id from another campaign returns
	// no transactions (SEC-IDOR-3).
	txs, total, err := h.reader.ListShopTransactions(c.Request().Context(), cc.Campaign.ID, entityID, cc.VisibilityRole(), auth.GetUserID(c), opts)
	if err != nil {
		return listError(err)
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
