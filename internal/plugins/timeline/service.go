package timeline

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
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

// CalendarEventLister fetches calendar events for the event picker.
// Implemented as an adapter in app/routes.go to avoid importing the calendar package.
type CalendarEventLister interface {
	ListEventsForCalendar(ctx context.Context, calendarID string, role int) ([]CalendarEventRef, error)
}

// CalendarEventRef is a lightweight reference to a calendar event used in the
// event picker when linking events to a timeline.
type CalendarEventRef struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Year        int     `json:"year"`
	Month       int     `json:"month"`
	Day         int     `json:"day"`
	Category    *string `json:"category,omitempty"`
	Visibility  string  `json:"visibility"`
	EntityID    *string `json:"entity_id,omitempty"`
	EntityName  string  `json:"entity_name,omitempty"`
	EntityIcon  string  `json:"entity_icon,omitempty"`
}

// TimelineService defines business logic for the timeline plugin.
type TimelineService interface {
	// Timeline CRUD.
	CreateTimeline(ctx context.Context, campaignID string, input CreateTimelineInput) (*Timeline, error)
	GetTimeline(ctx context.Context, timelineID string) (*Timeline, error)
	ListTimelines(ctx context.Context, campaignID string, role int, userID string) ([]Timeline, error)
	UpdateTimeline(ctx context.Context, timelineID string, input UpdateTimelineInput) error
	DeleteTimeline(ctx context.Context, timelineID string) error

	// Event linking (calendar events).
	LinkEvent(ctx context.Context, timelineID, eventID string, input LinkEventInput) (*EventLink, error)
	LinkAllEvents(ctx context.Context, timelineID string, role int) (int, error)
	UnlinkEvent(ctx context.Context, timelineID, eventID string) error
	ListTimelineEvents(ctx context.Context, timelineID string, role int, userID string) ([]EventLink, error)
	ListAvailableEvents(ctx context.Context, timelineID string, role int) ([]CalendarEventRef, error)

	// Event link visibility.
	UpdateEventLinkVisibility(ctx context.Context, timelineID, eventID string, input UpdateEventVisibilityInput) error

	// Standalone events.
	CreateStandaloneEvent(ctx context.Context, timelineID string, input CreateTimelineEventInput) (*TimelineEvent, error)
	GetStandaloneEvent(ctx context.Context, eventID string) (*TimelineEvent, error)
	UpdateStandaloneEvent(ctx context.Context, timelineID, eventID string, input UpdateTimelineEventInput) error
	DeleteStandaloneEvent(ctx context.Context, timelineID, eventID string) error

	// Entity groups.
	CreateEntityGroup(ctx context.Context, timelineID string, input CreateEntityGroupInput) (*EntityGroup, error)
	UpdateEntityGroup(ctx context.Context, timelineID string, groupID int, input UpdateEntityGroupInput) error
	DeleteEntityGroup(ctx context.Context, timelineID string, groupID int) error
	ListEntityGroups(ctx context.Context, timelineID string) ([]EntityGroup, error)
	AddGroupMember(ctx context.Context, timelineID string, groupID int, entityID string) error
	RemoveGroupMember(ctx context.Context, timelineID string, groupID int, entityID string) error

	// Search.
	SearchTimelines(ctx context.Context, campaignID, query string, role int) ([]map[string]string, error)

	// Calendar lookup.
	ListCalendars(ctx context.Context, campaignID string) ([]CalendarRef, error)
}

// timelineService is the default TimelineService implementation.
type timelineService struct {
	repo      TimelineRepository
	calLists  CalendarLister
	calEvents CalendarEventLister
}

// NewTimelineService creates a TimelineService backed by the given repository,
// calendar lister (for the selector dropdown), and event lister (for the event picker).
func NewTimelineService(repo TimelineRepository, calLists CalendarLister, calEvents CalendarEventLister) TimelineService {
	return &timelineService{repo: repo, calLists: calLists, calEvents: calEvents}
}

// CreateTimeline creates a new timeline in a campaign.
func (s *timelineService) CreateTimeline(ctx context.Context, campaignID string, input CreateTimelineInput) (*Timeline, error) {
	if input.Name == "" {
		return nil, apperror.NewValidation("timeline name is required")
	}
	if len(input.Name) > 255 {
		return nil, apperror.NewValidation("timeline name must be 255 characters or less")
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

// ListTimelines returns all timelines for a campaign, filtered by role-based
// visibility and per-user visibility rules.
func (s *timelineService) ListTimelines(ctx context.Context, campaignID string, role int, userID string) ([]Timeline, error) {
	timelines, err := s.repo.List(ctx, campaignID, role)
	if err != nil {
		return nil, fmt.Errorf("list timelines: %w", err)
	}

	// Apply per-user visibility rules (Owners always see everything).
	if role < 3 && userID != "" {
		filtered := timelines[:0]
		for _, t := range timelines {
			if canUserView(t.Visibility, t.VisibilityRules, role, userID) {
				filtered = append(filtered, t)
			}
		}
		timelines = filtered
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
	if err := validateVisibilityRules(input.VisibilityRules); err != nil {
		return err
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

// LinkEvent links a calendar event to a timeline. Requires the timeline
// to have a calendar (cannot link calendar events to calendar-free timelines).
func (s *timelineService) LinkEvent(ctx context.Context, timelineID, eventID string, input LinkEventInput) (*EventLink, error) {
	// Verify timeline exists.
	t, err := s.repo.GetByID(ctx, timelineID)
	if err != nil {
		return nil, fmt.Errorf("get timeline for link: %w", err)
	}
	if t == nil {
		return nil, apperror.NewNotFound("timeline not found")
	}
	if !t.HasCalendar() {
		return nil, apperror.NewValidation("cannot link calendar events to a timeline without a calendar")
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

// ListTimelineEvents returns all events for a timeline — both linked calendar
// events and standalone events — merged into a unified EventLink slice, sorted
// by date, and filtered by role-based and per-user visibility rules.
func (s *timelineService) ListTimelineEvents(ctx context.Context, timelineID string, role int, userID string) ([]EventLink, error) {
	// Fetch linked calendar events.
	events, err := s.repo.ListEventLinks(ctx, timelineID, role)
	if err != nil {
		return nil, fmt.Errorf("list timeline events: %w", err)
	}

	// Tag calendar events with their source.
	for i := range events {
		events[i].Source = "calendar"
	}

	// Fetch and merge standalone events.
	standalone, err := s.repo.ListStandaloneEvents(ctx, timelineID, role)
	if err != nil {
		return nil, fmt.Errorf("list standalone events: %w", err)
	}
	for _, se := range standalone {
		events = append(events, se.ToEventLink())
	}

	// Sort merged events by date then display order.
	sortEventLinks(events)

	// Apply per-user event link visibility rules (Owners always see everything).
	if role < 3 && userID != "" {
		filtered := events[:0]
		for _, el := range events {
			vis := el.EffectiveVisibility()
			if canUserView(vis, el.VisibilityRules, role, userID) {
				filtered = append(filtered, el)
			}
		}
		events = filtered
	}
	return events, nil
}

// sortEventLinks sorts events by year, month, day, then display order.
func sortEventLinks(events []EventLink) {
	for i := 1; i < len(events); i++ {
		for j := i; j > 0; j-- {
			a, b := events[j], events[j-1]
			if a.EventYear < b.EventYear ||
				(a.EventYear == b.EventYear && a.EventMonth < b.EventMonth) ||
				(a.EventYear == b.EventYear && a.EventMonth == b.EventMonth && a.EventDay < b.EventDay) ||
				(a.EventYear == b.EventYear && a.EventMonth == b.EventMonth && a.EventDay == b.EventDay && a.DisplayOrder < b.DisplayOrder) {
				events[j], events[j-1] = events[j-1], events[j]
			} else {
				break
			}
		}
	}
}

// ListAvailableEvents returns calendar events that can be linked to a timeline.
// Filters out events already linked, returning only unlinked events.
func (s *timelineService) ListAvailableEvents(ctx context.Context, timelineID string, role int) ([]CalendarEventRef, error) {
	t, err := s.repo.GetByID(ctx, timelineID)
	if err != nil {
		return nil, fmt.Errorf("get timeline for available events: %w", err)
	}
	if t == nil {
		return nil, apperror.NewNotFound("timeline not found")
	}

	if s.calEvents == nil || !t.HasCalendar() {
		return nil, nil
	}

	// Get all calendar events.
	allEvents, err := s.calEvents.ListEventsForCalendar(ctx, *t.CalendarID, role)
	if err != nil {
		return nil, fmt.Errorf("list calendar events: %w", err)
	}

	// Get already-linked event IDs.
	linked, err := s.repo.ListEventLinks(ctx, timelineID, role)
	if err != nil {
		return nil, fmt.Errorf("list linked events: %w", err)
	}
	linkedSet := make(map[string]bool, len(linked))
	for _, el := range linked {
		linkedSet[el.EventID] = true
	}

	// Filter to unlinked only.
	var available []CalendarEventRef
	for _, ev := range allEvents {
		if !linkedSet[ev.ID] {
			available = append(available, ev)
		}
	}
	return available, nil
}

// LinkAllEvents links all calendar events to a timeline that aren't already linked.
// Returns the number of newly linked events.
func (s *timelineService) LinkAllEvents(ctx context.Context, timelineID string, role int) (int, error) {
	available, err := s.ListAvailableEvents(ctx, timelineID, role)
	if err != nil {
		return 0, err
	}

	count, err := s.repo.CountEvents(ctx, timelineID)
	if err != nil {
		return 0, fmt.Errorf("count events: %w", err)
	}

	linked := 0
	for i, ev := range available {
		link := &EventLink{
			TimelineID:   timelineID,
			EventID:      ev.ID,
			DisplayOrder: count + i,
		}
		if err := s.repo.LinkEvent(ctx, link); err != nil {
			return linked, fmt.Errorf("link event %s: %w", ev.ID, err)
		}
		linked++
	}
	return linked, nil
}

// --- Standalone Event CRUD ---

// CreateStandaloneEvent creates a new standalone event directly on a timeline.
func (s *timelineService) CreateStandaloneEvent(ctx context.Context, timelineID string, input CreateTimelineEventInput) (*TimelineEvent, error) {
	// Verify timeline exists.
	t, err := s.repo.GetByID(ctx, timelineID)
	if err != nil {
		return nil, fmt.Errorf("get timeline for create event: %w", err)
	}
	if t == nil {
		return nil, apperror.NewNotFound("timeline not found")
	}

	// Validate required fields.
	if input.Name == "" {
		return nil, apperror.NewValidation("event name is required")
	}
	if len(input.Name) > 255 {
		return nil, apperror.NewValidation("event name must be 255 characters or less")
	}

	// Defaults.
	if input.Visibility == "" {
		input.Visibility = "everyone"
	}
	if input.Visibility != "everyone" && input.Visibility != "dm_only" {
		return nil, apperror.NewValidation("visibility must be 'everyone' or 'dm_only'")
	}
	if input.Color != nil && *input.Color != "" && !colorPattern.MatchString(*input.Color) {
		return nil, apperror.NewValidation("color must be a valid hex color")
	}

	// Sanitize HTML if provided (rich text descriptions from TipTap editor).
	var descHTML *string
	if input.DescriptionHTML != nil && *input.DescriptionHTML != "" {
		sanitized := sanitize.HTML(*input.DescriptionHTML)
		descHTML = &sanitized
	}

	// Determine display order (append to end).
	count, err := s.repo.CountStandaloneEvents(ctx, timelineID)
	if err != nil {
		return nil, fmt.Errorf("count standalone events: %w", err)
	}

	e := &TimelineEvent{
		ID:              generateID(),
		TimelineID:      timelineID,
		EntityID:        input.EntityID,
		Name:            input.Name,
		Description:     input.Description,
		DescriptionHTML: descHTML,
		Year:            input.Year,
		Month:           input.Month,
		Day:             input.Day,
		StartHour:       input.StartHour,
		StartMinute:     input.StartMinute,
		EndYear:         input.EndYear,
		EndMonth:        input.EndMonth,
		EndDay:          input.EndDay,
		EndHour:         input.EndHour,
		EndMinute:       input.EndMinute,
		IsRecurring:     input.IsRecurring,
		RecurrenceType:  input.RecurrenceType,
		Category:        input.Category,
		Visibility:      input.Visibility,
		DisplayOrder:    count,
		Label:           input.Label,
		Color:           input.Color,
		CreatedBy:       &input.CreatedBy,
	}

	if err := s.repo.CreateEvent(ctx, e); err != nil {
		return nil, fmt.Errorf("create standalone event: %w", err)
	}
	return e, nil
}

// GetStandaloneEvent returns a standalone event by ID.
func (s *timelineService) GetStandaloneEvent(ctx context.Context, eventID string) (*TimelineEvent, error) {
	e, err := s.repo.GetEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("get standalone event: %w", err)
	}
	if e == nil {
		return nil, apperror.NewNotFound("event not found")
	}
	return e, nil
}

// UpdateStandaloneEvent modifies an existing standalone event.
// timelineID is checked against the event's owner to prevent IDOR attacks.
func (s *timelineService) UpdateStandaloneEvent(ctx context.Context, timelineID, eventID string, input UpdateTimelineEventInput) error {
	e, err := s.repo.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("get event for update: %w", err)
	}
	if e == nil || e.TimelineID != timelineID {
		return apperror.NewNotFound("event not found")
	}

	if input.Name == "" {
		return apperror.NewValidation("event name is required")
	}
	if len(input.Name) > 255 {
		return apperror.NewValidation("event name must be 255 characters or less")
	}
	if input.Visibility != "everyone" && input.Visibility != "dm_only" {
		return apperror.NewValidation("visibility must be 'everyone' or 'dm_only'")
	}
	if input.Color != nil && *input.Color != "" && !colorPattern.MatchString(*input.Color) {
		return apperror.NewValidation("color must be a valid hex color")
	}

	// Sanitize HTML if provided (rich text descriptions from TipTap editor).
	var descHTML *string
	if input.DescriptionHTML != nil && *input.DescriptionHTML != "" {
		sanitized := sanitize.HTML(*input.DescriptionHTML)
		descHTML = &sanitized
	}

	e.EntityID = input.EntityID
	e.Name = input.Name
	e.Description = input.Description
	e.DescriptionHTML = descHTML
	e.Year = input.Year
	e.Month = input.Month
	e.Day = input.Day
	e.StartHour = input.StartHour
	e.StartMinute = input.StartMinute
	e.EndYear = input.EndYear
	e.EndMonth = input.EndMonth
	e.EndDay = input.EndDay
	e.EndHour = input.EndHour
	e.EndMinute = input.EndMinute
	e.IsRecurring = input.IsRecurring
	e.RecurrenceType = input.RecurrenceType
	e.Category = input.Category
	e.Visibility = input.Visibility
	e.VisibilityRules = input.VisibilityRules
	e.Label = input.Label
	e.Color = input.Color

	if err := s.repo.UpdateEvent(ctx, e); err != nil {
		return fmt.Errorf("update standalone event: %w", err)
	}
	return nil
}

// DeleteStandaloneEvent removes a standalone event from a timeline.
// timelineID is checked against the event's owner to prevent IDOR attacks.
func (s *timelineService) DeleteStandaloneEvent(ctx context.Context, timelineID, eventID string) error {
	e, err := s.repo.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("get event for delete: %w", err)
	}
	if e == nil || e.TimelineID != timelineID {
		return apperror.NewNotFound("event not found")
	}
	if err := s.repo.DeleteEvent(ctx, eventID); err != nil {
		return fmt.Errorf("delete standalone event: %w", err)
	}
	return nil
}

// CreateEntityGroup creates a new entity group for swim-lane organization.
func (s *timelineService) CreateEntityGroup(ctx context.Context, timelineID string, input CreateEntityGroupInput) (*EntityGroup, error) {
	if input.Name == "" {
		return nil, apperror.NewValidation("group name is required")
	}
	if len(input.Name) > 200 {
		return nil, apperror.NewValidation("group name must be 200 characters or less")
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
// timelineID scoping prevents cross-timeline IDOR.
func (s *timelineService) UpdateEntityGroup(ctx context.Context, timelineID string, groupID int, input UpdateEntityGroupInput) error {
	if input.Name == "" {
		return apperror.NewValidation("group name is required")
	}
	if len(input.Name) > 200 {
		return apperror.NewValidation("group name must be 200 characters or less")
	}
	if input.Color != "" && !colorPattern.MatchString(input.Color) {
		return apperror.NewValidation("color must be a valid hex color")
	}

	g := &EntityGroup{
		ID:         groupID,
		TimelineID: timelineID,
		Name:       input.Name,
		Color:      input.Color,
	}

	if err := s.repo.UpdateEntityGroup(ctx, g); err != nil {
		return fmt.Errorf("update entity group: %w", err)
	}
	return nil
}

// DeleteEntityGroup removes an entity group and its members.
// timelineID scoping prevents cross-timeline IDOR.
func (s *timelineService) DeleteEntityGroup(ctx context.Context, timelineID string, groupID int) error {
	if err := s.repo.DeleteEntityGroup(ctx, groupID, timelineID); err != nil {
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
// timelineID scoping prevents cross-timeline IDOR.
func (s *timelineService) AddGroupMember(ctx context.Context, timelineID string, groupID int, entityID string) error {
	if err := s.repo.AddGroupMember(ctx, groupID, timelineID, entityID); err != nil {
		return fmt.Errorf("add group member: %w", err)
	}
	return nil
}

// RemoveGroupMember removes an entity from an entity group.
// timelineID scoping prevents cross-timeline IDOR.
func (s *timelineService) RemoveGroupMember(ctx context.Context, timelineID string, groupID int, entityID string) error {
	if err := s.repo.RemoveGroupMember(ctx, groupID, timelineID, entityID); err != nil {
		return fmt.Errorf("remove group member: %w", err)
	}
	return nil
}

// SearchTimelines returns timelines matching a query as map results for the @mention system.
// Results are formatted to match the entity search JSON format used by editor_mention.js.
func (s *timelineService) SearchTimelines(ctx context.Context, campaignID, query string, role int) ([]map[string]string, error) {
	timelines, err := s.repo.Search(ctx, campaignID, query, role)
	if err != nil {
		return nil, fmt.Errorf("search timelines: %w", err)
	}

	results := make([]map[string]string, 0, len(timelines))
	for _, t := range timelines {
		results = append(results, map[string]string{
			"id":         t.ID,
			"name":       t.Name,
			"type_name":  "Timeline",
			"type_icon":  t.Icon,
			"type_color": t.Color,
			"url":        fmt.Sprintf("/campaigns/%s/timelines/%s", campaignID, t.ID),
		})
	}
	return results, nil
}

// UpdateEventLinkVisibility updates the visibility override and rules for an event link.
func (s *timelineService) UpdateEventLinkVisibility(ctx context.Context, timelineID, eventID string, input UpdateEventVisibilityInput) error {
	if input.VisibilityOverride != nil && *input.VisibilityOverride != "" {
		v := *input.VisibilityOverride
		if v != "everyone" && v != "dm_only" {
			return apperror.NewValidation("visibility_override must be 'everyone', 'dm_only', or empty")
		}
	}
	if err := validateVisibilityRules(input.VisibilityRules); err != nil {
		return err
	}
	return s.repo.UpdateEventLinkVisibility(ctx, timelineID, eventID, input.VisibilityOverride, input.VisibilityRules)
}

// ListCalendars returns available calendars for the calendar selector dropdown.
func (s *timelineService) ListCalendars(ctx context.Context, campaignID string) ([]CalendarRef, error) {
	if s.calLists == nil {
		return nil, nil
	}
	return s.calLists.ListCalendars(ctx, campaignID)
}

// --- Visibility Helpers ---

// canUserView checks whether a user can see an item based on its base visibility
// and per-user JSON rules. Owners (role >= 3) always see everything and should
// be checked before calling this function.
func canUserView(baseVisibility string, visRulesJSON *string, role int, userID string) bool {
	// Base visibility: dm_only requires role >= 3 (Owner).
	if baseVisibility == "dm_only" && role < 3 {
		return false
	}

	// Parse per-user JSON rules if present.
	if visRulesJSON == nil || *visRulesJSON == "" {
		return true
	}
	var rules VisibilityRules
	if err := json.Unmarshal([]byte(*visRulesJSON), &rules); err != nil {
		slog.Warn("unparseable visibility_rules JSON, failing open", slog.Any("error", err))
		return true // Fail open for existing items — validated on write path.
	}

	// AllowedUsers whitelist takes precedence.
	if len(rules.AllowedUsers) > 0 {
		for _, uid := range rules.AllowedUsers {
			if uid == userID {
				return true
			}
		}
		return false
	}

	// DeniedUsers blacklist.
	if len(rules.DeniedUsers) > 0 {
		for _, uid := range rules.DeniedUsers {
			if uid == userID {
				return false
			}
		}
	}

	return true
}

// validateVisibilityRules checks that a visibility_rules JSON string is
// well-formed if present. Returns a validation error on bad JSON.
func validateVisibilityRules(rulesJSON *string) error {
	if rulesJSON == nil || *rulesJSON == "" {
		return nil
	}
	var rules VisibilityRules
	if err := json.Unmarshal([]byte(*rulesJSON), &rules); err != nil {
		return apperror.NewValidation("visibility_rules must be valid JSON: " + err.Error())
	}
	return nil
}
