// Package campaigns — import.go handles parsing and applying campaign imports.
// Import creates a new campaign from a CampaignExport JSON document, remapping
// all IDs and foreign key references to avoid conflicts with existing data.
package campaigns

import (
	"encoding/json"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// DetectCampaignExport validates that the given JSON is a valid Chronicle
// campaign export and returns the parsed structure. Returns an error if the
// format is unrecognized or the version is unsupported.
func DetectCampaignExport(data []byte) (*CampaignExport, error) {
	// Quick format check via partial parse.
	var envelope struct {
		Format  string `json:"format"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, apperror.NewBadRequest("invalid JSON: " + err.Error())
	}

	if envelope.Format != ExportFormat {
		return nil, apperror.NewBadRequest(
			fmt.Sprintf("unsupported format %q, expected %q", envelope.Format, ExportFormat),
		)
	}
	if envelope.Version < 1 || envelope.Version > ExportVersion {
		return nil, apperror.NewBadRequest(
			fmt.Sprintf("unsupported version %d, max supported is %d", envelope.Version, ExportVersion),
		)
	}

	// Full parse.
	var export CampaignExport
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, apperror.NewBadRequest("failed to parse campaign export: " + err.Error())
	}

	// Basic validation.
	if export.Campaign.Name == "" {
		return nil, apperror.NewBadRequest("campaign name is required in export")
	}

	return &export, nil
}

// IDMap tracks the mapping from original IDs in the export to newly created IDs.
// Used during import to remap all foreign key references.
type IDMap struct {
	// EntityTypeIDs maps original entity type ID → new entity type ID.
	EntityTypeIDs map[int]int

	// EntityIDs maps original entity ID → new entity ID.
	EntityIDs map[string]string

	// EntitySlugToID maps entity slug → new entity ID (for cross-references).
	EntitySlugToID map[string]string

	// TagIDs maps original tag ID → new tag ID.
	TagIDs map[int]int

	// TagSlugToID maps tag slug → new tag ID.
	TagSlugToID map[string]int

	// MapIDs maps original map image ID → new map image ID (for media refs).
	MapIDs map[string]string

	// CampaignID is the new campaign's ID.
	CampaignID string

	// CalendarID is the new calendar's ID (if created).
	CalendarID string
}

// NewIDMap creates an empty ID mapping structure.
func NewIDMap(campaignID string) *IDMap {
	return &IDMap{
		EntityTypeIDs:  make(map[int]int),
		EntityIDs:      make(map[string]string),
		EntitySlugToID: make(map[string]string),
		TagIDs:         make(map[int]int),
		TagSlugToID:    make(map[string]int),
		MapIDs:         make(map[string]string),
		CampaignID:     campaignID,
	}
}
