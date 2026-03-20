package entities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// maxContentTemplateSize is the maximum allowed size for content JSON or HTML (100KB).
const maxContentTemplateSize = 102400

// ContentTemplateService handles business logic for content templates.
type ContentTemplateService interface {
	Create(ctx context.Context, campaignID string, input CreateContentTemplateInput) (*ContentTemplate, error)
	GetByID(ctx context.Context, id int) (*ContentTemplate, error)
	ListForCampaign(ctx context.Context, campaignID string) ([]ContentTemplate, error)
	ListForCampaignAndType(ctx context.Context, campaignID string, entityTypeID int) ([]ContentTemplate, error)
	Update(ctx context.Context, id int, input UpdateContentTemplateInput) (*ContentTemplate, error)
	Delete(ctx context.Context, id int) error
	SeedDefaults(ctx context.Context, campaignID string) error
}

type contentTemplateService struct {
	repo  ContentTemplateRepository
	types EntityTypeRepository
}

// NewContentTemplateService creates a new content template service.
func NewContentTemplateService(repo ContentTemplateRepository, types EntityTypeRepository) ContentTemplateService {
	return &contentTemplateService{repo: repo, types: types}
}

// Create creates a new campaign-scoped content template.
func (s *contentTemplateService) Create(ctx context.Context, campaignID string, input CreateContentTemplateInput) (*ContentTemplate, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("template name is required")
	}
	if len(name) > 200 {
		return nil, apperror.NewBadRequest("template name must be at most 200 characters")
	}

	desc := strings.TrimSpace(input.Description)
	if len(desc) > 500 {
		return nil, apperror.NewBadRequest("description must be at most 500 characters")
	}

	contentJSON := strings.TrimSpace(input.ContentJSON)
	if contentJSON == "" {
		return nil, apperror.NewBadRequest("template content is required")
	}
	if !json.Valid([]byte(contentJSON)) {
		return nil, apperror.NewBadRequest("content_json must be valid JSON")
	}
	if len(contentJSON) > maxContentTemplateSize {
		return nil, apperror.NewBadRequest("content_json exceeds maximum size")
	}
	if len(input.ContentHTML) > maxContentTemplateSize {
		return nil, apperror.NewBadRequest("content_html exceeds maximum size")
	}

	icon := strings.TrimSpace(input.Icon)
	if icon == "" {
		icon = "fa-file-lines"
	}

	// Validate entity type if specified.
	var entityTypeID *int
	if input.EntityTypeID > 0 {
		et, err := s.types.FindByID(ctx, input.EntityTypeID)
		if err != nil {
			return nil, apperror.NewBadRequest("invalid entity type")
		}
		if et.CampaignID != campaignID {
			return nil, apperror.NewBadRequest("entity type does not belong to this campaign")
		}
		entityTypeID = &input.EntityTypeID
	}

	t := &ContentTemplate{
		CampaignID:   &campaignID,
		EntityTypeID: entityTypeID,
		Name:         name,
		Description:  desc,
		ContentJSON:  contentJSON,
		ContentHTML:  strings.TrimSpace(input.ContentHTML),
		Icon:         icon,
		IsGlobal:     false,
	}

	if err := s.repo.Create(ctx, t); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("creating content template: %w", err))
	}
	return t, nil
}

// GetByID returns a content template by ID.
func (s *contentTemplateService) GetByID(ctx context.Context, id int) (*ContentTemplate, error) {
	return s.repo.FindByID(ctx, id)
}

// ListForCampaign returns all templates available to a campaign.
func (s *contentTemplateService) ListForCampaign(ctx context.Context, campaignID string) ([]ContentTemplate, error) {
	return s.repo.ListForCampaign(ctx, campaignID)
}

// ListForCampaignAndType returns templates matching a specific entity type.
func (s *contentTemplateService) ListForCampaignAndType(ctx context.Context, campaignID string, entityTypeID int) ([]ContentTemplate, error) {
	return s.repo.ListForCampaignAndType(ctx, campaignID, entityTypeID)
}

// Update modifies an existing content template.
func (s *contentTemplateService) Update(ctx context.Context, id int, input UpdateContentTemplateInput) (*ContentTemplate, error) {
	t, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, apperror.NewBadRequest("template name is required")
	}
	if len(name) > 200 {
		return nil, apperror.NewBadRequest("template name must be at most 200 characters")
	}

	contentJSON := strings.TrimSpace(input.ContentJSON)
	if contentJSON == "" {
		return nil, apperror.NewBadRequest("template content is required")
	}
	if !json.Valid([]byte(contentJSON)) {
		return nil, apperror.NewBadRequest("content_json must be valid JSON")
	}
	if len(contentJSON) > maxContentTemplateSize {
		return nil, apperror.NewBadRequest("content_json exceeds maximum size")
	}
	if len(input.ContentHTML) > maxContentTemplateSize {
		return nil, apperror.NewBadRequest("content_html exceeds maximum size")
	}

	t.Name = name
	t.Description = strings.TrimSpace(input.Description)
	t.ContentJSON = contentJSON
	t.ContentHTML = strings.TrimSpace(input.ContentHTML)
	if icon := strings.TrimSpace(input.Icon); icon != "" {
		t.Icon = icon
	}

	if err := s.repo.Update(ctx, t); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("updating content template: %w", err))
	}
	return t, nil
}

// Delete removes a content template.
func (s *contentTemplateService) Delete(ctx context.Context, id int) error {
	if _, err := s.repo.FindByID(ctx, id); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return apperror.NewInternal(fmt.Errorf("deleting content template: %w", err))
	}
	return nil
}

// SeedDefaults creates the default global-style content templates for a new campaign.
// Called during campaign creation to provide starter templates.
func (s *contentTemplateService) SeedDefaults(ctx context.Context, campaignID string) error {
	defaults := defaultContentTemplates(campaignID)
	for i := range defaults {
		if err := s.repo.Create(ctx, &defaults[i]); err != nil {
			return fmt.Errorf("seeding default template %q: %w", defaults[i].Name, err)
		}
	}
	return nil
}

// defaultContentTemplates returns the built-in starter templates for a campaign.
func defaultContentTemplates(campaignID string) []ContentTemplate {
	return []ContentTemplate{
		{
			CampaignID:  &campaignID,
			Name:        "Session Recap",
			Description: "Structured session summary with key events, NPCs, and next steps.",
			Icon:        "fa-scroll",
			SortOrder:   1,
			ContentJSON: sessionRecapJSON,
			ContentHTML: sessionRecapHTML,
		},
		{
			CampaignID:  &campaignID,
			Name:        "NPC Profile",
			Description: "Character profile with appearance, personality, and motivations.",
			Icon:        "fa-user",
			SortOrder:   2,
			ContentJSON: npcProfileJSON,
			ContentHTML: npcProfileHTML,
		},
		{
			CampaignID:  &campaignID,
			Name:        "Location",
			Description: "Place description with notable features, inhabitants, and hooks.",
			Icon:        "fa-location-dot",
			SortOrder:   3,
			ContentJSON: locationJSON,
			ContentHTML: locationHTML,
		},
		{
			CampaignID:  &campaignID,
			Name:        "Quest Log",
			Description: "Quest outline with objectives, rewards, and complications.",
			Icon:        "fa-list-check",
			SortOrder:   4,
			ContentJSON: questLogJSON,
			ContentHTML: questLogHTML,
		},
	}
}

// --- Default template content (TipTap ProseMirror JSON + HTML) ---

var sessionRecapJSON = `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Session Summary"}]},{"type":"paragraph","content":[{"type":"text","text":"Brief overview of what happened this session."}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Key Events"}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Event 1"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Event 2"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Event 3"}]}]}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"NPCs Encountered"}]},{"type":"paragraph","content":[{"type":"text","text":"List notable NPCs the party met or interacted with."}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Loot & Rewards"}]},{"type":"paragraph","content":[{"type":"text","text":"Items, gold, or other rewards gained."}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Next Steps"}]},{"type":"paragraph","content":[{"type":"text","text":"What the party plans to do next session."}]}]}`

var sessionRecapHTML = `<h2>Session Summary</h2><p>Brief overview of what happened this session.</p><h2>Key Events</h2><ul><li><p>Event 1</p></li><li><p>Event 2</p></li><li><p>Event 3</p></li></ul><h2>NPCs Encountered</h2><p>List notable NPCs the party met or interacted with.</p><h2>Loot &amp; Rewards</h2><p>Items, gold, or other rewards gained.</p><h2>Next Steps</h2><p>What the party plans to do next session.</p>`

var npcProfileJSON = `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Appearance"}]},{"type":"paragraph","content":[{"type":"text","text":"Physical description, distinguishing features, typical attire."}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Personality"}]},{"type":"paragraph","content":[{"type":"text","text":"Demeanor, quirks, speech patterns, values."}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Background"}]},{"type":"paragraph","content":[{"type":"text","text":"History, origins, how they came to be where they are."}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Motivations & Goals"}]},{"type":"paragraph","content":[{"type":"text","text":"What drives this character? What do they want?"}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Connections"}]},{"type":"paragraph","content":[{"type":"text","text":"Relationships with other NPCs, factions, or locations."}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"DM Notes"}]},{"type":"paragraph","content":[{"type":"text","text":"Secret information, plot hooks, potential developments."}]}]}`

var npcProfileHTML = `<h2>Appearance</h2><p>Physical description, distinguishing features, typical attire.</p><h2>Personality</h2><p>Demeanor, quirks, speech patterns, values.</p><h2>Background</h2><p>History, origins, how they came to be where they are.</p><h2>Motivations &amp; Goals</h2><p>What drives this character? What do they want?</p><h2>Connections</h2><p>Relationships with other NPCs, factions, or locations.</p><h2>DM Notes</h2><p>Secret information, plot hooks, potential developments.</p>`

var locationJSON = `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Overview"}]},{"type":"paragraph","content":[{"type":"text","text":"General description of this place — what a visitor sees and feels."}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Notable Features"}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Landmark or feature 1"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Landmark or feature 2"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Landmark or feature 3"}]}]}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Inhabitants"}]},{"type":"paragraph","content":[{"type":"text","text":"Who lives here? Key figures, factions, or creatures."}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"History"}]},{"type":"paragraph","content":[{"type":"text","text":"Notable past events that shaped this place."}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Hooks & Rumors"}]},{"type":"paragraph","content":[{"type":"text","text":"Adventure seeds, quests, or rumors connected to this location."}]}]}`

var locationHTML = `<h2>Overview</h2><p>General description of this place — what a visitor sees and feels.</p><h2>Notable Features</h2><ul><li><p>Landmark or feature 1</p></li><li><p>Landmark or feature 2</p></li><li><p>Landmark or feature 3</p></li></ul><h2>Inhabitants</h2><p>Who lives here? Key figures, factions, or creatures.</p><h2>History</h2><p>Notable past events that shaped this place.</p><h2>Hooks &amp; Rumors</h2><p>Adventure seeds, quests, or rumors connected to this location.</p>`

var questLogJSON = `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Objective"}]},{"type":"paragraph","content":[{"type":"text","text":"What needs to be accomplished?"}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Background"}]},{"type":"paragraph","content":[{"type":"text","text":"How was this quest discovered? Who gave it?"}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Key Steps"}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Step 1"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Step 2"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Step 3"}]}]}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Rewards"}]},{"type":"paragraph","content":[{"type":"text","text":"Expected rewards — gold, items, reputation, story progression."}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Complications"}]},{"type":"paragraph","content":[{"type":"text","text":"Potential obstacles, rival factions, time pressure, moral dilemmas."}]},{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Status"}]},{"type":"paragraph","content":[{"type":"text","text":"Current progress and notes."}]}]}`

var questLogHTML = `<h2>Objective</h2><p>What needs to be accomplished?</p><h2>Background</h2><p>How was this quest discovered? Who gave it?</p><h2>Key Steps</h2><ul><li><p>Step 1</p></li><li><p>Step 2</p></li><li><p>Step 3</p></li></ul><h2>Rewards</h2><p>Expected rewards — gold, items, reputation, story progression.</p><h2>Complications</h2><p>Potential obstacles, rival factions, time pressure, moral dilemmas.</p><h2>Status</h2><p>Current progress and notes.</p>`
