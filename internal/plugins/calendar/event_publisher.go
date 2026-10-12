package calendar

// Calendar change events the service hands to a CalendarEventPublisher. The
// calendar package knows nothing of WebSockets: internal/app maps these names
// to wire message types, so a service never imports a transport.
const (
	PubEventCreated        = "event.created"
	PubEventUpdated        = "event.updated"
	PubEventDeleted        = "event.deleted"
	PubDateAdvanced        = "date.advanced"
	PubWeatherChanged      = "calendar.weather.changed"
	PubStructureUpdated    = "calendar.structure.updated"
	PubSeasonChanged       = "calendar.season.changed"
	PubEraChanged          = "calendar.era.changed"
	PubMoonPhaseChanged    = "calendar.moon.phase_changed"
	PubCycleChanged        = "calendar.cycle.changed"
	PubFestivalChanged     = "calendar.festival.changed"
	PubWorldStateChanged   = "calendar.worldstate.changed"
	PubWeatherZonesChanged = "calendar.weather.zones.changed"
)

// AllPublishedEventTypes lists every event type a publisher can be handed.
// The app-layer adapter test walks it so an event type added here without a
// wire mapping fails the build instead of publishing into nothing.
func AllPublishedEventTypes() []string {
	return []string{
		PubEventCreated, PubEventUpdated, PubEventDeleted, PubDateAdvanced,
		PubWeatherChanged, PubStructureUpdated, PubSeasonChanged, PubEraChanged,
		PubMoonPhaseChanged, PubCycleChanged, PubFestivalChanged,
		PubWorldStateChanged, PubWeatherZonesChanged,
	}
}

// CalendarEventPublisher is the seam through which calendar writes reach live
// subscribers (the Foundry module). payload may be nil.
//
// The service publishes the stored, unfiltered row: it has no recipient. The
// implementation must therefore deliver only to DM-equivalent recipients, or
// filter per recipient; it must never broadcast the payload to players.
type CalendarEventPublisher interface {
	PublishCalendarEvent(eventType, campaignID, calendarID string, payload any)
}

// DatePayload is the date.advanced payload, in the shape the Foundry module
// reads (year, month, day, hour, minute).
type DatePayload struct {
	Year   int `json:"year"`
	Month  int `json:"month"`
	Day    int `json:"day"`
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
}

// SetEventPublisher wires the publisher. Reached by type assertion at wiring
// time like SetEntityVisibilityGate, so the CalendarService interface is
// unchanged for tests and mocks.
func (s *calendarService) SetEventPublisher(p CalendarEventPublisher) { s.publisher = p }

// publish is a no-op without a publisher so unit tests and tools that build
// the service bare keep working. It runs only after the write succeeded.
func (s *calendarService) publish(eventType, campaignID, calendarID string, payload any) {
	if s.publisher != nil {
		s.publisher.PublishCalendarEvent(eventType, campaignID, calendarID, payload)
	}
}

// publishAfter publishes a payload-less "refetch me" ping only when the write
// that precedes it succeeded, and passes the write's error through. Structure
// and weather changes carry no payload: the module refetches through its own
// role-filtered reads, so nothing hidden can ride on the ping.
func (s *calendarService) publishAfter(err error, eventType, campaignID, calendarID string) error {
	if err == nil {
		s.publish(eventType, campaignID, calendarID, nil)
	}
	return err
}
