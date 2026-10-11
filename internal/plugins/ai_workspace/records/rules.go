package records

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

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
//
// A chapter is written in the package's own chapter-file format, in the
// front matter: `intro`, `director` and `pages`, whose blocks are the book's
// blocks (boxes, dice rolls, fold-outs…). The book draws them with its usual
// look, so nothing here styles them. An update with `pages` replaces every
// page of the chapter at once.
type HouseRuleKind struct {
	Book RulebookAPI
	// SystemOf returns the campaign's game system id ("" for none).
	SystemOf func(ctx context.Context, campaignID string) string
}

// houseRuleDraftKeys are the front-matter keys that make up the chapter.
var houseRuleDraftKeys = []string{"intro", "director", "pages"}

func (HouseRuleKind) Name() string  { return "house-rule" }
func (HouseRuleKind) Label() string { return "House rule" }
func (HouseRuleKind) Doc() string {
	return "A house-rules chapter in the campaign's rulebook, matched by `name` (its title). " +
		"The chapter goes in the front matter: `intro` (a line under the title), optional `director: true` " +
		"(Directors only), and `pages`. Each page has `title` and `blocks`, and may have `director: true`. " +
		"Blocks (a block without `type` is text):\n\n" +
		"- text: `text`. `**bold**`, `*italic*`; a blank line starts a paragraph; `- ` lines are a list.\n" +
		"- `type: callout` (a highlighted box): `title`, `text`.\n" +
		"- `type: roll` (dice readers roll; the result's band lights up): `dice` like `2d10`, `label`, " +
		"`bands` lowest first, each with `label`, `text` and `max` (the highest total it covers; the last band has none).\n" +
		"- `type: cards` (cards that open for detail): `items`, each `title`, `summary`, `text`.\n" +
		"- `type: flaps` (headings that open to explain): `items`, each `title`, `text`.\n" +
		"- `type: example` (stepped through line by line): `title`, `steps` (a list).\n" +
		"- `type: creature`: `name`, `tagline`, `look`, `notice` (a list), `stats` (each `label`, `value`; Directors only), `note`.\n" +
		"- `type: note` (a Director's note, never shown to players): `text`.\n\n" +
		"Any block can have `director: true`. An update that has `pages` replaces all the chapter's pages, " +
		"so send the whole chapter, changed. An update without `pages` keeps them. `rename_to` renames it.\n\n" +
		"```\n---\nkind: house-rule\nname: Table rules\nintro: How we play at this table.\npages:\n" +
		"  - title: Fumbles\n    blocks:\n      - text: A couple of rules we added.\n" +
		"      - type: callout\n        title: Critical fumbles\n        text: A natural 2 drops your weapon. Picking it up is a **maneuver**.\n" +
		"      - type: roll\n        dice: 1d6\n        label: Heroic surge\n        bands:\n" +
		"          - max: 2\n            label: Nothing\n            text: The moment passes.\n" +
		"          - label: Surge\n            text: Gain 1 surge.\n---\n```"
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

// draft reads the chapter from the front matter. Older blocks wrote the
// introduction as the body, which still works when `intro` is absent.
func (k HouseRuleKind) draft(r Record, pkg *systems.BookPackage) (*systems.HouseChapterDraft, error) {
	sub := map[string]any{}
	for _, key := range houseRuleDraftKeys {
		if v, ok := r.Fields[key]; ok {
			sub[key] = v
		}
	}
	if _, ok := sub["intro"]; !ok {
		if body := strings.TrimSpace(r.Body); body != "" {
			sub["intro"] = body
		}
	}
	if _, ok := sub["director"]; ok {
		v, _ := r.Bool("director")
		sub["director"] = v
	}
	raw, err := yaml.Marshal(sub)
	if err != nil {
		return nil, apperror.NewBadRequest("the chapter could not be read")
	}
	return systems.ParseHouseChapterDraft(raw, pkg)
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
	if r.Action == ActionDelete {
		if ch == nil {
			return Plan{Error: "no house-rules chapter called " + quote(r.Name)}
		}
		return Plan{Summary: fmt.Sprintf("removes the chapter and its %s", plural(len(ch.Pages), "page", "pages"))}
	}
	d, err := k.draft(r, pkg)
	if err != nil {
		return Plan{Error: planError(err)}
	}

	var p Plan
	title, intro, director := r.Name, "", false
	switch r.Action {
	case ActionCreate:
		if ch != nil {
			return Plan{Error: "a house-rules chapter called " + quote(r.Name) + " already exists; use action: update"}
		}
		p.Summary = "new house-rules chapter"
		if d.HasPages() {
			p.Summary += ": " + describeDraft(d)
		}
	default:
		if ch == nil {
			return Plan{Error: "no house-rules chapter called " + quote(r.Name)}
		}
		title, intro, director = ch.Title, ch.Intro, ch.Director
		if s := r.Str("rename_to"); s != "" {
			title = s
		}
		p.Summary = "changes the chapter"
		if d.HasPages() {
			p.Summary = fmt.Sprintf("replaces its %s with %s", plural(len(ch.Pages), "page", "pages"), describeDraft(d))
			p.Warnings = replaceWarnings(*ch, d)
		}
	}
	if d.Director != nil && !*d.Director && director {
		p.Warnings = append(p.Warnings, "Players will be able to read this chapter; today only Directors can.")
	}
	if book := systems.HouseChapterPreview(pkg, title, intro, director, d); book != nil {
		if raw, err := json.Marshal(book); err == nil {
			p.Preview = string(raw)
		}
	}
	return p
}

// describeDraft is "2 pages, with 1 highlighted box, 1 dice roll".
func describeDraft(d *systems.HouseChapterDraft) string {
	s := plural(len(d.PageTitles()), "page", "pages")
	if b := d.Blocks(); b != "" {
		s += ", with " + b
	}
	return s
}

// replaceWarnings names what a whole-chapter replacement would lose: pages
// whose titles are gone, and Director-only parts the new version lacks
// (an AI working from the players' view never saw them).
func replaceWarnings(ch systems.BookChapterEntry, d *systems.HouseChapterDraft) []string {
	var out []string
	kept := map[string]bool{}
	for _, t := range d.PageTitles() {
		kept[strings.ToLower(t)] = true
	}
	var gone []string
	for _, pe := range ch.Pages {
		if t := strings.TrimSpace(pe.Page.Title); t != "" && !kept[strings.ToLower(t)] {
			gone = append(gone, quote(t))
		}
	}
	if len(gone) > 0 {
		out = append(out, "These pages are not in the new version and will be removed: "+strings.Join(gone, ", ")+".")
	}
	if had, has := systems.HiddenPartsOf(ch), d.HiddenParts(); had > has {
		out = append(out, fmt.Sprintf("The chapter has %s for Directors only; the new version has %d. Check nothing you hid from players is lost.", plural(had, "page or block", "pages or blocks"), has))
	}
	return out
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
	d, err := k.draft(r, pkg)
	if err != nil {
		return err
	}
	if d.HasPages() {
		id, title := "", r.Name
		if r.Action == ActionUpdate {
			id, title = ch.ID, r.Str("rename_to")
		}
		_, err = svc.WriteHouseChapter(ctx, campaignID, a.UserID, pkg, id, title, d)
		return err
	}

	// No pages: a new chapter starts with the editor's blank page, and an
	// update changes only the chapter's own lines.
	if r.Action == ActionCreate {
		if ch, err = svc.CreateChapter(ctx, campaignID, a.UserID, pkg, r.Name); err != nil {
			return err
		}
	}
	var in systems.UpdateBookChapterInput
	if d.Intro != nil && *d.Intro != "" {
		in.Intro = patch.Of(*d.Intro)
	}
	if s := r.Str("rename_to"); s != "" && r.Action == ActionUpdate {
		in.Title = patch.Of(s)
	}
	if d.Director != nil {
		in.Director = patch.Of(*d.Director)
	}
	if !in.Intro.Present() && !in.Title.Present() && !in.Director.Present() {
		return nil
	}
	_, err = svc.UpdateChapter(ctx, campaignID, pkg, ch.ID, in)
	return err
}

// Export writes each house chapter whole, in the format above, so an AI can
// send it back changed. As a player (Safe privacy) it is the players' view:
// Director-only chapters, pages and blocks are left out, and a line says so.
func (k HouseRuleKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	pkg, svc, err := k.Book.CampaignBookPackage(campaignID, k.SystemOf(ctx, campaignID))
	if err != nil || pkg == nil || svc == nil {
		return "", nil // no rulebook is not an error for the export
	}
	src, err := svc.EditorSource(ctx, campaignID, pkg)
	if err != nil {
		return "", err
	}
	director := a.CanAuthorDmOnly()
	var b strings.Builder
	hidden := 0
	for _, part := range src.Parts {
		for _, ch := range part.Chapters {
			if !ch.House {
				continue
			}
			if ch.Director && !director {
				hidden++
				continue
			}
			out, h, err := systems.HouseChapterYAML(ch, director)
			if err != nil {
				return "", err
			}
			hidden += h
			b.WriteString("### " + ch.Title + "\n\n```yaml\n" + string(out) + "```\n\n")
		}
	}
	if hidden > 0 {
		b.WriteString(fmt.Sprintf("(%s for Directors only %s left out here.)\n", plural(hidden, "chapter, page or block", "chapters, pages or blocks"), wereWas(hidden)))
	}
	return b.String(), nil
}

func wereWas(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}
