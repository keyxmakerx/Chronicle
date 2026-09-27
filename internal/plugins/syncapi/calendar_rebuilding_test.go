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

// CALV5-PLACEHOLDER: pins the one contract that matters while the calendar
// is rebuilt — the Foundry module must be told "unavailable", never "empty".
// V5 must delete this file along with the placeholder handler. TODO(#778):
// Foundry sync rewiring is deliberately out of scope for calendar-v5-sky-seams.

// TestCalendarRoutes_AnswerRebuilding is the whole contract: every exported
// method on the placeholder handler answers 503 with the structured body the
// module can switch on.
//
// It reflects over the handler rather than listing method names, so a method
// that survives into V5 without its real implementation cannot slip through by
// being forgotten here.
func TestCalendarRoutes_AnswerRebuilding(t *testing.T) {
	h := NewCalendarAPIHandler()
	ht := reflect.TypeOf(h)
	if ht.NumMethod() == 0 {
		t.Fatal("CalendarAPIHandler exports no methods — the route table cannot be held open")
	}

	for i := 0; i < ht.NumMethod(); i++ {
		m := ht.Method(i)
		t.Run(m.Name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)

			out := m.Func.Call([]reflect.Value{reflect.ValueOf(h), reflect.ValueOf(c)})
			if len(out) != 1 {
				t.Fatalf("%s: unexpected signature, want func(echo.Context) error", m.Name)
			}
			if err, _ := out[0].Interface().(error); err != nil {
				t.Fatalf("%s returned an error: %v", m.Name, err)
			}

			// 503, not 404: a 404 tells the module this Chronicle is too old to
			// have the endpoint, sending it down its old-build compatibility
			// path and hiding the rebuild from the GM.
			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("%s answered %d, want 503 — an empty 200 would tell the module the "+
					"campaign HAS a calendar and it is empty, and the module would apply that "+
					"emptiness to a live Foundry world", m.Name, rec.Code)
			}

			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("%s: body is not JSON the module can parse: %v", m.Name, err)
			}
			if body["error"] != "calendar_rebuilding" {
				t.Errorf("%s: error code %q, want %q — the module switches on this string",
					m.Name, body["error"], "calendar_rebuilding")
			}
			if strings.TrimSpace(body["message"]) == "" {
				t.Errorf("%s: empty message — a GM reads this one", m.Name)
			}
		})
	}
}
