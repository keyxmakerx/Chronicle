package app

import (
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// calendarWireTypes maps each event the calendar service publishes to the
// WebSocket type the Foundry module routes on (API-CONTRACT.md "WebSocket
// messages"). A calendar event type missing here would publish into nothing,
// so calendar_event_publisher_test.go walks calendar.AllPublishedEventTypes().
var calendarWireTypes = map[string]ws.MessageType{
	calendar.PubEventCreated:        ws.MsgCalendarEventCreated,
	calendar.PubEventUpdated:        ws.MsgCalendarEventUpdated,
	calendar.PubEventDeleted:        ws.MsgCalendarEventDeleted,
	calendar.PubDateAdvanced:        ws.MsgCalendarDateAdvanced,
	calendar.PubWeatherChanged:      ws.MsgCalendarWeatherChanged,
	calendar.PubStructureUpdated:    ws.MsgCalendarStructureUpdated,
	calendar.PubSeasonChanged:       ws.MsgCalendarSeasonChanged,
	calendar.PubEraChanged:          ws.MsgCalendarEraChanged,
	calendar.PubMoonPhaseChanged:    ws.MsgCalendarMoonPhaseChanged,
	calendar.PubCycleChanged:        ws.MsgCalendarCycleChanged,
	calendar.PubFestivalChanged:     ws.MsgCalendarFestivalChanged,
	calendar.PubWorldStateChanged:   ws.MsgCalendarWorldstateChanged,
	calendar.PubWeatherZonesChanged: ws.MsgCalendarWeatherZonesChanged,
}

// calendarEventPublisherAdapter bridges the calendar service to the
// WebSocket bus.
type calendarEventPublisherAdapter struct {
	bus ws.EventBus
}

// PublishCalendarEvent translates a calendar change into a WebSocket message.
//
// Every message is RequiresDM. The service publishes the stored row, which
// may be a dm_only event, one not yet announced, or a hidden moon, and the hub
// broadcasts anything not RequiresDM to every socket in the campaign,
// including players' browser sessions. Only Owner or DM-granted sockets
// (the Foundry GM key, which resolves to the owner) receive it; a player
// reads the calendar over the role-filtered HTTP routes, and a Foundry
// player sees what the GM's client republishes from an ?audience=players read.
func (a *calendarEventPublisherAdapter) PublishCalendarEvent(eventType, campaignID, calendarID string, payload any) {
	if campaignID == "" {
		return
	}
	msgType, ok := calendarWireTypes[eventType]
	if !ok {
		return
	}
	resourceID := calendarID
	switch p := payload.(type) {
	case *calendar.Event:
		if p == nil {
			return
		}
		resourceID = p.ID
		// The sync wire says "gm-only" where Chronicle stores "dm_only"; the
		// module treats any other value as public. Copy so the caller's event
		// is untouched.
		wire := *p
		if wire.Visibility == "dm_only" {
			wire.Visibility = "gm-only"
		}
		payload = &wire
	case map[string]string:
		// event.deleted: the change feed keys on the event id.
		if id := p["id"]; id != "" {
			resourceID = id
		}
	}
	msg := ws.NewMessage(msgType, campaignID, resourceID, payload)
	msg.RequiresDM = true
	a.bus.Publish(msg)
}
