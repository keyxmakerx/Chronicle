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
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
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

// --- Inline-secret redaction (P0: DM-secret egress) ---
//
// Inline GM secrets are authored as <span data-secret> in the rendered
// HTML and a "secret"-marked text node in the ProseMirror JSON. The
// documented contract (static/js/widgets/editor_secret.js) is that they
// are stripped server-side so players never receive the secret content.
// The web honors this in entities/handler.go (GetEntry / GetPlayerNotes)
// for MemberRole < RoleScribe; the /api/v1/* read path must mirror it or
// a player-role caller reads raw GM prose off entry_html / entry /
// player_notes — a confirmed launch-blocker leak.
//
// This is a ROLE-AWARE transform, distinct from the role-agnostic
// XSS sanitize above: Owners and Scribes see secrets (with a visual
// indicator client-side), so we only strip below the RoleScribe bar.
// The threshold mirrors the web verbatim (campaigns.RoleScribe), so the
// two paths can't drift on who sees secrets.

// stripEntitySecretsForEgress removes inline GM secrets from an entity
// response copy when the caller's role is below the secret-visibility
// bar (mirrors entities/handler.go GetEntry: MemberRole < RoleScribe).
// Owner/Scribe responses are left untouched. Safe on a nil pointer.
//
// Both representations are scrubbed: the rendered HTML fields via
// StripSecretsHTML (drops the whole <span data-secret> element) and the
// ProseMirror JSON fields via StripSecretsJSON (drops "secret"-marked
// text nodes). Stripping only one would leak the secret through the
// other, since clients may read either.
//
// The pointer's referent is replaced with a fresh pointer to a stripped
// copy (sanitize.* return new strings), so the source-of-truth model the
// caller scanned from the DB is not mutated in place.
func stripEntitySecretsForEgress(e *entities.Entity, role int) {
	if e == nil || role >= int(campaigns.RoleScribe) {
		return
	}
	e.Entry = stripSecretsJSONPtr(e.Entry)
	e.EntryHTML = stripSecretsHTMLPtr(e.EntryHTML)
	e.PlayerNotes = stripSecretsJSONPtr(e.PlayerNotes)
	e.PlayerNotesHTML = stripSecretsHTMLPtr(e.PlayerNotesHTML)
}

// stripEntitiesSecretsForEgress applies stripEntitySecretsForEgress to
// every element of a fresh slice (e.g. ListEntities / sync-pull output).
func stripEntitiesSecretsForEgress(es []entities.Entity, role int) {
	if role >= int(campaigns.RoleScribe) {
		return
	}
	for i := range es {
		stripEntitySecretsForEgress(&es[i], role)
	}
}

// stripSecretsHTMLPtr is the nullable-pointer companion to
// sanitize.StripSecretsHTML: nil in, nil out; otherwise a fresh pointer
// to the stripped HTML. The original referent is not mutated.
func stripSecretsHTMLPtr(p *string) *string {
	if p == nil {
		return nil
	}
	s := sanitize.StripSecretsHTML(*p)
	return &s
}

// stripSecretsJSONPtr is the nullable-pointer companion to
// sanitize.StripSecretsJSON: nil in, nil out; otherwise a fresh pointer
// to the stripped ProseMirror JSON. The original referent is not mutated.
func stripSecretsJSONPtr(p *string) *string {
	if p == nil {
		return nil
	}
	s := sanitize.StripSecretsJSON(*p)
	return &s
}
