package crafting

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// A material's picture is chosen in three layers: the item's own, else its
// first kind's as the campaign set it, else that kind's default. Pictures
// have their own names, so renaming or swapping a material never redraws
// anything else.

var tintPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// cleanPicture checks a picture choice; nil or an empty picture means
// "follow the default", which the caller stores as no row.
func cleanPicture(p *MaterialPicture) (*MaterialPicture, error) {
	if p == nil || p.Picture == "" {
		return nil, nil
	}
	out := MaterialPicture{Picture: p.Picture, Tint: strings.ToLower(strings.TrimSpace(p.Tint))}
	if !pictureKnown(out.Picture) {
		return nil, errInvalid(fmt.Sprintf("there is no picture %q", p.Picture))
	}
	if !tintPattern.MatchString(out.Tint) {
		return nil, errInvalid("a tint is a colour like #6a9c3a")
	}
	return &out, nil
}

// usable keeps a stored choice only while the library still has its
// picture, so a retired drawing falls back instead of drawing nothing.
func usable(p MaterialPicture) bool { return pictureKnown(p.Picture) && tintPattern.MatchString(p.Tint) }

// pictureBook resolves what each material wears for one campaign.
type pictureBook struct {
	kinds map[string]MaterialPicture
	items map[string]MaterialPicture
}

func (s *service) pictureBook(ctx context.Context, campaignID string) (*pictureBook, error) {
	kinds, err := s.repo.ListKindPictures(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	items, err := s.repo.ListItemPictures(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	return &pictureBook{kinds: kinds, items: items}, nil
}

// kind is what a kind wears in this campaign.
func (b *pictureBook) kind(k string) MaterialPicture {
	if p, ok := b.kinds[k]; ok && usable(p) {
		return p
	}
	for _, t := range Tags {
		if t.Key == k {
			return MaterialPicture{Picture: t.Picture, Tint: t.Tint}
		}
	}
	return MaterialPicture{}
}

// own is an item's own picture, nil when it follows its kind.
func (b *pictureBook) own(itemID string) *MaterialPicture {
	if p, ok := b.items[itemID]; ok && usable(p) {
		return &p
	}
	return nil
}

// item is what an item wears; tags are its known tags in library order. An
// item with no picture and no tag wears nothing, and the bench draws its
// plain bottle.
func (b *pictureBook) item(itemID string, tags []string) MaterialPicture {
	if p := b.own(itemID); p != nil {
		return *p
	}
	if len(tags) > 0 {
		return b.kind(tags[0])
	}
	return MaterialPicture{}
}

// tags is the library's kinds with this campaign's pictures on them.
func (b *pictureBook) tags() []Tag {
	out := make([]Tag, len(Tags))
	for i, t := range Tags {
		t.DefaultPicture, t.DefaultTint = t.Picture, t.Tint
		p := b.kind(t.Key)
		t.Picture, t.Tint = p.Picture, p.Tint
		out[i] = t
	}
	return out
}

// ---------- Saving ----------

func (s *service) SetKindPictures(ctx context.Context, campaignID string, v Viewer, in KindPicturesInput) ([]Tag, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if len(in.Kinds) == 0 {
		return nil, errInvalid("name at least one kind")
	}
	set := make(map[string]*MaterialPicture, len(in.Kinds))
	for k, p := range in.Kinds {
		if !tagKnown(k) {
			return nil, errInvalid(fmt.Sprintf("there is no crafting tag %q", k))
		}
		clean, err := cleanPicture(p)
		if err != nil {
			return nil, err
		}
		set[k] = clean
	}
	if err := s.repo.SetKindPictures(ctx, campaignID, set); err != nil {
		return nil, errInternal(err)
	}
	book, err := s.pictureBook(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return book.tags(), nil
}

func (s *service) SetItemPicture(ctx context.Context, campaignID, itemID string, v Viewer, in *MaterialPicture) (*MaterialPicture, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	clean, err := cleanPicture(in)
	if err != nil {
		return nil, err
	}
	e, err := s.dir.Entity(ctx, campaignID, itemID)
	if err != nil {
		return nil, errInternal(err)
	}
	if e == nil || !e.IsItem {
		return nil, errNotFound("item not found")
	}
	if err := s.repo.SetItemPicture(ctx, campaignID, itemID, clean); err != nil {
		return nil, errInternal(err)
	}
	return clean, nil
}
