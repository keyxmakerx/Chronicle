package extensions

import (
	"context"
	"encoding/json"
)

// wasmEntityAdapter implements EntityReader by delegating to function closures.
// This avoids importing heavy plugin packages directly — the caller wires
// concrete service methods at startup in app/routes.go.
type wasmEntityAdapter struct {
	getByID        func(ctx context.Context, id string) (json.RawMessage, error)
	search         func(ctx context.Context, campaignID, query string, limit int) (json.RawMessage, error)
	listTypes      func(ctx context.Context, campaignID string) (json.RawMessage, error)
}

// NewWASMEntityAdapter creates an EntityReader from function closures.
func NewWASMEntityAdapter(
	getByID func(ctx context.Context, id string) (json.RawMessage, error),
	search func(ctx context.Context, campaignID, query string, limit int) (json.RawMessage, error),
	listTypes func(ctx context.Context, campaignID string) (json.RawMessage, error),
) EntityReader {
	return &wasmEntityAdapter{
		getByID:   getByID,
		search:    search,
		listTypes: listTypes,
	}
}

// GetEntityJSON implements EntityReader.
func (a *wasmEntityAdapter) GetEntityJSON(ctx context.Context, campaignID, entityID string) (json.RawMessage, error) {
	return a.getByID(ctx, entityID)
}

// SearchEntitiesJSON implements EntityReader.
func (a *wasmEntityAdapter) SearchEntitiesJSON(ctx context.Context, campaignID, query string, limit int) (json.RawMessage, error) {
	return a.search(ctx, campaignID, query, limit)
}

// ListEntityTypesJSON implements EntityReader.
func (a *wasmEntityAdapter) ListEntityTypesJSON(ctx context.Context, campaignID string) (json.RawMessage, error) {
	return a.listTypes(ctx, campaignID)
}

// wasmCalendarAdapter implements CalendarReader by delegating to function closures.
type wasmCalendarAdapter struct {
	getCalendar func(ctx context.Context, campaignID string) (json.RawMessage, error)
	listEvents  func(ctx context.Context, campaignID string, limit int) (json.RawMessage, error)
}

// NewWASMCalendarAdapter creates a CalendarReader from function closures.
func NewWASMCalendarAdapter(
	getCalendar func(ctx context.Context, campaignID string) (json.RawMessage, error),
	listEvents func(ctx context.Context, campaignID string, limit int) (json.RawMessage, error),
) CalendarReader {
	return &wasmCalendarAdapter{
		getCalendar: getCalendar,
		listEvents:  listEvents,
	}
}

// GetCalendarJSON implements CalendarReader.
func (a *wasmCalendarAdapter) GetCalendarJSON(ctx context.Context, campaignID string) (json.RawMessage, error) {
	return a.getCalendar(ctx, campaignID)
}

// ListEventsJSON implements CalendarReader.
func (a *wasmCalendarAdapter) ListEventsJSON(ctx context.Context, campaignID string, limit int) (json.RawMessage, error) {
	return a.listEvents(ctx, campaignID, limit)
}

// wasmTagAdapter implements TagReader by delegating to a function closure.
type wasmTagAdapter struct {
	listTags func(ctx context.Context, campaignID string) (json.RawMessage, error)
}

// NewWASMTagAdapter creates a TagReader from a function closure.
func NewWASMTagAdapter(
	listTags func(ctx context.Context, campaignID string) (json.RawMessage, error),
) TagReader {
	return &wasmTagAdapter{listTags: listTags}
}

// ListTagsJSON implements TagReader.
func (a *wasmTagAdapter) ListTagsJSON(ctx context.Context, campaignID string) (json.RawMessage, error) {
	return a.listTags(ctx, campaignID)
}

// --- Write Adapters ---

// wasmEntityWriteAdapter implements EntityWriter by delegating to a function closure.
type wasmEntityWriteAdapter struct {
	updateFields func(ctx context.Context, entityID string, fieldsData json.RawMessage) error
}

// NewWASMEntityWriteAdapter creates an EntityWriter from a function closure.
func NewWASMEntityWriteAdapter(
	updateFields func(ctx context.Context, entityID string, fieldsData json.RawMessage) error,
) EntityWriter {
	return &wasmEntityWriteAdapter{updateFields: updateFields}
}

// UpdateFieldsJSON implements EntityWriter.
func (a *wasmEntityWriteAdapter) UpdateFieldsJSON(ctx context.Context, entityID string, fieldsData json.RawMessage) error {
	return a.updateFields(ctx, entityID, fieldsData)
}

// wasmCalendarWriteAdapter implements CalendarWriter by delegating to a function closure.
type wasmCalendarWriteAdapter struct {
	createEvent func(ctx context.Context, campaignID string, input json.RawMessage) (json.RawMessage, error)
}

// NewWASMCalendarWriteAdapter creates a CalendarWriter from a function closure.
func NewWASMCalendarWriteAdapter(
	createEvent func(ctx context.Context, campaignID string, input json.RawMessage) (json.RawMessage, error),
) CalendarWriter {
	return &wasmCalendarWriteAdapter{createEvent: createEvent}
}

// CreateEventJSON implements CalendarWriter.
func (a *wasmCalendarWriteAdapter) CreateEventJSON(ctx context.Context, campaignID string, input json.RawMessage) (json.RawMessage, error) {
	return a.createEvent(ctx, campaignID, input)
}

// wasmTagWriteAdapter implements TagWriter by delegating to function closures.
type wasmTagWriteAdapter struct {
	setTags func(ctx context.Context, entityID, campaignID string, tagIDs json.RawMessage) error
	getTags func(ctx context.Context, entityID string) (json.RawMessage, error)
}

// NewWASMTagWriteAdapter creates a TagWriter from function closures.
func NewWASMTagWriteAdapter(
	setTags func(ctx context.Context, entityID, campaignID string, tagIDs json.RawMessage) error,
	getTags func(ctx context.Context, entityID string) (json.RawMessage, error),
) TagWriter {
	return &wasmTagWriteAdapter{setTags: setTags, getTags: getTags}
}

// SetEntityTagsJSON implements TagWriter.
func (a *wasmTagWriteAdapter) SetEntityTagsJSON(ctx context.Context, entityID, campaignID string, tagIDs json.RawMessage) error {
	return a.setTags(ctx, entityID, campaignID, tagIDs)
}

// GetEntityTagsJSON implements TagWriter.
func (a *wasmTagWriteAdapter) GetEntityTagsJSON(ctx context.Context, entityID string) (json.RawMessage, error) {
	return a.getTags(ctx, entityID)
}

// wasmRelationWriteAdapter implements RelationWriter by delegating to a function closure.
type wasmRelationWriteAdapter struct {
	createRelation func(ctx context.Context, campaignID string, input json.RawMessage) (json.RawMessage, error)
}

// NewWASMRelationWriteAdapter creates a RelationWriter from a function closure.
func NewWASMRelationWriteAdapter(
	createRelation func(ctx context.Context, campaignID string, input json.RawMessage) (json.RawMessage, error),
) RelationWriter {
	return &wasmRelationWriteAdapter{createRelation: createRelation}
}

// CreateRelationJSON implements RelationWriter.
func (a *wasmRelationWriteAdapter) CreateRelationJSON(ctx context.Context, campaignID string, input json.RawMessage) (json.RawMessage, error) {
	return a.createRelation(ctx, campaignID, input)
}
