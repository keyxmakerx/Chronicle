package entities

// era_handler.go: the routes that make, change and remove a page's version
// for one era. The text, fields and picture routes answer the same shapes as
// the usual page's, so the editor, attributes and picture widgets work on a
// version unchanged; only their endpoint differs.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// eraTarget is the page and era an era route acts on, already checked.
type eraTarget struct {
	cc      *campaigns.CampaignContext
	entity  *Entity
	set     *PageEraSet
	eraID   int
	userID  string
	canEdit bool
}

// loadEraTarget checks the page and era for an era route. Someone who may
// not see other eras gets the same NotFound for any era but the current one,
// so the routes tell them nothing about eras still hidden from them.
func (h *Handler) loadEraTarget(c echo.Context) (*eraTarget, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return nil, apperror.NewMissingContext()
	}
	if h.eraPages == nil {
		return nil, apperror.NewNotFound("era not found")
	}
	ctx := c.Request().Context()
	entity, err := h.service.GetByID(ctx, c.Param("eid"))
	if err != nil {
		return nil, err
	}
	if entity.CampaignID != cc.Campaign.ID {
		return nil, apperror.NewNotFound("entity not found")
	}
	userID := auth.GetUserID(c)
	access, err := h.service.CheckEntityAccess(ctx, entity.ID, int(cc.VisibilityRole()), userID)
	if err != nil || !access.CanView {
		return nil, apperror.NewNotFound("entity not found")
	}
	eraID, err := strconv.Atoi(c.Param("eraID"))
	if err != nil || eraID <= 0 {
		return nil, apperror.NewNotFound("era not found")
	}
	set, err := h.eraPages.PageEras(ctx, cc.Campaign.ID, permissions.RequestViewer(int(cc.VisibilityRole()), userID))
	if err != nil {
		return nil, err
	}
	if set == nil || set.Era(eraID) == nil || (!cc.CanAuthorDmOnly() && eraID != set.CurrentID) {
		return nil, apperror.NewNotFound("era not found")
	}
	return &eraTarget{cc: cc, entity: entity, set: set, eraID: eraID, userID: userID, canEdit: access.CanEdit}, nil
}

// requireWrite refuses a change by someone who may not edit this page, or
// may not change this era's version.
func (t *eraTarget) requireWrite() error {
	if t.cc.MemberRole < campaigns.RoleScribe || !t.canEdit {
		return apperror.NewForbidden("you can't edit this page")
	}
	if !mayWriteEraVersion(t.cc, t.set, t.eraID) {
		return apperror.NewForbidden("only the game master can change other eras")
	}
	return nil
}

// version returns the page's version for the target era, or NotFound.
func (h *Handler) eraVersionOf(c echo.Context, t *eraTarget) (*PageEraVersion, error) {
	v, err := h.eraPages.Version(c.Request().Context(), t.entity.ID, t.eraID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, apperror.NewNotFound("this page has no version for that era")
	}
	return v, nil
}

// eraPageURL is the page opened on an era in the Eras panel.
func eraPageURL(campaignID, entityID string, eraID int, query string) string {
	u := fmt.Sprintf("/campaigns/%s/entities/%s?era=%d", campaignID, entityID, eraID)
	if query != "" {
		u += "&" + query
	}
	return u
}

// CreateEraVersionAPI starts a version of the page for an era and opens it
// for editing.
// POST /campaigns/:id/entities/:eid/eras/:eraID
func (h *Handler) CreateEraVersionAPI(c echo.Context) error {
	t, err := h.loadEraTarget(c)
	if err != nil {
		return err
	}
	if !t.cc.CanAuthorDmOnly() {
		return apperror.NewForbidden("only the game master can make era versions")
	}
	if err := t.requireWrite(); err != nil {
		return err
	}
	if err := h.eraPages.CreateVersion(c.Request().Context(), t.cc.Campaign.ID, t.entity.ID, t.eraID, t.userID); err != nil {
		return err
	}
	h.logAudit(c, t.cc.Campaign.ID, audit.ActionEntityUpdated, t.entity.ID, t.entity.Name)
	return middleware.HTMXRedirect(c, eraPageURL(t.cc.Campaign.ID, t.entity.ID, t.eraID, "hl=0"))
}

// DeleteEraVersionAPI removes the page's version for an era; the page shows
// its usual self in that era again.
// DELETE /campaigns/:id/entities/:eid/eras/:eraID
func (h *Handler) DeleteEraVersionAPI(c echo.Context) error {
	t, err := h.loadEraTarget(c)
	if err != nil {
		return err
	}
	if !t.cc.CanAuthorDmOnly() {
		return apperror.NewForbidden("only the game master can remove era versions")
	}
	if err := t.requireWrite(); err != nil {
		return err
	}
	if err := h.eraPages.DeleteVersion(c.Request().Context(), t.cc.Campaign.ID, t.entity.ID, t.eraID); err != nil {
		return err
	}
	h.logAudit(c, t.cc.Campaign.ID, audit.ActionEntityUpdated, t.entity.ID, t.entity.Name)
	return middleware.HTMXRedirect(c, eraPageURL(t.cc.Campaign.ID, t.entity.ID, t.eraID, ""))
}

// UpdateEraIdentityAPI changes the name and picture a page has in an era.
// Partial: an absent key keeps, an empty or null one goes back to the usual
// page's. Accepts JSON or the Eras panel's form.
// PATCH /campaigns/:id/entities/:eid/eras/:eraID
func (h *Handler) UpdateEraIdentityAPI(c echo.Context) error {
	t, err := h.loadEraTarget(c)
	if err != nil {
		return err
	}
	if err := t.requireWrite(); err != nil {
		return err
	}
	var name, image patch.Field[string]
	if strings.HasPrefix(c.Request().Header.Get(echo.HeaderContentType), echo.MIMEApplicationJSON) {
		var body struct {
			Name      patch.Field[string] `json:"name"`
			ImagePath patch.Field[string] `json:"image_path"`
		}
		if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
			return apperror.NewBadRequest("invalid JSON body")
		}
		name, image = body.Name, body.ImagePath
	} else {
		form, err := c.FormParams()
		if err != nil {
			return apperror.NewBadRequest("invalid form")
		}
		if form.Has("name") {
			name = patch.Of(form.Get("name"))
		}
		if form.Has("usual_image") {
			image = patch.Null[string]()
		}
	}
	if p, ok := image.Get(); ok && strings.TrimSpace(p) != "" {
		if err := h.service.CheckImagePath(c.Request().Context(), t.entity.ID, p); err != nil {
			return err
		}
	}
	if err := h.eraPages.SetVersionIdentity(c.Request().Context(), t.cc.Campaign.ID, t.entity.ID, t.eraID, name, image, t.userID); err != nil {
		return err
	}
	h.logAudit(c, t.cc.Campaign.ID, audit.ActionEntityUpdated, t.entity.ID, t.entity.Name)
	if middleware.IsHTMX(c) {
		return middleware.HTMXRedirect(c, eraPageURL(t.cc.Campaign.ID, t.entity.ID, t.eraID, c.QueryString()))
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// UpdateEraImageAPI sets the version's picture, for the picture widget.
// PUT /campaigns/:id/entities/:eid/eras/:eraID/image
func (h *Handler) UpdateEraImageAPI(c echo.Context) error {
	t, err := h.loadEraTarget(c)
	if err != nil {
		return err
	}
	if err := t.requireWrite(); err != nil {
		return err
	}
	var body struct {
		ImagePath string `json:"image_path"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	image := patch.Null[string]()
	if strings.TrimSpace(body.ImagePath) != "" {
		if err := h.service.CheckImagePath(c.Request().Context(), t.entity.ID, body.ImagePath); err != nil {
			return err
		}
		image = patch.Of(body.ImagePath)
	}
	if err := h.eraPages.SetVersionIdentity(c.Request().Context(), t.cc.Campaign.ID, t.entity.ID, t.eraID, patch.Field[string]{}, image, t.userID); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// GetEraEntryAPI returns the version's text, or the usual text while the
// version has none of its own, in the usual entry route's shape.
// GET /campaigns/:id/entities/:eid/eras/:eraID/entry
func (h *Handler) GetEraEntryAPI(c echo.Context) error {
	t, err := h.loadEraTarget(c)
	if err != nil {
		return err
	}
	v, err := h.eraVersionOf(c, t)
	if err != nil {
		return err
	}
	stored, storedHTML := t.entity.Entry, t.entity.EntryHTML
	if v.Entry != nil {
		stored, storedHTML = v.Entry, v.EntryHTML
	}
	entry, entryHTML, hidden := h.servedText(c, t.cc, stored, storedHTML)
	resp := map[string]any{"entry": entry, "entry_html": entryHTML}
	if t.cc.MemberRole >= campaigns.RoleScribe {
		resp["rev"] = v.EntryRev
		if hidden {
			resp["locked"] = true
		}
	}
	return c.JSON(http.StatusOK, resp)
}

// UpdateEraEntryAPI saves the version's text from the editor. A save on top
// of newer text answers 409 like the usual entry route.
// PUT /campaigns/:id/entities/:eid/eras/:eraID/entry
func (h *Handler) UpdateEraEntryAPI(c echo.Context) error {
	t, err := h.loadEraTarget(c)
	if err != nil {
		return err
	}
	if err := t.requireWrite(); err != nil {
		return err
	}
	// The version's text, or the usual text it starts from, may have hidden
	// parts a Scribe's copy left out.
	v, err := h.eraVersionOf(c, t)
	if err != nil {
		return err
	}
	stored, storedHTML := t.entity.Entry, t.entity.EntryHTML
	if v.Entry != nil {
		stored, storedHTML = v.Entry, v.EntryHTML
	}
	if h.textLocked(c, t.cc, stored, storedHTML) {
		return apperror.NewForbidden(entryLockedMessage)
	}
	var body struct {
		Entry     string `json:"entry"`
		EntryHTML string `json:"entry_html"`
		BaseRev   *int   `json:"base_rev"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	rev, clash, err := h.eraPages.SaveVersionEntry(c.Request().Context(), t.cc.Campaign.ID, t.entity.ID, t.eraID, body.Entry, body.EntryHTML, body.BaseRev, t.userID)
	if err != nil {
		return err
	}
	if clash != nil {
		// The newer text goes back to this writer, so it is served like a read.
		theirs, theirsHTML, _ := h.servedText(c, t.cc, clash.Entry, clash.EntryHTML)
		return c.JSON(http.StatusConflict, map[string]any{
			"error":      "edit_conflict",
			"rev":        clash.Rev,
			"entry":      theirs,
			"entry_html": theirsHTML,
			"by_name":    h.memberName(c, t.cc.Campaign.ID, clash.ByUserID),
			"at":         clash.At,
		})
	}
	h.logAudit(c, t.cc.Campaign.ID, audit.ActionEntityUpdated, t.entity.ID, t.entity.Name)
	return c.JSON(http.StatusOK, map[string]any{"status": "ok", "rev": rev})
}

// GetEraFieldsAPI returns the page's fields with the version's values laid
// over, in the usual fields route's shape. For the DM it adds era_changed:
// each changed field's usual value, so the widget can say what it usually is.
// GET /campaigns/:id/entities/:eid/eras/:eraID/fields
func (h *Handler) GetEraFieldsAPI(c echo.Context) error {
	t, err := h.loadEraTarget(c)
	if err != nil {
		return err
	}
	v, err := h.eraVersionOf(c, t)
	if err != nil {
		return err
	}
	et, err := h.service.GetEntityTypeByID(c.Request().Context(), t.entity.EntityTypeID)
	if err != nil {
		return err
	}
	overrides := t.entity.FieldOverrides
	if overrides == nil {
		overrides = &FieldOverrides{}
	}
	canSeeGM := t.cc.CanAuthorDmOnly()
	isOwner := t.entity.IsOwnedBy(t.userID)
	merged := mergeEraFields(t.entity.FieldsData, v.FieldsData)
	effective := MergeFields(et.Fields, t.entity.FieldOverrides)
	visible := FilterRestrictedFields(merged, et.Fields, canSeeGM, isOwner)
	resp := map[string]any{
		"fields":          effective,
		"fields_data":     visible,
		"field_overrides": overrides,
		"type_fields":     et.Fields,
		"shown": h.shownFields(c.Request().Context(), t.cc.Campaign.ID, t.entity, effective, visible,
			restrictedKeys(canSeeGM, isOwner, et.Fields, effective), int(t.cc.VisibilityRole()), t.userID, canSeeGM),
	}
	if t.cc.CanAuthorDmOnly() {
		usual := map[string]any{}
		for k := range v.FieldsData {
			usual[k] = t.entity.FieldsData[k]
		}
		resp["era_changed"] = usual
	}
	return c.JSON(http.StatusOK, resp)
}

// UpdateEraFieldsAPI saves the attributes form against the version: only the
// values that differ from the usual page are kept, so a field set back to its
// usual value stops being a change. fields_data and fields_patch both change
// only the keys they carry (null clears).
// PUT /campaigns/:id/entities/:eid/eras/:eraID/fields
func (h *Handler) UpdateEraFieldsAPI(c echo.Context) error {
	t, err := h.loadEraTarget(c)
	if err != nil {
		return err
	}
	if err := t.requireWrite(); err != nil {
		return err
	}
	v, err := h.eraVersionOf(c, t)
	if err != nil {
		return err
	}
	var body struct {
		FieldsData  map[string]any  `json:"fields_data"`
		FieldsPatch json.RawMessage `json:"fields_patch"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	usual := t.entity.FieldsData
	if body.FieldsPatch != nil && body.FieldsData != nil {
		return apperror.NewBadRequest("send fields_data or fields_patch, not both")
	}
	// The form sends only the fields it shows, so keys it leaves out (a field
	// hidden on this page, an old value) keep what the era has now rather
	// than read as cleared.
	changes := body.FieldsData
	if body.FieldsPatch != nil {
		if err := json.Unmarshal(body.FieldsPatch, &changes); err != nil || changes == nil {
			return apperror.NewBadRequest("fields_patch must be an object")
		}
	}
	// A Scribe is not served the hidden fields, so their save neither changes
	// nor clears them.
	if !t.cc.CanAuthorDmOnly() {
		et, err := h.service.GetEntityTypeByID(c.Request().Context(), t.entity.EntityTypeID)
		if err != nil {
			return err
		}
		changes = dropRestrictedFields(changes, restrictedFieldKeys(t.entity, et, false, t.entity.IsOwnedBy(t.userID)))
	}
	full := mergeEraFields(usual, v.FieldsData)
	before := mergeEraFields(usual, v.FieldsData)
	for k, val := range changes {
		if val == nil {
			delete(full, k)
			continue
		}
		full[k] = val
	}
	if lc, ok := h.service.(linkChecker); ok {
		if err := lc.checkLinkValues(c.Request().Context(), t.entity, before, full); err != nil {
			return err
		}
	}
	if err := h.eraPages.SaveVersionFields(c.Request().Context(), t.cc.Campaign.ID, t.entity.ID, t.eraID, eraFieldChanges(usual, full), t.userID); err != nil {
		return err
	}
	h.logAudit(c, t.cc.Campaign.ID, audit.ActionEntityUpdated, t.entity.ID, t.entity.Name)
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// memberName is a member's display name for a clash message, or "" when it
// can't be found.
func (h *Handler) memberName(c echo.Context, campaignID, userID string) string {
	if userID == "" || h.memberLister == nil {
		return ""
	}
	members, err := h.memberLister.ListMembers(c.Request().Context(), campaignID)
	if err != nil {
		return ""
	}
	for _, m := range members {
		if m.UserID == userID {
			return m.DisplayName
		}
	}
	return ""
}
