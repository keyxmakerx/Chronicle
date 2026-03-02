package timeline

import (
	"context"
	"crypto/rand"
	"fmt"
	"regexp"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// iconPattern validates FontAwesome icon class names to prevent XSS injection.
var iconPattern = regexp.MustCompile(`^fa-[a-z0-9-]+$`)

// colorPattern validates hex color values to prevent XSS injection.
var colorPattern = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// generateID creates a random UUID v4 string.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// CalendarLister fetches calendars for the calendar selector dropdown.
// Implemented as an adapter in app/routes.go to avoid importing the calendar package.
type CalendarLister interface {
	ListCalendars(ctx context.Context, campaignID string) ([]CalendarRef, error)
}

// CalendarRef is a lightweight reference to a calendar used in selector dropdowns.
type CalendarRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TimelineService defines business logic for the timeline plugin.
type TimelineService interface {
	// Timeline CRUD.
	CreateTimeline(ctx context.Context, campaignID string, input CreateTimelineInput) (*Timeline, error)
	GetTimeline(ctx context.Context, timelineID string) (*Timeline, error)
	ListTimelines(ctx context.Context, campaignID string, role int, userID string) ([]Timeline, error)
	UpdateTimeline(ctx context.Context, timelineID string, input UpdateTimelineInput) error
	DeleteTimeline(ctx context.Context, timelineID string) error

	// Event linking.
	LinkEvent(ctx context.Context, timelineID, eventID string, input LinkEventInput) (*EventLink, error)
	UnlinkEvent(ctx context.Context, timelineID, eventID string) error
	ListTimelineEvents(ctx context.Context, timelineID string, role int, userID string) ([]EventLink, error)

	// Entity groups.
	CreateEntityGroup(ctx context.Context, timelineID string, input CreateEntityGroupInput) (*EntityGroup, error)
	UpdateEntityGroup(ctx context.Context, groupID int, input UpdateEntityGroupInput) error
	DeleteEntityGroup(ctx context.Context, groupID int) error
	ListEntityGroups(ctx context.Context, timelineID string) ([]EntityGroup, error)
	AddGroupMember(ctx context.Context, groupID int, entityID string) error
	RemoveGroupMember(ctx context.Context, groupID int, entityID string) error

	// Calendar lookup.
	ListCalendars(ctx context.Context, campaignID string) ([]CalendarRef, error)
}

// timelineService is the default TimelineService implementation.
type timelineService struct {
	repo     TimelineRepository
	calLists CalendarLister
}

// NewTimelineService creates a TimelineService backed by the given repository
// and calendar lister (for the calendar selector dropdown).
func NewTimelineService(repo TimelineRepository, calLists CalendarLister) TimelineService {
	return &timelineService{repo: repo, calLists: calLists}
}

// CreateTimeline creates a new timeline in a campaign.
func (s *timelineService) CreateTimeline(ctx context.Context, campaignID string, input CreateTimelineInput) (*Timeline, error) {
	if input.Name == "" {
		return nil, apperror.NewValidation("timeline name is required")
	}
	if len(input.Name) > 255 {
		return nil, apperror.NewValidation("timeline name must be 255 characters or less")
	}
	if input.CalendarID == "" {
		return nil, apperror.NewValidation("calendar is required")
	}

	// Default values.
	if input.Color == "" {
		input.Color = "#6366f1"
	}
	if input.Icon == "" {
		input.Icon = "fa-timeline"
	}
	if input.Visibility == "" {
		input.Visibility = "everyone"
	}
	if input.ZoomDefault == "" {
		input.ZoomDefault = ZoomYear
	}

	// Validate.
	if input.Visibility != "everyone" && input.Visibility != "dm_only" {
		return nil, apperror.NewValidation("visibility must be 'everyone' or 'dm_only'")
	}
	if !IsValidZoom(input.ZoomDefault) {
		return nil, apperror.NewValidation("invalid zoom default level")
	}
	if !iconPattern.MatchString(input.Icon) {
		return nil, apperror.NewValidation("icon must be a valid FontAwesome class name")
	}
	if !colorPattern.MatchString(input.Color) {
		return nil, apperror.NewValidation("color must be a valid hex color")
	}

	t := &Timeline{
		ID:          generateID(),
		CampaignID:  campaignID,
		CalendarID:  input.CalendarID,
		Name:        input.Name,
		Description: input.Description,
		Color:       input.Color,
		Icon:        input.Icon,
		Visibility:  input.Visibility,
		ZoomDefault: input.ZoomDefault,
		CreatedBy:   &input.CreatedBy,
	}

	if err := s.repo.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("create timeline: %w", err)
	}
	return t, nil
}

// GetTimeline returns a timeline by ID, or a not-found error.
func (s *timelineService) GetTimeline(ctx context.Context, timelineID string) (*Timeline, error) {
	t, err := s.repo.GetByID(ctx, timelineID)
	if err != nil {
		return nil, fmt.Errorf("get timeline: %w", err)
	}
	if t == nil {
		return nil, apperror.NewNotFound("timeline not found")
	}
	return t, nil
}

// ListTimelines returns all timelines for a campaign, filtered by role-based visibility.
// The userID parameter is reserved for future per-user visibility rules.
func (s *timelineService) ListTimelines(ctx context.Context, campaignID string, role int, userID string) ([]Timeline, error) {
	timelines, err := s.repo.List(ctx, campaignID, role)
	if err != nil {
		return nil, fmt.Errorf("list timelines: %w", err)
	}
	return timelines, nil
}

// UpdateTimeline modifies an existing timeline.
func (s *timelineService) UpdateTimeline(ctx context.Context, timelineID string, input UpdateTimelineInput) error {
	t, err := s.repo.GetByID(ctx, timelineID)
	if err != nil {
		return fmt.Errorf("get timeline for update: %w", err)
	}
	if t == nil {
		return apperror.NewNotFound("timeline not found")
	}

	if input.Name == "" {
		return apperror.NewValidation("timeline name is required")
	}
	if len(input.Name) > 255 {
		return apperror.NewValidation("timeline name must be 255 characters or less")
	}
	if input.Visibility != "everyone" && input.Visibility != "dm_only" {
		return apperror.NewValidation("visibility must be 'everyone' or 'dm_only'")
	}
	if !IsValidZoom(input.ZoomDefault) {
		return apperror.NewValidation("invalid zoom default level")
	}
	if input.Icon != "" && !iconPattern.MatchString(input.Icon) {
		return apperror.NewValidation("icon must be a valid FontAwesome class name")
	}
	if input.Color != "" && !colorPattern.MatchString(input.Color) {
		return apperror.NewValidation("color must be a valid hex color")
	}

	t.Name = input.Name
	t.Description = input.Description
	t.DescriptionHTML = input.DescriptionHTML
	t.Color = input.Color
	t.Icon = input.Icon
	t.Visibility = input.Visibility
	t.VisibilityRules = input.VisibilityRules
	t.ZoomDefault = input.ZoomDefault

	if err := s.repo.Update(ctx, t); err != nil {
		return fmt.Errorf("update timeline: %w", err)
	}
	return nil
}

// DeleteTimeline removes a timeline and all associated data.
func (s *timelineService) DeleteTimeline(ctx context.Context, timelineID string) error {
	t, err := s.repo.GetByID(ctx, timelineID)
	if err != nil {
		return fmt.Errorf("get timeline for delete: %w", err)
	}
	if t == nil {
		return apperror.NewNotFound("timeline not found")
	}
	if err := s.repo.Delete(ctx, timelineID); err != nil {
		return fmt.Errorf("delete timeline: %w", err)
	}
	return nil
}

// LinkEvent links a calendar event to a timeline.
func (s *timelineService) LinkEvent(ctx context.Context, timelineID, eventID string, input LinkEventInput) (*EventLink, error) {
	// Verify timeline exists.
	t, err := s.repo.GetByID(ctx, timelineID)
	if err != nil {
		return nil, fmt.Errorf("get timeline for link: %w", err)
	}
	if t == nil {
		return nil, apperror.NewNotFound("timeline not found")
	}

	// Determine display order (append to end).
	count, err := s.repo.CountEvents(ctx, timelineID)
	if err != nil {
		return nil, fmt.Errorf("count events: %w", err)
	}

	link := &EventLink{
		TimelineID:   timelineID,
		EventID:      eventID,
		DisplayOrder: count,
		Label:        input.Label,
		ColorOverride: input.ColorOverride,
	}

	if err := s.repo.LinkEvent(ctx, link); err != nil {
		return nil, fmt.Errorf("link event: %w", err)
	}
	return link, nil
}

// UnlinkEvent removes a calendar event from a timeline.
func (s *timelineService) UnlinkEvent(ctx context.Context, timelineID, eventID string) error {
	if err := s.repo.UnlinkEvent(ctx, timelineID, eventID); err != nil {
		return fmt.Errorf("unlink event: %w", err)
	}
	return nil
}

// ListTimelineEvents returns all linked events for a timeline, filtered by visibility.
// The userID parameter is reserved for future per-user visibility rules.
func (s *timelineService) ListTimelineEvents(ctx context.Context, timelineID string, role int, userID string) ([]EventLink, error) {
	events, err := s.repo.ListEventLinks(ctx, timelineID, role)
	if err != nil {
		return nil, fmt.Errorf("list timeline events: %w", err)
	}
	return events, nil
}

// CreateEntityGroup creates a new entity group for swim-lane organization.
func (s *timelineService) CreateEntityGroup(ctx context.Context, timelineID string, input CreateEntityGroupInput) (*EntityGroup, error) {
	if input.Name == "" {
		return nil, apperror.NewValidation("group name is required")
	}
	if input.Color == "" {
		input.Color = "#6b7280"
	}
	if !colorPattern.MatchString(input.Color) {
		return nil, apperror.NewValidation("color must be a valid hex color")
	}

	g := &EntityGroup{
		TimelineID: timelineID,
		Name:       input.Name,
		Color:      input.Color,
	}

	if err := s.repo.CreateEntityGroup(ctx, g); err != nil {
		return nil, fmt.Errorf("create entity group: %w", err)
	}
	return g, nil
}

// UpdateEntityGroup modifies an existing entity group.
func (s *timelineService) UpdateEntityGroup(ctx context.Context, groupID int, input UpdateEntityGroupInput) error {
	if input.Name == "" {
		return apperror.NewValidation("group name is required")
	}
	if input.Color != "" && !colorPattern.MatchString(input.Color) {
		return apperror.NewValidation("color must be a valid hex color")
	}

	g := &EntityGroup{
		ID:    groupID,
		Name:  input.Name,
		Color: input.Color,
	}

	if err := s.repo.UpdateEntityGroup(ctx, g); err != nil {
		return fmt.Errorf("update entity group: %w", err)
	}
	return nil
}

// DeleteEntityGroup removes an entity group and its members.
func (s *timelineService) DeleteEntityGroup(ctx context.Context, groupID int) error {
	if err := s.repo.DeleteEntityGroup(ctx, groupID); err != nil {
		return fmt.Errorf("delete entity group: %w", err)
	}
	return nil
}

// ListEntityGroups returns all entity groups for a timeline with members.
func (s *timelineService) ListEntityGroups(ctx context.Context, timelineID string) ([]EntityGroup, error) {
	groups, err := s.repo.ListEntityGroups(ctx, timelineID)
	if err != nil {
		return nil, fmt.Errorf("list entity groups: %w", err)
	}
	return groups, nil
}

// AddGroupMember adds an entity to an entity group.
func (s *timelineService) AddGroupMember(ctx context.Context, groupID int, entityID string) error {
	if err := s.repo.AddGroupMember(ctx, groupID, entityID); err != nil {
		return fmt.Errorf("add group member: %w", err)
	}
	return nil
}

// RemoveGroupMember removes an entity from an entity group.
func (s *timelineService) RemoveGroupMember(ctx context.Context, groupID int, entityID string) error {
	if err := s.repo.RemoveGroupMember(ctx, groupID, entityID); err != nil {
		return fmt.Errorf("remove group member: %w", err)
	}
	return nil
}

// ListCalendars returns available calendars for the calendar selector dropdown.
func (s *timelineService) ListCalendars(ctx context.Context, campaignID string) ([]CalendarRef, error) {
	if s.calLists == nil {
		return nil, nil
	}
	return s.calLists.ListCalendars(ctx, campaignID)
}
