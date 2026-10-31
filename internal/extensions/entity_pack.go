package extensions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PackEntry is one page in an entity pack file. The file is a JSON array of
// these.
type PackEntry struct {
	Name      string         `json:"name"`
	TypeLabel string         `json:"type_label,omitempty"`
	Entry     string         `json:"entry,omitempty"`  // Markdown body.
	Fields    map[string]any `json:"fields,omitempty"` // Custom field values by key.
	// Private defaults to true: a pack may hold spoilers, so its pages start
	// GM-only and the owner reveals them. "private": false opts out.
	Private *bool `json:"private,omitempty"`
}

// IsPrivate reports whether the page starts GM-only.
func (e PackEntry) IsPrivate() bool {
	return e.Private == nil || *e.Private
}

// PackEntityCreator creates the pages an entity pack holds. Implemented in
// internal/app over the entities service.
type PackEntityCreator interface {
	// EntityTypeIDBySlug finds a campaign's entity type by slug.
	EntityTypeIDBySlug(ctx context.Context, campaignID, slug string) (int, error)
	// CreatePackEntity creates one page. It returns the new page's id even
	// when the page was created but its body could not be saved.
	CreatePackEntity(ctx context.Context, campaignID, userID string, typeID int, e PackEntry) (string, error)
}

// appliedState is what an earlier Apply already created in a campaign.
type appliedState struct {
	typeIDs map[string]int  // Template slug to created entity type id.
	packs   map[string]bool // Entity pack slugs already created.
}

// packRecordType is the provenance record type of a page from an entity pack.
func packRecordType(slug string) string { return "entity_pack:" + slug }

// alreadyApplied reads provenance to find what this extension already
// created in the campaign. A read failure yields an empty state, so content
// may be created twice rather than not at all.
func (a *contentApplier) alreadyApplied(ctx context.Context, campaignID, extensionID string) appliedState {
	st := appliedState{typeIDs: map[string]int{}, packs: map[string]bool{}}
	rows, err := a.repo.ListProvenance(ctx, campaignID, extensionID)
	if err != nil {
		slog.Warn("reading extension provenance", slog.Any("error", err))
		return st
	}
	for _, p := range rows {
		switch {
		case p.TableName == "entity_types":
			if id, err := strconv.Atoi(p.RecordID); err == nil {
				st.typeIDs[p.RecordType] = id
			}
		case p.TableName == "entities" && strings.HasPrefix(p.RecordType, "entity_pack:"):
			st.packs[strings.TrimPrefix(p.RecordType, "entity_pack:")] = true
		}
	}
	return st
}

// applyEntityPacks creates each pack's pages once per campaign. A pack names
// its category by slug: a template from this extension first, otherwise an
// entity type the campaign already has. A pack with an unknown category, an
// unreadable file or too many pages is skipped whole; a page that fails is
// skipped alone.
func (a *contentApplier) applyEntityPacks(
	ctx context.Context,
	campaignID, userID string,
	ext *Extension,
	packs []EntityPack,
	done appliedState,
) {
	for _, pack := range packs {
		if done.packs[pack.Slug] {
			continue
		}
		log := slog.With(slog.String("ext_id", ext.ExtID), slog.String("pack", pack.Slug))

		typeID, ok := done.typeIDs[pack.EntityTypeSlug]
		if !ok {
			id, err := a.packPages.EntityTypeIDBySlug(ctx, campaignID, pack.EntityTypeSlug)
			if err != nil {
				log.Warn("entity pack names a category the campaign does not have",
					slog.String("entity_type_slug", pack.EntityTypeSlug), slog.Any("error", err))
				continue
			}
			typeID = id
		}

		entries, err := a.readPackFile(ext.ExtID, pack.File)
		if err != nil {
			log.Warn("reading entity pack", slog.Any("error", err))
			continue
		}

		created := 0
		for _, e := range entries {
			id, err := a.packPages.CreatePackEntity(ctx, campaignID, userID, typeID, e)
			if id != "" {
				created++
				if perr := a.repo.CreateProvenance(ctx, &Provenance{
					CampaignID:  campaignID,
					ExtensionID: ext.ID,
					TableName:   "entities",
					RecordID:    id,
					RecordType:  packRecordType(pack.Slug),
				}); perr != nil {
					log.Warn("recording entity pack provenance", slog.Any("error", perr))
				}
			}
			if err != nil {
				log.Warn("entity pack page", slog.String("name", e.Name), slog.Any("error", err))
			}
		}
		log.Info("applied entity pack", slog.Int("pages", created), slog.Int("in_file", len(entries)))
	}
}

// readPackFile reads and checks one entity pack file from the extension's
// directory. The manifest validator already refused paths that climb out.
func (a *contentApplier) readPackFile(extID, file string) ([]PackEntry, error) {
	path := filepath.Join(a.extDir, extID, file)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > DefaultMaxFileSize {
		return nil, fmt.Errorf("%s is larger than %d bytes", file, DefaultMaxFileSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []PackEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%s is not a JSON array of pages: %w", file, err)
	}
	if len(entries) > DefaultMaxEntityPack {
		return nil, fmt.Errorf("%s holds %d pages (max %d)", file, len(entries), DefaultMaxEntityPack)
	}
	return entries, nil
}
