package records

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/entity_notes"
)

// PlayerNotesAPI is the slice of entity_notes.Service the player-note kind
// uses. That service lets only a note's writer change or remove it, so the
// kind can't reach anyone else's even if a check here were missed.
type PlayerNotesAPI interface {
	List(ctx context.Context, entityID string, viewer entity_notes.ViewerContext) ([]entity_notes.Note, error)
	Get(ctx context.Context, id string, viewer entity_notes.ViewerContext) (*entity_notes.Note, error)
	Create(ctx context.Context, entityID string, viewer entity_notes.ViewerContext, req entity_notes.CreateNoteRequest) (*entity_notes.Note, error)
	Update(ctx context.Context, id string, viewer entity_notes.ViewerContext, req entity_notes.UpdateNoteRequest) (*entity_notes.Note, error)
	Delete(ctx context.Context, id string, viewer entity_notes.ViewerContext) error
}

// KindPlayerNote is the player-note kind's name, which the feature gate
// switches off with the Player Notes add-on.
const KindPlayerNote = "player-note"

// PlayerNoteKind reaches only the operator's own Player Notes, the notes
// members leave on a page. Other members' notes are read with a lookup and
// never changed.
type PlayerNoteKind struct {
	Svc      PlayerNotesAPI
	Entities EntityLookup
}

func (PlayerNoteKind) Name() string  { return KindPlayerNote }
func (PlayerNoteKind) Label() string { return "Player Note" }
func (PlayerNoteKind) Doc() string {
	return "One of my own Player Notes on a page, matched by `name` (its title) and `page` (required). Other members' Player Notes can be read with a lookup but never changed. Optional `share`: `private` (default), `gm` (the DM only), `gm-scribes` (the DM and Scribes) or `everyone`; `rename_to`. The body is the note; write `@[Page Name]` in it to link a page.\n\n```\n---\nkind: player-note\nname: Prices here\npage: The Rusty Anchor\nshare: gm-scribes\n---\nRope is too cheap; raise it before next session.\n```"
}

// playerShares maps the block's `share` words to Player Notes audiences.
var playerShares = map[string]entity_notes.Audience{
	"private":    entity_notes.AudiencePrivate,
	"gm":         entity_notes.AudienceDMOnly,
	"gm-scribes": entity_notes.AudienceDMScribe,
	"everyone":   entity_notes.AudienceEveryone,
}

func (k PlayerNoteKind) page(ctx context.Context, campaignID string, r Record) (*entities.Entity, error) {
	name := r.Str("page")
	if name == "" {
		return nil, badRequestf("a Player Note needs the `page` it sits on")
	}
	e, err := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(name))
	if err != nil || e == nil || e.CampaignID != campaignID {
		return nil, badRequestf("no page called %s", quote(name))
	}
	return e, nil
}

// find is the operator's own note on the record's page with its title.
func (k PlayerNoteKind) find(ctx context.Context, campaignID string, a Actor, r Record) (*entity_notes.Note, *entities.Entity, error) {
	e, err := k.page(ctx, campaignID, r)
	if err != nil {
		return nil, nil, err
	}
	all, err := k.Svc.List(ctx, e.ID, entity_notes.ViewerFor(campaignID, a.Viewer()))
	if err != nil {
		return nil, e, apperror.NewBadRequest("could not read the Player Notes on " + e.Name)
	}
	var hit *entity_notes.Note
	for i := range all {
		n := &all[i]
		if n.AuthorUserID == a.UserID && n.CampaignID == campaignID && sameName(n.Title, r.Name) {
			if hit != nil {
				return nil, e, badRequestf("you have more than one Player Note called %s on %s", quote(r.Name), e.Name)
			}
			hit = n
		}
	}
	return hit, e, nil
}

func (k PlayerNoteKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if r.Name == "" {
		return Plan{Error: "a Player Note needs a name (its title)"}
	}
	if s := r.Str("share"); s != "" && playerShares[s] == "" {
		return Plan{Error: "share must be private, gm, gm-scribes or everyone"}
	}
	hit, e, err := k.find(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	switch r.Action {
	case ActionCreate:
		p := Plan{Summary: "your Player Note on " + e.Name}
		if hit != nil {
			p.Warnings = append(p.Warnings, "You already have a Player Note with this title there; this adds another")
		}
		return p
	case ActionUpdate:
		if hit == nil {
			return Plan{Error: "you have no Player Note called " + quote(r.Name) + " on " + e.Name}
		}
		return Plan{Summary: "changes your Player Note on " + e.Name}
	default:
		if hit == nil {
			return Plan{Error: "you have no Player Note called " + quote(r.Name) + " on " + e.Name}
		}
		return Plan{Summary: "removes your Player Note from " + e.Name}
	}
}

func (k PlayerNoteKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	hit, e, err := k.find(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	j, h, err := entry(r.Body, r.Links)
	if err != nil {
		return err
	}
	v := entity_notes.ViewerFor(campaignID, a.Viewer())
	share := playerShares[r.Str("share")]
	if r.Action == ActionCreate {
		req := entity_notes.CreateNoteRequest{Audience: share, Title: r.Name}
		if j != nil {
			req.Body, req.BodyHTML = json.RawMessage(*j), *h
		}
		_, err := k.Svc.Create(ctx, e.ID, v, req)
		return err
	}
	// Re-read the stored note: authorship is checked on what is saved, not
	// on the list it came from.
	if hit == nil {
		return apperror.NewBadRequest("it changed while you were reviewing; check it again")
	}
	stored, err := k.Svc.Get(ctx, hit.ID, v)
	if err != nil || stored.AuthorUserID != a.UserID || stored.CampaignID != campaignID {
		return apperror.NewBadRequest("that Player Note is not yours")
	}
	if r.Action == ActionDelete {
		return k.Svc.Delete(ctx, stored.ID, v)
	}
	req := entity_notes.UpdateNoteRequest{}
	if j != nil {
		req.Body, req.BodyHTML = json.RawMessage(*j), h
	}
	if s := strings.TrimSpace(r.Str("rename_to")); s != "" {
		req.Title = &s
	}
	if share != "" {
		req.Audience = &share
	}
	_, err = k.Svc.Update(ctx, stored.ID, v, req)
	return err
}

// Export is empty: Player Notes are read with the notes lookup.
func (PlayerNoteKind) Export(context.Context, string, Actor) (string, error) { return "", nil }
