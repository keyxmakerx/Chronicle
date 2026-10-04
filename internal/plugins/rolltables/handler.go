// The browser HTTP surface for roll tables. Handlers only bind, call the
// service and shape the response; the reads are open to scribes and the DM team, writes to the DM team only.
package rolltables

import (
	"io"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves the roll-table routes.
type Handler struct {
	svc Service
}

// NewHandler builds the handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// Get returns the campaign's tables.
//
// GET /campaigns/:id/roll-tables
func (h *Handler) Get(c echo.Context) error {
	cc, err := requireContext(c)
	if err != nil {
		return err
	}
	if !canRead(cc) {
		return apperror.NewForbidden(readDenyMsg)
	}
	doc, err := h.svc.Get(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, doc)
}

// Put replaces the campaign's whole set of tables.
//
// PUT /campaigns/:id/roll-tables
func (h *Handler) Put(c echo.Context) error {
	cc, err := requireContext(c)
	if err != nil {
		return err
	}
	if !cc.CanAuthorDmOnly() {
		return apperror.NewForbidden(writeDenyMsg)
	}
	// Read one byte past the limit so the service, which owns the size rule,
	// can tell "exactly at the limit" from "over" and word the error itself.
	body, rerr := io.ReadAll(io.LimitReader(c.Request().Body, MaxBodyBytes+1))
	if rerr != nil {
		return apperror.NewBadRequest("request body could not be read")
	}
	doc, err := h.svc.Put(c.Request().Context(), cc.Campaign.ID, body, auth.GetUserID(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, doc)
}

const (
	readDenyMsg  = "only the campaign owner, a scribe or a member with DM access may do this"
	writeDenyMsg = "only the campaign owner or a member with DM access may do this"
)

// canRead lets scribes read the tables (they edit pages and use the roller
// there) as well as the DM team; writes stay DM-team only.
func canRead(cc *campaigns.CampaignContext) bool {
	return cc.MemberRole >= campaigns.RoleScribe || cc.CanAuthorDmOnly()
}

// requireContext fetches the campaign context. Handlers re-check the
// capability themselves rather than trusting the route gate alone, so a
// mis-wired route cannot expose or overwrite the tables.
func requireContext(c echo.Context) (*campaigns.CampaignContext, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return nil, apperror.NewMissingContext()
	}
	return cc, nil
}
