package notes

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// The Journal reads the notes table through these views. Each starts from
// the notes the viewer can see (ListVisible), so nothing here can surface a
// title, a snippet or a count from a note the viewer could not open.

// IndexRow is one Journal note or folder as the list pane needs it: enough
// to draw, filter, group and peek at a row without loading its body.
type IndexRow struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	ParentID   *string    `json:"parentId,omitempty"`
	IsFolder   bool       `json:"isFolder"`
	UserID     string     `json:"userId"`
	Visibility Visibility `json:"visibility"`
	SharedWith []string   `json:"sharedWith,omitempty"`
	Pinned     bool       `json:"pinned"`
	Archived   bool       `json:"archived"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	Snippet    string     `json:"snippet,omitempty"`
	Links      []Link     `json:"links,omitempty"`
	InCount    int        `json:"inCount"`
	HasAudio   bool       `json:"hasAudio"`
	LockedBy   *string    `json:"lockedBy,omitempty"`
}

// JournalIndex is the Journal's list: every visible Journal note and folder.
type JournalIndex struct {
	Notes []IndexRow `json:"notes"`
}

// SearchHit is a note whose text matched, with the matching line.
type SearchHit struct {
	ID      string `json:"id"`
	Snippet string `json:"snippet"`
}

// NoteRef names a note the viewer can see, for backlink and reference lists.
type NoteRef struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Snippet  string  `json:"snippet,omitempty"`
	EntityID *string `json:"entityId,omitempty"` // set for a jot: the page it sits on
	Archived bool    `json:"archived"`
}

// NoteLabel is how a [[link]] to a note reads for this viewer.
type NoteLabel struct {
	Title    string  `json:"title"`
	Archived bool    `json:"archived"`
	EntityID *string `json:"entityId,omitempty"`
}

// BulkRequest applies one action to many notes. Only the caller's own notes
// change; the rest are reported back as skipped.
type BulkRequest struct {
	Action     string     `json:"action"` // archive, unarchive, move, visibility, delete
	IDs        []string   `json:"ids"`
	FolderID   string     `json:"folderId,omitempty"` // move: "" means the top level
	Visibility Visibility `json:"visibility,omitempty"`
	SharedWith []string   `json:"sharedWith,omitempty"`
}

// BulkResult lists which notes the action changed and which it left alone.
type BulkResult struct {
	Done    []string `json:"done"`
	Skipped []string `json:"skipped"`
}

// MaxBulkIDs caps one bulk request; the list pane selects in pages.
const MaxBulkIDs = 500

// hiddenNoteLabel is how a link to a note the viewer cannot see reads in
// plain text: never its title.
const hiddenNoteLabel = "a private note"

// snippetLen is the length of an index snippet: two lines of a peek card.
const snippetLen = 180

// JournalIndex returns the viewer's Journal: their visible Journal notes and
// folders, each with a snippet, its outgoing links and how many notes the
// viewer can see link to it.
func (s *noteService) JournalIndex(ctx context.Context, campaignID string, v permissions.Viewer) (*JournalIndex, error) {
	all, err := s.repo.ListVisible(ctx, campaignID, v, ListScope{})
	if err != nil {
		return nil, err
	}
	var audio map[string]bool
	if s.attRepo != nil {
		if audio, err = s.attRepo.NotesWithAttachments(ctx, campaignID); err != nil {
			return nil, err
		}
	}

	titles := make(map[string]string, len(all))
	for i := range all {
		titles[all[i].ID] = all[i].Title
	}
	label := func(id string) string {
		if t, ok := titles[id]; ok {
			return t
		}
		return hiddenNoteLabel
	}

	inCount := map[string]int{}
	links := make([][]Link, len(all))
	for i := range all {
		links[i] = ParseLinks(bodyOf(&all[i]))
		for _, l := range links[i] {
			if l.Kind == LinkNote && l.ID != all[i].ID {
				inCount[l.ID]++
			}
		}
	}

	idx := &JournalIndex{Notes: []IndexRow{}}
	for i := range all {
		n := &all[i]
		if n.EntityID != nil {
			continue // a jot: counted above, listed on its page
		}
		row := IndexRow{
			ID: n.ID, Title: n.Title, ParentID: n.ParentID, IsFolder: n.IsFolder,
			UserID: n.UserID, Visibility: n.Visibility, Pinned: n.Pinned, Archived: n.Archived,
			CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
			Links: links[i], InCount: inCount[n.ID], HasAudio: audio[n.ID],
		}
		// Only the owner sees who else a note is shared with.
		if n.UserID == v.UserID() {
			row.SharedWith = n.SharedWith
		}
		if !n.IsFolder {
			row.Snippet = snippet(PlainText(bodyOf(n), label), snippetLen)
		}
		if n.IsLocked() {
			row.LockedBy = n.LockedBy
		}
		idx.Notes = append(idx.Notes, row)
	}
	return idx, nil
}

// Search returns the viewer's notes whose title or text contains q, each
// with the matching line. Journal notes only unless withJots is set.
func (s *noteService) Search(ctx context.Context, campaignID string, v permissions.Viewer, q string, withJots bool) ([]SearchHit, error) {
	q = strings.TrimSpace(q)
	if len([]rune(q)) < 2 {
		return []SearchHit{}, nil
	}
	scope := ListScope{Kind: "campaign"}
	if withJots {
		scope = ListScope{}
	}
	all, err := s.repo.ListVisible(ctx, campaignID, v, scope)
	if err != nil {
		return nil, err
	}
	titles := titlesOf(all)
	hits := []SearchHit{}
	for i := range all {
		n := &all[i]
		if n.IsFolder {
			continue
		}
		text := PlainText(bodyOf(n), labelFrom(titles))
		switch {
		case containsFold(n.Title, q):
			hits = append(hits, SearchHit{ID: n.ID, Snippet: snippet(text, 90)})
		default:
			if m := matchSnippet(text, q); m != "" {
				hits = append(hits, SearchHit{ID: n.ID, Snippet: m})
			}
		}
		if len(hits) >= 200 {
			break
		}
	}
	return hits, nil
}

// Backlinks returns the notes the viewer can see that link to noteID. The
// caller has already checked the viewer can see noteID itself.
func (s *noteService) Backlinks(ctx context.Context, campaignID string, v permissions.Viewer, noteID string) ([]NoteRef, error) {
	linking, err := s.repo.ListVisibleLinking(ctx, campaignID, v, LinkNote, noteID)
	if err != nil {
		return nil, err
	}
	return s.refs(ctx, campaignID, v, linking, noteID)
}

// PageRefs returns the Journal notes the viewer can see that link to the
// page entityID.
func (s *noteService) PageRefs(ctx context.Context, campaignID string, v permissions.Viewer, entityID string) ([]NoteRef, error) {
	linking, err := s.repo.ListVisibleLinking(ctx, campaignID, v, LinkPage, entityID)
	if err != nil {
		return nil, err
	}
	journal := linking[:0]
	for _, n := range linking {
		if n.EntityID == nil {
			journal = append(journal, n)
		}
	}
	return s.refs(ctx, campaignID, v, journal, "")
}

// refs turns linking notes (already newest first) into NoteRefs, labelling
// any note links in their snippets through the viewer's own visibility.
func (s *noteService) refs(ctx context.Context, campaignID string, v permissions.Viewer, linking []Note, exclude string) ([]NoteRef, error) {
	var ids []string
	for _, n := range linking {
		ids = append(ids, linkTargets(&n)...)
	}
	labels, err := s.Labels(ctx, campaignID, v, ids)
	if err != nil {
		return nil, err
	}
	label := func(id string) string {
		if l := labels[id]; l != nil {
			return l.Title
		}
		return hiddenNoteLabel
	}
	out := []NoteRef{}
	for i := range linking {
		n := &linking[i]
		if n.ID == exclude || n.IsFolder {
			continue
		}
		out = append(out, NoteRef{
			ID: n.ID, Title: n.Title, EntityID: n.EntityID, Archived: n.Archived,
			Snippet: snippet(PlainText(bodyOf(n), label), 90),
		})
	}
	return out, nil
}

// Labels resolves [[note]] link targets for the viewer: the title of each
// note they can see, and nothing at all for one they cannot, so a missing
// note and a hidden one are indistinguishable.
func (s *noteService) Labels(ctx context.Context, campaignID string, v permissions.Viewer, ids []string) (map[string]*NoteLabel, error) {
	out := map[string]*NoteLabel{}
	var clean []string
	seen := map[string]bool{}
	for _, id := range ids {
		if idPattern.MatchString(id) && !seen[id] {
			seen[id] = true
			clean = append(clean, id)
		}
	}
	if len(clean) == 0 {
		return out, nil
	}
	if len(clean) > MaxBulkIDs {
		clean = clean[:MaxBulkIDs]
	}
	found, err := s.repo.FindByIDs(ctx, clean)
	if err != nil {
		return nil, err
	}
	for i := range found {
		n := &found[i]
		if n.CanView(v, campaignID) {
			out[n.ID] = &NoteLabel{Title: n.Title, Archived: n.Archived, EntityID: n.EntityID}
		}
	}
	return out, nil
}

// Bulk applies one owner-only action to each of the viewer's own notes in
// req.IDs; any other note is skipped, never an error, so one foreign row in
// a selection does not undo the rest.
func (s *noteService) Bulk(ctx context.Context, campaignID string, v permissions.Viewer, req BulkRequest) (*BulkResult, error) {
	if len(req.IDs) == 0 {
		return nil, apperror.NewBadRequest("no notes selected")
	}
	if len(req.IDs) > MaxBulkIDs {
		return nil, apperror.NewBadRequest("too many notes in one request")
	}
	var update *UpdateNoteRequest
	switch req.Action {
	case "archive", "unarchive":
		update = &UpdateNoteRequest{Archived: boolPtr(req.Action == "archive")}
	case "move":
		folder := req.FolderID
		update = &UpdateNoteRequest{ParentID: &folder}
	case "visibility":
		if !req.Visibility.Valid() {
			return nil, apperror.NewBadRequest("unknown visibility")
		}
		vis := req.Visibility
		update = &UpdateNoteRequest{Visibility: &vis, SharedWith: req.SharedWith}
	case "delete":
	default:
		return nil, apperror.NewBadRequest("unknown bulk action")
	}

	res := &BulkResult{Done: []string{}, Skipped: []string{}}
	for _, id := range req.IDs {
		n, err := s.repo.FindByID(ctx, id)
		if err != nil || !n.IsOwnedBy(v.UserID(), campaignID) {
			res.Skipped = append(res.Skipped, id)
			continue
		}
		if update == nil {
			err = s.Delete(ctx, id)
		} else {
			_, err = s.Update(ctx, id, v, *update)
		}
		if err != nil {
			// A refused move (bad folder) or visibility (nobody named) is
			// the same for every row: report it rather than skip them all.
			if isBadRequest(err) {
				return nil, err
			}
			res.Skipped = append(res.Skipped, id)
			continue
		}
		res.Done = append(res.Done, id)
	}
	return res, nil
}

// bodyOf is the HTML a note's links and text are read from.
func bodyOf(n *Note) string {
	if n.EntryHTML == nil {
		return ""
	}
	return *n.EntryHTML
}

// linkTargets lists the note ids a note links to.
func linkTargets(n *Note) []string {
	var ids []string
	for _, l := range ParseLinks(bodyOf(n)) {
		if l.Kind == LinkNote {
			ids = append(ids, l.ID)
		}
	}
	return ids
}

func titlesOf(ns []Note) map[string]string {
	m := make(map[string]string, len(ns))
	for i := range ns {
		m[ns[i].ID] = ns[i].Title
	}
	return m
}

// labelFrom labels note links from the viewer's visible titles.
func labelFrom(titles map[string]string) func(string) string {
	return func(id string) string {
		if t, ok := titles[id]; ok {
			return t
		}
		return hiddenNoteLabel
	}
}

func boolPtr(b bool) *bool { return &b }

func isBadRequest(err error) bool {
	var appErr *apperror.AppError
	return errors.As(err, &appErr) && appErr.Code == 400
}
