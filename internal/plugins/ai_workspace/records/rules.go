package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// RulebookAPI opens the campaign's editable rulebook (systems.SystemHandler).
type RulebookAPI interface {
	CampaignBookPackage(campaignID, systemID string) (*systems.BookPackage, systems.BookEditService, error)
}

// HouseRuleKind adds, changes and removes house-rules chapters. Only house
// chapters are ever matched, so the game system's own chapters cannot be
// touched (the book service refuses them too).
type HouseRuleKind struct {
	Book RulebookAPI
	// SystemOf returns the campaign's game system id ("" for none).
	SystemOf func(ctx context.Context, campaignID string) string
}

func (HouseRuleKind) Name() string  { return "house-rule" }
func (HouseRuleKind) Label() string { return "House rule" }
func (HouseRuleKind) Doc() string {
	return "A house-rules chapter in the campaign's rulebook, matched by `name` (its title). The body is the chapter's introduction text. Optional `director: true` keeps it to Directors; `rename_to`.\n\n```\n---\nkind: house-rule\nname: Critical fumbles\n---\nA natural 1 on an attack roll drops the weapon.\n```"
}

func (k HouseRuleKind) open(ctx context.Context, campaignID string, a Actor) (*systems.BookPackage, systems.BookEditService, error) {
	if !a.CanAuthorDmOnly() {
		return nil, nil, apperror.NewBadRequest("only the campaign's Directors can edit the rulebook")
	}
	return k.Book.CampaignBookPackage(campaignID, k.SystemOf(ctx, campaignID))
}

func (k HouseRuleKind) find(ctx context.Context, campaignID, name string, pkg *systems.BookPackage, svc systems.BookEditService) (*systems.BookChapterEntry, error) {
	src, err := svc.EditorSource(ctx, campaignID, pkg)
	if err != nil {
		return nil, apperror.NewBadRequest("could not read the rulebook")
	}
	for _, part := range src.Parts {
		for i := range part.Chapters {
			if ch := &part.Chapters[i]; ch.House && sameName(ch.Title, name) {
				return ch, nil
			}
		}
	}
	return nil, nil
}

func (k HouseRuleKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if r.Name == "" {
		return Plan{Error: "a house rule needs a name (its title)"}
	}
	pkg, svc, err := k.open(ctx, campaignID, a)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	ch, err := k.find(ctx, campaignID, r.Name, pkg, svc)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	switch r.Action {
	case ActionCreate:
		if ch != nil {
			return Plan{Error: "a house-rules chapter called " + quote(r.Name) + " already exists; use action: update"}
		}
		return Plan{Summary: "new house-rules chapter"}
	case ActionUpdate:
		if ch == nil {
			return Plan{Error: "no house-rules chapter called " + quote(r.Name)}
		}
		return Plan{Summary: "changes the chapter"}
	default:
		if ch == nil {
			return Plan{Error: "no house-rules chapter called " + quote(r.Name)}
		}
		return Plan{Summary: fmt.Sprintf("removes the chapter and its %d pages", len(ch.Pages))}
	}
}

func (k HouseRuleKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	pkg, svc, err := k.open(ctx, campaignID, a)
	if err != nil {
		return err
	}
	ch, err := k.find(ctx, campaignID, r.Name, pkg, svc)
	if err != nil {
		return err
	}
	if r.Action != ActionCreate && ch == nil {
		return apperror.NewBadRequest("it changed while you were reviewing; check it again")
	}
	if r.Action == ActionDelete {
		return svc.DeleteChapter(ctx, campaignID, pkg, ch.ID)
	}
	if r.Action == ActionCreate {
		if ch, err = svc.CreateChapter(ctx, campaignID, a.UserID, pkg, r.Name); err != nil {
			return err
		}
	}
	var in systems.UpdateBookChapterInput
	if body := strings.TrimSpace(r.Body); body != "" {
		in.Intro = patch.Of(body)
	}
	if s := r.Str("rename_to"); s != "" && r.Action == ActionUpdate {
		in.Title = patch.Of(s)
	}
	if v, ok := r.Bool("director"); ok {
		in.Director = patch.Of(v)
	}
	if !in.Intro.Present() && !in.Title.Present() && !in.Director.Present() {
		return nil
	}
	_, err = svc.UpdateChapter(ctx, campaignID, pkg, ch.ID, in)
	return err
}

func (k HouseRuleKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	pkg, svc, err := k.open(ctx, campaignID, a)
	if err != nil {
		return "", nil // no rulebook is not an error for the export
	}
	src, err := svc.EditorSource(ctx, campaignID, pkg)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, part := range src.Parts {
		for _, ch := range part.Chapters {
			if ch.House {
				b.WriteString("- " + ch.Title + "\n")
			}
		}
	}
	return b.String(), nil
}
