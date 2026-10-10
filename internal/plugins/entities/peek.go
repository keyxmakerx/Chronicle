package entities

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// PeekFact is one label/value row shown under the title of a peeked page.
type PeekFact struct {
	Label string
	Value string
}

// PeekView is everything the side panel draws. It is built only from data
// that has already passed the same checks as the full page, so the template
// never has to decide who may see what.
type PeekView struct {
	Name     string
	Kind     string
	KindIcon string
	Private  bool
	Facts    []PeekFact
	// BodyHTML is already stripped of GM-only content for viewers who may not
	// see it and sanitized; the template renders it as-is.
	BodyHTML string
	FullURL  string
}

// Peek serves the read-only body of a page for the side panel
// (GET /campaigns/:id/entities/:eid/peek).
//
// It repeats Show's gates in the same order and answers a page the viewer
// cannot see exactly as Show does (the same NotFound), so the panel can never
// reveal that a page exists or what it is called. Anything it draws is the
// subset of the full page a viewer is allowed to read: field values pass
// FilterRestrictedFields and the entry passes the same secret stripping as
// GetEntry. Editing is never offered here.
func (h *Handler) Peek(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	// A person who pastes the address into the browser gets the real page
	// instead of a bare fragment.
	if c.Request().Header.Get("Sec-Fetch-Dest") == "document" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/campaigns/%s/entities/%s", cc.Campaign.ID, c.Param("eid")))
	}

	ctx := c.Request().Context()
	entity, err := h.service.GetByID(ctx, c.Param("eid"))
	if err != nil {
		return err
	}

	// IDOR protection: this route is public-capable, so a page from another
	// campaign must look like no page at all.
	if entity.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity not found")
	}

	// VisibilityRole, not MemberRole, for the same reason as Show: a Co-DM is
	// promoted for reading DM-only pages.
	userID := auth.GetUserID(c)
	access, err := h.service.CheckEntityAccess(ctx, entity.ID, int(cc.VisibilityRole()), userID)
	if err != nil || !access.CanView {
		return apperror.NewNotFound("entity not found")
	}

	entityType, err := h.service.GetEntityTypeByID(ctx, entity.EntityTypeID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("get entity type %d: %w", entity.EntityTypeID, err))
	}

	canSeeGM := cc.MemberRole >= campaigns.RoleScribe

	view := PeekView{
		Name:     entity.Name,
		Kind:     entityType.Name,
		KindIcon: strings.TrimSpace(strings.TrimPrefix(entityType.Icon, "fa-solid ")),
		Private:  entity.IsPrivate,
		FullURL:  fmt.Sprintf("/campaigns/%s/entities/%s", cc.Campaign.ID, entity.ID),
	}
	if entity.TypeLabel != nil && *entity.TypeLabel != "" {
		view.Kind += " · " + *entity.TypeLabel
	}

	// Same field rules as the page: GM-only and owner-only values are dropped
	// before anything is listed.
	// A campaign that switched the attributes addon off shows no field values
	// on the page, so the panel shows none either.
	fieldsData := FilterRestrictedFields(entity.FieldsData, entityType.Fields, canSeeGM, entity.IsOwnedBy(userID))
	if !h.isAddonEnabled(ctx, cc.Campaign.ID, "attributes") {
		fieldsData = nil
	}
	for _, fd := range MergeFields(entityType.Fields, entity.FieldOverrides) {
		val, ok := fieldsData[fd.Key]
		if !ok || val == nil {
			continue
		}
		s := fmt.Sprintf("%v", val)
		if s == "" {
			continue
		}
		view.Facts = append(view.Facts, PeekFact{Label: fd.Label, Value: s})
	}

	// The rendered text is what the panel shows. The editor saves it together
	// with the JSON document, and there is no server-side renderer for the
	// JSON, so the HTML is stripped here the way GetEntry strips both forms.
	if entity.EntryHTML != nil {
		body := *entity.EntryHTML
		if !canSeeGM {
			body = sanitize.StripSecretsHTML(body)
		}
		// Stored text is sanitized on write; doing it again is cheap and keeps
		// a historical row from reaching the panel unsanitized.
		view.BodyHTML = sanitize.HTML(body)
	}

	// The answer depends on who asks, so no shared cache may keep it.
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return middleware.Render(c, http.StatusOK, EntityPeekPanel(view))
}
