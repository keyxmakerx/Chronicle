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
	GetByID(ctx context.Context, id string) (*notes.Note, error)
	Create(ctx context.Context, campaignID string, creator permissions.Viewer, req notes.CreateNoteRequest) (*notes.Note, error)
	Update(ctx context.Context, id string, editor permissions.Viewer, req notes.UpdateNoteRequest) (*notes.Note, error)
	Delete(ctx context.Context, id string) error
}

// NoteKind reaches only the operator's OWN notes: Journal notes and jots on
// pages. Notes other people wrote are never listed, changed or removed, even
// when they are shared with the operator; ownNotes is the one gate, and
// Apply re-checks ownership on the stored note before every write.
type NoteKind struct {
	Svc      NotesAPI
	Entities EntityLookup
}

func (NoteKind) Name() string  { return "note" }
func (NoteKind) Label() string { return "Note" }
func (NoteKind) Doc() string {
	return "One of my own notes, matched by `name` (its title). Without `page` it is a Journal note; with `page: <page name>` it is my jot on that page. Optional `share`: `private` (default), `gm` or `party`; `rename_to`. The body is the note; write `@[Page Name]` or `@[Page Name|words]` in it to link a page.\n\n```\n---\nkind: note\nname: Who poisoned the duke?\npage: Duke Varrin\n---\nThe cook had the means. Check the cellar key.\n```"
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
	hit, _, err := k.find(ctx, campaignID, a, r)
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
		if hit == nil {
			return Plan{Error: "you have no note called " + quote(r.Name) + " in " + strings.TrimPrefix(where(r), "your ")}
		}
		return Plan{Summary: "changes it in " + where(r)}
	default:
		if hit == nil {
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

// Export is empty: the Notes category of the export already lists the
// operator's own notes.
func (NoteKind) Export(context.Context, string, Actor) (string, error) { return "", nil }
