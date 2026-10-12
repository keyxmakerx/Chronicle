package campaigns

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// customizeSectionLook is the draft key for the campaign's look.
const customizeSectionLook = "look"

// lookSection is the look's value in the Customize draft. Draft is the look
// editor's own UI state, kept opaque so the server never has to follow the
// editor's changes; Input is what Save applies.
type lookSection struct {
	Draft json.RawMessage  `json:"draft"`
	Input *AppearanceInput `json:"input"`
}

// applyLookSection stores the look through the same path as PUT /appearance,
// so a Save from the draft and a direct API call can never disagree.
func (h *Handler) applyLookSection(ctx context.Context, cc *CampaignContext, userID string, raw json.RawMessage) (json.RawMessage, error) {
	var sec lookSection
	if err := json.Unmarshal(raw, &sec); err != nil {
		return nil, apperror.NewBadRequest("the look in the draft is not valid")
	}
	if sec.Input == nil {
		return nil, nil
	}
	if err := h.saveAppearance(ctx, cc, userID, *sec.Input); err != nil {
		return nil, err
	}
	return nil, nil
}

// LookDraftPictures maps each picture the draft's look uses to a URL the
// page can show. A name the campaign neither stores nor owns is left out: a
// hand-edited draft must never make the page mint a URL for another
// campaign's file.
func (h *Handler) LookDraftPictures(ctx context.Context, cc *CampaignContext, draft json.RawMessage) map[string]string {
	out := map[string]string{}
	sections, err := parseCustomizeDraft(draft)
	if err != nil {
		return out
	}
	raw, ok := sections[customizeSectionLook]
	if !ok {
		return out
	}
	var sec lookSection
	if err := json.Unmarshal(raw, &sec); err != nil || sec.Input == nil {
		return out
	}
	stored := appearanceStoredPictures(cc)
	for _, name := range sec.Input.AppearancePictures() {
		if !stored[name] {
			if h.mediaUploader == nil {
				continue
			}
			owns, err := h.mediaUploader.OwnsFile(ctx, cc.Campaign.ID, name)
			if err != nil {
				slog.Warn("checking a draft picture failed", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
				continue
			}
			if !owns {
				continue
			}
		}
		out[name] = layouts.MediaURL(ctx, name)
	}
	return out
}
