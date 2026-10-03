package calendar

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

const foundryServicePayload = `{"schema_version":1,"source":"calendaria","name":"Harptos","description":"Realms",
"current_year":1492,"current_month":3,"current_day":7,"current_hour":13,"current_minute":45,
"months":[{"name":"Hammer","days":30},{"name":"Alturiak","days":30},{"name":"Ches","days":30}],
"weekdays":[{"name":"One"}]}`

func appErrCode(t *testing.T, err error) int {
	t.Helper()
	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("want an *apperror.AppError, got %T: %v", err, err)
	}
	return ae.Code
}

func TestImportFoundryCalendar(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		existing    []Calendar
		createErr   error
		wantCode    int // 0 = success
		wantCreated int
	}{
		{name: "creates the first calendar", payload: foundryServicePayload, wantCreated: 1},
		{name: "conflict when the campaign already has a calendar", payload: foundryServicePayload,
			existing: []Calendar{{ID: "c1", CampaignID: testCampaignA}}, wantCode: http.StatusConflict},
		{name: "conflict even when the existing calendar is hidden", payload: foundryServicePayload,
			existing: []Calendar{{ID: "c1", CampaignID: testCampaignA, Visibility: "dm_only"}}, wantCode: http.StatusConflict},
		{name: "racing import loses on the default-calendar unique key", payload: foundryServicePayload,
			createErr: errors.New("Error 1062: Duplicate entry 'camp-a' for key 'idx_default'"), wantCode: http.StatusConflict},
		{name: "other create errors are not turned into conflicts", payload: foundryServicePayload,
			createErr: errors.New("connection refused"), wantCode: -1},
		{name: "malformed JSON is a bad request", payload: `{"schema_version":`, wantCode: http.StatusBadRequest},
		{name: "wrong schema version is a bad request", payload: `{"schema_version":2,"source":"calendaria","months":[{"name":"M","days":30}]}`, wantCode: http.StatusBadRequest},
		{name: "other source is a bad request", payload: `{"schema_version":1,"source":"simple-calendar","months":[{"name":"M","days":30}]}`, wantCode: http.StatusBadRequest},
		{name: "no months is a bad request", payload: `{"schema_version":1,"source":"calendaria","months":[]}`, wantCode: http.StatusBadRequest},
		{name: "oversized structure is a bad request", payload: `{"schema_version":1,"source":"calendaria","months":[` + repeatJSON(`{"name":"M","days":1}`, 500) + `]}`, wantCode: http.StatusBadRequest},
		{name: "hour out of range is a validation error", payload: withTime(foundryServicePayload, 24, 0), wantCode: http.StatusUnprocessableEntity},
		{name: "negative minute is a validation error", payload: withTime(foundryServicePayload, 5, -1), wantCode: http.StatusUnprocessableEntity},
		{name: "minute 60 is a validation error", payload: withTime(foundryServicePayload, 5, 60), wantCode: http.StatusUnprocessableEntity},
		{name: "last valid time of day", payload: withTime(foundryServicePayload, 23, 59), wantCreated: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var created []*Calendar
			var listCampaign string
			repo := &fakeCalendarRepo{
				listByCampaignFn: func(_ context.Context, id string) ([]Calendar, error) {
					listCampaign = id
					return tc.existing, nil
				},
				createFn: func(_ context.Context, cal *Calendar) error {
					if tc.createErr != nil {
						return tc.createErr
					}
					created = append(created, cal)
					return nil
				},
			}
			svc := newTestCalendarService(repo, nil, nil, nil)

			cal, warnings, err := svc.ImportFoundryCalendar(context.Background(), testCampaignA, []byte(tc.payload))

			switch {
			case tc.wantCode == -1:
				if err == nil {
					t.Fatal("expected an error")
				}
				if code := apperror.SafeCode(err); code == http.StatusConflict {
					t.Errorf("a non-duplicate error became a 409: %v", err)
				}
			case tc.wantCode > 0:
				if err == nil {
					t.Fatalf("expected a %d, got a calendar", tc.wantCode)
				}
				if got := appErrCode(t, err); got != tc.wantCode {
					t.Errorf("code = %d (%v), want %d", got, err, tc.wantCode)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
			}
			if len(created) != tc.wantCreated {
				t.Fatalf("calendars created = %d, want %d", len(created), tc.wantCreated)
			}
			if tc.wantCode != 0 {
				if cal != nil || warnings != nil {
					t.Errorf("a failed import must return no calendar or warnings, got %v / %v", cal, warnings)
				}
				return
			}
			if listCampaign != testCampaignA {
				t.Errorf("existing-calendar check ran for %q, want %q", listCampaign, testCampaignA)
			}
			got := created[0]
			if !got.IsDefault {
				t.Error("imported calendar must be the campaign default")
			}
			if got.CampaignID != testCampaignA || got.Name != "Harptos" {
				t.Errorf("calendar = %+v", got)
			}
			if len(warnings) == 0 {
				t.Error("expected at least the day-length warning")
			}
		})
	}
}

// TestImportFoundryCalendar_CarriesTimeAndDate checks the payload's current
// date and time of day land on the created row, since the module syncs from it.
func TestImportFoundryCalendar_CarriesTimeAndDate(t *testing.T) {
	var created *Calendar
	repo := &fakeCalendarRepo{createFn: func(_ context.Context, cal *Calendar) error { created = cal; return nil }}
	var applied *Calendar
	repo.applyImportFn = func(_ context.Context, cal *Calendar, _ *ImportResult) error { applied = cal; return nil }
	svc := newTestCalendarService(repo, nil, nil, nil)

	cal, _, err := svc.ImportFoundryCalendar(context.Background(), testCampaignA, []byte(foundryServicePayload))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []*Calendar{cal, applied, created} {
		if c == nil {
			t.Fatal("calendar missing")
		}
		if c.CurrentYear != 1492 || c.CurrentMonth != 3 || c.CurrentDay != 7 || c.CurrentHour != 13 || c.CurrentMinute != 45 {
			t.Errorf("date/time = %d-%d-%d %d:%d, want 1492-3-7 13:45", c.CurrentYear, c.CurrentMonth, c.CurrentDay, c.CurrentHour, c.CurrentMinute)
		}
	}
	if cal.Description == nil || *cal.Description != "Realms" {
		t.Errorf("Description = %v, want Realms", cal.Description)
	}
}

// TestImportFoundryCalendar_FailedApplyLeavesNoCalendar: the cleanup that
// import already relies on must run for this path too, so a retry is not
// blocked by an orphaned row (which would make every retry a 409).
func TestImportFoundryCalendar_FailedApplyLeavesNoCalendar(t *testing.T) {
	var deleted []string
	var id string
	repo := &fakeCalendarRepo{
		createFn:      func(_ context.Context, cal *Calendar) error { id = cal.ID; return nil },
		applyImportFn: func(context.Context, *Calendar, *ImportResult) error { return errors.New("tx failed") },
		deleteFn:      func(_ context.Context, calID string) error { deleted = append(deleted, calID); return nil },
	}
	svc := newTestCalendarService(repo, nil, nil, nil)
	if _, _, err := svc.ImportFoundryCalendar(context.Background(), testCampaignA, []byte(foundryServicePayload)); err == nil {
		t.Fatal("expected the apply failure to surface")
	}
	if len(deleted) != 1 || deleted[0] != id {
		t.Errorf("deleted = %v, want the created calendar %q", deleted, id)
	}
}

func TestGetPrimaryCalendarForViewer(t *testing.T) {
	owner := permissions.RequestViewer(permissions.RoleOwner, "u-owner")
	player := permissions.RequestViewer(permissions.RolePlayer, "u-player")
	cal := func(id, vis string) Calendar {
		return Calendar{ID: id, CampaignID: testCampaignA, Name: id, Visibility: vis}
	}
	def := cal("default", "everyone")
	hiddenDefault := cal("default", "dm_only")

	tests := []struct {
		name     string
		def      *Calendar
		list     []Calendar
		viewer   permissions.Viewer
		wantID   string
		wantCode int
	}{
		{name: "default wins over the list", def: &def, list: []Calendar{cal("first", "everyone")}, viewer: player, wantID: "default"},
		{name: "no default falls back to the first calendar", list: []Calendar{cal("first", "everyone"), cal("second", "everyone")}, viewer: player, wantID: "first"},
		{name: "none at all is not found", viewer: player, wantCode: http.StatusNotFound},
		{name: "hidden first calendar is not found for a player", list: []Calendar{cal("first", "dm_only"), cal("second", "everyone")}, viewer: player, wantCode: http.StatusNotFound},
		{name: "hidden first calendar is visible to the owner", list: []Calendar{cal("first", "dm_only")}, viewer: owner, wantID: "first"},
		{name: "hidden default is not found and does not fall back", def: &hiddenDefault, list: []Calendar{cal("first", "everyone")}, viewer: player, wantCode: http.StatusNotFound},
		{name: "another campaign's calendar is not found", def: &Calendar{ID: "x", CampaignID: "camp-b", Visibility: "everyone"}, viewer: owner, wantCode: http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeCalendarRepo{
				getDefaultFn:     func(context.Context, string) (*Calendar, error) { return tc.def, nil },
				listByCampaignFn: func(context.Context, string) ([]Calendar, error) { return append([]Calendar(nil), tc.list...), nil },
			}
			svc := newTestCalendarService(repo, nil, nil, nil)
			got, err := svc.GetPrimaryCalendarForViewer(context.Background(), testCampaignA, tc.viewer)
			if tc.wantCode != 0 {
				if err == nil {
					t.Fatalf("expected %d, got %+v", tc.wantCode, got)
				}
				if code := appErrCode(t, err); code != tc.wantCode {
					t.Errorf("code = %d, want %d", code, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ID != tc.wantID {
				t.Errorf("calendar = %q, want %q", got.ID, tc.wantID)
			}
		})
	}
}

func TestValidateImportCurrentTime(t *testing.T) {
	tests := []struct {
		name         string
		hoursPerDay  int
		minutesPerHr int
		hour, minute int
		wantErr      bool
	}{
		{"defaults accept 23:59", 0, 0, 23, 59, false},
		{"defaults reject hour 24", 0, 0, 24, 0, true},
		{"defaults reject minute 60", 0, 0, 0, 60, true},
		{"negative hour", 24, 60, -1, 0, true},
		{"negative minute", 24, 60, 0, -1, true},
		{"custom day length accepts its last hour", 10, 100, 9, 99, false},
		{"custom day length rejects hour 10", 10, 100, 10, 0, true},
		{"custom hour length rejects minute 100", 10, 100, 0, 100, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ir := &ImportResult{
				Settings: ImportedSettings{HoursPerDay: tc.hoursPerDay, MinutesPerHour: tc.minutesPerHr},
				Today:    ImportedToday{Hour: tc.hour, Minute: tc.minute},
			}
			err := validateImportCurrentTime(ir)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && appErrCode(t, err) != http.StatusUnprocessableEntity {
				t.Errorf("code = %d, want 422", appErrCode(t, err))
			}
		})
	}
}

func repeatJSON(item string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			out += ","
		}
		out += item
	}
	return out
}

// withTime swaps the payload's time of day without hand-editing the JSON in
// every case.
func withTime(payload string, hour, minute int) string {
	r := payload
	r = strings.Replace(r, `"current_hour":13`, `"current_hour":`+strconv.Itoa(hour), 1)
	return strings.Replace(r, `"current_minute":45`, `"current_minute":`+strconv.Itoa(minute), 1)
}
