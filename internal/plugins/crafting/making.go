package crafting

import (
	"context"
	"errors"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ItemMaker makes a new Armory item. Implemented in internal/app over the
// entities plugin. The item is the Director's only until they share it or a
// hero takes one, so a made-up material never shows players a page early.
type ItemMaker interface {
	CreateItem(ctx context.Context, campaignID, userID, name string) (id string, err error)
}

// LayoutPlacer puts the "How it's made" block on the campaign's item pages,
// after their entry, where a layout doesn't already place it. It answers
// how many page types it changed.
type LayoutPlacer interface {
	PlaceItemBlock(ctx context.Context, campaignID string) (int, error)
	// ItemBlockPlaced says whether every item page type already places it.
	ItemBlockPlaced(ctx context.Context, campaignID string) (bool, error)
}

// ItemBlockType is the page block that shows "How it's made" and "In
// crafting" on an Armory item's page.
const ItemBlockType = "crafting_item"

// MaterialInput makes a new Armory item with crafting kinds, for a
// generated recipe or a pack that needs a material the campaign lacks.
type MaterialInput struct {
	Name  string   `json:"name"`
	Kinds []string `json:"kinds"`
}

// CreateMaterial makes a Director-only Armory item and gives it its kinds.
func (s *service) CreateMaterial(ctx context.Context, campaignID string, v Viewer, in MaterialInput) (*MaterialItem, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	name, err := cleanName(in.Name, maxNameLen, "the item's name")
	if err != nil {
		return nil, err
	}
	kinds, err := cleanKinds(in.Kinds)
	if err != nil {
		return nil, err
	}
	id, err := s.makeItem(ctx, campaignID, v, name)
	if err != nil {
		return nil, err
	}
	if len(kinds) > 0 {
		if err := s.repo.SetTags(ctx, campaignID, id, kinds); err != nil {
			return nil, errInternal(err)
		}
	}
	return &MaterialItem{ID: id, Name: name, Tags: kinds}, nil
}

func (s *service) makeItem(ctx context.Context, campaignID string, v Viewer, name string) (string, error) {
	if s.projects.Items == nil {
		return "", apperror.NewNotFound("making Armory items from crafting is not set up on this server")
	}
	id, err := s.projects.Items.CreateItem(ctx, campaignID, v.UserID, name)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) {
			return "", err
		}
		return "", errInternal(err)
	}
	return id, nil
}

// cleanKinds keeps the known kinds, once each, and refuses one that isn't.
func cleanKinds(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, k := range in {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		if !tagKnown(k) {
			return nil, errInvalid("there is no crafting kind " + quote(k))
		}
		seen[k] = true
		out = append(out, k)
	}
	return out, nil
}

// ItemBlock says whether every item page shows "How it's made".
func (s *service) ItemBlock(ctx context.Context, campaignID string, v Viewer) (bool, error) {
	if err := directorOnly(v); err != nil {
		return false, err
	}
	if s.projects.Layouts == nil {
		return true, nil
	}
	ok, err := s.projects.Layouts.ItemBlockPlaced(ctx, campaignID)
	if err != nil {
		return false, errInternal(err)
	}
	return ok, nil
}

// PlaceItemBlock puts "How it's made" on every item page type that lacks
// it. It is the Director's own choice, pressed in the Crafting app: a
// feature never draws a block on a page by itself.
func (s *service) PlaceItemBlock(ctx context.Context, campaignID string, v Viewer) (int, error) {
	if err := directorOnly(v); err != nil {
		return 0, err
	}
	if s.projects.Layouts == nil {
		return 0, apperror.NewNotFound("placing blocks from crafting is not set up on this server")
	}
	n, err := s.projects.Layouts.PlaceItemBlock(ctx, campaignID)
	if err != nil {
		return 0, errInternal(err)
	}
	return n, nil
}
