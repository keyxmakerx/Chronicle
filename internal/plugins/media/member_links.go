package media

import (
	"context"
	"log/slog"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// memberLinkPath matches a media path a page embeds: a file, or one of its
// thumbnails. Any query (an old ?expires=&sig=) is dropped before matching.
var memberLinkPath = regexp.MustCompile(`^/media/([0-9a-fA-F-]{36})(?:/thumb/(300|800))?$`)

// maxMemberLinks bounds one SignedLinksForMember call.
const maxMemberLinks = 100

// SignedLinksForMember signs media paths for a campaign member whose
// requests carry no session cookie: the notes pages shown inside an outside
// app (the Foundry notebook) authenticate by a Bearer grant, and an <img> or
// <audio> there can't send one.
//
// Each path naming a file of campaignID that userID may open, by the same
// ADR-058 check a signed-in request from them gets, maps to a path signed
// for ViewerAPIKey (the cookieless viewer, as the sync API signs for
// Foundry). Every other path is left out, so a hidden file and a missing
// one look the same.
func (h *Handler) SignedLinksForMember(ctx context.Context, campaignID, userID string, paths []string) map[string]string {
	out := map[string]string{}
	if h.signer == nil || h.memberChecker == nil || campaignID == "" || userID == "" {
		return out
	}
	if !h.memberChecker.IsCampaignMember(campaignID, userID) {
		return out
	}
	// Each distinct file (and thumbnail size) is checked once, however many
	// spellings of its path arrive, and the cap counts files, not strings:
	// the access check can be a campaign-wide scan.
	done := map[string]string{}
	for _, raw := range paths {
		path, _, _ := strings.Cut(raw, "?")
		m := memberLinkPath.FindStringSubmatch(path)
		if m == nil {
			continue
		}
		id, err := uuid.Parse(m[1])
		if err != nil {
			continue
		}
		key := id.String() + "/" + m[2]
		if link, ok := done[key]; ok {
			if link != "" {
				out[raw] = link
			}
			continue
		}
		if len(done) >= maxMemberLinks {
			break
		}
		done[key] = ""

		file, err := h.service.GetByID(ctx, id.String())
		if err != nil || file.CampaignID == nil || *file.CampaignID != campaignID {
			continue
		}
		// Only campaign content; avatars and backdrops have their own rules.
		if file.UsageType != UsageAttachment && file.UsageType != UsageEntityImage && file.UsageType != UsageNoteImage {
			continue
		}
		var allowed bool
		if file.IsNotePicture() {
			allowed, err = h.checkNoteImageAccess(ctx, file, userID)
		} else {
			allowed, err = h.checkEntityScopedAccess(ctx, file, userID)
		}
		if err != nil {
			slog.Error("media: member link access check failed; leaving it out",
				slog.String("file_id", file.ID), slog.Any("error", err))
			continue
		}
		if !allowed {
			continue
		}
		link := h.signer.Sign(file.ID, ViewerAPIKey, SignedURLTTL)
		if m[2] != "" {
			link = h.signer.SignThumb(file.ID, m[2], ViewerAPIKey, SignedURLTTL)
		}
		done[key] = link
		out[raw] = link
	}
	return out
}
