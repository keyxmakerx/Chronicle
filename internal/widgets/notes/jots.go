package notes

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// A jot is a note on a page (EntityID set). "Send to Journal" copies one into
// a new Journal note and remembers it on the jot (LinkedNoteID), so the two
// stay linked both ways: the note links to the jot's page, and the jot opens
// the note.

// PageNamer names pages for a viewer: only pages of the campaign the viewer
// may see, and nothing at all for the rest. An adapter over the entities
// service satisfies it, so this widget never imports a plugin.
type PageNamer interface {
	PageNames(ctx context.Context, campaignID string, v permissions.Viewer, ids []string) (map[string]string, error)
}

// SendResult is a jot and the Journal note it was sent to.
type SendResult struct {
	Jot      *Note `json:"jot"`
	Note     *Note `json:"note"`
	Existing bool  `json:"existing"` // sent before; nothing new was made
}

// SendToJournal copies the viewer's own jot into a new, private Journal note
// that links back to the jot's page, and records the note on the jot. A jot
// sent before returns the note it became, while that note still exists and
// the viewer can see it. pageName is the page's name as the viewer sees it,
// or "" when it is unknown.
func (s *noteService) SendToJournal(ctx context.Context, campaignID string, v permissions.Viewer, jotID, pageName string) (*SendResult, error) {
	jot, err := s.repo.FindByID(ctx, jotID)
	if err != nil {
		return nil, err
	}
	if !jot.IsOwnedBy(v.UserID(), campaignID) || jot.EntityID == nil || jot.IsFolder {
		return nil, apperror.NewNotFound("jot not found")
	}
	if jot.LinkedNoteID != nil {
		if n, err := s.repo.FindByID(ctx, *jot.LinkedNoteID); err == nil && n.EntityID == nil && n.CanView(v, campaignID) {
			return &SendResult{Jot: jot, Note: n, Existing: true}, nil
		}
	}

	uid := v.UserID()
	body := sanitize.HTML(sentBody(jot, campaignID, pageName))
	note := &Note{
		ID:           generateID(),
		CampaignID:   campaignID,
		UserID:       uid,
		Title:        jot.Title,
		Content:      []Block{},
		EntryHTML:    &body,
		Color:        "#374151",
		LastEditedBy: &uid,
	}
	note.derive() // private: no sharing columns set
	if err := s.repo.Create(ctx, note); err != nil {
		return nil, err
	}
	if err := s.repo.SyncPictureBindings(ctx, note.ID, campaignID, uid, note.EntryHTML); err != nil {
		return nil, err
	}
	created, err := s.repo.FindByID(ctx, note.ID)
	if err != nil {
		return nil, err
	}
	s.publish("created", created, audienceOf(created))

	jot.LinkedNoteID = &created.ID
	if err := s.repo.Update(ctx, jot); err != nil {
		return nil, err
	}
	updated, err := s.repo.FindByID(ctx, jot.ID)
	if err != nil {
		return nil, err
	}
	s.publish("updated", updated, audienceOf(updated))
	return &SendResult{Jot: updated, Note: created}, nil
}

// sentBody is the new Journal note's HTML: a line linking the jot's page,
// then the jot's own text. A jot written in the old block editor has its
// text blocks become paragraphs and its checklists become a checklist.
func sentBody(jot *Note, campaignID, pageName string) string {
	var b strings.Builder
	eid := *jot.EntityID
	label := strings.TrimSpace(pageName)
	if label == "" {
		label = "its page"
	}
	href := fmt.Sprintf("/campaigns/%s/entities/%s", campaignID, eid)
	fmt.Fprintf(&b, `<p><em>From a jot on <a data-mention-id="%s" href="%s" data-entity-preview="%s/preview">%s</a>.</em></p>`,
		html.EscapeString(eid), html.EscapeString(href), html.EscapeString(href), html.EscapeString(label))
	if jot.EntryHTML != nil && strings.TrimSpace(*jot.EntryHTML) != "" {
		b.WriteString(*jot.EntryHTML)
	} else {
		for _, blk := range jot.Content {
			if blk.Type == "text" && strings.TrimSpace(blk.Value) != "" {
				for _, line := range strings.Split(blk.Value, "\n") {
					fmt.Fprintf(&b, "<p>%s</p>", html.EscapeString(line))
				}
			}
		}
	}
	for _, blk := range jot.Content {
		if blk.Type != "checklist" || len(blk.Items) == 0 {
			continue
		}
		b.WriteString(`<ul data-type="taskList">`)
		for _, it := range blk.Items {
			fmt.Fprintf(&b, `<li data-checked="%t" data-type="taskItem"><div><p>%s</p></div></li>`, it.Checked, html.EscapeString(it.Text))
		}
		b.WriteString(`</ul>`)
	}
	return b.String()
}
