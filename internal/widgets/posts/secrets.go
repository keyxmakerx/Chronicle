package posts

import (
	"encoding/json"
	"log/slog"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// servePosts gives each post's body as this viewer may read it: GM-only
// content (secret text, GM pictures, rollers) removed, and mentions of pages
// they can't open read "a hidden page". A post a player may see can still
// carry GM-only parts inside it, so hiding private posts alone is not enough.
func (h *Handler) servePosts(c echo.Context, cc *campaigns.CampaignContext, posts []Post) {
	texts := make([]*string, 0, 2*len(posts))
	entries := make([]string, len(posts))
	for i := range posts {
		if len(posts[i].Entry) > 0 {
			entries[i] = sanitize.StripSecretsJSON(string(posts[i].Entry))
			texts = append(texts, &entries[i])
		}
		if posts[i].EntryHTML != nil {
			stripped := sanitize.StripSecretsHTML(*posts[i].EntryHTML)
			posts[i].EntryHTML = &stripped
			texts = append(texts, posts[i].EntryHTML)
		}
	}
	h.hideMentions(c, cc, texts...)
	for i := range posts {
		if len(posts[i].Entry) > 0 {
			posts[i].Entry = json.RawMessage(entries[i])
		}
	}
}

// hideMentions hides, in place, the mentions in texts of pages this viewer
// can't open, and reports how many it hid.
func (h *Handler) hideMentions(c echo.Context, cc *campaigns.CampaignContext, texts ...*string) int {
	n, err := sanitize.HideMentionsFor(c.Request().Context(), h.mentions, cc.Campaign.ID, int(cc.VisibilityRole()), auth.GetUserID(c), texts...)
	if err != nil {
		slog.Warn("posts: checking which mentioned pages a reader can open failed; all are hidden",
			slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
	}
	return n
}

// hideRefusal answers a Scribe who tries to make a post GM-only.
const hideRefusal = "only the owner or a Co-DM can make a post GM-only"

// requirePostWrite keeps a Scribe's writes to what they are served. A
// GM-only post is not theirs to see, so it reads as missing; and a post whose
// text has GM-only parts or names a page they can't open is served to them
// without those, so changing or deleting its text would lose them.
func (h *Handler) requirePostWrite(c echo.Context, cc *campaigns.CampaignContext, p *Post, changesText bool) error {
	if cc.CanAuthorDmOnly() {
		return nil
	}
	if p.IsPrivate {
		return apperror.NewNotFound("post not found")
	}
	if !changesText {
		return nil
	}
	entry, html := string(p.Entry), ""
	if p.EntryHTML != nil {
		html = *p.EntryHTML
	}
	if sanitize.HasSecrets(entry, html) || h.hideMentions(c, cc, &entry, &html) > 0 {
		return apperror.NewForbidden("parts of this post are hidden from players, so only the owner or a Co-DM can change it")
	}
	return nil
}
