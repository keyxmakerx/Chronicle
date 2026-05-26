// egress_sanitize.go — defense-in-depth sanitization on /api/v1/*
// response payloads. Per C-SEC-CHUNK-6-AMENDED + operator decision
// D-C6.1 (redirect to syncapi handlers), the entity, note, and
// calendar-event GET handlers re-sanitize HTML fields before
// serialization. INGRESS sanitization (write path in each
// plugin's service.go) is the primary defense; these helpers cover
// historical rows or tooling-inserted rows that slipped past
// ingress, on Foundry-consumer surfaces.
//
// Scope: ONLY the /api/v1/* group handlers. The backup/restore
// path (export_adapters.go / ExportCampaign / POST import) is
// deliberately NOT re-sanitized — operator decision D4=(c) carves
// it out as lossless. Touching either would silently mutate user
// content during round-trip; don't.
//
// Cites: cordinator/decisions/2026-05-21-core-tenets.md §T-B1;
// cordinator/reports/chronicle/2026-05-22-c-security-audit.md §M-4,
// §0.5 D4=(c); cordinator/decisions/2026-05-26-chronicle-production-
// safety-system.md; cordinator/dispatches/chronicle/C-SEC-CHUNK-6-
// AMENDED.md.
package syncapi

import (
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// sanitizeEntityHTMLForEgress re-sanitizes the HTML-typed fields on
// an entity response copy. Safe to call on a nil pointer (no-op).
// The pointer's referent IS mutated — callers pass a freshly-scanned
// model whose pointer they own; the source-of-truth DB row is not
// touched because the scan produced a separate struct.
func sanitizeEntityHTMLForEgress(e *entities.Entity) {
	if e == nil {
		return
	}
	e.EntryHTML = sanitize.HTMLPtr(e.EntryHTML)
	e.PlayerNotesHTML = sanitize.HTMLPtr(e.PlayerNotesHTML)
}

// sanitizeEntitiesHTMLForEgress applies sanitizeEntityHTMLForEgress
// to every element of a fresh slice (e.g. ListEntities output).
func sanitizeEntitiesHTMLForEgress(es []entities.Entity) {
	for i := range es {
		sanitizeEntityHTMLForEgress(&es[i])
	}
}

// sanitizeNoteHTMLForEgress re-sanitizes the EntryHTML field on a
// note response copy. Safe on nil.
func sanitizeNoteHTMLForEgress(n *notes.Note) {
	if n == nil {
		return
	}
	n.EntryHTML = sanitize.HTMLPtr(n.EntryHTML)
}

// sanitizeNotesHTMLForEgress applies sanitizeNoteHTMLForEgress to
// every element of a fresh slice (e.g. ListNotes output).
func sanitizeNotesHTMLForEgress(ns []notes.Note) {
	for i := range ns {
		sanitizeNoteHTMLForEgress(&ns[i])
	}
}

// sanitizeCalendarEventHTMLForEgress re-sanitizes the
// DescriptionHTML field on a calendar event response copy. Safe on
// nil.
func sanitizeCalendarEventHTMLForEgress(e *calendar.Event) {
	if e == nil {
		return
	}
	e.DescriptionHTML = sanitize.HTMLPtr(e.DescriptionHTML)
}

// sanitizeCalendarEventsHTMLForEgress applies the per-event variant
// to every element of a fresh slice (e.g. ListEvents output).
func sanitizeCalendarEventsHTMLForEgress(es []calendar.Event) {
	for i := range es {
		sanitizeCalendarEventHTMLForEgress(&es[i])
	}
}
