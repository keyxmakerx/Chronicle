package entities

import "context"

// EntityHookDispatcher tells extension (WASM) plugins that a page changed.
// Implemented by the extensions hook dispatcher; the context carries the
// write's origin so the dispatcher can skip a plugin's own writes.
type EntityHookDispatcher interface {
	EntityChanged(ctx context.Context, eventType, campaignID string, entity *Entity)
}

// SetHookDispatcher wires the extension hook dispatcher. Optional.
func (s *entityService) SetHookDispatcher(d EntityHookDispatcher) {
	s.hooks = d
}

// publishEntity announces a page change to the live-sync bus and to extension
// plugins. Every page write announces through here so the two never disagree.
func (s *entityService) publishEntity(ctx context.Context, eventType, campaignID, entityID string, entity *Entity) {
	s.events.PublishEntityEvent(eventType, campaignID, entityID, entity)
	if s.hooks != nil && entity != nil {
		s.hooks.EntityChanged(ctx, eventType, campaignID, entity)
	}
}
