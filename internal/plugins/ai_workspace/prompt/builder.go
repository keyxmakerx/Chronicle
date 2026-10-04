// Package prompt builds the "Copy AI Prompt" output for the AI
// Workspace settings tab. Operator picks schema bits + content bits +
// a custom instruction; this package assembles them into a single
// markdown document via the template in template.go.
//
// Reuses internal/plugins/ai_workspace/aiexport.Service.Generate for
// the optional "Existing world context" section — same renderer that
// powers the AI Export tab, same privacy modes, same SEC-6 egress
// sanitization. Don't duplicate privacy logic.
//
// The prompt template is a Go text/template (not html/template) — AI
// tools consume markdown, not HTML, so no template-side escaping is
// needed. The `join` helper is registered for the Subcategories list.
package prompt

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// EntityLister is the narrow contract the prompt builder needs from
// the entities service. Mirrors aiexport.EntityLister minus the
// per-entity-row Tags batch — the prompt builder doesn't need tags
// inline on the schema list.
type EntityLister interface {
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	List(ctx context.Context, campaignID string, typeID int, role int, userID string, opts entities.ListOptions) ([]entities.Entity, int, error)
}

// TagLister is the narrow contract for the Tags vocabulary section.
// Kept in the interface for future-proofing; BuildPrompt doesn't call
// it yet.
type TagLister interface {
	ListByCampaign(ctx context.Context, campaignID string, includeDmOnly bool) ([]tags.Tag, error)
}

// Exporter is the narrow contract for the "Existing world context"
// section. Implemented by *aiexport.Service.
type Exporter interface {
	Generate(ctx context.Context, campaignName, ownerID, campaignID string, opts aiexport.Options) (string, error)
}

// Service is the prompt builder's entry point. Wired in app/routes.go
// with the same plugin Services that back the aiexport renderer.
type Service struct {
	Entities EntityLister
	Tags     TagLister // optional; unused for now
	Exporter Exporter
}

// NewService constructs the prompt builder. Exporter is required;
// Entities is required if the picker enables any schema bits;
// Tags can be nil.
func NewService(ents EntityLister, tg TagLister, exp Exporter) *Service {
	return &Service{Entities: ents, Tags: tg, Exporter: exp}
}

// Input describes the picker state submitted by the prompt builder
// form. Each schema bool maps to a `{{ if .Include* }}` conditional
// in the template (see template.go); ContentMode controls the
// `Existing world context` block.
type Input struct {
	// --- Schema picker ---

	IncludeEntityTypes        bool
	IncludeCategoriesInUse    bool
	IncludeFrontMatterExample bool

	// IncludeSampleEntity + IncludeTagsVocabulary are template-
	// supported but not yet exposed by the picker UI.
	IncludeSampleEntity   bool
	IncludeTagsVocabulary bool

	// --- Content picker ---

	// ContentMode is one of "none", "all", or a comma-separated list
	// of category slugs (mapping to aiexport.Category values). "none"
	// short-circuits the Exporter call; the "Existing world context"
	// block is omitted from the rendered prompt.
	ContentMode string

	Privacy               aiexport.PrivacyMode
	IncludeSessionGMNotes bool

	// --- Custom ---

	OperatorInstruction string

	// Capabilities is the opening one-line-per-kind list
	// (records.Registry.Capabilities); every prompt carries it.
	Capabilities string
	// RecordDocs describes the non-page kinds (records.Registry.Docs);
	// "" leaves the section out.
	RecordDocs string
	// RecordContext lists today's records (records.Registry.ExportAll),
	// added to the world context when content is included.
	RecordContext string
}

// Build assembles the prompt markdown by combining the schema fetch
// (Entities + optional Tags), the content fetch (via Exporter, only
// when ContentMode != "none"), and the operator instruction, then
// rendering through the template.
//
// Errors from the schema/content fetch abort the build — better to
// surface the failure than ship a half-prompt missing chunks.
func (s *Service) Build(
	ctx context.Context,
	campaignName, ownerID, campaignID string,
	in Input,
) (string, error) {
	data := templateData{
		IncludeEntityTypes:        in.IncludeEntityTypes,
		IncludeCategoriesInUse:    in.IncludeCategoriesInUse,
		IncludeFrontMatterExample: in.IncludeFrontMatterExample,
		IncludeSampleEntity:       in.IncludeSampleEntity,
		IncludeTagsVocabulary:     in.IncludeTagsVocabulary,
		ContentMode:               in.ContentMode,
		OperatorInstruction:       strings.TrimSpace(in.OperatorInstruction),
		RecordDocs:                strings.TrimSpace(in.RecordDocs),
		Capabilities:              strings.TrimSpace(in.Capabilities),
	}

	if in.IncludeEntityTypes || in.IncludeCategoriesInUse {
		if s.Entities == nil {
			return "", fmt.Errorf("prompt: entity lister not wired")
		}
		types, err := s.Entities.GetEntityTypes(ctx, campaignID)
		if err != nil {
			return "", fmt.Errorf("prompt: load entity types: %w", err)
		}
		if in.IncludeEntityTypes {
			data.EntityTypes = mapEntityTypes(types)
		}
		if in.IncludeCategoriesInUse {
			cats, err := s.categoriesInUse(ctx, campaignID, types)
			if err != nil {
				return "", fmt.Errorf("prompt: count categories in use: %w", err)
			}
			data.CategoriesInUse = cats
		}
	}

	if in.ContentMode != "" && in.ContentMode != "none" {
		if s.Exporter == nil {
			return "", fmt.Errorf("prompt: exporter not wired")
		}
		opts := aiexport.Options{
			Privacy:               in.Privacy,
			IncludeSessionGMNotes: in.IncludeSessionGMNotes,
		}
		if in.ContentMode != "all" {
			for _, slug := range strings.Split(in.ContentMode, ",") {
				slug = strings.TrimSpace(slug)
				if slug != "" {
					opts.Categories = append(opts.Categories, aiexport.Category(slug))
				}
			}
		}
		out, err := s.Exporter.Generate(ctx, campaignName, ownerID, campaignID, opts)
		if err != nil {
			return "", fmt.Errorf("prompt: render content: %w", err)
		}
		data.ExportedContent = out
		if rc := strings.TrimSpace(in.RecordContext); rc != "" {
			data.ExportedContent += "\n\n" + rc
		}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("prompt: render template: %w", err)
	}
	return buf.String(), nil
}

// mapEntityTypes adapts the entities.EntityType slice to the slim
// template-shape (Name / Slug / PresetCategory string). Sort stable
// by entity-type SortOrder + Name for deterministic prompt output.
func mapEntityTypes(types []entities.EntityType) []entityTypeView {
	out := make([]entityTypeView, 0, len(types))
	for _, t := range types {
		if !t.Enabled {
			continue
		}
		preset := ""
		if t.PresetCategory != nil {
			preset = *t.PresetCategory
		}
		out = append(out, entityTypeView{
			Name:           t.Name,
			Slug:           t.Slug,
			PresetCategory: preset,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// categoriesInUse builds the "Categories currently in use" rows: per
// entity type, the distinct TypeLabel values + entity count. Pages
// without a TypeLabel show as `(uncategorised)` in the rendered
// prompt so the operator can see uncategorised volume.
//
// A single List call per type pages through up to 1000 entities. A
// dedicated repo method returning DISTINCT TypeLabel + count without
// loading bodies would be more efficient, but this is fine at current
// campaign sizes.
func (s *Service) categoriesInUse(
	ctx context.Context,
	campaignID string,
	types []entities.EntityType,
) ([]categoryInUseView, error) {
	out := make([]categoryInUseView, 0, len(types))
	for _, t := range types {
		if !t.Enabled {
			continue
		}
		ents, total, err := s.Entities.List(ctx, campaignID, t.ID, 0, "", entities.ListOptions{
			Page: 1, PerPage: 1000,
		})
		if err != nil {
			return nil, err
		}
		if total == 0 {
			continue
		}
		seen := make(map[string]struct{})
		var labels []string
		for _, e := range ents {
			label := "(uncategorised)"
			if e.TypeLabel != nil && strings.TrimSpace(*e.TypeLabel) != "" {
				label = strings.TrimSpace(*e.TypeLabel)
			}
			if _, ok := seen[label]; ok {
				continue
			}
			seen[label] = struct{}{}
			labels = append(labels, label)
		}
		sort.Strings(labels)
		out = append(out, categoryInUseView{
			TypeName:      t.Name,
			Subcategories: labels,
			Count:         total,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TypeName < out[j].TypeName })
	return out, nil
}
