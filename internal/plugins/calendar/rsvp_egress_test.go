package calendar_test

// Egress pin for calendar-event RSVPs (C-CAL-RSVP-P1), mirroring the sessions
// own-tables egress guard (sessions/availability_egress_test.go). RSVP data —
// responses, tokens, the collect_rsvps flag, per-person notes — lives in the
// calendar plugin's OWN tables (migration 013) and must NEVER ride the campaign
// export, the calendar export, or the AI export payloads. All three exports are
// hand-written per-aggregate, so a new table is invisible by construction — but
// only until someone grafts an RSVP-shaped field onto an export struct or adds
// an RSVP AI-export category. This test fails loudly the moment either happens.
//
// Structural guard (no DB). It reflects from three export ROOTS —
// campaigns.CampaignExport, the calendar-native ChronicleExport, and the AI
// export category set — so an RSVP-shaped field added ANYWHERE in any of those
// aggregates trips it.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// rsvpTokens are the field-name / json-tag fragments that mark RSVP-owned data
// which must stay out of export egress.
var rsvpTokens = []string{"rsvp"}

// mentionsRSVPData reports whether a struct field name or its json tag hints at
// RSVP-owned data that must not be exported.
func mentionsRSVPData(f reflect.StructField) string {
	name := strings.ToLower(f.Name)
	tag := strings.ToLower(f.Tag.Get("json"))
	for _, tok := range rsvpTokens {
		if strings.Contains(name, tok) || strings.Contains(tag, tok) {
			return tok
		}
	}
	return ""
}

// assertNoRSVPFields walks a struct type (recursing into nested struct, slice,
// and pointer fields) and fails if any field references RSVP data. A visited-set
// guards against cycles in the type graph.
func assertNoRSVPFields(t *testing.T, typ reflect.Type, path string, seen map[reflect.Type]bool) {
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
		if tok := mentionsRSVPData(f); tok != "" {
			t.Errorf("egress leak: %s.%s references RSVP data (%q) — it must stay out of export payloads (C-CAL-RSVP-P1)", path, f.Name, tok)
		}
		ft := f.Type
		for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft.PkgPath() != "time" {
			assertNoRSVPFields(t, ft, path+"."+f.Name, seen)
		}
	}
}

func TestRSVP_AbsentFromCampaignExport(t *testing.T) {
	assertNoRSVPFields(t, reflect.TypeOf(campaigns.CampaignExport{}), "CampaignExport", map[reflect.Type]bool{})
}

func TestRSVP_AbsentFromCalendarExport(t *testing.T) {
	assertNoRSVPFields(t, reflect.TypeOf(calendar.ChronicleExport{}), "ChronicleExport", map[reflect.Type]bool{})
}

func TestRSVP_AbsentFromAIExportCategories(t *testing.T) {
	for _, c := range aiexport.AllCategories() {
		lc := strings.ToLower(string(c))
		for _, tok := range rsvpTokens {
			if strings.Contains(lc, tok) {
				t.Errorf("egress leak: AI export category %q exposes RSVP data (%q) by default", c, tok)
			}
		}
	}
}
