package app

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/extensions"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// wasmEntityHooks turns page changes into entity.* hooks for WASM plugins.
// The payload names the page; a plugin that needs more calls get_entity.
type wasmEntityHooks struct {
	d *extensions.HookDispatcher
}

// wasmEntityHookPayload is the payload of every entity.* hook.
type wasmEntityHookPayload struct {
	EntityID     string `json:"entity_id"`
	Name         string `json:"name,omitempty"`
	EntityTypeID int    `json:"entity_type_id,omitempty"`
}

// EntityChanged implements entities.EntityHookDispatcher.
func (h wasmEntityHooks) EntityChanged(ctx context.Context, eventType, campaignID string, e *entities.Entity) {
	payload, err := json.Marshal(wasmEntityHookPayload{EntityID: e.ID, Name: e.Name, EntityTypeID: e.EntityTypeID})
	if err != nil {
		slog.Warn("entity hook payload", slog.Any("error", err))
		return
	}
	switch eventType {
	case "created":
		h.d.DispatchEntityCreated(ctx, campaignID, payload)
	case "updated":
		h.d.DispatchEntityUpdated(ctx, campaignID, payload)
	case "deleted":
		h.d.DispatchEntityDeleted(ctx, campaignID, payload)
	}
}

// wasmTagHooks turns tag changes on a page into tag.* hooks for WASM plugins.
type wasmTagHooks struct {
	d *extensions.HookDispatcher
}

// wasmTagHookPayload is the payload of every tag.* hook.
type wasmTagHookPayload struct {
	EntityID string `json:"entity_id"`
	TagID    int    `json:"tag_id"`
	TagName  string `json:"tag_name"`
}

func (h wasmTagHooks) payload(entityID string, t tags.Tag) json.RawMessage {
	b, _ := json.Marshal(wasmTagHookPayload{EntityID: entityID, TagID: t.ID, TagName: t.Name})
	return b
}

// TagAdded implements tags.TagHookDispatcher.
func (h wasmTagHooks) TagAdded(ctx context.Context, campaignID, entityID string, t tags.Tag) {
	h.d.DispatchTagAdded(ctx, campaignID, h.payload(entityID, t))
}

// TagRemoved implements tags.TagHookDispatcher.
func (h wasmTagHooks) TagRemoved(ctx context.Context, campaignID, entityID string, t tags.Tag) {
	h.d.DispatchTagRemoved(ctx, campaignID, h.payload(entityID, t))
}
