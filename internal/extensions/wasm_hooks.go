package extensions

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

// HookDispatcher fires events to WASM plugins that have registered for them.
// Hooks are dispatched asynchronously in a fire-and-forget manner — plugin
// failures do not affect the originating operation.
type HookDispatcher struct {
	manager *PluginManager
}

// NewHookDispatcher creates a hook dispatcher backed by the plugin manager.
func NewHookDispatcher(manager *PluginManager) *HookDispatcher {
	return &HookDispatcher{manager: manager}
}

// Dispatch sends a hook event to all loaded WASM plugins that listen for it.
// The dispatch runs each plugin call in a goroutine so the caller is not blocked.
// Errors are logged but never returned — hooks are best-effort.
func (d *HookDispatcher) Dispatch(ctx context.Context, event HookEvent) {
	targets := d.manager.PluginsForHook(event.Type)
	if len(targets) == 0 {
		return
	}

	// Serialize the event payload.
	eventJSON, err := json.Marshal(event)
	if err != nil {
		slog.Warn("failed to serialize hook event",
			slog.String("type", event.Type),
			slog.Any("error", err),
		)
		return
	}

	slog.Debug("dispatching hook event",
		slog.String("type", event.Type),
		slog.Int("targets", len(targets)),
	)

	for _, target := range targets {
		go func(extID, slug string) {
			// Create a fresh context with timeout for the hook call.
			hookCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Set campaign context for host functions.
			hookCtx = WithCampaignID(hookCtx, event.CampaignID)
			hookCtx = WithExtensionID(hookCtx, extID)
			d.manager.hostEnv.SetCallContext(extID, slug, event.CampaignID)
			defer d.manager.hostEnv.ClearCallContext(extID, slug)

			// Call the plugin's "on_hook" function with the event data.
			resp, err := d.manager.Call(hookCtx, extID, slug, "on_hook", json.RawMessage(eventJSON))
			if err != nil {
				slog.Warn("hook dispatch failed",
					slog.String("ext_id", extID),
					slog.String("slug", slug),
					slog.String("hook", event.Type),
					slog.Any("error", err),
				)
				return
			}

			if resp != nil && resp.Error != "" {
				slog.Warn("hook handler returned error",
					slog.String("ext_id", extID),
					slog.String("slug", slug),
					slog.String("hook", event.Type),
					slog.String("error", resp.Error),
				)
			}

			// Log any plugin output.
			if resp != nil && len(resp.Logs) > 0 {
				for _, logMsg := range resp.Logs {
					slog.Info("WASM hook log",
						slog.String("ext_id", extID),
						slog.String("slug", slug),
						slog.String("hook", event.Type),
						slog.String("message", logMsg),
					)
				}
			}
		}(target.extID, target.slug)
	}
}

// DispatchEntityCreated fires the entity.created hook.
func (d *HookDispatcher) DispatchEntityCreated(ctx context.Context, campaignID string, payload json.RawMessage) {
	d.Dispatch(ctx, HookEvent{
		Type:       HookEntityCreated,
		CampaignID: campaignID,
		Payload:    payload,
		Timestamp:  time.Now().UTC(),
	})
}

// DispatchEntityUpdated fires the entity.updated hook.
func (d *HookDispatcher) DispatchEntityUpdated(ctx context.Context, campaignID string, payload json.RawMessage) {
	d.Dispatch(ctx, HookEvent{
		Type:       HookEntityUpdated,
		CampaignID: campaignID,
		Payload:    payload,
		Timestamp:  time.Now().UTC(),
	})
}

// DispatchEntityDeleted fires the entity.deleted hook.
func (d *HookDispatcher) DispatchEntityDeleted(ctx context.Context, campaignID string, payload json.RawMessage) {
	d.Dispatch(ctx, HookEvent{
		Type:       HookEntityDeleted,
		CampaignID: campaignID,
		Payload:    payload,
		Timestamp:  time.Now().UTC(),
	})
}

// DispatchCalendarEventCreated fires the calendar.event_created hook.
func (d *HookDispatcher) DispatchCalendarEventCreated(ctx context.Context, campaignID string, payload json.RawMessage) {
	d.Dispatch(ctx, HookEvent{
		Type:       HookCalendarEventCreated,
		CampaignID: campaignID,
		Payload:    payload,
		Timestamp:  time.Now().UTC(),
	})
}

// DispatchCalendarEventUpdated fires the calendar.event_updated hook.
func (d *HookDispatcher) DispatchCalendarEventUpdated(ctx context.Context, campaignID string, payload json.RawMessage) {
	d.Dispatch(ctx, HookEvent{
		Type:       HookCalendarEventUpdated,
		CampaignID: campaignID,
		Payload:    payload,
		Timestamp:  time.Now().UTC(),
	})
}

// DispatchCalendarEventDeleted fires the calendar.event_deleted hook.
func (d *HookDispatcher) DispatchCalendarEventDeleted(ctx context.Context, campaignID string, payload json.RawMessage) {
	d.Dispatch(ctx, HookEvent{
		Type:       HookCalendarEventDeleted,
		CampaignID: campaignID,
		Payload:    payload,
		Timestamp:  time.Now().UTC(),
	})
}

// DispatchTagAdded fires the tag.added hook.
func (d *HookDispatcher) DispatchTagAdded(ctx context.Context, campaignID string, payload json.RawMessage) {
	d.Dispatch(ctx, HookEvent{
		Type:       HookTagAdded,
		CampaignID: campaignID,
		Payload:    payload,
		Timestamp:  time.Now().UTC(),
	})
}

// DispatchTagRemoved fires the tag.removed hook.
func (d *HookDispatcher) DispatchTagRemoved(ctx context.Context, campaignID string, payload json.RawMessage) {
	d.Dispatch(ctx, HookEvent{
		Type:       HookTagRemoved,
		CampaignID: campaignID,
		Payload:    payload,
		Timestamp:  time.Now().UTC(),
	})
}

// DispatchCampaignDeleted fires the campaign.deleted hook so WASM plugins
// can clean up in-memory caches or other campaign-scoped state.
func (d *HookDispatcher) DispatchCampaignDeleted(ctx context.Context, campaignID string) {
	d.Dispatch(ctx, HookEvent{
		Type:       HookCampaignDeleted,
		CampaignID: campaignID,
		Payload:    json.RawMessage(`{}`),
		Timestamp:  time.Now().UTC(),
	})
}
