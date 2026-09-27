// egress_sanitize.go — defense-in-depth sanitization on /api/v1/*
// response payloads. The entity, note, and calendar-event GET handlers
// re-sanitize HTML fields before serialization. INGRESS sanitization
// (write path in each plugin's service.go) is the primary defense; these
// helpers cover historical rows or tooling-inserted rows that slipped past
// ingress, on Foundry-consumer surfaces.
//
// Scope: ONLY the /api/v1/* group handlers. The backup/restore path
// (export_adapters.go / ExportCampaign / POST import) is deliberately NOT
// re-sanitized: it is carved out as lossless. Touching it would silently
// mutate user content during round-trip; don't.
package syncapi

import (
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

// CALV5-PLACEHOLDER: V5 must restore sanitizeCalendarEventHTMLForEgress (+
// slice variant), re-sanitizing Event.DescriptionHTML before it leaves for
// Foundry, in the same change as the GetEvent/ListEvents handlers — pinned
// by egress_sanitize_test.go. TODO(#778): Foundry sync rewiring is
// deliberately out of scope for calendar-v5-sky-seams.

// --- Inline-secret redaction (DM-secret egress) ---
//
// Inline GM secrets are authored as <span data-secret> in the rendered
// HTML and a "secret"-marked text node in the ProseMirror JSON. They must
// be stripped server-side so players never receive the secret content; the
// web enforces this in entities/handler.go (GetEntry / GetPlayerNotes) for
// MemberRole < RoleScribe, and the /api/v1/* read path must mirror it or a
// player-role caller reads raw GM prose off entry_html / entry /
// player_notes.
//
// This is a ROLE-AWARE transform, distinct from the role-agnostic XSS
// sanitize above: Owners and Scribes see secrets, so only the caller below
// RoleScribe gets stripped. The threshold mirrors the web verbatim so the
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

// stripEntityFieldsForEgress removes GM-only and owner-only field VALUES from
// a single entity's FieldsData for a caller who is neither GM-tier nor that
// entity's claimed owner. Unlike the inline secret strip above (which only
// touches the HTML/JSON prose fields), this scrubs the custom-field map,
// where a system marks fields gm_only (e.g. Draw Steel's director gm_notes)
// or owner_only (e.g. Draw Steel's backstory). resolveFields returns the
// entity type's declared field defs — the source of both markers. No-op for
// GM/owner/Bearer callers (role >= Scribe). FilterRestrictedFields never
// mutates the input map (nil-safe).
func stripEntityFieldsForEgress(e *entities.Entity, role int, userID string, resolveFields func(typeID int) []entities.FieldDefinition) {
	if e == nil || role >= int(campaigns.RoleScribe) {
		return
	}
	e.FieldsData = entities.FilterRestrictedFields(e.FieldsData, resolveFields(e.EntityTypeID), false, e.IsOwnedBy(userID))
}

// stripEntitiesFieldsForEgress applies stripEntityFieldsForEgress across
// a fresh slice (ListEntities). The caller passes a resolveFields backed by
// a single batched type load (map[typeID][]FieldDefinition) so this is not
// an N+1 query.
func stripEntitiesFieldsForEgress(es []entities.Entity, role int, userID string, resolveFields func(typeID int) []entities.FieldDefinition) {
	if role >= int(campaigns.RoleScribe) {
		return
	}
	for i := range es {
		stripEntityFieldsForEgress(&es[i], role, userID, resolveFields)
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
