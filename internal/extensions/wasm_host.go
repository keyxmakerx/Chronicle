package extensions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	extism "github.com/extism/go-sdk"
)

// EntityReader provides read-only access to entities for WASM host functions.
type EntityReader interface {
	GetEntityJSON(ctx context.Context, campaignID, entityID string) (json.RawMessage, error)
	SearchEntitiesJSON(ctx context.Context, campaignID, query string, limit int) (json.RawMessage, error)
	ListEntityTypesJSON(ctx context.Context, campaignID string) (json.RawMessage, error)
}

// CalendarReader provides read-only access to calendar data for WASM host functions.
type CalendarReader interface {
	GetCalendarJSON(ctx context.Context, campaignID string) (json.RawMessage, error)
	ListEventsJSON(ctx context.Context, campaignID string, limit int) (json.RawMessage, error)
}

// TagReader provides read-only access to tags for WASM host functions.
type TagReader interface {
	ListTagsJSON(ctx context.Context, campaignID string) (json.RawMessage, error)
}

// EntityWriter provides write access to entity fields for WASM host functions.
type EntityWriter interface {
	UpdateFieldsJSON(ctx context.Context, entityID string, fieldsData json.RawMessage) error
}

// CalendarWriter provides write access to calendar events for WASM host functions.
type CalendarWriter interface {
	CreateEventJSON(ctx context.Context, campaignID string, input json.RawMessage) (json.RawMessage, error)
}

// TagWriter provides write access to entity tags for WASM host functions.
type TagWriter interface {
	SetEntityTagsJSON(ctx context.Context, entityID, campaignID string, tagIDs json.RawMessage) error
	GetEntityTagsJSON(ctx context.Context, entityID string) (json.RawMessage, error)
}

// RelationWriter provides write access to entity relations for WASM host functions.
type RelationWriter interface {
	CreateRelationJSON(ctx context.Context, campaignID string, input json.RawMessage) (json.RawMessage, error)
}

// KVStore provides per-plugin key-value storage backed by the extension_data table.
type KVStore interface {
	Get(ctx context.Context, campaignID, extensionID, key string) (json.RawMessage, error)
	Set(ctx context.Context, campaignID, extensionID, key string, value json.RawMessage) error
	Delete(ctx context.Context, campaignID, extensionID, key string) error
}

// HostEnvironment holds the service adapters that provide data to WASM plugins
// through host functions. It manages per-call context (campaign ID, plugin identity)
// and collects plugin log output.
type HostEnvironment struct {
	entityReader   EntityReader
	entityWriter   EntityWriter
	calendarReader CalendarReader
	calendarWriter CalendarWriter
	tagReader      TagReader
	tagWriter      TagWriter
	relationWriter RelationWriter
	kvStore        KVStore

	// pluginManager is a back-reference for plugin-to-plugin messaging.
	pluginManager *PluginManager

	// Per-call state, keyed by "extID:slug".
	mu          sync.RWMutex
	callContext map[string]*callState
}

// callState holds per-invocation context for a WASM plugin call.
type callState struct {
	campaignID  string
	extensionID string
	logs        []string
}

// NewHostEnvironment creates a new host environment with the given service adapters.
// Write adapters are set via setter methods after construction (they may not be
// available during initial wiring).
func NewHostEnvironment(
	entityReader EntityReader,
	calendarReader CalendarReader,
	tagReader TagReader,
	kvStore KVStore,
) *HostEnvironment {
	return &HostEnvironment{
		entityReader:   entityReader,
		calendarReader: calendarReader,
		tagReader:      tagReader,
		kvStore:        kvStore,
		callContext:    make(map[string]*callState),
	}
}

// SetEntityWriter injects the entity write adapter.
func (h *HostEnvironment) SetEntityWriter(w EntityWriter) { h.entityWriter = w }

// SetCalendarWriter injects the calendar write adapter.
func (h *HostEnvironment) SetCalendarWriter(w CalendarWriter) { h.calendarWriter = w }

// SetTagWriter injects the tag write adapter.
func (h *HostEnvironment) SetTagWriter(w TagWriter) { h.tagWriter = w }

// SetRelationWriter injects the relation write adapter.
func (h *HostEnvironment) SetRelationWriter(w RelationWriter) { h.relationWriter = w }

// SetPluginManager sets the back-reference for plugin-to-plugin messaging.
func (h *HostEnvironment) SetPluginManager(pm *PluginManager) { h.pluginManager = pm }

// SetCallContext sets the campaign and extension context for a plugin call.
// Must be called before invoking a WASM function.
func (h *HostEnvironment) SetCallContext(extID, slug, campaignID string) {
	key := pluginKey(extID, slug)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.callContext[key] = &callState{
		campaignID:  campaignID,
		extensionID: extID,
	}
}

// ClearCallContext removes the call context after a plugin call completes.
func (h *HostEnvironment) ClearCallContext(extID, slug string) {
	key := pluginKey(extID, slug)
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.callContext, key)
}

// DrainLogs returns and clears accumulated log messages for a plugin.
func (h *HostEnvironment) DrainLogs(extID, slug string) []string {
	key := pluginKey(extID, slug)
	h.mu.Lock()
	defer h.mu.Unlock()

	cs, ok := h.callContext[key]
	if !ok || len(cs.logs) == 0 {
		return nil
	}
	logs := cs.logs
	cs.logs = nil
	return logs
}

// getCallState returns the current call state for a plugin. Thread-safe.
func (h *HostEnvironment) getCallState(extID, slug string) *callState {
	key := pluginKey(extID, slug)
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.callContext[key]
}

// appendLog adds a log message for the current plugin call. Thread-safe.
func (h *HostEnvironment) appendLog(extID, slug, msg string) {
	key := pluginKey(extID, slug)
	h.mu.Lock()
	defer h.mu.Unlock()
	if cs, ok := h.callContext[key]; ok {
		// Limit log accumulation to 100 messages per call.
		if len(cs.logs) < 100 {
			cs.logs = append(cs.logs, msg)
		}
	}
}

// BuildHostFunctions creates the Extism host function definitions based on
// the plugin's declared capabilities. Only functions matching the capability
// set are included, enforcing the principle of least privilege.
func (h *HostEnvironment) BuildHostFunctions(capabilities map[string]bool) []extism.HostFunction {
	var funcs []extism.HostFunction

	// Log capability — always available when declared.
	if capabilities[string(CapLog)] {
		funcs = append(funcs, h.buildLogFunction())
	}

	// Entity read capability.
	if capabilities[string(CapEntityRead)] {
		funcs = append(funcs, h.buildGetEntityFunction())
		funcs = append(funcs, h.buildSearchEntitiesFunction())
		funcs = append(funcs, h.buildListEntityTypesFunction())
	}

	// Calendar read capability.
	if capabilities[string(CapCalendarRead)] {
		funcs = append(funcs, h.buildGetCalendarFunction())
		funcs = append(funcs, h.buildListEventsFunction())
	}

	// Tag read capability.
	if capabilities[string(CapTagRead)] {
		funcs = append(funcs, h.buildListTagsFunction())
	}

	// Entity write capability.
	if capabilities[string(CapEntityWrite)] && h.entityWriter != nil {
		funcs = append(funcs, h.buildUpdateEntityFieldsFunction())
	}

	// Calendar write capability.
	if capabilities[string(CapCalendarWrite)] && h.calendarWriter != nil {
		funcs = append(funcs, h.buildCreateEventFunction())
	}

	// Tag write capability.
	if capabilities[string(CapTagWrite)] && h.tagWriter != nil {
		funcs = append(funcs, h.buildSetEntityTagsFunction())
		funcs = append(funcs, h.buildGetEntityTagsFunction())
	}

	// Relation write capability.
	if capabilities[string(CapRelationWrite)] && h.relationWriter != nil {
		funcs = append(funcs, h.buildCreateRelationFunction())
	}

	// KV store capability.
	if capabilities[string(CapKVStore)] {
		funcs = append(funcs, h.buildKVGetFunction())
		funcs = append(funcs, h.buildKVSetFunction())
		funcs = append(funcs, h.buildKVDeleteFunction())
	}

	// Message capability (plugin-to-plugin).
	if capabilities[string(CapMessage)] && h.pluginManager != nil {
		funcs = append(funcs, h.buildSendMessageFunction())
	}

	return funcs
}

// --- Host Function Builders ---

// buildLogFunction creates the chronicle_log host function.
// Input: JSON {"level": "info|warn|error", "message": "..."}
func (h *HostEnvironment) buildLogFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"chronicle_log",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				return
			}

			var req struct {
				Level   string `json:"level"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return
			}

			// Truncate message to prevent abuse.
			if len(req.Message) > 1024 {
				req.Message = req.Message[:1024] + "...(truncated)"
			}

			// Find the calling plugin's context from the function closure.
			// Host functions don't receive plugin identity directly, so we
			// log with a generic prefix. The call-site sets context.
			slog.Info("WASM plugin log",
				slog.String("level", req.Level),
				slog.String("message", req.Message),
			)
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{},
	)
}

// buildGetEntityFunction creates the get_entity host function.
// Input: JSON {"entity_id": "..."}
// Output: JSON entity data or error.
func (h *HostEnvironment) buildGetEntityFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"get_entity",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				h.writeHostError(p, stack, "failed to read input")
				return
			}

			var req struct {
				EntityID string `json:"entity_id"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeHostError(p, stack, "invalid input JSON")
				return
			}

			// Get campaign ID from the broadest available call context.
			campaignID := h.getCampaignIDFromContext(ctx)
			if campaignID == "" {
				h.writeHostError(p, stack, "no campaign context")
				return
			}

			result, err := h.entityReader.GetEntityJSON(ctx, campaignID, req.EntityID)
			if err != nil {
				h.writeHostError(p, stack, fmt.Sprintf("get_entity failed: %v", err))
				return
			}

			h.writeHostResult(p, stack, result)
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// buildSearchEntitiesFunction creates the search_entities host function.
// Input: JSON {"query": "...", "limit": 20}
// Output: JSON array of entity summaries.
func (h *HostEnvironment) buildSearchEntitiesFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"search_entities",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				h.writeHostError(p, stack, "failed to read input")
				return
			}

			var req struct {
				Query string `json:"query"`
				Limit int    `json:"limit"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeHostError(p, stack, "invalid input JSON")
				return
			}

			if req.Limit <= 0 || req.Limit > 100 {
				req.Limit = 20
			}

			campaignID := h.getCampaignIDFromContext(ctx)
			if campaignID == "" {
				h.writeHostError(p, stack, "no campaign context")
				return
			}

			result, err := h.entityReader.SearchEntitiesJSON(ctx, campaignID, req.Query, req.Limit)
			if err != nil {
				h.writeHostError(p, stack, fmt.Sprintf("search_entities failed: %v", err))
				return
			}

			h.writeHostResult(p, stack, result)
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// buildListEntityTypesFunction creates the list_entity_types host function.
// Input: empty or JSON {}
// Output: JSON array of entity types.
func (h *HostEnvironment) buildListEntityTypesFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"list_entity_types",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			campaignID := h.getCampaignIDFromContext(ctx)
			if campaignID == "" {
				h.writeHostError(p, stack, "no campaign context")
				return
			}

			result, err := h.entityReader.ListEntityTypesJSON(ctx, campaignID)
			if err != nil {
				h.writeHostError(p, stack, fmt.Sprintf("list_entity_types failed: %v", err))
				return
			}

			h.writeHostResult(p, stack, result)
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// buildGetCalendarFunction creates the get_calendar host function.
// Input: empty or JSON {}
// Output: JSON calendar configuration.
func (h *HostEnvironment) buildGetCalendarFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"get_calendar",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			campaignID := h.getCampaignIDFromContext(ctx)
			if campaignID == "" {
				h.writeHostError(p, stack, "no campaign context")
				return
			}

			result, err := h.calendarReader.GetCalendarJSON(ctx, campaignID)
			if err != nil {
				h.writeHostError(p, stack, fmt.Sprintf("get_calendar failed: %v", err))
				return
			}

			h.writeHostResult(p, stack, result)
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// buildListEventsFunction creates the list_events host function.
// Input: JSON {"limit": 50}
// Output: JSON array of calendar events.
func (h *HostEnvironment) buildListEventsFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"list_events",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				h.writeHostError(p, stack, "failed to read input")
				return
			}

			var req struct {
				Limit int `json:"limit"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				req.Limit = 50
			}
			if req.Limit <= 0 || req.Limit > 500 {
				req.Limit = 50
			}

			campaignID := h.getCampaignIDFromContext(ctx)
			if campaignID == "" {
				h.writeHostError(p, stack, "no campaign context")
				return
			}

			result, err := h.calendarReader.ListEventsJSON(ctx, campaignID, req.Limit)
			if err != nil {
				h.writeHostError(p, stack, fmt.Sprintf("list_events failed: %v", err))
				return
			}

			h.writeHostResult(p, stack, result)
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// buildListTagsFunction creates the list_tags host function.
// Input: empty or JSON {}
// Output: JSON array of tags.
func (h *HostEnvironment) buildListTagsFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"list_tags",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			campaignID := h.getCampaignIDFromContext(ctx)
			if campaignID == "" {
				h.writeHostError(p, stack, "no campaign context")
				return
			}

			result, err := h.tagReader.ListTagsJSON(ctx, campaignID)
			if err != nil {
				h.writeHostError(p, stack, fmt.Sprintf("list_tags failed: %v", err))
				return
			}

			h.writeHostResult(p, stack, result)
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// buildKVGetFunction creates the kv_get host function.
// Input: JSON {"key": "..."}
// Output: JSON value or null.
func (h *HostEnvironment) buildKVGetFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"kv_get",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				h.writeHostError(p, stack, "failed to read input")
				return
			}

			var req struct {
				Key string `json:"key"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeHostError(p, stack, "invalid input JSON")
				return
			}

			campaignID := h.getCampaignIDFromContext(ctx)
			if campaignID == "" {
				h.writeHostError(p, stack, "no campaign context")
				return
			}

			// Use the calling plugin's extension ID from context.
			extID := h.getExtensionIDFromContext(ctx)

			result, err := h.kvStore.Get(ctx, campaignID, extID, req.Key)
			if err != nil {
				h.writeHostResult(p, stack, json.RawMessage("null"))
				return
			}

			h.writeHostResult(p, stack, result)
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// buildKVSetFunction creates the kv_set host function.
// Input: JSON {"key": "...", "value": <any>}
// Output: JSON {"ok": true} or error.
func (h *HostEnvironment) buildKVSetFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"kv_set",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				h.writeHostError(p, stack, "failed to read input")
				return
			}

			var req struct {
				Key   string          `json:"key"`
				Value json.RawMessage `json:"value"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeHostError(p, stack, "invalid input JSON")
				return
			}

			if req.Key == "" {
				h.writeHostError(p, stack, "key is required")
				return
			}
			// Limit key length.
			if len(req.Key) > 100 {
				h.writeHostError(p, stack, "key too long (max 100 chars)")
				return
			}
			// Limit value size.
			if len(req.Value) > 64*1024 {
				h.writeHostError(p, stack, "value too large (max 64 KB)")
				return
			}

			campaignID := h.getCampaignIDFromContext(ctx)
			if campaignID == "" {
				h.writeHostError(p, stack, "no campaign context")
				return
			}

			extID := h.getExtensionIDFromContext(ctx)

			if err := h.kvStore.Set(ctx, campaignID, extID, req.Key, req.Value); err != nil {
				h.writeHostError(p, stack, fmt.Sprintf("kv_set failed: %v", err))
				return
			}

			h.writeHostResult(p, stack, json.RawMessage(`{"ok":true}`))
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// buildKVDeleteFunction creates the kv_delete host function.
// Input: JSON {"key": "..."}
// Output: JSON {"ok": true} or error.
func (h *HostEnvironment) buildKVDeleteFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"kv_delete",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				h.writeHostError(p, stack, "failed to read input")
				return
			}

			var req struct {
				Key string `json:"key"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeHostError(p, stack, "invalid input JSON")
				return
			}

			campaignID := h.getCampaignIDFromContext(ctx)
			if campaignID == "" {
				h.writeHostError(p, stack, "no campaign context")
				return
			}

			extID := h.getExtensionIDFromContext(ctx)

			if err := h.kvStore.Delete(ctx, campaignID, extID, req.Key); err != nil {
				h.writeHostError(p, stack, fmt.Sprintf("kv_delete failed: %v", err))
				return
			}

			h.writeHostResult(p, stack, json.RawMessage(`{"ok":true}`))
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// --- Write Host Function Builders ---

// buildUpdateEntityFieldsFunction creates the update_entity_fields host function.
// Input: JSON {"entity_id": "...", "fields": {...}}
// Output: JSON {"ok": true} or error.
func (h *HostEnvironment) buildUpdateEntityFieldsFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"update_entity_fields",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				h.writeHostError(p, stack, "failed to read input")
				return
			}

			var req struct {
				EntityID string          `json:"entity_id"`
				Fields   json.RawMessage `json:"fields"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeHostError(p, stack, "invalid input JSON")
				return
			}

			if req.EntityID == "" {
				h.writeHostError(p, stack, "entity_id is required")
				return
			}
			if len(req.Fields) == 0 {
				h.writeHostError(p, stack, "fields is required")
				return
			}
			// Limit fields payload size.
			if len(req.Fields) > 256*1024 {
				h.writeHostError(p, stack, "fields too large (max 256 KB)")
				return
			}

			if err := h.entityWriter.UpdateFieldsJSON(ctx, req.EntityID, req.Fields); err != nil {
				h.writeHostError(p, stack, fmt.Sprintf("update_entity_fields failed: %v", err))
				return
			}

			h.writeHostResult(p, stack, json.RawMessage(`{"ok":true}`))
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// buildCreateEventFunction creates the create_event host function.
// Input: JSON with event fields (title, description, date, etc.)
// Output: JSON created event or error.
func (h *HostEnvironment) buildCreateEventFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"create_event",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				h.writeHostError(p, stack, "failed to read input")
				return
			}

			// Validate it's valid JSON.
			if !json.Valid(input) {
				h.writeHostError(p, stack, "invalid input JSON")
				return
			}

			// Limit input size.
			if len(input) > 64*1024 {
				h.writeHostError(p, stack, "input too large (max 64 KB)")
				return
			}

			campaignID := h.getCampaignIDFromContext(ctx)
			if campaignID == "" {
				h.writeHostError(p, stack, "no campaign context")
				return
			}

			result, err := h.calendarWriter.CreateEventJSON(ctx, campaignID, json.RawMessage(input))
			if err != nil {
				h.writeHostError(p, stack, fmt.Sprintf("create_event failed: %v", err))
				return
			}

			h.writeHostResult(p, stack, result)
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// buildSetEntityTagsFunction creates the set_entity_tags host function.
// Input: JSON {"entity_id": "...", "tag_ids": [1, 2, 3]}
// Output: JSON {"ok": true} or error.
func (h *HostEnvironment) buildSetEntityTagsFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"set_entity_tags",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				h.writeHostError(p, stack, "failed to read input")
				return
			}

			var req struct {
				EntityID string          `json:"entity_id"`
				TagIDs   json.RawMessage `json:"tag_ids"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeHostError(p, stack, "invalid input JSON")
				return
			}

			if req.EntityID == "" {
				h.writeHostError(p, stack, "entity_id is required")
				return
			}

			campaignID := h.getCampaignIDFromContext(ctx)
			if campaignID == "" {
				h.writeHostError(p, stack, "no campaign context")
				return
			}

			if err := h.tagWriter.SetEntityTagsJSON(ctx, req.EntityID, campaignID, req.TagIDs); err != nil {
				h.writeHostError(p, stack, fmt.Sprintf("set_entity_tags failed: %v", err))
				return
			}

			h.writeHostResult(p, stack, json.RawMessage(`{"ok":true}`))
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// buildGetEntityTagsFunction creates the get_entity_tags host function.
// Input: JSON {"entity_id": "..."}
// Output: JSON array of tags.
func (h *HostEnvironment) buildGetEntityTagsFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"get_entity_tags",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				h.writeHostError(p, stack, "failed to read input")
				return
			}

			var req struct {
				EntityID string `json:"entity_id"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeHostError(p, stack, "invalid input JSON")
				return
			}

			if req.EntityID == "" {
				h.writeHostError(p, stack, "entity_id is required")
				return
			}

			result, err := h.tagWriter.GetEntityTagsJSON(ctx, req.EntityID)
			if err != nil {
				h.writeHostError(p, stack, fmt.Sprintf("get_entity_tags failed: %v", err))
				return
			}

			h.writeHostResult(p, stack, result)
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// buildCreateRelationFunction creates the create_relation host function.
// Input: JSON with relation fields (source_entity_id, target_entity_id, relation_type, etc.)
// Output: JSON created relation or error.
func (h *HostEnvironment) buildCreateRelationFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"create_relation",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				h.writeHostError(p, stack, "failed to read input")
				return
			}

			// Validate it's valid JSON.
			if !json.Valid(input) {
				h.writeHostError(p, stack, "invalid input JSON")
				return
			}

			// Limit input size.
			if len(input) > 64*1024 {
				h.writeHostError(p, stack, "input too large (max 64 KB)")
				return
			}

			campaignID := h.getCampaignIDFromContext(ctx)
			if campaignID == "" {
				h.writeHostError(p, stack, "no campaign context")
				return
			}

			result, err := h.relationWriter.CreateRelationJSON(ctx, campaignID, json.RawMessage(input))
			if err != nil {
				h.writeHostError(p, stack, fmt.Sprintf("create_relation failed: %v", err))
				return
			}

			h.writeHostResult(p, stack, result)
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// buildSendMessageFunction creates the send_message host function for
// plugin-to-plugin communication. The target plugin's "on_message" function
// is called asynchronously.
// Input: JSON {"target_ext_id": "...", "target_slug": "...", "payload": {...}}
// Output: JSON {"ok": true} or error.
func (h *HostEnvironment) buildSendMessageFunction() extism.HostFunction {
	return extism.NewHostFunctionWithStack(
		"send_message",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				h.writeHostError(p, stack, "failed to read input")
				return
			}

			var req struct {
				TargetExtID string          `json:"target_ext_id"`
				TargetSlug  string          `json:"target_slug"`
				Payload     json.RawMessage `json:"payload"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				h.writeHostError(p, stack, "invalid input JSON")
				return
			}

			if req.TargetExtID == "" || req.TargetSlug == "" {
				h.writeHostError(p, stack, "target_ext_id and target_slug are required")
				return
			}

			// Limit message size.
			if len(req.Payload) > 64*1024 {
				h.writeHostError(p, stack, "payload too large (max 64 KB)")
				return
			}

			// Build the message envelope with sender info.
			senderExtID := h.getExtensionIDFromContext(ctx)
			envelope, _ := json.Marshal(map[string]any{
				"sender_ext_id": senderExtID,
				"payload":       req.Payload,
			})

			// Dispatch asynchronously — fire and forget.
			go func() {
				callCtx := ctx
				if campaignID := h.getCampaignIDFromContext(ctx); campaignID != "" {
					callCtx = WithCampaignID(context.Background(), campaignID)
				} else {
					callCtx = context.Background()
				}

				_, err := h.pluginManager.Call(callCtx, req.TargetExtID, req.TargetSlug, "on_message", json.RawMessage(envelope))
				if err != nil {
					slog.Warn("plugin message delivery failed",
						slog.String("target", pluginKey(req.TargetExtID, req.TargetSlug)),
						slog.Any("error", err),
					)
				}
			}()

			h.writeHostResult(p, stack, json.RawMessage(`{"ok":true}`))
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

// --- Helper methods ---

// writeHostResult writes a success response to the WASM plugin's output.
func (h *HostEnvironment) writeHostResult(p *extism.CurrentPlugin, stack []uint64, data json.RawMessage) {
	result, _ := json.Marshal(map[string]json.RawMessage{"data": data})
	offset, err := p.WriteBytes(result)
	if err != nil {
		slog.Warn("failed to write host result", slog.Any("error", err))
		return
	}
	stack[0] = offset
}

// writeHostError writes an error response to the WASM plugin's output.
func (h *HostEnvironment) writeHostError(p *extism.CurrentPlugin, stack []uint64, msg string) {
	result, _ := json.Marshal(map[string]string{"error": msg})
	offset, err := p.WriteBytes(result)
	if err != nil {
		slog.Warn("failed to write host error", slog.Any("error", err))
		return
	}
	if len(stack) > 0 {
		stack[0] = offset
	}
}

// getCampaignIDFromContext extracts the campaign ID from the Go context.
func (h *HostEnvironment) getCampaignIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(contextKeyCampaignID).(string); ok {
		return id
	}
	return ""
}

// getExtensionIDFromContext extracts the extension ID from the Go context.
func (h *HostEnvironment) getExtensionIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(contextKeyExtensionID).(string); ok {
		return id
	}
	return ""
}

// contextKeyExtensionID is used to pass extension ID through context.
const contextKeyExtensionID contextKey = "wasm_extension_id"

// WithExtensionID returns a context with the extension ID set.
func WithExtensionID(ctx context.Context, extID string) context.Context {
	return context.WithValue(ctx, contextKeyExtensionID, extID)
}
