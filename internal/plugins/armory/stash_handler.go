// stash_handler.go provides the HTTP endpoints for stashes, moves and the
// downtime switch. Thin handlers: bind the request, call the service, render
// a fragment or answer with an HTMX trigger. Every rule — who may see, move or
// answer — is in the service; the handler only hands it the caller.
package armory

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// StashHandler serves the stash, move and downtime endpoints.
type StashHandler struct {
	svc StashService
}

// NewStashHandler creates the stash handler.
func NewStashHandler(svc StashService) *StashHandler {
	return &StashHandler{svc: svc}
}

// caller extracts the campaign and the acting user. The role is the
// VisibilityRole so a DM-granted co-DM is treated as Owner, matching every
// other armory path.
func caller(c echo.Context) (*campaigns.CampaignContext, Actor, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return nil, Actor{}, apperror.NewMissingContext()
	}
	return cc, Actor{UserID: auth.GetUserID(c), Role: cc.VisibilityRole()}, nil
}

// done answers a successful write. HTMX callers get a toast and a signal that
// makes every armory panel on the page reload itself; anything else is sent
// back to the stashes page.
func done(c echo.Context, cc *campaigns.CampaignContext, message string) error {
	if !middleware.IsHTMX(c) {
		return c.Redirect(http.StatusSeeOther, stashesURL(cc.Campaign.ID))
	}
	trigger := map[string]any{"armory-moved": true}
	if message != "" {
		trigger["chronicle:notify"] = map[string]string{"message": message, "type": "success"}
	}
	c.Response().Header().Set("HX-Trigger", triggerJSON(trigger))
	c.Response().Header().Set("HX-Reswap", "none")
	return c.NoContent(http.StatusNoContent)
}

// triggerJSON encodes an HX-Trigger header value with every non-ASCII
// character escaped (\u00d7 for ×). Browsers read header bytes as Latin-1, so
// an item name or "×" sent raw would arrive garbled in the toast.
func triggerJSON(v any) string {
	b, _ := json.Marshal(v)
	var sb strings.Builder
	for _, r := range string(b) {
		switch {
		case r < 0x80:
			sb.WriteRune(r)
		case r <= 0xFFFF:
			fmt.Fprintf(&sb, `\u%04x`, r)
		default:
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&sb, `\u%04x\u%04x`, hi, lo)
		}
	}
	return sb.String()
}

// Page renders GET /campaigns/:id/armory/stashes.
func (h *StashHandler) Page(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	view, err := h.svc.StashesPage(c.Request().Context(), cc.Campaign.ID, a)
	if err != nil {
		return err
	}
	csrf := middleware.GetCSRFToken(c)
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, StashesContent(cc, view, csrf))
	}
	return middleware.Render(c, http.StatusOK, StashesPage(cc, view, csrf))
}

// CreateStash handles POST /armory/stashes.
func (h *StashHandler) CreateStash(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	_, err = h.svc.CreateStash(c.Request().Context(), cc.Campaign.ID, a, CreateStashInput{
		Name:     c.FormValue("name"),
		Location: c.FormValue("location"),
	})
	if err != nil {
		return err
	}
	return done(c, cc, "Stash added.")
}

// updateStashRequest is the JSON wire form of PUT /armory/stashes/:sid. It is
// a PARTIAL update: patch.Field keeps "absent" and "null" apart, so a
// name-only push cannot wipe the location.
type updateStashRequest struct {
	Name     patch.Field[string] `json:"name"`
	Location patch.Field[string] `json:"location"`
}

func (r updateStashRequest) input() UpdateStashInput {
	return UpdateStashInput(r)
}

// UpdateStash handles PUT /armory/stashes/:sid.
func (h *StashHandler) UpdateStash(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	id, err := strconv.Atoi(c.Param("sid"))
	if err != nil {
		return notFound("stash")
	}

	var in UpdateStashInput
	if strings.HasPrefix(c.Request().Header.Get("Content-Type"), "application/json") {
		var req updateStashRequest
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return apperror.NewBadRequest("invalid JSON body")
		}
		in = req.input()
	} else {
		// A form sends only the fields it has. A blank location means "clear".
		form, err := c.FormParams()
		if err != nil {
			return apperror.NewBadRequest("invalid form")
		}
		if v, ok := form["name"]; ok && len(v) > 0 {
			in.Name = patch.Of(v[0])
		}
		if v, ok := form["location"]; ok && len(v) > 0 {
			if strings.TrimSpace(v[0]) == "" {
				in.Location = patch.Null[string]()
			} else {
				in.Location = patch.Of(v[0])
			}
		}
	}
	if err := h.svc.UpdateStash(c.Request().Context(), cc.Campaign.ID, a, id, in); err != nil {
		return err
	}
	return done(c, cc, "Stash saved.")
}

// DeleteStash handles DELETE /armory/stashes/:sid.
func (h *StashHandler) DeleteStash(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	id, err := strconv.Atoi(c.Param("sid"))
	if err != nil {
		return notFound("stash")
	}
	if err := h.svc.DeleteStash(c.Request().Context(), cc.Campaign.ID, a, id); err != nil {
		return err
	}
	return done(c, cc, "Stash deleted.")
}

// SetViewers handles PUT /armory/stashes/:sid/viewers. The body is the full
// set of characters that may see the stash, so an unchecked box is a removal.
func (h *StashHandler) SetViewers(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	id, err := strconv.Atoi(c.Param("sid"))
	if err != nil {
		return notFound("stash")
	}
	form, err := c.FormParams()
	if err != nil {
		return apperror.NewBadRequest("invalid form")
	}
	if err := h.svc.SetStashViewers(c.Request().Context(), cc.Campaign.ID, a, id, form["viewer"]); err != nil {
		return err
	}
	return done(c, cc, "Who can see this stash was saved.")
}

// AddItem handles POST /armory/stashes/:sid/items.
func (h *StashHandler) AddItem(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	id, err := strconv.Atoi(c.Param("sid"))
	if err != nil {
		return notFound("stash")
	}
	qty, err := strconv.Atoi(strings.TrimSpace(c.FormValue("quantity")))
	if err != nil {
		return apperror.NewBadRequest("Enter a quantity of at least 1.")
	}
	if err := h.svc.AddStashItem(c.Request().Context(), cc.Campaign.ID, a, id, c.FormValue("item_id"), qty); err != nil {
		return err
	}
	return done(c, cc, "Item added to the stash.")
}

// StashHistory handles GET /armory/stashes/:sid/history.
func (h *StashHandler) StashHistory(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	id, err := strconv.Atoi(c.Param("sid"))
	if err != nil {
		return notFound("stash")
	}
	lines, err := h.svc.StashHistory(c.Request().Context(), cc.Campaign.ID, a, id)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, MoveHistoryList(lines))
}

// MoveDialog handles GET /armory/move: the dialog for one item or money line.
func (h *StashHandler) MoveDialog(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	from := Endpoint{Kind: c.QueryParam("from_kind"), ID: c.QueryParam("from_id")}
	view, err := h.svc.MoveDialog(c.Request().Context(), cc.Campaign.ID, a, c.QueryParam("kind"), from, c.QueryParam("item_id"))
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, MoveDialog(view, middleware.GetCSRFToken(c)))
}

// GiveDialog handles GET /armory/give: the Give box. ?character= fixes the
// recipient (the character page's box), ?item= fixes the item (the Armory
// card's box); ?list=items&q= answers the box's search with the list alone.
func (h *StashHandler) GiveDialog(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	view, err := h.svc.GiveDialog(c.Request().Context(), cc.Campaign.ID, a, c.QueryParam("character"), c.QueryParam("item"), c.QueryParam("q"))
	if err != nil {
		return err
	}
	switch {
	case view.Item != nil:
		return middleware.Render(c, http.StatusOK, GiveCardBox(view))
	case c.QueryParam("list") == "items":
		return middleware.Render(c, http.StatusOK, GiveItemChoices(view))
	}
	return middleware.Render(c, http.StatusOK, GiveBox(view))
}

// Give handles POST /armory/give. Besides the usual reload signal it answers
// with armory-given, which the Give box reads for its toast and to flash the
// line the item landed on.
func (h *StashHandler) Give(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	// A blank "how many" means one; a map has none.
	qty := 1
	if v := strings.TrimSpace(c.FormValue("quantity")); v != "" {
		if qty, err = strconv.Atoi(v); err != nil {
			return apperror.NewBadRequest(giveQuantityMessage)
		}
	}
	out, err := h.svc.Give(c.Request().Context(), cc.Campaign.ID, a, GiveInput{
		CharacterID: c.FormValue("character_id"),
		ItemID:      c.FormValue("item_id"),
		MapID:       c.FormValue("map_id"),
		Quantity:    qty,
	})
	if err != nil {
		return err
	}
	if !middleware.IsHTMX(c) {
		return c.Redirect(http.StatusSeeOther, stashesURL(cc.Campaign.ID))
	}
	msg := fmt.Sprintf("Gave %s %d × %s", out.CharacterName, out.Move.Quantity, out.ItemName)
	c.Response().Header().Set("HX-Trigger", triggerJSON(map[string]any{
		"armory-moved": true,
		"armory-given": map[string]string{"message": msg, "itemId": out.Move.ItemEntityID, "characterId": out.Move.To.ID},
	}))
	c.Response().Header().Set("HX-Reswap", "none")
	return c.NoContent(http.StatusNoContent)
}

// parseDestination splits the "kind:id" value of the To select.
func parseDestination(v string) Endpoint {
	kind, id, _ := strings.Cut(v, ":")
	return Endpoint{Kind: kind, ID: id}
}

// Move handles POST /armory/moves.
func (h *StashHandler) Move(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	in := MoveInput{
		Kind:         c.FormValue("kind"),
		ItemEntityID: c.FormValue("item_id"),
		Amount:       c.FormValue("amount"),
		From:         Endpoint{Kind: c.FormValue("from_kind"), ID: c.FormValue("from_id")},
		To:           parseDestination(c.FormValue("to")),
	}
	if in.Kind == MoveKindItem {
		q, err := strconv.Atoi(strings.TrimSpace(c.FormValue("quantity")))
		if err != nil {
			return apperror.NewBadRequest("Enter a quantity of at least 1.")
		}
		in.Quantity = q
	}
	out, err := h.svc.Move(c.Request().Context(), cc.Campaign.ID, a, in)
	if err != nil {
		return err
	}
	if out.Applied {
		return done(c, cc, "Moved.")
	}
	return done(c, cc, "Asked the GM. It stays with you until they answer.")
}

// moveID reads the :mid path parameter.
func moveID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("mid"), 10, 64)
	if err != nil || id < 1 {
		return 0, notFound("request")
	}
	return id, nil
}

// Approve handles POST /armory/moves/:mid/approve.
func (h *StashHandler) Approve(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	id, err := moveID(c)
	if err != nil {
		return err
	}
	m, err := h.svc.Approve(c.Request().Context(), cc.Campaign.ID, a, id)
	if err != nil {
		return err
	}
	if m.Status == MoveFailed {
		return doneWithTone(c, cc, "The request could not go through: "+m.Reason, "error")
	}
	return done(c, cc, "Approved.")
}

// Decline handles POST /armory/moves/:mid/decline.
func (h *StashHandler) Decline(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	id, err := moveID(c)
	if err != nil {
		return err
	}
	if _, err := h.svc.Decline(c.Request().Context(), cc.Campaign.ID, a, id); err != nil {
		return err
	}
	return done(c, cc, "Turned down.")
}

// doneWithTone is done with a notification type other than success.
func doneWithTone(c echo.Context, cc *campaigns.CampaignContext, message, tone string) error {
	if !middleware.IsHTMX(c) {
		return c.Redirect(http.StatusSeeOther, stashesURL(cc.Campaign.ID))
	}
	c.Response().Header().Set("HX-Trigger", triggerJSON(map[string]any{
		"armory-moved":     true,
		"chronicle:notify": map[string]string{"message": message, "type": tone},
	}))
	c.Response().Header().Set("HX-Reswap", "none")
	return c.NoContent(http.StatusNoContent)
}

// SetDowntime handles POST /armory/downtime (Owner visibility). There is no
// page control for it yet; the service and this endpoint are the backend.
func (h *StashHandler) SetDowntime(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	var open bool
	switch c.FormValue("open") {
	case "true":
		open = true
	case "false":
	default:
		return apperror.NewBadRequest("Choose Open or Closed.")
	}
	res, err := h.svc.SetDowntime(c.Request().Context(), cc.Campaign.ID, a, open)
	if err != nil {
		return err
	}
	switch {
	case !open:
		return done(c, cc, "Downtime is closed.")
	case res.Failed > 0:
		return doneWithTone(c, cc, "Downtime is open. "+plural(res.Applied, "request")+" went through; "+strconv.Itoa(res.Failed)+" could not.", "warning")
	case res.Applied > 0:
		return done(c, cc, "Downtime is open. "+plural(res.Applied, "request")+" went through.")
	}
	return done(c, cc, "Downtime is open.")
}

// CharacterPanel handles GET /armory/characters/:eid/panel. A character the
// viewer has no business with answers with an empty fragment, so the page that
// asked for it simply shows nothing there.
func (h *StashHandler) CharacterPanel(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	view, err := h.svc.CharacterPanel(c.Request().Context(), cc.Campaign.ID, a, c.Param("eid"))
	if err != nil {
		return err
	}
	if view == nil {
		return c.NoContent(http.StatusOK)
	}
	return middleware.Render(c, http.StatusOK, CharacterPanel(view, middleware.GetCSRFToken(c)))
}

// CharacterHistory handles GET /armory/characters/:eid/history.
func (h *StashHandler) CharacterHistory(c echo.Context) error {
	cc, a, err := caller(c)
	if err != nil {
		return err
	}
	lines, err := h.svc.CharacterHistory(c.Request().Context(), cc.Campaign.ID, a, c.Param("eid"))
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, MoveHistoryList(lines))
}
