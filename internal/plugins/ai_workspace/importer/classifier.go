// classifier.go upgrades a ParsedPage's Status from the parser's
// StatusNew → StatusConflict or StatusNewCategory based on live
// campaign state. The parser is repo-free (testable without a DB);
// classification needs the entity-type registry + a same-slug
// lookup, so it lives in this file with a narrow interface the
// handler injects.
//
// Per scoping §2.2 (the review screen's Status column drives the
// per-row UI variant) + §3.8 (per-category create-once decision).

package importer

import (
	"context"
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
}

// Classification is the per-page outcome of Classify. Stored on the
// review-row struct that ReviewScreen renders; the commit handler
// (Phase 5) consumes the same struct to decide create vs map vs
// skip / rename / overwrite.
type Classification struct {
	// Status is the upgraded value (StatusNew / StatusConflict /
	// StatusNewCategory / StatusParseError). StatusParseError is
	// carried over from the parser without re-checking.
	Status ParseStatus

	// ResolvedSlug is the entity slug derived from the page name
	// (via entities.Slugify). Empty when the page has no name.
	// Phase 5 uses this to call entityService.Create / Update.
	ResolvedSlug string

	// ConflictEntity is non-nil when an entity with ResolvedSlug
	// already exists in the campaign. Phase 5 reads this to drive
	// the Rename / Overwrite handling.
	ConflictEntity *entities.Entity

	// ProposedTypeSlug is the FrontMatter.Type lowercased + trimmed.
	// "" when no type was specified in FM (operator picks via bulk
	// default at review time).
	ProposedTypeSlug string

	// ExistingType is non-nil when ProposedTypeSlug names a current
	// entity type. Phase 5 uses ExistingType.ID for entity creation.
	ExistingType *entities.EntityType

	// IsNewCategory is true when ProposedTypeSlug is non-empty AND
	// ExistingType is nil. Drives the review screen's "Create new"
	// / "Map to existing" radio.
	IsNewCategory bool

	// AvailableTypes is the full list of campaign entity types,
	// passed through to the review-screen dropdowns. Cached on the
	// classifier to avoid N+1 fetches per page.
	AvailableTypes []entities.EntityType

	// Reason (V1.5 / C-AI-WORKSPACE-V1-G) carries the human-readable
	// explanation for StatusActionMismatch rows. Empty for all other
	// statuses. The review-screen templ renders this verbatim on the
	// row card so the operator sees why the action verb is at odds
	// with live campaign state.
	Reason string
}

// Classifier holds the lookup + a per-classify-call type cache.
// Construct one per request via NewClassifier; reuse across the
// full page-list classification to amortise the GetEntityTypes
// call.
type Classifier struct {
	lookup        CampaignLookup
	campaignID    string
	cachedTypes   []entities.EntityType
	typesBySlug   map[string]*entities.EntityType
	typesFetched  bool
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
	return out, nil
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

	// Slug lookup. The interpretation depends on the front-matter
	// action verb (V1.5 / C-AI-WORKSPACE-V1-G):
	//
	//   - action=create (default): existing-slug means StatusConflict;
	//     operator picks Skip / Rename / Update via the per-row
	//     conflict-mode dropdown.
	//   - action=update: existing-slug is the LEGITIMATE target;
	//     classified as StatusNew (ready to commit). Missing slug is
	//     StatusActionMismatch.
	//   - action=delete: same shape as update — existing-slug is
	//     legitimate; missing slug is StatusActionMismatch.
	//
	// For all three actions, ConflictEntity is set to the existing
	// row when it exists so the review-screen templ can render the
	// entity's current name + ID alongside the action chip.
	existing, _ := c.lookup.GetBySlug(ctx, c.campaignID, cls.ResolvedSlug)
	if existing != nil {
		cls.ConflictEntity = existing
	}

	switch p.FrontMatter.Action {
	case ActionUpdate:
		if existing == nil {
			cls.Status = StatusActionMismatch
			cls.Reason = "Cannot update: no existing entity named " +
				`"` + p.Name + `"` + " in this campaign."
			return cls, nil
		}
		// existing != nil: legitimate update target. Status stays at
		// StatusNew (carried from the parser); UI branches on the
		// row's Action chip to render "Update" vs "New".
	case ActionDelete:
		if existing == nil {
			cls.Status = StatusActionMismatch
			cls.Reason = "Cannot delete: no existing entity named " +
				`"` + p.Name + `"` + " in this campaign."
			return cls, nil
		}
		// existing != nil: legitimate delete target.
	default:
		// action=create (or empty, defaulted by parser): existing V1
		// conflict semantics. Existing slug → StatusConflict so the
		// operator picks Skip / Rename / Update via the conflict-mode
		// dropdown.
		if existing != nil {
			cls.Status = StatusConflict
		}
	}

	// Category detection — FrontMatter.Type vs known types. For
	// action=delete the category is irrelevant (commit doesn't read
	// it), but classifying it consistently keeps the templ branching
	// simple. For action=update, the category is also irrelevant at
	// commit but populating it preserves the existing UI shape.
	cls.ProposedTypeSlug = strings.ToLower(strings.TrimSpace(p.FrontMatter.Type))
	if cls.ProposedTypeSlug != "" {
		if et, ok := c.typesBySlug[cls.ProposedTypeSlug]; ok {
			cls.ExistingType = et
		} else {
			cls.IsNewCategory = true
			// New-category status wins over Conflict at the chip
			// level for action=create — the operator must resolve
			// the category (Create new / Map to existing) before
			// Rename / Update becomes meaningful. ActionMismatch
			// (set above for update/delete with no target) stays;
			// no-such-category doesn't change the diagnosis.
			if cls.Status != StatusActionMismatch {
				cls.Status = StatusNewCategory
			}
		}
	}

	return cls, nil
}
