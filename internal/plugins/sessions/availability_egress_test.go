package sessions_test

// The own-tables egress guard (C-SCHED-P1 / design §5): availability lives in
// its OWN tables and must never ride the campaign export or the AI export
// payloads. Both exports are hand-written per-aggregate, so a new table is
// invisible by construction — but only as long as nobody grafts an availability
// field onto the session export struct or adds an availability AI-export
// category. This test fails loudly the moment either happens.
//
// It is a structural guard (no DB needed): it reflects over the exported
// payload types and scans the AI export category set for any "availab*" leak.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// mentionsAvailability reports whether a struct field name or its json tag
// hints at availability data.
func mentionsAvailability(f reflect.StructField) bool {
	if strings.Contains(strings.ToLower(f.Name), "avail") {
		return true
	}
	return strings.Contains(strings.ToLower(f.Tag.Get("json")), "avail")
}

// assertNoAvailabilityFields walks a struct type (recursing into nested struct
// slice/pointer fields) and fails if any field references availability.
func assertNoAvailabilityFields(t *testing.T, typ reflect.Type, path string) {
	t.Helper()
	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if mentionsAvailability(f) {
			t.Errorf("egress leak: %s.%s references availability — it must stay out of export payloads", path, f.Name)
		}
		ft := f.Type
		for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft.PkgPath() != "time" {
			assertNoAvailabilityFields(t, ft, path+"."+f.Name)
		}
	}
}

func TestAvailability_AbsentFromCampaignExport(t *testing.T) {
	assertNoAvailabilityFields(t, reflect.TypeOf(campaigns.ExportSession{}), "ExportSession")
	assertNoAvailabilityFields(t, reflect.TypeOf(campaigns.ExportAttendee{}), "ExportAttendee")
}

func TestAvailability_AbsentFromAIExportCategories(t *testing.T) {
	for _, c := range aiexport.AllCategories() {
		if strings.Contains(strings.ToLower(string(c)), "avail") {
			t.Errorf("egress leak: AI export category %q exposes availability by default", c)
		}
	}
}
