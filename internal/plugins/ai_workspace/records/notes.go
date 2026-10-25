package records

import (
	"context"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer/htmlconv"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// NotesAPI is the slice of notes.NoteService the note kind uses.
type NotesAPI interface {
	ListByUserAndCampaign(ctx context.Context, userID, campaignID string) ([]notes.Note, error)
	ListVisible(ctx context.Context, campaignID string, v permissions.Viewer, scope notes.ListScope) ([]notes.Note, error)
	GetByID(ctx context.Context, id string) (*notes.Note, error)
	Create(ctx context.Context, campaignID string, creator permissions.Viewer, req notes.CreateNoteRequest) (*notes.Note, error)
	Update(ctx context.Context, id string, editor permissions.Viewer, req notes.UpdateNoteRequest) (*notes.Note, error)
	Delete(ctx context.Context, id string) error
}

// NoteKind reaches the operator's OWN notes: Journal notes and jots on
// pages. A note someone else wrote is changed only when it is shared with
// the operator and the owner's switch for its writer's kind (co-DM or
// Scribe) is on, and then only its title and text: never removed, moved or
// reshared, and never a player's. Apply re-checks all of it on the stored
// note before every write.
type NoteKind struct {
	Svc      NotesAPI
	Entities EntityLookup
	// Access is the owner's switches for shared notes; nil keeps the AI to
	// the operator's own.
	Access NoteAccessAPI
}

func (NoteKind) Name() string  { return "note" }
func (NoteKind) Label() string { return "Note" }
func (NoteKind) Doc() string {
	return "One of my own notes, matched by `name` (its title). Without `page` it is a Journal note, my game night entries among them; with `page: <page name>` it is my jot on that page. Notes other members shared with me can be read with a lookup; one a co-DM or Scribe wrote can be updated (title and text only) when the `notes` lookup says so, naming its writer with `by`. Optional `share`: `private` (default), `gm` or `party`; `rename_to`. The body is the note; write `@[Page Name]` or `@[Page Name|words]` in it to link a page.\n\n```\n---\nkind: note\nname: Who poisoned the duke?\npage: Duke Varrin\n---\nThe cook had the means. Check the cellar key.\n```"
}

// ownNotes is the operator's own, non-folder notes. Shared-with-me notes
// are dropped here, so nothing below can address them.
func (k NoteKind) ownNotes(ctx context.Context, campaignID string, a Actor) ([]notes.Note, error) {
	all, err := k.Svc.ListByUserAndCampaign(ctx, a.UserID, campaignID)
	if err != nil {
		return nil, apperror.NewBadRequest("could not read your notes")
	}
	var out []notes.Note
	for _, n := range all {
		if n.IsOwnedBy(a.UserID, campaignID) && !n.IsFolder {
			out = append(out, n)
		}
	}
	return out, nil
}

func (k NoteKind) pageID(ctx context.Context, campaignID string, r Record) (*string, error) {
	name := r.Str("page")
	if name == "" {
		return nil, nil
	}
	e, err := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(name))
	if err != nil || e == nil || e.CampaignID != campaignID {
		return nil, badRequestf("no page called %s", quote(name))
	}
	return &e.ID, nil
}

// find matches by title and place (a jot's page, or the Journal).
func (k NoteKind) find(ctx context.Context, campaignID string, a Actor, r Record) (*notes.Note, *string, error) {
	page, err := k.pageID(ctx, campaignID, r)
	if err != nil {
		return nil, nil, err
	}
	own, err := k.ownNotes(ctx, campaignID, a)
	if err != nil {
		return nil, page, err
	}
	var hit *notes.Note
	for i := range own {
		n := &own[i]
		samePlace := (page == nil && n.EntityID == nil) || (page != nil && n.EntityID != nil && *n.EntityID == *page)
		if samePlace && sameName(n.Title, r.Name) {
			if hit != nil {
				return nil, page, badRequestf("you have more than one note called %s there", quote(r.Name))
			}
			hit = n
		}
	}
	return hit, page, nil
}

// sharedHit is a note another member wrote and shared with the operator,
// matched for an update, with its writer.
type sharedHit struct {
	note   *notes.Note
	writer NoteWriter
}

// findShared matches a note someone else wrote by title, place and the
// optional `by` (the writer's name). Only notes the operator can view are
// candidates, through the notes' own visibility rule.
func (k NoteKind) findShared(ctx context.Context, campaignID string, a Actor, r Record, page *string) (*sharedHit, error) {
	if k.Access == nil {
		return nil, nil
	}
	all, err := k.Svc.ListVisible(ctx, campaignID, a.Viewer(), notes.ListScope{})
	if err != nil {
		return nil, apperror.NewBadRequest("could not read the notes shared with you")
	}
	by := r.Str("by")
	var hits []sharedHit
	for i := range all {
		n := &all[i]
		if n.IsOwnedBy(a.UserID, campaignID) || n.IsFolder || !n.CanView(a.Viewer(), campaignID) {
			continue
		}
		samePlace := (page == nil && n.EntityID == nil) || (page != nil && n.EntityID != nil && *n.EntityID == *page)
		if !samePlace || !sameName(n.Title, r.Name) {
			continue
		}
		w, err := k.Access.NoteWriter(ctx, campaignID, n.UserID)
		if err != nil {
			return nil, apperror.NewBadRequest("could not tell who wrote " + quote(r.Name))
		}
		if by != "" && !sameName(w.Name, by) {
			continue
		}
		hits = append(hits, sharedHit{note: n, writer: w})
	}
	switch len(hits) {
	case 0:
		return nil, nil
	case 1:
		return &hits[0], nil
	}
	return nil, badRequestf("more than one note called %s is shared with you there; add `by` with the writer's name", quote(r.Name))
}

// sharedPlan is the review row for changing a note someone else wrote.
func (k NoteKind) sharedPlan(ctx context.Context, campaignID string, r Record, h *sharedHit) Plan {
	who := firstNonEmpty(h.writer.Name, "another member")
	if r.Action == ActionDelete {
		return Plan{Error: "only " + who + " can remove " + quote(r.Name) + "; your AI can change its title and text at most"}
	}
	access, err := k.Access.NoteAccess(ctx, campaignID)
	if err != nil {
		return Plan{Error: "could not read which shared notes your AI may change"}
	}
	if !access.Allows(h.writer.Kind) {
		return Plan{Error: access.Refusal(h.writer.Kind)}
	}
	p := Plan{Summary: "changes a note " + who + " (" + h.writer.Kind + ") shared with you: its title and text only"}
	if r.Str("share") != "" {
		p.Warnings = append(p.Warnings, "`share` is left out: only "+who+" can change who sees it")
	}
	return p
}

func where(r Record) string {
	if p := r.Str("page"); p != "" {
		return "your jot on " + p
	}
	return "your Journal"
}

func (k NoteKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if r.Name == "" {
		return Plan{Error: "a note needs a name (its title)"}
	}
	if s := r.Str("share"); s != "" && s != "private" && s != "gm" && s != "party" {
		return Plan{Error: "share must be private, gm or party"}
	}
	hit, page, err := k.find(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	switch r.Action {
	case ActionCreate:
		p := Plan{Summary: "in " + where(r)}
		if hit != nil {
			p.Warnings = append(p.Warnings, "You already have a note with this title there; this adds another")
		}
		return p
	case ActionUpdate:
		if hit == nil || r.Str("by") != "" {
			if sh, err := k.findShared(ctx, campaignID, a, r, page); err != nil {
				return Plan{Error: planError(err)}
			} else if sh != nil {
				return k.sharedPlan(ctx, campaignID, r, sh)
			}
		}
		if hit == nil || r.Str("by") != "" {
			return Plan{Error: "you have no note called " + quote(r.Name) + " in " + strings.TrimPrefix(where(r), "your ")}
		}
		return Plan{Summary: "changes it in " + where(r)}
	default:
		// `by` names someone else's note, so it never removes the operator's
		// own note with the same title.
		if hit == nil || r.Str("by") != "" {
			if sh, err := k.findShared(ctx, campaignID, a, r, page); err == nil && sh != nil {
				return k.sharedPlan(ctx, campaignID, r, sh)
			}
			return Plan{Error: "you have no note called " + quote(r.Name) + " in " + strings.TrimPrefix(where(r), "your ")}
		}
		return Plan{Summary: "removes it from " + where(r)}
	}
}

// entry converts the body to the editor's stored pair (ProseMirror JSON +
// sanitized HTML); the notes service sanitizes the HTML again on save.
func entry(body string, links *importer.PageLinks) (*string, *string, error) {
	h, err := bodyHTML(body, links)
	if err != nil || h == nil {
		return nil, nil, err
	}
	j, err := htmlconv.Convert(*h)
	if err != nil {
		return nil, nil, err
	}
	return &j, h, nil
}

func (k NoteKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	hit, page, err := k.find(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	j, h, err := entry(r.Body, r.Links)
	if err != nil {
		return err
	}
	vis := notes.Visibility(r.Str("share"))
	if r.Action == ActionCreate {
		n, err := k.Svc.Create(ctx, campaignID, a.Viewer(), notes.CreateNoteRequest{
			EntityID: page, Title: r.Name, Content: []notes.Block{}, Visibility: vis,
		})
		if err != nil {
			return err
		}
		if j == nil {
			return nil
		}
		_, err = k.Svc.Update(ctx, n.ID, a.Viewer(), notes.UpdateNoteRequest{Entry: j, EntryHTML: h})
		return err
	}
	if r.Action == ActionUpdate && (hit == nil || r.Str("by") != "") {
		return k.applyShared(ctx, campaignID, a, r, page, j, h)
	}
	// Re-read the stored note: ownership is checked on what is saved, not
	// on the list it came from.
	if hit == nil {
		return apperror.NewBadRequest("it changed while you were reviewing; check it again")
	}
	stored, err := k.Svc.GetByID(ctx, hit.ID)
	if err != nil || !stored.IsOwnedBy(a.UserID, campaignID) {
		return apperror.NewBadRequest("that note is not yours")
	}
	if r.Action == ActionDelete {
		return k.Svc.Delete(ctx, stored.ID)
	}
	req := notes.UpdateNoteRequest{Entry: j, EntryHTML: h}
	if s := r.Str("rename_to"); s != "" {
		req.Title = &s
	}
	if vis != "" {
		req.Visibility = &vis
	}
	_, err = k.Svc.Update(ctx, stored.ID, a.Viewer(), req)
	return err
}

// applyShared changes the title and text of a note someone else wrote,
// after checking again on the stored note that the operator can view it,
// that its writer's switch is still on, and that nobody is editing it.
func (k NoteKind) applyShared(ctx context.Context, campaignID string, a Actor, r Record, page *string, j, h *string) error {
	sh, err := k.findShared(ctx, campaignID, a, r, page)
	if err != nil {
		return err
	}
	if sh == nil {
		return apperror.NewBadRequest("it changed while you were reviewing; check it again")
	}
	stored, err := k.Svc.GetByID(ctx, sh.note.ID)
	if err != nil || stored.CampaignID != campaignID || stored.IsFolder ||
		stored.IsOwnedBy(a.UserID, campaignID) || !stored.CanView(a.Viewer(), campaignID) {
		return apperror.NewBadRequest("that note is no longer shared with you")
	}
	w, err := k.Access.NoteWriter(ctx, campaignID, stored.UserID)
	if err != nil {
		return apperror.NewBadRequest("could not tell who wrote " + quote(r.Name))
	}
	access, err := k.Access.NoteAccess(ctx, campaignID)
	if err != nil {
		return apperror.NewBadRequest("could not read which shared notes your AI may change")
	}
	if !access.Allows(w.Kind) {
		return apperror.NewBadRequest(access.Refusal(w.Kind))
	}
	if stored.IsLocked() && !stored.IsLockedByUser(a.UserID) {
		return apperror.NewBadRequest(firstNonEmpty(w.Name, "Its writer") + " is editing " + quote(r.Name) + " right now; paste it again later")
	}
	// Title and text only, as when someone the note is shared with edits it
	// in the Journal.
	req := notes.UpdateNoteRequest{Entry: j, EntryHTML: h}
	if s := r.Str("rename_to"); s != "" {
		req.Title = &s
	}
	_, err = k.Svc.Update(ctx, stored.ID, a.Viewer(), req)
	return err
}

// Export is empty: the Notes category of the export already lists the
// operator's own notes.
func (NoteKind) Export(context.Context, string, Actor) (string, error) { return "", nil }
