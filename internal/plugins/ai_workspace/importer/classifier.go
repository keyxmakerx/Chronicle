// classifier.go upgrades a ParsedPage's Status from the parser's
// StatusNew → StatusConflict or StatusNewCategory based on live
// campaign state. The parser is repo-free (testable without a DB);
// classification needs the entity-type registry + a same-slug
// lookup, so it lives in this file with a narrow interface the
// handler injects.

package importer

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// CampaignLookup is the narrow contract the classifier needs from
// the entities service. Kept narrow so plugin-isolation + tests
// don't need the 30-method EntityService surface.
type CampaignLookup interface {
	GetBySlug(ctx context.Context, campaignID, slug string) (*entities.Entity, error)
	GetEntityTypeBySlug(ctx context.Context, campaignID, slug string) (*entities.EntityType, error)
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	// CheckEntityAccess lets @[Name] links skip pages the viewer cannot see.
	CheckEntityAccess(ctx context.Context, entityID string, role int, userID string) (*entities.EffectivePermission, error)
}

// Classification is the per-page outcome of Classify. Stored on the
// review-row struct that ReviewScreen renders; the commit handler
// consumes the same struct to decide create vs map vs skip / rename
// / overwrite.
type Classification struct {
	// Status is the upgraded value (StatusNew / StatusConflict /
	// StatusNewCategory / StatusParseError). StatusParseError is
	// carried over from the parser without re-checking.
	Status ParseStatus

	// ResolvedSlug is the entity slug derived from the page name
	// (via entities.Slugify). Empty when the page has no name.
	// The commit handler uses this to call entityService.Create / Update.
	ResolvedSlug string

	// ConflictEntity is non-nil when an entity with ResolvedSlug
	// already exists in the campaign. The commit handler reads this
	// to drive the Rename / Overwrite handling.
	ConflictEntity *entities.Entity

	// GMOnly counts the GM-only parts of ConflictEntity's current text. The
	// review screen puts a "Replace the text anyway" tick on any row that
	// could update such a page.
	GMOnly GMOnlyParts

	// ProposedTypeSlug is the FrontMatter.Type lowercased + trimmed.
	// "" when no type was specified in FM (operator picks via bulk
	// default at review time).
	ProposedTypeSlug string

	// ExistingType is non-nil when ProposedTypeSlug names a current
	// entity type. The commit handler uses ExistingType.ID for entity
	// creation.
	ExistingType *entities.EntityType

	// IsNewCategory is true when ProposedTypeSlug is non-empty AND
	// ExistingType is nil. Drives the review screen's "Create new"
	// / "Map to existing" radio.
	IsNewCategory bool

	// AvailableTypes is the full list of campaign entity types,
	// passed through to the review-screen dropdowns. Cached on the
	// classifier to avoid N+1 fetches per page.
	AvailableTypes []entities.EntityType

	// Reason is the human-readable explanation for StatusActionMismatch
	// rows, rendered verbatim by the review-screen templ. Empty for
	// all other statuses.
	Reason string

	// FieldsSummary is the review line for the row's `fields:`, checked
	// against the category the row will have; "" when it sets none.
	FieldsSummary string

	// SheetKind names the sheet a game system can check the row's numbers
	// against ("creature", or the category's preset category); "" when
	// there is nothing to check. SheetFields is the sheet as the row leaves
	// it (the page's own fields under the AI's, on an update) and
	// SheetWritten the keys the AI wrote, the only ones a check may offer
	// to replace.
	SheetKind    string
	SheetFields  map[string]any
	SheetWritten []string
}

// Classifier holds the lookup + a per-classify-call type cache.
// Construct one per request via NewClassifier; reuse across the
// full page-list classification to amortise the GetEntityTypes
// call.
type Classifier struct {
	lookup       CampaignLookup
	campaignID   string
	cachedTypes  []entities.EntityType
	typesBySlug  map[string]*entities.EntityType
	typesFetched bool
	// eras and viewer resolve `era:` blocks; without eras such a block is
	// reported as unavailable.
	eras   EraLookup
	viewer Viewer
	// folders checks `folder:` names; nil refuses any row that names one.
	folders        FolderAccess
	pendingFolders map[string]bool
	// folderSeen keeps hasFolder's answers by type and lower-cased name.
	folderSeen map[string]bool
	// rollTables resolves {roll{Name}} lines, so an update that keeps a
	// page's table rollers is not counted as losing them.
	rollTables map[string]string
	// foundryHas says where the campaign's Foundry world keeps a monster of
	// that name, or "" when it reported none; nil skips the check.
	foundryHas func(name string) string
}

// WithFoundryMonsters lets the classifier warn when a new creature page
// names a monster Foundry already has, which would put it in Foundry twice.
func (c *Classifier) WithFoundryMonsters(has func(name string) string) *Classifier {
	c.foundryHas = has
	return c
}

// WithEras lets the classifier check `era:` blocks against the main
// calendar's eras as the importing viewer sees them.
func (c *Classifier) WithEras(l EraLookup, v Viewer) *Classifier {
	c.eras, c.viewer = l, v
	return c
}

// WithRollTables gives the classifier the tables {roll{Name}} lines can
// name, as PageLinks.SetRollTables takes them.
func (c *Classifier) WithRollTables(ids map[string]string) *Classifier {
	c.rollTables = ids
	return c
}

// NewClassifier constructs a per-request classifier. The lookup is
// the live entities.EntityService (or a stub in tests).
func NewClassifier(lookup CampaignLookup, campaignID string) *Classifier {
	return &Classifier{lookup: lookup, campaignID: campaignID}
}

// ClassifyAll runs Classify on every page in the slice and returns
// the classifications in the same order. Caller pairs them with the
// original ParsedPage slice by index. ctx cancellation surfaces as
// a partial result + the original error.
func (c *Classifier) ClassifyAll(ctx context.Context, pages []ParsedPage) ([]Classification, error) {
	if err := c.ensureTypes(ctx); err != nil {
		return nil, err
	}
	out := make([]Classification, len(pages))
	for i, p := range pages {
		cls, err := c.classifyOne(ctx, p)
		if err != nil {
			return out, err
		}
		out[i] = cls
	}
	c.checkParents(ctx, pages, out)
	c.checkFolders(ctx, pages, out)
	c.checkEras(ctx, pages, out)
	c.checkFields(pages, out)
	c.checkFoundryDoubles(pages, out)
	return out, nil
}

// checkFoundryDoubles tells the DM when a new creature page has the name of
// a monster Foundry already has. The page is matched to that monster and
// plays with Foundry's numbers, so a stat block on it goes unused unless the
// DM switches the page to its own numbers; it is a note, not a refusal.
func (c *Classifier) checkFoundryDoubles(pages []ParsedPage, cls []Classification) {
	if c.foundryHas == nil {
		return
	}
	for i, p := range pages {
		if cls[i].Status != StatusNew || cls[i].ConflictEntity != nil || p.FrontMatter.Action == ActionDelete ||
			cls[i].ExistingType == nil || c.sheetKind(cls[i].ExistingType) != "creature" {
			continue
		}
		if where := c.foundryHas(p.Name); where != "" {
			pages[i].Warnings = append(pages[i].Warnings, "Foundry already has a monster called "+p.Name+" ("+where+"). This page will be matched to it and play with Foundry's numbers, so it only needs lore; switch it to its own numbers on the page to play a house-ruled copy.")
		}
	}
}

// checkFields checks each row's `fields:` against the sheet of the
// category it will have: the page's own on an update (an update never
// changes the category), the front matter's on a create. A row whose
// fields don't fit becomes StatusFieldProblem; the committer checks again
// against the category finally picked.
func (c *Classifier) checkFields(pages []ParsedPage, cls []Classification) {
	for i, p := range pages {
		if len(p.FrontMatter.Fields) == 0 || p.Status == StatusParseError || cls[i].Status == StatusActionMismatch ||
			p.FrontMatter.Action == ActionDelete {
			continue
		}
		if p.FrontMatter.IsEra() {
			cls[i].Status = StatusFieldProblem
			cls[i].Reason = "Sheet fields can't be set on an era version. Put them on the usual page's block."
			continue
		}
		var et *entities.EntityType
		switch {
		case p.FrontMatter.Action == ActionUpdate && cls[i].ConflictEntity != nil:
			et = c.typeByID(cls[i].ConflictEntity.EntityTypeID)
		case cls[i].IsNewCategory:
			cls[i].Status = StatusFieldProblem
			cls[i].Reason = "The category " + `"` + cls[i].ProposedTypeSlug + `"` + " is new, so it has no sheet fields yet. Use an existing category, or leave the fields out."
			continue
		case cls[i].ExistingType != nil:
			et = cls[i].ExistingType
		}
		if et == nil {
			n := len(p.FrontMatter.Fields)
			cls[i].FieldsSummary = fmt.Sprintf("Sets %d sheet %s, checked against the category picked here", n, pluralWord(n, "field", "fields"))
			continue
		}
		checked, problems := CheckFields(et.Fields, p.FrontMatter.Fields)
		if len(problems) > 0 {
			cls[i].Status = StatusFieldProblem
			cls[i].Reason = "These sheet fields don't fit the " + et.Name + " category: " + strings.Join(problems, " ")
			continue
		}
		cls[i].FieldsSummary = "Sheet: " + FieldsSummary(et.Fields, checked)
		if kind := c.sheetKind(et); kind != "" {
			var stored map[string]any
			if p.FrontMatter.Action == ActionUpdate && cls[i].ConflictEntity != nil {
				stored = cls[i].ConflictEntity.FieldsData
			}
			cls[i].SheetKind = kind
			cls[i].SheetFields, cls[i].SheetWritten = sheetForCheck(stored, checked)
		}
	}
}

// sheetKind is what a game system's review check knows the category as:
// "creature" for a creature type or one of its sub-types, otherwise the
// category's preset category; "" for a category no system made.
func (c *Classifier) sheetKind(et *entities.EntityType) string {
	for _, id := range entities.CreatureTypeIDs(c.cachedTypes) {
		if id == et.ID {
			return "creature"
		}
	}
	if et.PresetCategory != nil {
		return *et.PresetCategory
	}
	return ""
}

// sheetForCheck lays the AI's checked values over the page's stored ones,
// so a check of an update sees the level and organization the page already
// has, and lists the keys the AI wrote.
func sheetForCheck(stored, checked map[string]any) (map[string]any, []string) {
	out := make(map[string]any)
	for k, v := range stored {
		out[k] = v
	}
	written := make([]string, 0, len(checked))
	for k, v := range checked {
		out[k] = v
		written = append(written, k)
	}
	sort.Strings(written)
	return out, written
}

// typeByID finds a cached category by id; nil when it is not there.
func (c *Classifier) typeByID(id int) *entities.EntityType {
	for i := range c.cachedTypes {
		if c.cachedTypes[i].ID == id {
			return &c.cachedTypes[i]
		}
	}
	return nil
}

// checkParents turns an unusable `parent:` into a row problem. It runs
// after every row is classified because "created earlier in this paste"
// depends on the other rows, and it only ever downgrades a row, never
// hides an existing problem.
func (c *Classifier) checkParents(ctx context.Context, pages []ParsedPage, cls []Classification) {
	for i, p := range pages {
		if p.Status == StatusParseError || cls[i].Status == StatusParseError ||
			cls[i].Status == StatusActionMismatch || p.FrontMatter.Action == ActionDelete ||
			p.FrontMatter.IsEra() {
			continue
		}
		key, ok := p.FrontMatter.ParentKey()
		if !ok {
			continue
		}
		if problem := c.parentProblem(ctx, pages, i, key); problem != "" {
			cls[i].Status = StatusParentProblem
			cls[i].Reason = problem
		}
	}
}

// parentProblem explains, in plain words, why page i's parent cannot be
// used; "" means it resolves. Names match by slug, so two pastes of the
// same name are the only way to be ambiguous: the campaign itself keeps
// slugs unique.
func (c *Classifier) parentProblem(ctx context.Context, pages []ParsedPage, i int, key string) string {
	named := strings.TrimSpace(pages[i].FrontMatter.Parent)
	quoted := `"` + named + `"`
	if key == "" {
		return "The parent " + quoted + " is not a usable page name."
	}
	if key == entities.Slugify(pages[i].Name) {
		return "A page cannot be its own parent. Name a different page, or remove the parent line."
	}
	earlier, deleted := 0, false
	for j, q := range pages {
		if q.Status == StatusParseError || q.FrontMatter.IsEra() || entities.Slugify(q.Name) != key {
			continue
		}
		if j < i {
			if q.FrontMatter.Action == ActionDelete {
				deleted = true
				continue
			}
			earlier++
		}
	}
	switch {
	case earlier > 1:
		return "The parent " + quoted + " matches more than one page in this paste. Give each page a distinct name."
	case earlier == 1:
		return ""
	case deleted:
		return "The parent " + quoted + " is being deleted in this paste."
	}
	if existing, _ := c.lookup.GetBySlug(ctx, c.campaignID, key); existing != nil {
		return ""
	}
	for j := i + 1; j < len(pages); j++ {
		if pages[j].Status != StatusParseError && !pages[j].FrontMatter.IsEra() && entities.Slugify(pages[j].Name) == key {
			return "The parent " + quoted + " comes later in this paste. Move it above this page."
		}
	}
	return "No page called " + quoted + " exists in this campaign or earlier in this paste."
}

// ensureTypes lazy-loads the entity-type registry. Called by
// ClassifyAll before any per-page work to amortise the call.
func (c *Classifier) ensureTypes(ctx context.Context) error {
	if c.typesFetched {
		return nil
	}
	types, err := c.lookup.GetEntityTypes(ctx, c.campaignID)
	if err != nil {
		return err
	}
	c.cachedTypes = types
	c.typesBySlug = make(map[string]*entities.EntityType, len(types))
	for i, t := range types {
		c.typesBySlug[strings.ToLower(t.Slug)] = &c.cachedTypes[i]
	}
	// An AI writes the category as either name ("race" or "races"), so the
	// plural name finds the type too, unless another type's slug is that word.
	for i, t := range types {
		if alias := entities.Slugify(t.NamePlural); alias != "" {
			if _, taken := c.typesBySlug[alias]; !taken {
				c.typesBySlug[alias] = &c.cachedTypes[i]
			}
		}
	}
	c.typesFetched = true
	return nil
}

// classifyOne computes the upgraded status + the lookup metadata
// for a single ParsedPage.
func (c *Classifier) classifyOne(ctx context.Context, p ParsedPage) (Classification, error) {
	cls := Classification{
		Status:         p.Status,
		AvailableTypes: c.cachedTypes,
	}
	if p.Status == StatusParseError {
		// Parser-level error carries through; no slug derivation.
		return cls, nil
	}

	cls.ResolvedSlug = entities.Slugify(p.Name)

	// An era block names an existing page and never creates or renames it,
	// so slug conflicts and categories do not apply; checkEras decides
	// whether its era and page exist.
	if p.FrontMatter.IsEra() {
		return cls, nil
	}

	// Interpretation of an existing slug depends on the front-matter
	// action: create treats it as StatusConflict (operator picks
	// Skip/Rename/Update); update/delete treat it as the legitimate
	// target, and a MISSING slug is StatusActionMismatch instead.
	// ConflictEntity is set whenever a match exists so the
	// review-screen can show its current name/ID next to the chip.
	existing, _ := c.lookup.GetBySlug(ctx, c.campaignID, cls.ResolvedSlug)
	if existing != nil {
		cls.ConflictEntity = existing
		cls.GMOnly = GMOnlyPartsForMarkdown(existing, p.Body, c.rollTables)
	}

	switch p.FrontMatter.Action {
	case ActionUpdate:
		if existing == nil {
			cls.Status = StatusActionMismatch
			cls.Reason = "Cannot update: no existing entity named " +
				`"` + p.Name + `"` + " in this campaign."
			return cls, nil
		}
		// existing target: status stays StatusNew.
	case ActionDelete:
		if existing == nil {
			cls.Status = StatusActionMismatch
			cls.Reason = "Cannot delete: no existing entity named " +
				`"` + p.Name + `"` + " in this campaign."
			return cls, nil
		}
	default: // action=create (or empty, defaulted by parser)
		if existing != nil {
			cls.Status = StatusConflict
		}
	}

	// Category detection (FrontMatter.Type vs known types). Classified
	// for update/delete too, even though commit ignores it there, to
	// keep the templ branching and UI shape uniform.
	cls.ProposedTypeSlug = strings.ToLower(strings.TrimSpace(p.FrontMatter.Type))
	if cls.ProposedTypeSlug != "" {
		if et, ok := c.typesBySlug[cls.ProposedTypeSlug]; ok {
			cls.ExistingType = et
		} else {
			cls.IsNewCategory = true
			// NewCategory wins over Conflict: the operator must resolve
			// the category before Rename/Update applies. ActionMismatch
			// (set above) stays unaffected.
			if cls.Status != StatusActionMismatch {
				cls.Status = StatusNewCategory
			}
		}
	}

	return cls, nil
}
