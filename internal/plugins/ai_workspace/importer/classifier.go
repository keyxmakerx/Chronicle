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

	// Conflict detection — entity-by-slug.
	existing, err := c.lookup.GetBySlug(ctx, c.campaignID, cls.ResolvedSlug)
	if err == nil && existing != nil {
		cls.ConflictEntity = existing
		cls.Status = StatusConflict
	}

	// Category detection — FrontMatter.Type vs known types.
	cls.ProposedTypeSlug = strings.ToLower(strings.TrimSpace(p.FrontMatter.Type))
	if cls.ProposedTypeSlug != "" {
		if et, ok := c.typesBySlug[cls.ProposedTypeSlug]; ok {
			cls.ExistingType = et
		} else {
			cls.IsNewCategory = true
			// New-category status wins over Conflict at the chip
			// level — the operator must resolve the category
			// (Create new / Map to existing) before Rename / Overwrite
			// becomes meaningful. The Conflict info is still
			// available via cls.ConflictEntity so the templ can
			// render both signals on the row.
			cls.Status = StatusNewCategory
		}
	}

	return cls, nil
}
