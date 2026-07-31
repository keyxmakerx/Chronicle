package calendar_test

// Egress guard for the per-viewer Bench disclosure state (C-CALV4-BENCH-R2
// slice R2-1, [BR2-5] SIGNED), modelled on rsvp_egress_test.go and on the
// availability precedent it in turn came from.
//
// `calendar_active.bench_sections` is DISPLAY STATE on a member-scoped row —
// which four sections of one person's Bench they have collapsed. It is not
// campaign content, it is not authored, and it belongs in no export. The
// exports are hand-written per-aggregate, so a new column is invisible by
// construction; this test fails loudly the moment somebody grafts a
// disclosure-shaped field onto an export struct or adds a matching AI-export
// category.
//
// It is a STRUCTURAL guard: no DB, no fixture, nothing to keep in sync.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// benchSectionTokens are the field-name / json-tag fragments that would mark
// this preference having escaped. "disclosure" and "collapsed" are included
// because they are the two most likely spellings for a re-export of the same
// fact under another name.
var benchSectionTokens = []string{"benchsection", "bench_section", "disclosure", "collapsed"}

func mentionsBenchSectionData(f reflect.StructField) string {
	hay := strings.ToLower(f.Name + " " + f.Tag.Get("json") + " " + f.Tag.Get("db"))
	for _, tok := range benchSectionTokens {
		if strings.Contains(hay, tok) {
			return tok
		}
	}
	return ""
}

func assertNoBenchSectionFields(t *testing.T, typ reflect.Type, path string, seen map[reflect.Type]bool) {
	t.Helper()
	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return
	}
	seen[typ] = true
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if tok := mentionsBenchSectionData(f); tok != "" {
			t.Errorf("egress leak: %s.%s references Bench disclosure state (%q) — it is per-viewer "+
				"display state on a member-scoped row and rides no export (C-CALV4-BENCH-R2 [BR2-5])",
				path, f.Name, tok)
		}
		ft := f.Type
		for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Map {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft.PkgPath() != "time" {
			assertNoBenchSectionFields(t, ft, path+"."+f.Name, seen)
		}
	}
}

func TestBenchSections_AbsentFromExports(t *testing.T) {
	assertNoBenchSectionFields(t, reflect.TypeOf(campaigns.CampaignExport{}), "CampaignExport", map[reflect.Type]bool{})
	assertNoBenchSectionFields(t, reflect.TypeOf(calendar.ChronicleExport{}), "ChronicleExport", map[reflect.Type]bool{})
}

func TestBenchSections_AbsentFromAIExportCategories(t *testing.T) {
	for _, c := range aiexport.AllCategories() {
		lc := strings.ToLower(string(c))
		for _, tok := range benchSectionTokens {
			if strings.Contains(lc, tok) {
				t.Errorf("egress leak: AI export category %q exposes Bench disclosure state (%q)", c, tok)
			}
		}
	}
}
