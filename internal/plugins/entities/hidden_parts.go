package entities

// hidden_parts.go — what a writer who is served the player copy of a page
// may change. Hidden pages and the hidden parts of a page are the owner's and
// Co-DMs'; a Scribe sees what players see, so they write only to pages they
// can see, and a save of theirs must never erase what their copy left out.

import (
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// entryLockedMessage is the refusal a writer gets for text with hidden parts.
const entryLockedMessage = "Parts of this text are hidden from players, so only the owner or a Co-DM can edit it."

// requirePageWrite guards a Scribe route on one page. The owner and Co-DMs
// pass; anyone else must be allowed to view and edit the page, and a page
// they can't view answers 404, as if it were missing.
func (h *Handler) requirePageWrite(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := h.pageWriteErr(c, c.Param("eid")); err != nil {
			return err
		}
		return next(c)
	}
}

// pageWriteErr is requirePageWrite's check, for a route that names the page
// in its body rather than its path.
func (h *Handler) pageWriteErr(c echo.Context, entityID string) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if cc.CanAuthorDmOnly() {
		return nil
	}
	access, err := h.service.CheckEntityAccess(c.Request().Context(), entityID, cc.VisibilityRole(), auth.GetUserID(c))
	if err != nil || !access.CanView {
		return apperror.NewNotFound("entity not found")
	}
	if !access.CanEdit {
		return apperror.NewForbidden("you can't edit this page")
	}
	return nil
}

// pageViewErr refuses a page the viewer cannot see as not found.
func (h *Handler) pageViewErr(c echo.Context, entityID string) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if cc.CanAuthorDmOnly() {
		return nil
	}
	access, err := h.service.CheckEntityAccess(c.Request().Context(), entityID, cc.VisibilityRole(), auth.GetUserID(c))
	if err != nil || !access.CanView {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// textLocked reports whether this writer may not save over the stored text:
// they are served a copy with the hidden parts taken out, so their save
// would delete those parts.
func textLocked(cc *campaigns.CampaignContext, stored, storedHTML *string) bool {
	if cc.CanAuthorDmOnly() {
		return false
	}
	return sanitize.HasSecrets(deref(stored), deref(storedHTML))
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// restrictedFieldKeys lists the fields this writer is not served: GM-only
// fields unless they see GM content, and owner-only fields unless they also
// own the page. A field added to just this page counts like a category field.
func restrictedFieldKeys(entity *Entity, et *EntityType, canSeeGM, isOwner bool) map[string]struct{} {
	if canSeeGM || et == nil {
		return nil
	}
	defs := et.Fields
	if entity.FieldOverrides != nil && len(entity.FieldOverrides.Added) > 0 {
		defs = append(append([]FieldDefinition{}, et.Fields...), entity.FieldOverrides.Added...)
	}
	var keys map[string]struct{}
	for i := range defs {
		if defs[i].GMOnly || (defs[i].OwnerOnly && !isOwner) {
			if keys == nil {
				keys = make(map[string]struct{}, 2)
			}
			keys[defs[i].Key] = struct{}{}
		}
	}
	return keys
}

// keepRestrictedFields is a whole-map save from a writer who was not served
// the restricted fields: those keep their stored values, whatever the save
// carries for them, and every other field is the writer's.
func keepRestrictedFields(stored, incoming map[string]any, keys map[string]struct{}) map[string]any {
	if len(keys) == 0 {
		return incoming
	}
	out := make(map[string]any, len(incoming)+len(keys))
	for k, v := range incoming {
		if _, hidden := keys[k]; !hidden {
			out[k] = v
		}
	}
	for k := range keys {
		if v, ok := stored[k]; ok {
			out[k] = v
		}
	}
	return out
}

// dropRestrictedFields takes out of a partial save the fields its writer was
// not served, so they can neither change nor clear a value they can't see.
func dropRestrictedFields(patch map[string]any, keys map[string]struct{}) map[string]any {
	if len(keys) == 0 {
		return patch
	}
	out := make(map[string]any, len(patch))
	for k, v := range patch {
		if _, hidden := keys[k]; !hidden {
			out[k] = v
		}
	}
	return out
}

// servedCopy is the page as this viewer is served it: for anyone but the
// owner and Co-DMs, the text without its hidden parts and the fields without
// the restricted values. The stored entity is not changed.
func servedCopy(cc *campaigns.CampaignContext, entity *Entity, et *EntityType, userID string) *Entity {
	if entity == nil || cc.CanAuthorDmOnly() {
		return entity
	}
	cp := *entity
	if cp.Entry != nil {
		s := sanitize.StripSecretsJSON(*cp.Entry)
		cp.Entry = &s
	}
	if cp.EntryHTML != nil {
		s := sanitize.StripSecretsHTML(*cp.EntryHTML)
		cp.EntryHTML = &s
	}
	if cp.PlayerNotes != nil {
		s := sanitize.StripSecretsJSON(*cp.PlayerNotes)
		cp.PlayerNotes = &s
	}
	if cp.PlayerNotesHTML != nil {
		s := sanitize.StripSecretsHTML(*cp.PlayerNotesHTML)
		cp.PlayerNotesHTML = &s
	}
	if et != nil {
		cp.FieldsData = FilterRestrictedFields(cp.FieldsData, et.Fields, false, cp.IsOwnedBy(userID))
	}
	return &cp
}

// editablePageIDs keeps the ids of pages this caller may edit, in order. The
// owner and Co-DMs keep every id; the ids come from a request body, so for
// anyone else each is checked like requirePageWrite checks one page.
func (h *Handler) editablePageIDs(c echo.Context, cc *campaigns.CampaignContext, ids []string) []string {
	if cc.CanAuthorDmOnly() {
		return ids
	}
	ctx := c.Request().Context()
	role, userID := cc.VisibilityRole(), auth.GetUserID(c)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if access, err := h.service.CheckEntityAccess(ctx, id, role, userID); err == nil && access.CanEdit {
			out = append(out, id)
		}
	}
	return out
}

// requireCreatableType refuses a Scribe a new page in the DM-only category:
// the page would vanish from their own view as soon as it was made.
func (h *Handler) requireCreatableType(c echo.Context, cc *campaigns.CampaignContext, typeID int) error {
	if cc.CanAuthorDmOnly() {
		return nil
	}
	if et, err := h.service.GetEntityTypeByID(c.Request().Context(), typeID); err == nil && et.DMOnly() {
		return apperror.NewForbidden("only the owner or a Co-DM can add pages to this category")
	}
	return nil
}
