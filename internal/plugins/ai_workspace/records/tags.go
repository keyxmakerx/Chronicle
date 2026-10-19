package records

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// TagsAPI is the slice of tags.TagService the tag kind and the tag lookups
// need; the concrete service satisfies it as-is.
type TagsAPI interface {
	ListByCampaign(ctx context.Context, campaignID string, includeDmOnly bool) ([]tags.Tag, error)
	Create(ctx context.Context, campaignID string, name, color string, dmOnly bool) (*tags.Tag, error)
	Update(ctx context.Context, id int, input tags.UpdateTagInput) (*tags.Tag, error)
	Delete(ctx context.Context, id int) error
	GetEntityTagsBatch(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]tags.Tag, error)
}

// tagColor is the same shape the tags service accepts, so a bad colour is
// refused at review instead of failing at commit.
var tagColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// maxTagName is tags.name's column width.
const maxTagName = 100

// tagBatch keeps the IN list of one tag query a sensible size.
const tagBatch = 500

// TagKind makes, renames, recolours, hides and removes the campaign's tags,
// matched by name the way a page block's `tags:` matches them (by slug).
// Which pages carry a tag is set on the page block, never here.
type TagKind struct {
	Svc TagsAPI
	// Pages lets a removal say how many pages lose the tag; nil leaves the
	// count out.
	Pages PagesAPI
}

func (TagKind) Name() string  { return "tag" }
func (TagKind) Label() string { return "Tag" }
func (TagKind) Doc() string {
	return "A tag the campaign puts on pages, matched by `name` (\"Trade Hub\" finds the tag trade-hub). Keys: `color` (#RRGGBB; a new tag is grey when it has none), `dm_only` (true hides the tag and its filter from players), `rename_to`. Putting a tag on or off a page is done on the page block with `tags:`, which makes missing tags itself; use this kind to tidy the tags themselves. Removing a tag takes it off every page that carries it, and only the owner or a co-DM may change tags. Ask `what: tags` to see the tags and how many pages carry each.\n\n```\n---\nkind: tag\nname: Gods\ncolor: \"#8b5cf6\"\ndm_only: false\n---\n```\n\n```\n---\nkind: tag\naction: update\nname: Gods\nrename_to: Deities\n---\n```"
}

// find is the campaign tag the record names, or nil.
func (k TagKind) find(ctx context.Context, campaignID, name string) (*tags.Tag, []tags.Tag, error) {
	all, err := k.Svc.ListByCampaign(ctx, campaignID, true)
	if err != nil {
		return nil, nil, err
	}
	return matchTag(all, name), all, nil
}

// matchTag finds a tag by slug, so spelling and spacing differences still
// land on the tag a page block would use.
func matchTag(all []tags.Tag, name string) *tags.Tag {
	slug := importer.TagSlug(name)
	for i := range all {
		if all[i].Slug == slug {
			return &all[i]
		}
	}
	return nil
}

// tagKeys reads and checks color and dm_only; only written keys are set.
func tagKeys(r Record) (color *string, dmOnly *bool, err error) {
	if r.Has("color") {
		c := r.Str("color")
		if !tagColor.MatchString(c) {
			return nil, nil, badRequestf("color: %q is not a colour written #RRGGBB", c)
		}
		color = &c
	}
	if r.Has("dm_only") {
		// Anything but true or false is refused: Bool reads an unknown word
		// as false, which would make a tag the AI meant to hide public.
		var v bool
		switch t := r.Fields["dm_only"].(type) {
		case bool:
			v = t
		case string:
			switch strings.ToLower(strings.TrimSpace(t)) {
			case "true":
				v = true
			case "false":
			default:
				return nil, nil, badRequestf("dm_only: %q is not true or false", t)
			}
		default:
			return nil, nil, badRequestf("dm_only: %v is not true or false", t)
		}
		dmOnly = &v
	}
	return color, dmOnly, nil
}

func (k TagKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if !a.CanAuthorDmOnly() {
		return Plan{Error: "only the owner or a co-DM can change tags"}
	}
	if r.Name == "" {
		return Plan{Error: "a tag needs a name"}
	}
	if len([]rune(r.Name)) > maxTagName {
		return Plan{Error: fmt.Sprintf("a tag name can be at most %d characters", maxTagName)}
	}
	color, dmOnly, err := tagKeys(r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	hit, all, err := k.find(ctx, campaignID, r.Name)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	switch r.Action {
	case ActionCreate:
		if hit != nil {
			return Plan{Error: "a tag called " + quote(hit.Name) + " already exists; use action: update"}
		}
		if r.Has("rename_to") {
			return Plan{Error: "rename_to only applies to action: update"}
		}
		s := "makes the tag"
		if color != nil {
			s += " in " + *color
		}
		if dmOnly != nil && *dmOnly {
			s += ", hidden from players"
		}
		return Plan{Summary: s}
	case ActionUpdate:
		if hit == nil {
			return Plan{Error: "no tag called " + quote(r.Name)}
		}
		var parts []string
		if to := r.Str("rename_to"); to != "" {
			if len([]rune(to)) > maxTagName {
				return Plan{Error: fmt.Sprintf("rename_to: a tag name can be at most %d characters", maxTagName)}
			}
			if other := matchTag(all, to); other != nil && other.ID != hit.ID {
				return Plan{Error: "rename_to: a tag called " + quote(other.Name) + " already exists"}
			}
			parts = append(parts, "renamed to "+quote(to))
		}
		if color != nil {
			parts = append(parts, "colour "+*color)
		}
		if dmOnly != nil {
			if *dmOnly {
				parts = append(parts, "hidden from players")
			} else {
				parts = append(parts, "shown to players")
			}
		}
		if len(parts) == 0 {
			return Plan{Error: "nothing to change: give rename_to, color or dm_only"}
		}
		return Plan{Summary: strings.Join(parts, ", ")}
	default:
		if hit == nil {
			return Plan{Error: "no tag called " + quote(r.Name)}
		}
		s := "removes the tag from every page that carries it"
		if n, ok := k.pageCount(ctx, campaignID, a, hit.ID); ok {
			s = fmt.Sprintf("removes the tag from %s", pageCountText(n))
		}
		return Plan{Summary: s}
	}
}

func pageCountText(n int) string {
	if n == 1 {
		return "1 page"
	}
	return fmt.Sprintf("%d pages", n)
}

// pageCount is how many pages carry the tag; ok is false when pages can't
// be counted, so the plan then says "every page" instead of a number.
func (k TagKind) pageCount(ctx context.Context, campaignID string, a Actor, id int) (int, bool) {
	if k.Pages == nil {
		return 0, false
	}
	run := &lookupRun{l: &Lookups{Pages: k.Pages}, ctx: ctx, campaignID: campaignID, a: a}
	pages, err := run.visiblePages()
	if err != nil || run.pagesCut {
		return 0, false
	}
	by, err := pageTags(ctx, k.Svc, pages, true)
	if err != nil {
		return 0, false
	}
	n := 0
	for _, ts := range by {
		for _, t := range ts {
			if t.ID == id {
				n++
				break
			}
		}
	}
	return n, true
}

func (k TagKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	color, dmOnly, err := tagKeys(r)
	if err != nil {
		return err
	}
	if r.Action == ActionCreate {
		c, d := "", false
		if color != nil {
			c = *color
		}
		if dmOnly != nil {
			d = *dmOnly
		}
		_, err := k.Svc.Create(ctx, campaignID, r.Name, c, d)
		return err
	}
	hit, _, err := k.find(ctx, campaignID, r.Name)
	if err != nil {
		return err
	}
	if hit == nil {
		return apperror.NewBadRequest("it changed while you were reviewing; check it again")
	}
	if r.Action == ActionDelete {
		return k.Svc.Delete(ctx, hit.ID)
	}
	name := hit.Name
	if to := r.Str("rename_to"); to != "" {
		name = to
	}
	_, err = k.Svc.Update(ctx, hit.ID, tags.UpdateTagInput{Name: name, Color: color, DmOnly: dmOnly})
	return err
}

// Export lists every tag in this kind's own format, so an AI can update or
// remove one by name.
func (k TagKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	all, err := k.Svc.ListByCampaign(ctx, campaignID, true)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, t := range all {
		b.WriteString("---\nkind: tag\nname: " + quote(t.Name) + "\ncolor: " + quote(t.Color) + "\n")
		if t.DmOnly {
			b.WriteString("dm_only: true\n")
		}
		b.WriteString("---\n\n")
	}
	return b.String(), nil
}

// pageTags reads the tags on each page in batches. includeDmOnly must be
// true only for a reader who may see Directors-only tags.
func pageTags(ctx context.Context, svc TagsAPI, pages []entities.Entity, includeDmOnly bool) (map[string][]tags.Tag, error) {
	out := make(map[string][]tags.Tag, len(pages))
	for i := 0; i < len(pages); i += tagBatch {
		end := i + tagBatch
		if end > len(pages) {
			end = len(pages)
		}
		ids := make([]string, 0, end-i)
		for _, p := range pages[i:end] {
			ids = append(ids, p.ID)
		}
		got, err := svc.GetEntityTagsBatch(ctx, ids, includeDmOnly)
		if err != nil {
			return nil, err
		}
		for id, ts := range got {
			out[id] = ts
		}
	}
	return out, nil
}

// pageTagNames is the names on one page, sorted, for index lines.
func pageTagNames(ts []tags.Tag) []string {
	names := make([]string, 0, len(ts))
	for _, t := range ts {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	return names
}

// ---- lookups ----

// tags answers `what: tags`: the tags the Actor may see and how many pages
// (among those they may see) carry each.
func (run *lookupRun) tagsLookup() (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Tags"}
	if run.l.Tags == nil {
		return ans, badRequestf("tags can't be looked up on this server")
	}
	all, err := run.l.Tags.ListByCampaign(run.ctx, run.campaignID, run.director())
	if err != nil {
		return ans, err
	}
	counts := map[int]int{}
	counted := false
	if pages, err := run.visiblePages(); err == nil {
		if by, err := pageTags(run.ctx, run.l.Tags, pages, run.director()); err == nil {
			counted = true
			for _, ts := range by {
				for _, t := range ts {
					counts[t.ID]++
				}
			}
		}
	}
	ans.Chip = fmt.Sprintf("Tags · %d", len(all))
	var b strings.Builder
	fmt.Fprintf(&b, "## Tags (%d)\n\n", len(all))
	var lines []string
	for _, t := range all {
		line := "- " + t.Name
		if t.Color != "" {
			line += ", " + t.Color
		}
		if t.DmOnly {
			line += ", Directors only"
		}
		if counted {
			line += ", " + pageCountText(counts[t.ID])
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		b.WriteString("None yet.\n")
	}
	writeLines(&b, lines)
	ans.Text = b.String()
	return ans, nil
}
