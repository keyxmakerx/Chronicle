package calendar_test

// Own-tables egress guard for event RSVPs (C-CAL-RSVP-P1), modelled on the
// scheduler's sessions/availability_egress_test.go.
//
// RSVP data — who is coming, who declined, and the free-text "these times work
// better" notes — lives in the calendar plugin's OWN tables and must never ride
// the campaign export or the AI export. Both exports are hand-written
// per-aggregate, so a new table is invisible by construction; this test fails
// loudly the moment somebody grafts an RSVP-shaped field onto an export struct
// or adds an RSVP AI-export category.
//
// It is a STRUCTURAL guard (no DB needed). It reflects from the campaign export
// ROOT (campaigns.CampaignExport) so an RSVP-shaped field added ANYWHERE in the
// aggregate trips it, walks the calendar plugin's own export root
// (calendar.ChronicleExport — the /calendars/:calId/export payload, which DOES
// carry events and would be the natural place for a well-meaning "and their
// RSVPs" addition), and scans the AI export category set for the same leak.
//
// Note this is stricter than it looks: Event.CollectRSVPs is a legitimate field
// on the live aggregate, and the guard proves it did NOT reach either export —
// ExportEvent is a separate, hand-mapped struct.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// rsvpTokens are the field-name / json-tag fragments that mark RSVP-owned data.
// "attend" is included because "attendees" is the most likely spelling for an
// accidental re-export of the same information under another name.
var rsvpTokens = []string{"rsvp", "attend"}

// rsvpEgressAllowlist names export paths that legitimately match a token.
//
// SESSION attendance is a different, pre-existing concept from CALENDAR EVENT
// RSVPs — that distinction is the whole reason C-CAL-RSVP-P1 built separate
// tables rather than reusing session_attendees — and it has always ridden the
// campaign export. Allowlisting the one known path (rather than dropping the
// "attend" token) keeps the guard armed: a NEW attendee-shaped field anywhere
// else in the aggregate still fails.
var rsvpEgressAllowlist = map[string]bool{
	"CampaignExport.Sessions.Attendees": true,
}

// mentionsRSVPData reports whether a struct field name or json tag hints at
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
// map-value, and pointer fields) and fails if any field references RSVP data.
// A visited-set guards against cycles in the type graph.
func assertNoRSVPFields(t *testing.T, typ reflect.Type, path string, seen map[reflect.Type]bool) {
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
		if tok := mentionsRSVPData(f); tok != "" && !rsvpEgressAllowlist[path+"."+f.Name] {
			t.Errorf("egress leak: %s.%s references RSVP data (%q) — event RSVPs must stay out of export payloads (C-CAL-RSVP-P1)",
				path, f.Name, tok)
		}
		ft := f.Type
		for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Map {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft.PkgPath() != "time" {
			assertNoRSVPFields(t, ft, path+"."+f.Name, seen)
		}
	}
}

func TestEventRSVPs_AbsentFromCampaignExport(t *testing.T) {
	assertNoRSVPFields(t, reflect.TypeOf(campaigns.CampaignExport{}), "CampaignExport", map[reflect.Type]bool{})
}

func TestEventRSVPs_AbsentFromCalendarExport(t *testing.T) {
	// The calendar's own export root — the closest surface to the new tables and
	// therefore the likeliest accidental leak path.
	assertNoRSVPFields(t, reflect.TypeOf(calendar.ChronicleExport{}), "ChronicleExport", map[reflect.Type]bool{})
}

// EXTENDED by C-CALV4-RSVP-P8 §5/§8.3.
//
// The Bench RSVP panel prints, beside every answer, that member's IANA zone and
// their local clock for the session. Those are user-account facts about where a
// person physically is, reached through a member-scoped DTO
// (sessions.OverlayMember.TZ) — and they are strictly MORE sensitive than the
// answer word beside them, because an answer is about one evening and a zone is
// about someone's life. The RSVP tokens above would not catch a field named
// Timezone or LocalTime, so the zone vocabulary gets its own pass over the same
// two export roots. The sessions plugin holds the mirror of this assertion over
// its own DTO (availability_egress_test.go).
var rsvpZoneTokens = []string{"timezone", "localtime", "local_time"}

// rsvpZoneAllowlist names export paths that legitimately match a zone token.
//
// A CALENDAR's anchor zone is not a PERSON's zone. `Calendar.RealTimeZone` is
// the IANA zone a real-time calendar computes its own date from — campaign
// configuration the operator typed, with no subject — and it has always ridden
// the calendar export because a calendar is unusable without it. Allowlisting
// the one known path keeps the guard armed on every other: a new
// member-shaped zone field anywhere in either aggregate still fails.
var rsvpZoneAllowlist = map[string]bool{
	"ChronicleExport.Calendar.RealTimeZone": true,
}

func TestEventRSVPs_PerMemberZoneAbsentFromExports(t *testing.T) {
	assertNoZoneFields(t, reflect.TypeOf(campaigns.CampaignExport{}), "CampaignExport", map[reflect.Type]bool{})
	assertNoZoneFields(t, reflect.TypeOf(calendar.ChronicleExport{}), "ChronicleExport", map[reflect.Type]bool{})
}

// assertNoZoneFields is assertNoRSVPFields over rsvpZoneTokens. It is a second
// walk rather than a widened token list on the first, because the two failure
// messages have to say different things: an RSVP leak and a whereabouts leak
// are not the same finding and must not be reported as one.
func assertNoZoneFields(t *testing.T, typ reflect.Type, path string, seen map[reflect.Type]bool) {
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
		name, tag := strings.ToLower(f.Name), strings.ToLower(f.Tag.Get("json"))
		for _, tok := range rsvpZoneTokens {
			if rsvpZoneAllowlist[path+"."+f.Name] {
				break
			}
			if strings.Contains(name, tok) || strings.Contains(tag, tok) {
				t.Errorf("egress leak: %s.%s exposes a member's whereabouts (%q) — per-member "+
					"zones and local clocks are Bench-render data, not export data (C-CALV4-RSVP-P8 §5)",
					path, f.Name, tok)
			}
		}
		ft := f.Type
		for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Map {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft.PkgPath() != "time" {
			assertNoZoneFields(t, ft, path+"."+f.Name, seen)
		}
	}
}

func TestEventRSVPs_AbsentFromAIExportCategories(t *testing.T) {
	for _, c := range aiexport.AllCategories() {
		lc := strings.ToLower(string(c))
		for _, tok := range rsvpTokens {
			if strings.Contains(lc, tok) {
				t.Errorf("egress leak: AI export category %q exposes RSVP data (%q) by default", c, tok)
			}
		}
	}
}
