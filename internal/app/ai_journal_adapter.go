package app

import (
	"context"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// aiJournalAdapter answers the AI's note lookups from the notes widget. Every
// row comes from ListVisible, the notes' own visibility rule, so a note its
// writer keeps private never reaches another member's AI.
type aiJournalAdapter struct {
	notes   notes.NoteService
	members campaigns.CampaignService
}

func (j aiJournalAdapter) JournalNotes(ctx context.Context, campaignID string, v permissions.Viewer) ([]records.JournalNote, error) {
	// The index tags game-night entries with their night. Like opening the
	// Journal, it first makes the viewer's own entries that are due.
	idx, err := j.notes.JournalIndex(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	nights := map[string]*notes.NightTag{}
	for _, row := range idx.Notes {
		if row.Night != nil {
			nights[row.ID] = row.Night
		}
	}
	all, err := j.notes.ListVisible(ctx, campaignID, v, notes.ListScope{})
	if err != nil {
		return nil, err
	}
	folders := map[string]*notes.Note{}
	for i := range all {
		if all[i].IsFolder {
			folders[all[i].ID] = &all[i]
		}
	}
	names := map[string]string{}
	if j.members != nil {
		if ms, err := j.members.ListMembers(ctx, campaignID); err == nil {
			for _, m := range ms {
				names[m.UserID] = m.DisplayName
			}
		}
	}
	var out []records.JournalNote
	for i := range all {
		n := &all[i]
		if n.IsFolder {
			continue
		}
		jn := records.JournalNote{
			ID: n.ID, Title: n.Title, AuthorID: n.UserID, Author: names[n.UserID],
			Folder: folderPath(n, folders), Audience: string(n.Visibility),
			Archived: n.Archived, Updated: n.UpdatedAt, Body: n.EntryHTML,
		}
		if n.EntityID != nil {
			jn.PageID = *n.EntityID
		}
		if t := nights[n.ID]; t != nil {
			jn.Night = strings.TrimSpace(t.Name + ", " + t.Date)
		}
		out = append(out, jn)
	}
	return out, nil
}

// folderPath is a note's folders from the top, as far as the viewer can see
// them; a folder the viewer can't open ends the path.
func folderPath(n *notes.Note, folders map[string]*notes.Note) string {
	var path []string
	seen := map[string]bool{}
	for p := n.ParentID; p != nil && !seen[*p]; {
		seen[*p] = true
		f, ok := folders[*p]
		if !ok {
			break
		}
		path = append([]string{f.Title}, path...)
		p = f.ParentID
	}
	return strings.Join(path, " / ")
}
