package entities

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// LayoutPresetService handles business logic for layout presets.
type LayoutPresetService interface {
	Create(ctx context.Context, campaignID string, input CreateLayoutPresetInput) (*LayoutPreset, error)
	GetByID(ctx context.Context, id int) (*LayoutPreset, error)
	ListForCampaign(ctx context.Context, campaignID string) ([]LayoutPreset, error)
	Update(ctx context.Context, id int, input UpdateLayoutPresetInput) (*LayoutPreset, error)
	Delete(ctx context.Context, id int) error
	SeedDefaults(ctx context.Context, campaignID string) error
}

type layoutPresetService struct {
	repo LayoutPresetRepository
}

// NewLayoutPresetService creates a new layout preset service.
func NewLayoutPresetService(repo LayoutPresetRepository) LayoutPresetService {
	return &layoutPresetService{repo: repo}
}

// Create creates a new campaign-scoped layout preset.
func (s *layoutPresetService) Create(ctx context.Context, campaignID string, input CreateLayoutPresetInput) (*LayoutPreset, error) {
	name, desc, icon, layoutJSON, err := s.validateInput(input.Name, input.Description, input.Icon, input.LayoutJSON)
	if err != nil {
		return nil, err
	}

	p := &LayoutPreset{
		CampaignID:  campaignID,
		Name:        name,
		Description: desc,
		LayoutJSON:  layoutJSON,
		Icon:        icon,
		IsBuiltin:   false,
	}

	if err := s.repo.Create(ctx, p); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating layout preset: %w", err))
	}

	slog.Info("layout preset created",
		slog.Int("preset_id", p.ID),
		slog.String("campaign_id", campaignID),
		slog.String("name", name),
	)
	return p, nil
}

// GetByID returns a layout preset by ID.
func (s *layoutPresetService) GetByID(ctx context.Context, id int) (*LayoutPreset, error) {
	return s.repo.FindByID(ctx, id)
}

// ListForCampaign returns all layout presets for a campaign.
func (s *layoutPresetService) ListForCampaign(ctx context.Context, campaignID string) ([]LayoutPreset, error) {
	return s.repo.ListForCampaign(ctx, campaignID)
}

// Update modifies an existing layout preset.
func (s *layoutPresetService) Update(ctx context.Context, id int, input UpdateLayoutPresetInput) (*LayoutPreset, error) {
	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	name, desc, icon, layoutJSON, err := s.validateInput(input.Name, input.Description, input.Icon, input.LayoutJSON)
	if err != nil {
		return nil, err
	}

	p.Name = name
	p.Description = desc
	p.LayoutJSON = layoutJSON
	p.Icon = icon

	if err := s.repo.Update(ctx, p); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("updating layout preset: %w", err))
	}

	slog.Info("layout preset updated",
		slog.Int("preset_id", id),
		slog.String("name", name),
	)
	return p, nil
}

// Delete removes a layout preset.
func (s *layoutPresetService) Delete(ctx context.Context, id int) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return apperror.NewInternal(fmt.Errorf("deleting layout preset: %w", err))
	}

	slog.Info("layout preset deleted", slog.Int("preset_id", id))
	return nil
}

// validateInput validates and normalizes layout preset input fields.
func (s *layoutPresetService) validateInput(name, desc, icon, layoutJSONRaw string) (string, string, string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", "", "", apperror.NewBadRequest("preset name is required")
	}
	if len(name) > 200 {
		return "", "", "", "", apperror.NewBadRequest("preset name must be at most 200 characters")
	}

	desc = strings.TrimSpace(desc)
	if len(desc) > 500 {
		return "", "", "", "", apperror.NewBadRequest("description must be at most 500 characters")
	}

	layoutJSON := strings.TrimSpace(layoutJSONRaw)
	if layoutJSON == "" {
		return "", "", "", "", apperror.NewBadRequest("layout JSON is required")
	}
	if !json.Valid([]byte(layoutJSON)) {
		return "", "", "", "", apperror.NewBadRequest("layout_json must be valid JSON")
	}
	if len(layoutJSON) > maxLayoutSize {
		return "", "", "", "", apperror.NewBadRequest("layout_json exceeds maximum size")
	}

	// Validate structural integrity by unmarshaling and checking constraints.
	var layout EntityTypeLayout
	if err := json.Unmarshal([]byte(layoutJSON), &layout); err != nil {
		return "", "", "", "", apperror.NewBadRequest("layout_json is not a valid layout structure")
	}
	// Skip block type + singleton validation for presets — block availability
	// is campaign-dependent, and presets ship pre-vetted (any singleton
	// duplication would be a packaging bug, not a user input we need to
	// surface a friendly message for).
	if err := ValidateLayout(layout, nil, nil); err != nil {
		return "", "", "", "", err
	}

	icon = strings.TrimSpace(icon)
	if icon == "" {
		icon = "fa-table-columns"
	}

	return name, desc, icon, layoutJSON, nil
}

// SeedDefaults creates built-in layout presets for a new campaign.
// Called during campaign creation to provide starter presets.
func (s *layoutPresetService) SeedDefaults(ctx context.Context, campaignID string) error {
	presets := defaultLayoutPresets(campaignID)
	for i := range presets {
		if err := s.repo.Create(ctx, &presets[i]); err != nil {
			return fmt.Errorf("seeding layout preset %q: %w", presets[i].Name, err)
		}
	}
	return nil
}

// defaultLayoutPresets returns the built-in starter layout presets.
func defaultLayoutPresets(campaignID string) []LayoutPreset {
	return []LayoutPreset{
		{
			CampaignID:  campaignID,
			Name:        "Standard",
			Description: "Two-column layout with content on the left and sidebar on the right.",
			Icon:        "fa-table-columns",
			SortOrder:   1,
			IsBuiltin:   true,
			LayoutJSON:  mustMarshalLayout(DefaultLayout()),
		},
		{
			CampaignID:  campaignID,
			Name:        "Wide Content",
			Description: "Single full-width column for content-heavy entities.",
			Icon:        "fa-expand",
			SortOrder:   2,
			IsBuiltin:   true,
			LayoutJSON:  mustMarshalLayout(wideContentLayout()),
		},
		{
			CampaignID:  campaignID,
			Name:        "Sidebar Left",
			Description: "Image and attributes on the left, content on the right.",
			Icon:        "fa-arrow-left",
			SortOrder:   3,
			IsBuiltin:   true,
			LayoutJSON:  mustMarshalLayout(sidebarLeftLayout()),
		},
		{
			CampaignID:  campaignID,
			Name:        "Compact Profile",
			Description: "Two-row layout with title and image on top, content and details below.",
			Icon:        "fa-id-card",
			SortOrder:   4,
			IsBuiltin:   true,
			LayoutJSON:  mustMarshalLayout(compactProfileLayout()),
		},
	}
}

// mustMarshalLayout serializes a layout to JSON, panicking on error (safe for
// compile-time constants).
func mustMarshalLayout(layout EntityTypeLayout) string {
	b, err := json.Marshal(layout)
	if err != nil {
		panic("failed to marshal layout preset: " + err.Error())
	}
	return string(b)
}

// wideContentLayout returns a single full-width column layout.
func wideContentLayout() EntityTypeLayout {
	return EntityTypeLayout{
		Rows: []TemplateRow{
			{
				ID: "row-1",
				Columns: []TemplateColumn{
					{
						ID:    "col-1-1",
						Width: 12,
						Blocks: []TemplateBlock{
							{ID: "blk-title", Type: "title"},
							{ID: "blk-image", Type: "image"},
							{ID: "blk-entry", Type: "entry"},
							{ID: "blk-attrs", Type: "attributes"},
							{ID: "blk-details", Type: "details"},
							{ID: "blk-tags", Type: "tags"},
						},
					},
				},
			},
			permissionsRow(),
		},
	}
}

// sidebarLeftLayout returns a layout with sidebar on the left.
func sidebarLeftLayout() EntityTypeLayout {
	return EntityTypeLayout{
		Rows: []TemplateRow{
			{
				ID: "row-1",
				Columns: []TemplateColumn{
					{
						ID:    "col-1-1",
						Width: 4,
						Blocks: []TemplateBlock{
							{ID: "blk-image", Type: "image"},
							{ID: "blk-attrs", Type: "attributes"},
							{ID: "blk-details", Type: "details"},
						},
					},
					{
						ID:    "col-1-2",
						Width: 8,
						Blocks: []TemplateBlock{
							{ID: "blk-title", Type: "title"},
							{ID: "blk-entry", Type: "entry"},
						},
					},
				},
			},
			permissionsRow(),
		},
	}
}

// permissionsRow returns a full-width row with a single permissions
// block. Shared by every preset so freshly seeded entity types include
// the per-entity sharing UI by default.
func permissionsRow() TemplateRow {
	return TemplateRow{
		ID: "row-perm",
		Columns: []TemplateColumn{
			{
				ID:    "col-perm",
				Width: 12,
				Blocks: []TemplateBlock{
					{ID: "blk-perm", Type: "permissions"},
				},
			},
		},
	}
}

// compactProfileLayout returns a two-row layout with title+image on top.
func compactProfileLayout() EntityTypeLayout {
	return EntityTypeLayout{
		Rows: []TemplateRow{
			{
				ID: "row-1",
				Columns: []TemplateColumn{
					{
						ID:    "col-1-1",
						Width: 6,
						Blocks: []TemplateBlock{
							{ID: "blk-title", Type: "title"},
						},
					},
					{
						ID:    "col-1-2",
						Width: 6,
						Blocks: []TemplateBlock{
							{ID: "blk-image", Type: "image"},
						},
					},
				},
			},
			{
				ID: "row-2",
				Columns: []TemplateColumn{
					{
						ID:    "col-2-1",
						Width: 8,
						Blocks: []TemplateBlock{
							{ID: "blk-entry", Type: "entry"},
						},
					},
					{
						ID:    "col-2-2",
						Width: 4,
						Blocks: []TemplateBlock{
							{ID: "blk-attrs", Type: "attributes"},
							{ID: "blk-details", Type: "details"},
						},
					},
				},
			},
			permissionsRow(),
		},
	}
}
