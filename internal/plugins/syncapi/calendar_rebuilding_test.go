package syncapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// calendarRoutesNotRebuilt is every CalendarAPIHandler method that still
// answers calendarRebuilding. TODO(#869): it shrinks to empty, then this file
// and calendarRebuilding go.
var calendarRoutesNotRebuilt = map[string]bool{
	"GetEventCategories":     true,
	"GetWorldState":          true,
	"AdvanceDate":            true,
	"AdvanceTime":            true,
	"UpdateCalendarSettings": true,
	"UpdateMonths":           true,
	"UpdateWeekdays":         true,
	"UpdateMoons":            true,
	"UpdateEras":             true,
	"UpdateSeasons":          true,
	"UpdateEventCategories":  true,
	"SetWeather":             true,
	"UpdateCycles":           true,
	"UpdateFestivals":        true,
	"ExportCalendar":         true,
	"ImportCalendar":         true,
}

// TestCalendarRoutes_NotRebuiltAnswerRebuilding: a route not rebuilt yet
// answers 503 calendar_rebuilding, never an empty 200 (which the module would
// apply to a live world as "this calendar is empty") or a 404 (which it reads
// as an old Chronicle). The handlers here touch no service, so a zero handler
// is enough.
func TestCalendarRoutes_NotRebuiltAnswerRebuilding(t *testing.T) {
	h := &CalendarAPIHandler{}
	ht := reflect.TypeOf(h)
	for name := range calendarRoutesNotRebuilt {
		m, ok := ht.MethodByName(name)
		if !ok {
			t.Errorf("%s is listed as not rebuilt but CalendarAPIHandler has no such method", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
			out := m.Func.Call([]reflect.Value{reflect.ValueOf(h), reflect.ValueOf(c)})
			if err, _ := out[0].Interface().(error); err != nil {
				t.Fatalf("returned an error: %v", err)
			}
			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("answered %d, want 503", rec.Code)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if body["error"] != "calendar_rebuilding" || strings.TrimSpace(body["message"]) == "" {
				t.Errorf("body = %v, want error calendar_rebuilding and a message", body)
			}
		})
	}
}

// TestCalendarRoutes_RebuiltAreReal: every other exported route method has a
// real body. Reading the source, not calling it, so a method that quietly
// falls back to calendarRebuilding is caught without a service fake.
func TestCalendarRoutes_RebuiltAreReal(t *testing.T) {
	ht := reflect.TypeOf(&CalendarAPIHandler{})
	for i := 0; i < ht.NumMethod(); i++ {
		name := ht.Method(i).Name
		if calendarRoutesNotRebuilt[name] {
			continue
		}
		body := readHandlerBody(t, "calendar_api_handler.go", name)
		if strings.Contains(body, "calendarRebuilding(") {
			t.Errorf("%s still answers calendarRebuilding; add it to calendarRoutesNotRebuilt or rebuild it", name)
		}
	}
}

// TestCalendarRoutes_CreateCalendarNotAvailable: the module's import button
// shows its "no create endpoint yet" message on a 404, and only then.
func TestCalendarRoutes_CreateCalendarNotAvailable(t *testing.T) {
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", nil), rec)
	if err := (&CalendarAPIHandler{}).CreateCalendar(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "calendar_import_unavailable") {
		t.Errorf("CreateCalendar = %d %s, want 404 calendar_import_unavailable", rec.Code, rec.Body.String())
	}
}
