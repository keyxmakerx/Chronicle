package sessions_test

// Pins that scheduler data — recurring availability, per-date exceptions,
// slot proposals, per-option responses, and scheduler notifications — lives
// in its own tables and must never ride the campaign export or the AI export
// payloads (RC-12.5). Both exports are hand-written per-aggregate, so a new
// table is invisible by construction unless a scheduler-shaped field is
// grafted onto an export struct or a scheduler AI-export category is added.
// It is a structural guard (no DB needed), reflecting from the campaign
// export root (campaigns.CampaignExport) so a leak anywhere in the aggregate
// trips it, and scanning the AI export category set for the same leak.

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
// per-option responses, and scheduler notifications. "timezone" is included
// because a member's zone is a user-account fact that says where they
// physically are — a new DTO field carrying it must extend this pin, not
// sidestep it.
var schedulerTokens = []string{"avail", "proposal", "notification", "timezone"}

// isAllowedExportZone is the one zone the campaign export may carry: the
// time zone a GM picked for a calendar that follows real time. It is a
// setting of their world rather than where any member is, and the owner
// chose to keep it in backups. Any other zone field still fails.
func isAllowedExportZone(typ reflect.Type, f reflect.StructField) bool {
	return typ == reflect.TypeOf(campaigns.ExportCalendarData{}) && f.Name == "RealTimeZone"
}

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
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return
	}
	seen[typ] = true
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if isAllowedExportZone(typ, f) {
			continue
		}
		if tok := mentionsSchedulerData(f); tok != "" {
			t.Errorf("egress leak: %s.%s references scheduler data (%q) — it must stay out of export payloads (RC-12.5)", path, f.Name, tok)
		}
		ft := f.Type
		for ft.Kind() == reflect.Pointer || ft.Kind() == reflect.Slice {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft.PkgPath() != "time" {
			assertNoSchedulerFields(t, ft, path+"."+f.Name, seen)
		}
	}
}

func TestScheduler_AbsentFromCampaignExport(t *testing.T) {
	// Walk from the export ROOT so any scheduler-shaped field anywhere in the
	// aggregate, not only on the session/attendee leaves, trips the guard.
	assertNoSchedulerFields(t, reflect.TypeOf(campaigns.CampaignExport{}), "CampaignExport", map[reflect.Type]bool{})
}

// Proves the field the export guard is protecting actually exists on the
// DTO, so the egress test above can't be satisfied by quietly removing
// OverlayMember.TZ. TZ empty must stay distinguishable from "UTC" — a clock
// rendered for a zone-less member would otherwise be a guess presented as
// fact.
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
	// The co-DM marker rides beside the role rather than encoded into it.
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
