package timeline

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
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

// CalendarEraLister fetches calendar eras for the D3 visualization background bands.
// Implemented as an adapter in app/routes.go to avoid importing the calendar package.
type CalendarEraLister interface {
	ListEras(ctx context.Context, calendarID string) ([]CalendarEra, error)
}

// CalendarEra is a lightweight reference to a calendar era for D3 visualization.
type CalendarEra struct {
	Name      string `json:"name"`
	StartYear int    `json:"start_year"`
	EndYear   *int   `json:"end_year,omitempty"`
	Color     string `json:"color"`
}

// CalendarEventRef is a lightweight reference to a calendar event used in the
// event picker when linking events to a timeline.
type CalendarEventRef struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Year       int     `json:"year"`
	Month      int     `json:"month"`
	Day        int     `json:"day"`
	Category   *string `json:"category,omitempty"`
	Visibility string  `json:"visibility"`
	EntityID   *string `json:"entity_id,omitempty"`
	EntityName string  `json:"entity_name,omitempty"`
	EntityIcon string  `json:"entity_icon,omitempty"`
}

// TimelineService defines business logic for the timeline plugin.
type TimelineService interface {
	// Timeline CRUD.
	CreateTimeline(ctx context.Context, campaignID string, input CreateTimelineInput) (*Timeline, error)
	GetTimeline(ctx context.Context, timelineID string) (*Timeline, error)
	// GetTimelineForViewer is GetTimeline's viewer-aware sibling: it returns
	// NotFound (never the timeline) unless the timeline both belongs to
	// campaignID (the cross-campaign IDOR guard) and is visible to v under
	// the same role + per-user rules ListTimelines applies
	// (timelineVisibleToViewer, ADR-058). Use this, not GetTimeline, on any
	// route a viewer weaker than Owner can reach.
	GetTimelineForViewer(ctx context.Context, timelineID, campaignID string, v permissions.Viewer) (*Timeline, error)
	// ListTimelines / ListTimelinesForCalendar / ListTimelineEvents take a
	// permissions.Viewer rather than (role, userID), so an anonymous visitor
	// (empty user id) cannot match the system-trusted bypass (ADR-049). A
	// caller that really is trusted says so with permissions.SystemViewer.
	ListTimelines(ctx context.Context, campaignID string, v permissions.Viewer) ([]Timeline, error)
	ListTimelinesForCalendar(ctx context.Context, calendarID string, v permissions.Viewer) ([]Timeline, error)
	UpdateTimeline(ctx context.Context, timelineID string, input UpdateTimelineInput) error
	DeleteTimeline(ctx context.Context, timelineID string) error

	// Event linking (calendar events).
	LinkEvent(ctx context.Context, timelineID, eventID string, input LinkEventInput) (*EventLink, error)
	LinkAllEvents(ctx context.Context, timelineID string, role int) (int, error)
	UnlinkEvent(ctx context.Context, timelineID, eventID string) error
	// ListTimelineEvents also takes campaignID, so a non-owner's result can
	// be narrowed by EntityVisibilityGate (a linked entity's visibility is a
	// separate check from the event's own).
	ListTimelineEvents(ctx context.Context, timelineID, campaignID string, v permissions.Viewer) ([]EventLink, error)
	ListAvailableEvents(ctx context.Context, timelineID string, role int) ([]CalendarEventRef, error)

	// Event link visibility.
	UpdateEventLinkVisibility(ctx context.Context, timelineID, eventID string, input UpdateEventVisibilityInput) error

	// Standalone events. UpdateStandaloneEvent and DeleteStandaloneEvent take
	// canAuthorDmOnly (Owner or a co-DM grant,
	// campaigns.CampaignContext.CanAuthorDmOnly) so a caller who cannot
	// author dm_only content gets the same NotFound a missing id would give
	// when the stored event is dm_only, regardless of which fields the
	// request touches.
	CreateStandaloneEvent(ctx context.Context, timelineID string, input CreateTimelineEventInput) (*TimelineEvent, error)
	GetStandaloneEvent(ctx context.Context, eventID string) (*TimelineEvent, error)
	UpdateStandaloneEvent(ctx context.Context, timelineID, eventID string, input UpdateTimelineEventInput, canAuthorDmOnly bool) error
	DeleteStandaloneEvent(ctx context.Context, timelineID, eventID string, canAuthorDmOnly bool) error

	// Entity groups.
	CreateEntityGroup(ctx context.Context, timelineID string, input CreateEntityGroupInput) (*EntityGroup, error)
	UpdateEntityGroup(ctx context.Context, timelineID string, groupID int, input UpdateEntityGroupInput) error
	DeleteEntityGroup(ctx context.Context, timelineID string, groupID int) error
	// ListEntityGroups takes campaignID and a Viewer so a non-owner's result
	// can be narrowed by EntityVisibilityGate, same as ListTimelineEvents:
	// a group member's entity link is a separate visibility check the caller
	// must supply.
	ListEntityGroups(ctx context.Context, timelineID, campaignID string, v permissions.Viewer) ([]EntityGroup, error)
	AddGroupMember(ctx context.Context, timelineID string, groupID int, entityID string) error
	RemoveGroupMember(ctx context.Context, timelineID string, groupID int, entityID string) error

	// Event connections.
	CreateConnection(ctx context.Context, timelineID string, input CreateConnectionInput) (*EventConnection, error)
	DeleteConnection(ctx context.Context, timelineID string, connectionID int) error
	ListConnections(ctx context.Context, timelineID string) ([]EventConnection, error)

	// Search. Takes userID alongside role so the per-user visibility layer
	// can build the same permissions.Viewer ListTimelines builds — role
	// alone is not enough to keep a restricted timeline's name from leaking.
	SearchTimelines(ctx context.Context, campaignID, query string, role int, userID string) ([]map[string]string, error)

	// Calendar lookup.
	ListCalendars(ctx context.Context, campaignID string) ([]CalendarRef, error)
	ListCalendarEras(ctx context.Context, calendarID string) ([]CalendarEra, error)
}

// EntityVisibilityGate resolves which of a set of entity IDs a viewer (role +
// userID) may see, applying the entities plugin's own canonical visibility
// policy (default is_private, custom per-subject grants, tag grants). Wraps
// entities.EntityService.FilterViewableEntityIDs — the same seam maps,
// media, npcs and sessions use — so a timeline event or entity-group member
// naming a dm_only/private entity never leaks that entity's name/icon to a
// viewer who could not otherwise see it.
type EntityVisibilityGate interface {
	FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error)
}

// timelineService is the default TimelineService implementation.
type timelineService struct {
	repo           TimelineRepository
	calLists       CalendarLister
	calEvents      CalendarEventLister
	calEras        CalendarEraLister
	bindingCleaner BindingCleaner
	entityGate     EntityVisibilityGate
}

// BindingCleaner sweeps a deleted instance's widget bindings. Implemented by
// widgetbindings.Service; injected via SetBindingCleaner. Optional — nil
// means no binding framework wired (the render-time guard + Sweep backstop it).
type BindingCleaner interface {
	OnInstanceDeleted(ctx context.Context, campaignID, widgetType, instanceID string) (int, error)
}

// NewTimelineService creates a TimelineService backed by the given repository,
// calendar lister (for the selector dropdown), event lister (for the event picker),
// and era lister (for visualization background bands).
func NewTimelineService(repo TimelineRepository, calLists CalendarLister, calEvents CalendarEventLister, calEras CalendarEraLister) TimelineService {
	return &timelineService{repo: repo, calLists: calLists, calEvents: calEvents, calEras: calEras}
}

// SetBindingCleaner injects the widget-binding cleanup hook (wired at app
// startup). Reached via a type assertion in routes.go so the TimelineService
// interface stays unchanged.
func (s *timelineService) SetBindingCleaner(c BindingCleaner) { s.bindingCleaner = c }

// SetEntityVisibilityGate injects the entity-visibility check used by
// ListTimelineEvents and ListEntityGroups (wired post-construction, like
// SetBindingCleaner). Nil is a valid — if unwired — value: both list methods
// fail closed and blank every entity-linked row's id/name/icon rather than
// risk showing one nothing verified as viewable.
func (s *timelineService) SetEntityVisibilityGate(g EntityVisibilityGate) { s.entityGate = g }

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

// GetTimelineForViewer returns a timeline only if it belongs to campaignID
// and v may see it. Both failure modes — wrong campaign, and right campaign
// but hidden from this viewer — return the same NotFound: a Forbidden would
// let an anonymous prober confirm a dm_only timeline exists at this id
// merely by asking (ADR-055 rule 3, the existence-oracle problem).
//
// This applies the same visibility predicate (timelineVisibleToViewer) that
// ListTimelines applies via filterTimelinesByUser, rather than re-deriving
// it here. Use this, not GetTimeline, on any route reachable by a viewer
// weaker than Owner.
func (s *timelineService) GetTimelineForViewer(ctx context.Context, timelineID, campaignID string, v permissions.Viewer) (*Timeline, error) {
	t, err := s.repo.GetByID(ctx, timelineID)
	if err != nil {
		return nil, fmt.Errorf("get timeline: %w", err)
	}
	if t == nil || t.GetCampaignID() != campaignID || !timelineVisibleToViewer(*t, v) {
		return nil, apperror.NewNotFound("timeline not found")
	}
	return t, nil
}

// ListTimelines returns all timelines for a campaign, filtered by role-based
// visibility and per-user visibility rules.
func (s *timelineService) ListTimelines(ctx context.Context, campaignID string, v permissions.Viewer) ([]Timeline, error) {
	timelines, err := s.repo.List(ctx, campaignID, v.Role())
	if err != nil {
		return nil, fmt.Errorf("list timelines: %w", err)
	}
	timelines = filterTimelinesByUser(timelines, v)
	if err := s.recountEventsForViewer(ctx, timelines, v); err != nil {
		return nil, err
	}
	return timelines, nil
}

// recountEventsForViewer overwrites each timeline's SQL-computed EventCount
// with the number of events this viewer can actually open, reusing the same
// per-event filter ListTimelineEvents applies to its rows (filterEventLinksByUser)
// so the two can't disagree (ADR-055 rule 3: a count is content too). The SQL
// count only ever applied the dm_only predicate; per-user visibility_rules
// are Go-side, so without this step a viewer excluded from an event only by
// rules saw a count one higher than their event list — revealing that a
// hidden event exists.
//
// Owners/co-DMs and system callers keep the cheap SQL count: SkipsPerUserRules
// means they see every event, so recounting would just repeat the SQL's own
// answer at the cost of two extra queries per timeline.
func (s *timelineService) recountEventsForViewer(ctx context.Context, timelines []Timeline, v permissions.Viewer) error {
	if v.SkipsPerUserRules() {
		return nil
	}
	role := v.Role()
	for i := range timelines {
		events, err := s.timelineEventLinks(ctx, timelines[i].ID, role)
		if err != nil {
			return fmt.Errorf("recount timeline %s events: %w", timelines[i].ID, err)
		}
		timelines[i].EventCount = len(filterEventLinksByUser(events, role, v))
	}
	return nil
}

// filterTimelinesByUser applies the per-user visibility layer to a timeline
// slice. Owners/co-DMs and declared system callers get the list unchanged.
// A viewer with no user id (an anonymous visitor) does NOT skip this layer
// (ADR-049) — trust is a stated property of permissions.Viewer that no
// request-derived viewer can hold.
//
// It compacts in place (`timelines[:0]`), so the caller's backing array is
// mutated and must not be read again.
func filterTimelinesByUser(timelines []Timeline, v permissions.Viewer) []Timeline {
	filtered := timelines[:0]
	for _, t := range timelines {
		if timelineVisibleToViewer(t, v) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// timelineVisibleToViewer is the one predicate behind every timeline
// visibility decision in this package (ADR-058): filterTimelinesByUser's
// per-row filter and GetTimelineForViewer's single-item lookup both call
// it, so those paths cannot drift out of step.
//
// Owners/co-DMs and declared system callers bypass the per-user layer
// entirely (SkipsPerUserRules). Everyone else — including an anonymous
// viewer, whose empty user id must never be read as system trust (ADR-049)
// — goes through canUserView's role + allow/deny-list checks.
func timelineVisibleToViewer(t Timeline, v permissions.Viewer) bool {
	if v.SkipsPerUserRules() {
		return true
	}
	return canUserView(t.Visibility, t.VisibilityRules, v.Role(), v.UserID())
}

// ListTimelinesForCalendar returns the timelines bound to a calendar,
// role-filtered and per-user visibility-filtered (same rules as
// ListTimelines). Exposed cross-plugin (calendar's Calendars dashboard) via
// the TimelineService interface — the calendar plugin reaches it through an
// adapter, never a repo import (plugin-isolation convention).
func (s *timelineService) ListTimelinesForCalendar(ctx context.Context, calendarID string, v permissions.Viewer) ([]Timeline, error) {
	timelines, err := s.repo.ListByCalendar(ctx, calendarID, v.Role())
	if err != nil {
		return nil, fmt.Errorf("list timelines for calendar: %w", err)
	}
	timelines = filterTimelinesByUser(timelines, v)
	if err := s.recountEventsForViewer(ctx, timelines, v); err != nil {
		return nil, err
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

	// Load-merge-write (ADR-054). `t` is the row as stored, so every merge
	// below defaults to the stored value: only a key the caller actually
	// sent can change anything. Name is the one exception: it stays
	// required on every call (fails loudly with 400 when blank).
	name := input.Name
	if name == "" {
		return apperror.NewValidation("timeline name is required")
	}
	if len(name) > 255 {
		return apperror.NewValidation("timeline name must be 255 characters or less")
	}
	visibility := input.Visibility.Val(t.Visibility)
	if visibility != "everyone" && visibility != "dm_only" {
		return apperror.NewValidation("visibility must be 'everyone' or 'dm_only'")
	}
	zoom := input.ZoomDefault.Val(t.ZoomDefault)
	if !IsValidZoom(zoom) {
		return apperror.NewValidation("invalid zoom default level")
	}
	icon := input.Icon.Val(t.Icon)
	if icon != "" && !iconPattern.MatchString(icon) {
		return apperror.NewValidation("icon must be a valid FontAwesome class name")
	}
	color := input.Color.Val(t.Color)
	if color != "" && !colorPattern.MatchString(color) {
		return apperror.NewValidation("color must be a valid hex color")
	}
	visRules := input.VisibilityRules.Ptr(t.VisibilityRules)
	if err := validateVisibilityRules(visRules); err != nil {
		return err
	}

	t.Name = name
	t.Description = input.Description.Ptr(t.Description)
	t.DescriptionHTML = input.DescriptionHTML.Ptr(t.DescriptionHTML)
	t.Color = color
	t.Icon = icon
	t.Visibility = visibility
	t.VisibilityRules = visRules
	t.ZoomDefault = zoom

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
	// Widget-binding delete hook: sweep this timeline's bindings. Best-effort
	// — the render-time orphan guard + Sweep backstop it.
	if s.bindingCleaner != nil {
		_, _ = s.bindingCleaner.OnInstanceDeleted(ctx, t.CampaignID, WidgetTypeTimeline, timelineID)
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
		TimelineID:    timelineID,
		EventID:       eventID,
		DisplayOrder:  count,
		Label:         input.Label,
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
//
// It additionally blanks EventEntityID/Name/Icon on any remaining event whose
// linked entity the viewer isn't separately permitted to see, via
// EntityVisibilityGate — mirrors mapService.ListMarkers, so a timeline can't
// name a private/dm_only entity to a viewer who couldn't otherwise see it.
func (s *timelineService) ListTimelineEvents(ctx context.Context, timelineID, campaignID string, v permissions.Viewer) ([]EventLink, error) {
	role := v.Role()
	events, err := s.timelineEventLinks(ctx, timelineID, role)
	if err != nil {
		return nil, err
	}
	events = filterEventLinksByUser(events, role, v)

	// Owners/co-DMs already see every entity link unfiltered elsewhere in the
	// app; the entity link they see here is the entity link that exists.
	if permissions.CanSeeDmOnly(role) {
		return events, nil
	}

	entityIDs := make([]string, 0, len(events))
	seen := make(map[string]bool, len(events))
	for _, el := range events {
		if el.EventEntityID == nil || *el.EventEntityID == "" || seen[*el.EventEntityID] {
			continue
		}
		seen[*el.EventEntityID] = true
		entityIDs = append(entityIDs, *el.EventEntityID)
	}
	if len(entityIDs) == 0 {
		return events, nil
	}

	viewable, err := s.viewableEntityIDs(ctx, campaignID, entityIDs, role, v.UserID())
	if err != nil {
		return nil, fmt.Errorf("filter viewable event entities: %w", err)
	}

	for i := range events {
		if events[i].EventEntityID == nil || *events[i].EventEntityID == "" {
			continue
		}
		if !viewable[*events[i].EventEntityID] {
			// Blank the ID too, not just the name/icon: a bare id still
			// tells the viewer a specific hidden entity exists.
			events[i].EventEntityID = nil
			events[i].EventEntityName = ""
			events[i].EventEntityIcon = ""
		}
	}
	return events, nil
}

// viewableEntityIDs resolves which of entityIDs this viewer may see via the
// injected EntityVisibilityGate. Fails closed (nothing viewable) if the gate
// isn't wired — a misconfiguration, not a policy outcome, same as maps'
// ListMarkers when its own gate is unset.
func (s *timelineService) viewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error) {
	if s.entityGate == nil {
		slog.Error("timeline: entity visibility gate not configured; blanking all linked entity names",
			slog.String("campaign_id", campaignID))
		return map[string]bool{}, nil
	}
	return s.entityGate.FilterViewableEntityIDs(ctx, campaignID, entityIDs, role, userID)
}

// timelineEventLinks fetches a timeline's linked calendar events and
// standalone events and merges them into one sorted EventLink slice,
// unfiltered by per-user visibility. It is the shared read both
// ListTimelineEvents (the rows a viewer opens) and recountEventsForViewer
// (the count a timeline reports) build on, so the two can't diverge.
func (s *timelineService) timelineEventLinks(ctx context.Context, timelineID string, role int) ([]EventLink, error) {
	events, err := s.repo.ListEventLinks(ctx, timelineID, role)
	if err != nil {
		return nil, fmt.Errorf("list timeline events: %w", err)
	}
	for i := range events {
		events[i].Source = "calendar"
	}

	standalone, err := s.repo.ListStandaloneEvents(ctx, timelineID, role)
	if err != nil {
		return nil, fmt.Errorf("list standalone events: %w", err)
	}
	for _, se := range standalone {
		events = append(events, se.ToEventLink())
	}

	sortEventLinks(events)
	return events, nil
}

// filterEventLinksByUser applies the per-user visibility layer to a merged
// event-link slice. Owners/co-DMs and declared system callers see
// everything; an anonymous viewer does not (ADR-049).
func filterEventLinksByUser(events []EventLink, role int, v permissions.Viewer) []EventLink {
	if v.SkipsPerUserRules() {
		return events
	}
	filtered := events[:0]
	for _, el := range events {
		vis := el.EffectiveVisibility()
		if canUserView(vis, el.VisibilityRules, role, v.UserID()) {
			filtered = append(filtered, el)
		}
	}
	return filtered
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
func (s *timelineService) UpdateStandaloneEvent(ctx context.Context, timelineID, eventID string, input UpdateTimelineEventInput, canAuthorDmOnly bool) error {
	e, err := s.repo.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("get event for update: %w", err)
	}
	if e == nil || e.TimelineID != timelineID {
		return apperror.NewNotFound("event not found")
	}
	// A stored dm_only event is invisible to a caller who cannot author
	// dm_only content: answer the same NotFound a missing id would give,
	// whatever fields this update touches, so this can't be used to confirm
	// the event exists or to change it without ever seeing it.
	if e.Visibility == "dm_only" && !canAuthorDmOnly {
		return apperror.NewNotFound("event not found")
	}

	// Load-merge-write: `e` is the row as stored, so every merge below
	// defaults to the stored value — only a key the caller actually sent
	// can change anything.
	name := input.Name.Val(e.Name)
	if name == "" {
		return apperror.NewValidation("event name is required")
	}
	if len(name) > 255 {
		return apperror.NewValidation("event name must be 255 characters or less")
	}
	visibility := input.Visibility.Val(e.Visibility)
	if visibility != "everyone" && visibility != "dm_only" {
		return apperror.NewValidation("visibility must be 'everyone' or 'dm_only'")
	}
	color := input.Color.Ptr(e.Color)
	if color != nil && *color != "" && !colorPattern.MatchString(*color) {
		return apperror.NewValidation("color must be a valid hex color")
	}

	// Sanitize HTML if provided (rich text descriptions from TipTap editor).
	// An ABSENT description_html preserves the stored HTML — this is the
	// field whose loss made a rename silently discard a formatted write-up.
	// A present empty string, or an explicit null, still clears, exactly as
	// before.
	if input.DescriptionHTML.Present() {
		if v, ok := input.DescriptionHTML.Get(); ok && v != "" {
			sanitized := sanitize.HTML(v)
			e.DescriptionHTML = &sanitized
		} else {
			e.DescriptionHTML = nil
		}
	}

	e.EntityID = input.EntityID.Ptr(e.EntityID)
	e.Name = name
	e.Description = input.Description.Ptr(e.Description)
	e.Year = input.Year.Val(e.Year)
	e.Month = input.Month.Val(e.Month)
	e.Day = input.Day.Val(e.Day)
	e.StartHour = input.StartHour.Ptr(e.StartHour)
	e.StartMinute = input.StartMinute.Ptr(e.StartMinute)
	e.EndYear = input.EndYear.Ptr(e.EndYear)
	e.EndMonth = input.EndMonth.Ptr(e.EndMonth)
	e.EndDay = input.EndDay.Ptr(e.EndDay)
	e.EndHour = input.EndHour.Ptr(e.EndHour)
	e.EndMinute = input.EndMinute.Ptr(e.EndMinute)
	e.IsRecurring = input.IsRecurring.Val(e.IsRecurring)
	e.RecurrenceType = input.RecurrenceType.Ptr(e.RecurrenceType)
	e.Category = input.Category.Ptr(e.Category)
	e.Visibility = visibility
	e.VisibilityRules = input.VisibilityRules.Ptr(e.VisibilityRules)
	e.Label = input.Label.Ptr(e.Label)
	e.Color = color

	if err := s.repo.UpdateEvent(ctx, e); err != nil {
		return fmt.Errorf("update standalone event: %w", err)
	}
	return nil
}

// DeleteStandaloneEvent removes a standalone event from a timeline.
// timelineID is checked against the event's owner to prevent IDOR attacks.
func (s *timelineService) DeleteStandaloneEvent(ctx context.Context, timelineID, eventID string, canAuthorDmOnly bool) error {
	e, err := s.repo.GetEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("get event for delete: %w", err)
	}
	if e == nil || e.TimelineID != timelineID {
		return apperror.NewNotFound("event not found")
	}
	// See UpdateStandaloneEvent: a stored dm_only event answers the same
	// NotFound a missing id would to a caller who cannot author dm_only
	// content.
	if e.Visibility == "dm_only" && !canAuthorDmOnly {
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
// ListEntityGroups additionally blanks a member's EntityID/Name/Icon when the
// viewer isn't separately permitted to see that entity, via
// EntityVisibilityGate — same reasoning as ListTimelineEvents.
func (s *timelineService) ListEntityGroups(ctx context.Context, timelineID, campaignID string, v permissions.Viewer) ([]EntityGroup, error) {
	groups, err := s.repo.ListEntityGroups(ctx, timelineID)
	if err != nil {
		return nil, fmt.Errorf("list entity groups: %w", err)
	}

	role := v.Role()
	if permissions.CanSeeDmOnly(role) {
		return groups, nil
	}

	entityIDs := make([]string, 0, len(groups))
	seen := make(map[string]bool, len(groups))
	for _, g := range groups {
		for _, m := range g.Members {
			if m.EntityID == "" || seen[m.EntityID] {
				continue
			}
			seen[m.EntityID] = true
			entityIDs = append(entityIDs, m.EntityID)
		}
	}
	if len(entityIDs) == 0 {
		return groups, nil
	}

	viewable, err := s.viewableEntityIDs(ctx, campaignID, entityIDs, role, v.UserID())
	if err != nil {
		return nil, fmt.Errorf("filter viewable group member entities: %w", err)
	}

	for gi := range groups {
		for mi := range groups[gi].Members {
			m := &groups[gi].Members[mi]
			if m.EntityID == "" || viewable[m.EntityID] {
				continue
			}
			m.EntityID = ""
			m.EntityName = ""
			m.EntityIcon = ""
		}
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
//
// repo.Search only narrows by the SQL-expressible half of visibility
// (`t.visibility = 'everyone'` unless the role can see dm_only). It cannot
// express the per-user visibility_rules allow/deny list, so this runs the
// results through filterTimelinesByUser afterward, the same call List
// already uses, not a second copy of the predicate (ADR-058).
func (s *timelineService) SearchTimelines(ctx context.Context, campaignID, query string, role int, userID string) ([]map[string]string, error) {
	timelines, err := s.repo.Search(ctx, campaignID, query, role)
	if err != nil {
		return nil, fmt.Errorf("search timelines: %w", err)
	}
	timelines = filterTimelinesByUser(timelines, permissions.RequestViewer(role, userID))

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

// ListCalendarEras returns eras for a calendar (used by the D3 visualization).
func (s *timelineService) ListCalendarEras(ctx context.Context, calendarID string) ([]CalendarEra, error) {
	if s.calEras == nil {
		return nil, nil
	}
	return s.calEras.ListEras(ctx, calendarID)
}

// --- Visibility Helpers ---

// canUserView checks whether a user can see an item based on its base visibility
// and per-user JSON rules. Owners always see everything and should be checked
// before calling this function.
func canUserView(baseVisibility string, visRulesJSON *string, role int, userID string) bool {
	// Base visibility: dm_only requires Owner role.
	if baseVisibility == "dm_only" && !permissions.CanSeeDmOnly(role) {
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

	// A non-empty DeniedUsers also excludes an anonymous viewer (ADR-049):
	// a logged-out visitor can't be proven not to be the player it names.
	if permissions.DeniesAnonymous(rules.DeniedUsers, userID) {
		return false
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

// --- Event Connections ---

// CreateConnection creates a visual connection between two events on the timeline.
func (s *timelineService) CreateConnection(ctx context.Context, timelineID string, input CreateConnectionInput) (*EventConnection, error) {
	if input.SourceID == "" || input.TargetID == "" {
		return nil, apperror.NewValidation("source_id and target_id are required")
	}
	if input.SourceID == input.TargetID {
		return nil, apperror.NewValidation("cannot connect an event to itself")
	}
	if input.SourceType != "calendar" && input.SourceType != "standalone" {
		return nil, apperror.NewValidation("source_type must be 'calendar' or 'standalone'")
	}
	if input.TargetType != "calendar" && input.TargetType != "standalone" {
		return nil, apperror.NewValidation("target_type must be 'calendar' or 'standalone'")
	}
	if input.Style == "" {
		input.Style = "arrow"
	}
	if !IsValidConnectionStyle(input.Style) {
		return nil, apperror.NewValidation("invalid connection style")
	}
	if input.Color != nil && *input.Color != "" && !colorPattern.MatchString(*input.Color) {
		return nil, apperror.NewValidation("color must be a valid hex color")
	}

	conn := &EventConnection{
		TimelineID: timelineID,
		SourceID:   input.SourceID,
		TargetID:   input.TargetID,
		SourceType: input.SourceType,
		TargetType: input.TargetType,
		Label:      input.Label,
		Color:      input.Color,
		Style:      input.Style,
	}
	if err := s.repo.CreateConnection(ctx, conn); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("create connection: %w", err))
	}
	return conn, nil
}

// DeleteConnection removes a connection from a timeline.
func (s *timelineService) DeleteConnection(ctx context.Context, timelineID string, connectionID int) error {
	return s.repo.DeleteConnection(ctx, connectionID, timelineID)
}

// ListConnections returns all connections for a timeline.
func (s *timelineService) ListConnections(ctx context.Context, timelineID string) ([]EventConnection, error) {
	return s.repo.ListConnections(ctx, timelineID)
}
