package sessions_test

// The own-tables egress guard (C-SCHED-P1 / design §5; extended C-SCHED-P2 0b):
// scheduler data — recurring availability, per-date exceptions, slot proposals,
// per-option responses, and scheduler notifications — lives in its OWN tables
// and must never ride the campaign export or the AI export payloads (RC-12.5).
// Both exports are hand-written per-aggregate, so a new table is invisible by
// construction — but only as long as nobody grafts a scheduler-shaped field onto
// an export struct or adds a scheduler AI-export category. This test fails loudly
// the moment either happens.
//
// It is a structural guard (no DB needed). It reflects from the campaign export
// ROOT (campaigns.CampaignExport) so a scheduler-shaped field added ANYWHERE in
// the export aggregate — not just on ExportSession/ExportAttendee — trips it, and
// scans the AI export category set for the same leak.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

// schedulerTokens are the field-name / json-tag fragments that mark data which
// must stay out of export egress: availability, exceptions, slot proposals,
// per-option responses, and scheduler notifications.
//
// EXTENDED by C-CALV4-RSVP-P8 §5 with "timezone". OverlayMember gained a TZ
// field this slice — a per-member IANA zone on a member-scoped DTO — and the
// dispatch's rule is that a new DTO field EXTENDS this pin rather than
// sidestepping it. A member's zone is a user-account fact that says where they
// physically are; it is exactly the kind of field a well-meaning "and their
// local times" addition would graft onto an export struct.
var schedulerTokens = []string{"avail", "proposal", "notification", "timezone"}

// mentionsSchedulerData reports whether a struct field name or its json tag
// hints at any scheduler-owned data that must not be exported.
func mentionsSchedulerData(f reflect.StructField) string {
	name := strings.ToLower(f.Name)
	tag := strings.ToLower(f.Tag.Get("json"))
	for _, tok := range schedulerTokens {
		if strings.Contains(name, tok) || strings.Contains(tag, tok) {
			return tok
		}
	}
	return ""
}

// assertNoSchedulerFields walks a struct type (recursing into nested struct,
// slice, and pointer fields) and fails if any field references scheduler data.
// A visited-set guards against cycles in the type graph.
func assertNoSchedulerFields(t *testing.T, typ reflect.Type, path string, seen map[reflect.Type]bool) {
	t.Helper()
	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return
	}
	seen[typ] = true
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if tok := mentionsSchedulerData(f); tok != "" {
			t.Errorf("egress leak: %s.%s references scheduler data (%q) — it must stay out of export payloads (RC-12.5)", path, f.Name, tok)
		}
		ft := f.Type
		for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft.PkgPath() != "time" {
			assertNoSchedulerFields(t, ft, path+"."+f.Name, seen)
		}
	}
}

func TestScheduler_AbsentFromCampaignExport(t *testing.T) {
	// Walk from the export ROOT so any scheduler-shaped field anywhere in the
	// aggregate (0b), not only on the session/attendee leaves, trips the guard.
	assertNoSchedulerFields(t, reflect.TypeOf(campaigns.CampaignExport{}), "CampaignExport", map[reflect.Type]bool{})
}

// The pin moves WITH INTENT, not by deletion (C-CALV4-RSVP-P8 §5). The guard
// above only proves a zone is absent from the exports; this proves the field it
// is guarding actually exists on the DTO, so nobody can satisfy the egress test
// by quietly removing OverlayMember.TZ — and it re-states the invariant the
// Bench's clock column depends on: EMPTY is a first-class state, distinguishable
// from "UTC", because a clock rendered for a zone-less member is a guess
// presented as a fact (ADR-048 §18).
func TestScheduler_OverlayMemberCarriesZoneAndItIsOmitEmpty(t *testing.T) {
	f, ok := reflect.TypeOf(sessions.OverlayMember{}).FieldByName("TZ")
	if !ok {
		t.Fatal("OverlayMember.TZ is gone — the per-member clock has no source, " +
			"and the egress guard above is guarding nothing (C-CALV4-RSVP-P8 §5)")
	}
	if got := f.Tag.Get("json"); got != "tz,omitempty" {
		t.Errorf(`OverlayMember.TZ json tag = %q, want "tz,omitempty"`, got)
	}
	if f.Type.Kind() != reflect.String {
		t.Errorf("OverlayMember.TZ is %s; it must be a plain string so \"\" can mean "+
			"NOT SET without a second nil/empty distinction", f.Type)
	}
	// The co-DM marker rides beside the role rather than encoded into it (WG-4).
	if _, ok := reflect.TypeOf(sessions.OverlayMember{}).FieldByName("IsCoDM"); !ok {
		t.Error("OverlayMember.IsCoDM is gone — a co-DM would be labelled a plain " +
			"player on a permission surface again (ADR-048 §17)")
	}
}

func TestScheduler_AbsentFromAIExportCategories(t *testing.T) {
	for _, c := range aiexport.AllCategories() {
		lc := strings.ToLower(string(c))
		for _, tok := range schedulerTokens {
			if strings.Contains(lc, tok) {
				t.Errorf("egress leak: AI export category %q exposes scheduler data (%q) by default", c, tok)
			}
		}
	}
}
