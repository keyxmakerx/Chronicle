package sessions

import (
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// recapLockedMessage is the refusal a writer gets for a recap whose copy of
// theirs left something out.
const recapLockedMessage = "Parts of this recap are hidden from players, so only the owner or a Co-DM can edit it."

// SetMentionHider injects the check that hides mentions of pages a reader
// can't open. Unset, every mention in a served recap is hidden.
func (h *Handler) SetMentionHider(m sanitize.MentionHider) {
	h.mentions = m
}

// serveRecap turns s's recap, in place, into what this viewer may read. Every
// member reads the recap, so for anyone but the owner and Co-DMs GM-only parts
// are removed (a recap stored before saves were stripped may still hold some)
// and a mention of a page they can't open reads "a hidden page". hidden
// reports whether their copy left anything out, so a save from it would erase
// that.
func (h *Handler) serveRecap(c echo.Context, cc *campaigns.CampaignContext, s *Session) (hidden bool) {
	if cc.CanAuthorDmOnly() {
		return false
	}
	doc := s.Recap != nil && isEditorDoc(*s.Recap)
	var texts []*string
	if doc {
		if sanitize.HasSecrets(*s.Recap, "") {
			hidden = true
		}
		clean := sanitize.StripSecretsJSON(*s.Recap)
		s.Recap = &clean
		texts = append(texts, s.Recap)
	}
	if s.RecapHTML != nil {
		clean := sanitize.StripSecretsHTML(*s.RecapHTML)
		if clean != *s.RecapHTML {
			hidden = true
		}
		s.RecapHTML = &clean
		texts = append(texts, s.RecapHTML)
	}
	n, err := sanitize.HideMentionsFor(c.Request().Context(), h.mentions, cc.Campaign.ID, int(cc.VisibilityRole()), auth.GetUserID(c), texts...)
	if err != nil {
		slog.Warn("sessions: checking which mentioned pages a reader can open failed; all are hidden",
			slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
	}
	return hidden || n > 0
}

// isEditorDoc reports whether a stored recap is the page editor's
// ProseMirror JSON. Recaps saved from the calendar's plain box hold the
// typed text instead, and must never be handed to the editor as a document.
func isEditorDoc(recap string) bool {
	var d struct {
		Type string `json:"type"`
	}
	return json.Unmarshal([]byte(recap), &d) == nil && d.Type == "doc"
}

// hasRecap reports whether anyone has written the night up.
func hasRecap(s *Session) bool {
	return s.RecapHTML != nil && strings.TrimSpace(*s.RecapHTML) != ""
}

// recapEditorBody is the recap in the page editor's load contract: the
// document when there is one, else the HTML, which the editor opens as is.
func recapEditorBody(s *Session) map[string]any {
	out := map[string]any{"entry_html": strPtrVal(s.RecapHTML)}
	if s.Recap != nil && isEditorDoc(*s.Recap) {
		out["entry"] = *s.Recap
	}
	return out
}

// recapOpen reports whether a night's page shows its recap. A recap needs
// no Mark Complete: it opens on the night's own day, so the table can write
// it up that evening, and stays once written. A cancelled night has none.
func recapOpen(s *Session, today string) bool {
	if s.Status == StatusCancelled {
		return false
	}
	if s.Status == StatusCompleted || hasRecap(s) {
		return true
	}
	return s.ScheduledDate != nil && *s.ScheduledDate != "" && *s.ScheduledDate <= today
}
