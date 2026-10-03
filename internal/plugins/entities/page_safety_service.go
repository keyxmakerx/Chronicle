package entities

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// SetPageSafety wires history, the Trash and the save-clash check into the
// entity service. Without it Delete removes pages for good and nothing is
// versioned, which is what the unit tests' fake repositories expect.
func (s *entityService) SetPageSafety(repo PageSafetyRepository) {
	s.safety = repo
}

// recordVersion adds the page's new title and text to its history. Errors
// are logged, not returned: a save must never fail because history did.
//
// before is the state the save replaced. A page with no history yet gets it
// recorded first (as a baseline) so that very save can be undone too. A save
// by the same person within versionCoalesceWindow of their last one updates
// that row instead of adding another.
func (s *entityService) recordVersion(ctx context.Context, before, after *Entity, kind string) {
	if s.safety == nil || after == nil {
		return
	}
	if k, ok := ctx.Value(versionKindKey{}).(string); ok {
		kind = k
	}
	now := time.Now().UTC()
	var actor *string
	if id := actorFrom(ctx); id != "" {
		actor = &id
	}

	latest, err := s.safety.LatestVersion(ctx, after.ID)
	if err != nil {
		slog.Warn("page history: reading latest version", slog.String("entity_id", after.ID), slog.Any("error", err))
		return
	}

	if latest == nil && before != nil && kind != VersionCreated {
		// Dated when it was last saved, but always before this save.
		at := before.UpdatedAt
		if !at.Before(now) {
			at = now.Add(-time.Millisecond)
		}
		base := &EntityVersion{
			ID: generateUUID(), EntityID: after.ID, Kind: VersionBaseline,
			Name: before.Name, Entry: before.Entry, EntryHTML: before.EntryHTML,
			CreatedAt: at, UpdatedAt: at,
		}
		if err := s.safety.InsertVersion(ctx, base); err != nil {
			slog.Warn("page history: recording baseline", slog.String("entity_id", after.ID), slog.Any("error", err))
			return
		}
	}

	if kind == VersionEdit && latest != nil && latest.Kind == VersionEdit &&
		sameActor(latest.UserID, actor) && now.Sub(latest.UpdatedAt) < versionCoalesceWindow {
		latest.Name, latest.Entry, latest.EntryHTML, latest.UpdatedAt = after.Name, after.Entry, after.EntryHTML, now
		if err := s.safety.ReplaceVersionContent(ctx, latest); err != nil {
			slog.Warn("page history: updating version", slog.String("entity_id", after.ID), slog.Any("error", err))
		}
		return
	}

	v := &EntityVersion{
		ID: generateUUID(), EntityID: after.ID, UserID: actor, Kind: kind,
		Name: after.Name, Entry: after.Entry, EntryHTML: after.EntryHTML,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.safety.InsertVersion(ctx, v); err != nil {
		slog.Warn("page history: recording version", slog.String("entity_id", after.ID), slog.Any("error", err))
		return
	}
	if err := s.safety.PruneVersions(ctx, after.ID, MaxVersionsPerPage); err != nil {
		slog.Warn("page history: pruning", slog.String("entity_id", after.ID), slog.Any("error", err))
	}
}

func sameActor(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// EntryRev returns the page text's revision; 0 without page safety.
func (s *entityService) EntryRev(ctx context.Context, entityID string) (int, error) {
	if s.safety == nil {
		return 0, nil
	}
	return s.safety.EntryRev(ctx, entityID)
}

// SaveEntry saves the editor's text. Without baseRev (an older editor, or no
// page safety) it behaves exactly like UpdateEntry. With it, the save only
// lands if nobody else changed the text since that revision; otherwise the
// stored text and its author come back as an EntryConflict and nothing is
// written, so the editor can let the person choose.
func (s *entityService) SaveEntry(ctx context.Context, entityID, entryJSON, entryHTML string, baseRev *int) (int, *EntryConflict, error) {
	if baseRev == nil || s.safety == nil {
		if err := s.UpdateEntry(ctx, entityID, entryJSON, entryHTML); err != nil {
			return 0, nil, err
		}
		rev, err := s.EntryRev(ctx, entityID)
		return rev, nil, err
	}
	if strings.TrimSpace(entryJSON) == "" {
		return 0, nil, apperror.NewBadRequest("entry content is required")
	}
	entryHTML = sanitize.HTML(entryHTML)

	before, err := s.entities.FindByID(ctx, entityID)
	if err != nil {
		return 0, nil, err
	}
	searchText := buildSearchText(entryHTML, before.FieldsData)

	rev, ok, err := s.safety.SaveEntryAtRev(ctx, entityID, entryJSON, entryHTML, searchText, *baseRev)
	if err != nil {
		return 0, nil, err
	}
	if !ok {
		current, err := s.entities.FindByID(ctx, entityID)
		if err != nil {
			return 0, nil, err
		}
		conflict := &EntryConflict{Rev: rev, Entry: current.Entry, EntryHTML: current.EntryHTML, At: current.UpdatedAt}
		if latest, err := s.safety.LatestVersion(ctx, entityID); err == nil && latest != nil {
			conflict.ByName = latest.UserName
			conflict.At = latest.UpdatedAt
		}
		return rev, conflict, nil
	}

	after, err := s.entities.FindByID(ctx, entityID)
	if err != nil {
		return rev, nil, nil
	}
	s.recordVersion(ctx, before, after, VersionEdit)
	s.events.PublishEntityEvent("updated", after.CampaignID, entityID, after)
	return rev, nil, nil
}

// --- History and Trash, as the owner and scribes use them ---

// PageSafetyService serves the History panel and the Trash page.
type PageSafetyService interface {
	// History returns a page's versions, newest first, each with what
	// changed since the one before it.
	History(ctx context.Context, entityID string) ([]VersionView, error)
	// RestoreVersion puts a version's title and text back on the page,
	// saved as a new version so the restore can be undone too.
	RestoreVersion(ctx context.Context, entityID, versionID string) error

	ListTrash(ctx context.Context, campaignID string) ([]TrashItem, error)
	// RestoreFromTrash brings a page and its sub-pages back.
	RestoreFromTrash(ctx context.Context, campaignID, entityID string) (*TrashItem, error)
	RetentionDays(ctx context.Context) int
	// StartPurger deletes for good, hourly, pages that have waited out the
	// retention. It returns when ctx ends.
	StartPurger(ctx context.Context)
}

// VersionView is a version with its changes against the version before it.
type VersionView struct {
	EntityVersion
	Added   int        // lines added
	Removed int        // lines removed
	Diff    []DiffPart // word-level, for the selected version's preview
	First   bool       // the oldest version: nothing to compare with
}

type pageSafetyService struct {
	repo      PageSafetyRepository
	entities  EntityRepository
	svc       *entityService
	retention TrashRetention
}

// NewPageSafetyService builds the History/Trash service. svc must be the
// entity service SetPageSafety was called on, so restores are versioned and
// broadcast like any other save.
func NewPageSafetyService(repo PageSafetyRepository, entities EntityRepository, svc EntityService, retention TrashRetention) PageSafetyService {
	es, _ := svc.(*entityService)
	return &pageSafetyService{repo: repo, entities: entities, svc: es, retention: retention}
}

func (p *pageSafetyService) History(ctx context.Context, entityID string) ([]VersionView, error) {
	versions, err := p.repo.ListVersions(ctx, entityID, MaxVersionsPerPage)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	out := make([]VersionView, len(versions))
	for i := range versions {
		out[i].EntityVersion = versions[i]
		newer := versionText(&versions[i])
		if i+1 < len(versions) {
			older := versionText(&versions[i+1])
			out[i].Diff = DiffWords(older, newer)
			out[i].Added, out[i].Removed = CountLineChanges(older, newer)
		} else {
			out[i].First = true
			out[i].Diff = DiffWords("", newer)
		}
	}
	return out, nil
}

// versionText is what a version's diff compares: its title then its text,
// one paragraph per line.
func versionText(v *EntityVersion) string {
	body := ""
	if v.EntryHTML != nil {
		body = HTMLToPlainLines(*v.EntryHTML)
	}
	return strings.TrimSpace(v.Name + "\n" + body)
}

func (p *pageSafetyService) RestoreVersion(ctx context.Context, entityID, versionID string) error {
	v, err := p.repo.FindVersion(ctx, entityID, versionID)
	if err != nil {
		return err
	}
	after, err := p.svc.restoreVersion(ctx, entityID, v)
	if err != nil {
		return err
	}
	slog.Info("page version restored", slog.String("entity_id", entityID), slog.String("version_id", versionID), slog.String("name", after.Name))
	return nil
}

// restoreVersion writes a version's title and text back to the page. It
// writes the stored HTML rather than going through Update, whose entry
// input is the edit form's and would re-derive entry_html from it.
func (s *entityService) restoreVersion(ctx context.Context, entityID string, v *EntityVersion) (*Entity, error) {
	entity, err := s.entities.FindByID(ctx, entityID)
	if err != nil {
		return nil, err
	}
	before := *entity

	if name := strings.TrimSpace(v.Name); name != "" && name != entity.Name {
		slug, err := s.generateSlug(ctx, entity.CampaignID, name)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		entity.Name, entity.Slug = name, slug
	}
	entity.Entry, entity.EntryHTML = nil, nil
	if v.Entry != nil && strings.TrimSpace(*v.Entry) != "" {
		entry := *v.Entry
		html := ""
		if v.EntryHTML != nil {
			html = sanitize.HTML(*v.EntryHTML)
		}
		entity.Entry, entity.EntryHTML = &entry, &html
	}
	entity.UpdatedAt = time.Now().UTC()
	if err := s.entities.Update(ctx, entity); err != nil {
		return nil, apperror.NewInternal(err)
	}
	if entity.Entry != nil {
		// Update leaves search_text alone; UpdateEntry refreshes it.
		if err := s.entities.UpdateEntry(ctx, entityID, *entity.Entry, *entity.EntryHTML,
			buildSearchText(*entity.EntryHTML, entity.FieldsData)); err != nil {
			return nil, err
		}
	}
	s.recordVersion(withVersionKind(ctx, VersionRestore), &before, entity, VersionRestore)
	s.events.PublishEntityEvent("updated", entity.CampaignID, entityID, entity)
	return entity, nil
}

// versionKindKey lets a caller label the history row a save produces (a
// restore goes through the same Update as an edit).
type versionKindKey struct{}

func withVersionKind(ctx context.Context, kind string) context.Context {
	return context.WithValue(ctx, versionKindKey{}, kind)
}

func (p *pageSafetyService) ListTrash(ctx context.Context, campaignID string) ([]TrashItem, error) {
	items, err := p.repo.ListTrash(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return items, nil
}

func (p *pageSafetyService) RestoreFromTrash(ctx context.Context, campaignID, entityID string) (*TrashItem, error) {
	item, err := p.repo.FindTrashItem(ctx, campaignID, entityID)
	if err != nil {
		return nil, err
	}
	if _, err := p.repo.RestoreTrashItem(ctx, campaignID, entityID); err != nil {
		return nil, err
	}
	slog.Info("page restored from trash", slog.String("entity_id", entityID), slog.Int("sub_pages", item.SubPages))
	if p.svc != nil {
		if e, err := p.entities.FindByID(ctx, entityID); err == nil {
			p.svc.events.PublishEntityEvent("created", campaignID, entityID, e)
		}
	}
	return item, nil
}

func (p *pageSafetyService) RetentionDays(ctx context.Context) int {
	if p.retention == nil {
		return DefaultTrashRetentionDays
	}
	return NormalizeTrashRetention(p.retention.TrashRetentionDays(ctx))
}

// NormalizeTrashRetention maps any stored value onto an offered choice,
// falling back to the default, so a bad setting never purges early.
func NormalizeTrashRetention(days int) int {
	for _, d := range TrashRetentionChoices {
		if d == days {
			return d
		}
	}
	return DefaultTrashRetentionDays
}

// DaysLeft is how many whole days, rounded up, a Trash item has left.
func DaysLeft(deletedAt time.Time, retentionDays int, now time.Time) int {
	left := deletedAt.Add(time.Duration(retentionDays) * 24 * time.Hour).Sub(now)
	if left <= 0 {
		return 0
	}
	return int((left + 24*time.Hour - 1) / (24 * time.Hour))
}

func (p *pageSafetyService) purgeOnce(ctx context.Context) {
	cutoff := time.Now().UTC().AddDate(0, 0, -p.RetentionDays(ctx))
	n, err := p.repo.PurgeTrashedBefore(ctx, cutoff)
	if err != nil {
		slog.Error("trash purge failed", slog.Any("error", err))
		return
	}
	if n > 0 {
		slog.Info("trash purged", slog.Int("pages", n), slog.Time("cutoff", cutoff))
	}
}

func (p *pageSafetyService) StartPurger(ctx context.Context) {
	p.purgeOnce(ctx)
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.purgeOnce(ctx)
		}
	}
}
