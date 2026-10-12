package campaigns

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// emptyDraft is what the API reports when there is no draft.
var emptyDraft = json.RawMessage(`{}`)

// SetCustomizeDrafts wires the draft store. Late-bound like the other
// optional collaborators; the draft routes answer 500 until it is set.
func (h *Handler) SetCustomizeDrafts(svc CustomizeDraftService) {
	h.customizeDrafts = svc
}

// CustomizeDraftFor returns the owner's stored draft for the Customize page to
// start from: its version and its sections. A failure is logged and reads as
// no draft, so the page still opens.
func (h *Handler) CustomizeDraftFor(ctx context.Context, campaignID, userID string) (version int, draft json.RawMessage) {
	if h.customizeDrafts == nil {
		return 0, emptyDraft
	}
	d, err := h.customizeDrafts.Get(ctx, campaignID, userID)
	if err != nil {
		slog.Error("reading customize draft failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return 0, emptyDraft
	}
	if d == nil || len(d.Draft) == 0 {
		return 0, emptyDraft
	}
	return d.Version, d.Draft
}

// draftRequest is the body of PUT /customize/draft.
type draftRequest struct {
	Version int             `json:"version"`
	Draft   json.RawMessage `json:"draft"`
}

// draftContext does the checks every draft route shares: a campaign, an owner
// and a wired store. The route group already requires the owner; this keeps
// the handler safe if it is ever mounted elsewhere.
func (h *Handler) draftContext(c echo.Context) (*CampaignContext, string, error) {
	cc := GetCampaignContext(c)
	if cc == nil {
		return nil, "", apperror.NewMissingContext()
	}
	if cc.MemberRole < RoleOwner {
		return nil, "", apperror.NewForbidden("only campaign owners can use Customize")
	}
	if h.customizeDrafts == nil {
		return nil, "", apperror.NewInternal(errors.New("customize drafts not wired"))
	}
	return cc, auth.GetUserID(c), nil
}

// GetCustomizeDraftAPI handles GET /campaigns/:id/customize/draft.
func (h *Handler) GetCustomizeDraftAPI(c echo.Context) error {
	cc, userID, err := h.draftContext(c)
	if err != nil {
		return err
	}
	d, err := h.customizeDrafts.Get(c.Request().Context(), cc.Campaign.ID, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, draftPayload(d))
}

// PutCustomizeDraftAPI handles PUT /campaigns/:id/customize/draft. The page
// calls it (debounced) as the owner edits.
func (h *Handler) PutCustomizeDraftAPI(c echo.Context) error {
	cc, userID, err := h.draftContext(c)
	if err != nil {
		return err
	}
	var req draftRequest
	if err := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, maxCustomizeDraftBytes+4096)).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	ctx := c.Request().Context()
	version, err := h.customizeDrafts.Save(ctx, cc.Campaign.ID, userID, req.Version, req.Draft, h.CustomizeSectionKeys())
	if err != nil {
		return h.draftConflictOr(c, cc, userID, err)
	}
	return c.JSON(http.StatusOK, map[string]int{"version": version})
}

// DeleteCustomizeDraftAPI handles DELETE /campaigns/:id/customize/draft?version=n.
func (h *Handler) DeleteCustomizeDraftAPI(c echo.Context) error {
	cc, userID, err := h.draftContext(c)
	if err != nil {
		return err
	}
	version := 0
	if v := c.QueryParam("version"); v != "" {
		version, err = strconv.Atoi(v)
		if err != nil || version < 0 {
			return apperror.NewBadRequest("invalid draft version")
		}
	}
	if err := h.customizeDrafts.Discard(c.Request().Context(), cc.Campaign.ID, userID, version); err != nil {
		return h.draftConflictOr(c, cc, userID, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// publishRequest is the body of POST /customize/publish. The draft itself is
// never sent: Save applies what the server stored.
type publishRequest struct {
	Version int `json:"version"`
}

// publishFailure names a section that did not apply and why, in words safe to
// show the owner.
type publishFailure struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

// publishLocks serialises publishes per owner and campaign so a double click
// cannot apply the same draft twice; the second request then finds a newer
// version and gets a Conflict.
var publishLocks sync.Map

func publishLock(campaignID, userID string) *sync.Mutex {
	l, _ := publishLocks.LoadOrStore(campaignID+"|"+userID, &sync.Mutex{})
	return l.(*sync.Mutex)
}

// PublishCustomizeAPI handles POST /campaigns/:id/customize/publish: it applies
// every section of the stored draft, each by the code that owns it. A section
// that fails stays in the draft, with whatever of it was not applied, so one
// bad section never costs the owner the rest of their work.
func (h *Handler) PublishCustomizeAPI(c echo.Context) error {
	cc, userID, err := h.draftContext(c)
	if err != nil {
		return err
	}
	var req publishRequest
	if err := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 4096)).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	ctx := c.Request().Context()

	lock := publishLock(cc.Campaign.ID, userID)
	lock.Lock()
	defer lock.Unlock()

	stored, err := h.customizeDrafts.Get(ctx, cc.Campaign.ID, userID)
	if err != nil {
		return err
	}
	if stored == nil {
		if req.Version != 0 {
			return h.draftConflictOr(c, cc, userID, errDraftChanged)
		}
		return c.JSON(http.StatusOK, map[string]any{
			"applied": []string{}, "failed": []publishFailure{}, "version": 0, "draft": emptyDraft,
		})
	}
	if stored.Version != req.Version {
		return h.draftConflictOr(c, cc, userID, errDraftChanged)
	}
	sections, err := parseCustomizeDraft(stored.Draft)
	if err != nil {
		return err
	}

	applied := []string{}
	failed := []publishFailure{}
	// outcome maps a section key to what is left of it; a nil value means it
	// applied completely. A section that failed and kept its raw value has no
	// entry, so write-back leaves it as stored.
	outcome := map[string]json.RawMessage{}
	for _, sec := range h.customizeSections {
		raw, ok := sections[sec.Key]
		if !ok {
			continue
		}
		remaining, applyErr := sec.Apply(ctx, cc, userID, raw)
		if applyErr != nil {
			var ae *apperror.AppError
			if !errors.As(applyErr, &ae) {
				slog.Error("customize section failed", slog.String("section", sec.Key), slog.Any("error", applyErr))
			}
			failed = append(failed, publishFailure{Key: sec.Key, Message: apperror.UserMessage(applyErr, "this part could not be saved")})
			if len(remaining) > 0 {
				outcome[sec.Key] = remaining
			}
			continue
		}
		applied = append(applied, sec.Key)
		if len(remaining) > 0 && !bytes.Equal(bytes.TrimSpace(remaining), []byte("null")) {
			outcome[sec.Key] = remaining
		} else {
			outcome[sec.Key] = nil
		}
	}

	after := h.writeBackPublish(ctx, cc.Campaign.ID, userID, stored, sections, outcome)

	failedKeys := make([]string, len(failed))
	for i, f := range failed {
		failedKeys[i] = f.Key
	}
	if len(applied)+len(failed) > 0 {
		h.logAuditCtx(ctx, cc.Campaign.ID, userID, "campaign.customize.published",
			map[string]any{"applied": applied, "failed": failedKeys})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"applied": applied,
		"failed":  failed,
		"version": after.Version,
		"draft":   after.Draft,
	})
}

// writeBackPublish stores what is left of the draft after a publish. The
// owner may have kept editing in another tab meanwhile, so a section is only
// replaced or removed while it is still exactly what was applied; a newer
// edit is kept. It returns the draft as it now stands.
func (h *Handler) writeBackPublish(ctx context.Context, campaignID, userID string, cur *CustomizeDraft, loaded map[string]json.RawMessage, outcome map[string]json.RawMessage) *CustomizeDraft {
	if len(outcome) == 0 {
		return cur
	}
	for attempt := 0; attempt < 2; attempt++ {
		sections, err := parseCustomizeDraft(cur.Draft)
		if err != nil {
			break
		}
		for key, rest := range outcome {
			if !bytes.Equal(sections[key], loaded[key]) {
				continue
			}
			if rest == nil {
				delete(sections, key)
			} else {
				sections[key] = rest
			}
		}
		raw, err := json.Marshal(sections)
		if err != nil {
			break
		}
		keys := make([]string, 0, len(sections))
		for k := range sections {
			keys = append(keys, k)
		}
		version, err := h.customizeDrafts.Save(ctx, campaignID, userID, cur.Version, raw, keys)
		if err == nil {
			if version == 0 {
				return &CustomizeDraft{Version: 0, Draft: emptyDraft}
			}
			return &CustomizeDraft{Version: version, Draft: raw}
		}
		if apperror.SafeCode(err) != http.StatusConflict {
			slog.Error("storing the remaining customize draft failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
			break
		}
		next, gerr := h.customizeDrafts.Get(ctx, campaignID, userID)
		if gerr != nil {
			break
		}
		if next == nil {
			return &CustomizeDraft{Version: 0, Draft: emptyDraft}
		}
		cur = next
	}
	// The sections are applied but the draft could not be trimmed; report the
	// draft as stored so the page shows the truth.
	if next, err := h.customizeDrafts.Get(ctx, campaignID, userID); err == nil && next != nil {
		return next
	}
	return &CustomizeDraft{Version: 0, Draft: emptyDraft}
}

// draftPayload is the {version, draft} shape the draft routes and a conflict
// answer share.
func draftPayload(d *CustomizeDraft) map[string]any {
	if d == nil || len(d.Draft) == 0 {
		return map[string]any{"version": 0, "draft": emptyDraft}
	}
	return map[string]any{"version": d.Version, "draft": d.Draft}
}

// draftConflictOr answers a version Conflict with the standard error body plus
// the current version and draft, so the page can show what changed without a
// second request. Any other error is returned for the error handler.
func (h *Handler) draftConflictOr(c echo.Context, cc *CampaignContext, userID string, err error) error {
	if apperror.SafeCode(err) != http.StatusConflict {
		return err
	}
	cur, gerr := h.customizeDrafts.Get(c.Request().Context(), cc.Campaign.ID, userID)
	if gerr != nil {
		return gerr
	}
	body := draftPayload(cur)
	body["error"] = "conflict"
	body["message"] = apperror.SafeMessage(err)
	return c.JSON(http.StatusConflict, body)
}
