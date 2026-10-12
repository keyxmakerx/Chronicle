// era_rows.go is the importer's side of pages by era: a page block with
// `era:` writes that era's version of a page instead of the page itself.
// What a version stores and which eras exist belong to the calendar plugin;
// the importer reaches them only through EraWriter, wired in internal/app.

package importer

import (
	"context"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// EraRef is one era of the campaign's main calendar as the importing owner
// sees it.
type EraRef struct {
	ID   int
	Name string
}

// EraLookup lists the main calendar's eras. The classifier needs only this,
// so a review can name a bad era before anything is written.
type EraLookup interface {
	Eras(ctx context.Context, campaignID string, v Viewer) ([]EraRef, error)
}

// EraVersionWrite is what an era block changes in a page's version. An empty
// field leaves that part of the version as it was; the text arrives already
// converted and sanitized.
type EraVersionWrite struct {
	KnownAs   string
	EntryJSON string
	EntryHTML string
}

// EraWriter writes and removes a page's versions. Creating a missing version
// is the writer's job, so a block can be pasted twice.
type EraWriter interface {
	EraLookup
	WriteVersion(ctx context.Context, campaignID, entityID string, eraID int, userID string, in EraVersionWrite) error
	DeleteVersion(ctx context.Context, campaignID, entityID string, eraID int) error
}

// FindEra matches a name against eras the way every other name in the
// importer is matched: ignoring case and surrounding space.
func FindEra(eras []EraRef, name string) (EraRef, bool) {
	name = strings.TrimSpace(name)
	for _, e := range eras {
		if strings.EqualFold(strings.TrimSpace(e.Name), name) {
			return e, true
		}
	}
	return EraRef{}, false
}

// eraNotFoundReason says which era was not found and which ones exist. The
// importer is owner-only and the list is what that owner can see, so naming
// them reveals nothing new.
func eraNotFoundReason(name string, eras []EraRef) string {
	if len(eras) == 0 {
		return "The main calendar has no eras, so there is no " + strconv.Quote(name) + " version to write."
	}
	names := make([]string, len(eras))
	for i, e := range eras {
		names[i] = e.Name
	}
	return "The main calendar has no era called " + strconv.Quote(name) + ". Its eras are: " + strings.Join(names, ", ") + "."
}

const (
	eraUnavailableReason = "This server cannot write pages by era, so this block was not imported."
	eraLookupFailReason  = "Could not read the main calendar's eras just now. Try again in a moment."
)

// eraNoPageReason is the plain-words answer to a version block whose page
// does not exist.
func eraNoPageReason(era, page string) string {
	return "Cannot write the " + strconv.Quote(era) + " version: no page named " + strconv.Quote(page) +
		" exists in this campaign or above this block in the paste. Create the page first, in a block without era:."
}

// checkEras downgrades era rows whose era or page cannot be found. It runs
// after every row is classified because "created earlier in this paste"
// depends on the other rows, and it only ever downgrades a row.
func (c *Classifier) checkEras(ctx context.Context, pages []ParsedPage, cls []Classification) {
	var eras []EraRef
	var erasErr error
	fetched := false
	for i, p := range pages {
		if !p.FrontMatter.IsEra() || p.Status == StatusParseError || cls[i].Status == StatusParseError {
			continue
		}
		if c.eras == nil {
			cls[i].Status, cls[i].Reason = StatusActionMismatch, eraUnavailableReason
			continue
		}
		if !fetched {
			eras, erasErr = c.eras.Eras(ctx, c.campaignID, c.viewer)
			fetched = true
		}
		if erasErr != nil {
			cls[i].Status, cls[i].Reason = StatusActionMismatch, eraLookupFailReason
			continue
		}
		if _, ok := FindEra(eras, p.FrontMatter.EraName()); !ok {
			cls[i].Status, cls[i].Reason = StatusActionMismatch, eraNotFoundReason(p.FrontMatter.EraName(), eras)
			continue
		}
		if reason := c.eraPageProblem(ctx, pages, i); reason != "" {
			cls[i].Status, cls[i].Reason = StatusActionMismatch, reason
		}
	}
}

// eraPageProblem explains why era row i has no page to write to; "" means it
// does. The page is an existing one by slug, or a plain page block above it
// in the paste.
func (c *Classifier) eraPageProblem(ctx context.Context, pages []ParsedPage, i int) string {
	p := pages[i]
	slug := entities.Slugify(p.Name)
	created, deleted := false, false
	for _, q := range pages[:i] {
		if q.Status == StatusParseError || q.FrontMatter.IsEra() || entities.Slugify(q.Name) != slug {
			continue
		}
		if q.FrontMatter.Action == ActionDelete {
			deleted = true
		} else {
			created = true
		}
	}
	if created {
		return ""
	}
	if deleted {
		return "The page " + strconv.Quote(p.Name) + " is being deleted earlier in this paste, so it has no " +
			strconv.Quote(p.FrontMatter.EraName()) + " version to write."
	}
	if slug != "" {
		if existing, _ := c.lookup.GetBySlug(ctx, c.campaignID, slug); existing != nil {
			return ""
		}
	}
	return eraNoPageReason(p.FrontMatter.EraName(), p.Name)
}
