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
)

// schedulerTokens are the field-name / json-tag fragments that mark data which
// must stay out of export egress: availability, exceptions, slot proposals,
// per-option responses, and scheduler notifications.
var schedulerTokens = []string{"avail", "proposal", "notification"}

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
