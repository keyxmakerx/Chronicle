// era_look_test.go covers the era look fields: their validation on every
// write path, partial updates (absent keeps, null clears), the calendar-wide
// SaveEraLook, and that a calendar-format export carries them through a
// re-import.
package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

func lookCalRepo(stored []Era) *fakeCalendarRepo {
	return &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA}, nil
		},
		getEraByIDFn: func(_ context.Context, id int) (*Era, error) {
			for i := range stored {
				if stored[i].ID == id {
					e := stored[i]
					return &e, nil
				}
			}
			return nil, nil
		},
		getErasFn: func(context.Context, string) ([]Era, error) { return append([]Era(nil), stored...), nil },
	}
}

func appCode(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

func TestCreateEra_LookFieldValidation(t *testing.T) {
	base := func() EraInput {
		return EraInput{Name: "Age", StartYear: 1, StartMonth: 1, StartDay: 1, Color: "#123456"}
	}
	tests := []struct {
		name     string
		mutate   func(*EraInput)
		gate     EntityVisibilityGate
		wantCode int
	}{
		{"defaults are fine", func(*EraInput) {}, nil, 0},
		{"ink with a second colour and own feel", func(e *EraInput) {
			e.Style, e.Color2, e.Feel = EraStyleInk, strp("#abc"), strp(EraFeelLively)
		}, nil, 0},
		{"unknown style", func(e *EraInput) { e.Style = "smoke" }, nil, 400},
		{"second colour not hex", func(e *EraInput) { e.Color2 = strp("red;}") }, nil, 400},
		{"custom is not an era's own feel", func(e *EraInput) { e.Feel = strp(EraFeelCustom) }, nil, 400},
		{"lore page with no gate wired", func(e *EraInput) { e.LoreEntityID = strp("ent-1") }, nil, 400},
		{"lore page outside the campaign", func(e *EraInput) { e.LoreEntityID = strp("ent-other") },
			fakeEntityGate{allowed: map[string]bool{"ent-1": true}}, 400},
		{"lore page in the campaign", func(e *EraInput) { e.LoreEntityID = strp("ent-1") },
			fakeEntityGate{allowed: map[string]bool{"ent-1": true}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var created EraInput
			repo := lookCalRepo(nil)
			repo.createEraFn = func(_ context.Context, _ string, in EraInput) (*Era, error) {
				created = in
				return &Era{ID: 1}, nil
			}
			svc := newTestCalendarService(repo, nil, nil, nil)
			if tt.gate != nil {
				svc.(*calendarService).SetEntityVisibilityGate(tt.gate)
			}
			in := base()
			tt.mutate(&in)
			_, err := svc.CreateEra(context.Background(), "cal-1", testCampaignA, in)
			if got := appCode(err); got != tt.wantCode || (tt.wantCode == 0 && err != nil) {
				t.Fatalf("code = %d (err %v), want %d", got, err, tt.wantCode)
			}
			if tt.wantCode == 0 && created.Style == "" {
				t.Error("style should default to gas")
			}
		})
	}
}

func TestUpdateEra_LookFieldsArePartial(t *testing.T) {
	stored := Era{ID: 7, CalendarID: "cal-1", Name: "Age", StartYear: 1, StartMonth: 1, StartDay: 1, Color: "#111111",
		Color2: strp("#222222"), Style: EraStyleInk, Feel: strp(EraFeelLively), LoreEntityID: strp("ent-1"),
		DMNote: strp("secret"), HiddenUntilBegins: true}
	tests := []struct {
		name  string
		input UpdateEraInput
		check func(t *testing.T, got EraInput)
	}{
		{"absent keeps everything (and does not recheck the lore page)", UpdateEraInput{Name: "Age"}, func(t *testing.T, got EraInput) {
			if got.Style != EraStyleInk || *got.Color2 != "#222222" || *got.Feel != EraFeelLively ||
				*got.LoreEntityID != "ent-1" || *got.DMNote != "secret" || !got.HiddenUntilBegins {
				t.Errorf("fields changed: %+v", got)
			}
		}},
		{"null clears the note, second colour, feel and lore page", UpdateEraInput{Name: "Age",
			DMNote: patch.Null[string](), Color2: patch.Null[string](), Feel: patch.Null[string](), LoreEntityID: patch.Null[string]()},
			func(t *testing.T, got EraInput) {
				if got.DMNote != nil || got.Color2 != nil || got.Feel != nil || got.LoreEntityID != nil {
					t.Errorf("null should clear: %+v", got)
				}
				if got.Style != EraStyleInk {
					t.Errorf("absent style should keep ink, got %q", got.Style)
				}
			}},
		{"present replaces", UpdateEraInput{Name: "Age", Style: patch.Of(EraStyleGas), HiddenUntilBegins: patch.Of(false),
			DMNote: patch.Of("new")}, func(t *testing.T, got EraInput) {
			if got.Style != EraStyleGas || got.HiddenUntilBegins || *got.DMNote != "new" {
				t.Errorf("present should replace: %+v", got)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got EraInput
			repo := lookCalRepo([]Era{stored})
			repo.updateEraFn = func(_ context.Context, _ string, _ int, in EraInput) error {
				got = in
				return nil
			}
			// No entity gate: a lore check would fail closed, so passing
			// proves an absent lore id is not rechecked.
			svc := newTestCalendarService(repo, nil, nil, nil)
			if err := svc.UpdateEra(context.Background(), 7, "cal-1", testCampaignA, tt.input); err != nil {
				t.Fatal(err)
			}
			tt.check(t, got)
		})
	}

	t.Run("a bad style is refused", func(t *testing.T) {
		svc := newTestCalendarService(lookCalRepo([]Era{stored}), nil, nil, nil)
		err := svc.UpdateEra(context.Background(), 7, "cal-1", testCampaignA, UpdateEraInput{Name: "Age", Style: patch.Of("fog")})
		if appCode(err) != 400 {
			t.Errorf("got %v, want 400", err)
		}
	})
}

func TestSaveEraLook(t *testing.T) {
	stored := []Era{{ID: 1, CalendarID: "cal-1", Color: "#111111", Color2: strp("#222222"), Style: EraStyleGas}}
	okLook := EraLook{ColorsOn: true, Feel: EraFeelCustom, Intensity: 1.4, Speed: 0.3}
	tests := []struct {
		name     string
		look     EraLook
		eras     []EraLookEra
		wantCode int
	}{
		{"custom look, partial era", okLook, []EraLookEra{{ID: 1, Style: patch.Of(EraStyleInk)}}, 0},
		{"unknown feel", EraLook{Feel: "wild", Intensity: 1, Speed: 1}, nil, 400},
		{"intensity too high", EraLook{Feel: EraFeelCustom, Intensity: 3.5, Speed: 1}, nil, 400},
		{"negative speed", EraLook{Feel: EraFeelCustom, Intensity: 1, Speed: -0.1}, nil, 400},
		{"an era from another calendar", okLook, []EraLookEra{{ID: 99, Color: patch.Of("#000000")}}, 404},
		{"bad era colour", okLook, []EraLookEra{{ID: 1, Color: patch.Of("url(x)")}}, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wrote []EraLookWrite
			repo := lookCalRepo(stored)
			repo.saveEraLookFn = func(_ context.Context, _ string, _ EraLook, eras []EraLookWrite) error {
				wrote = eras
				return nil
			}
			svc := newTestCalendarService(repo, nil, nil, nil)
			err := svc.SaveEraLook(context.Background(), "cal-1", testCampaignA, tt.look, tt.eras)
			if got := appCode(err); got != tt.wantCode || (tt.wantCode == 0 && err != nil) {
				t.Fatalf("code = %d (err %v), want %d", got, err, tt.wantCode)
			}
			if tt.wantCode == 0 {
				w := wrote[0]
				if w.Style != EraStyleInk || w.Color != "#111111" || w.Color2 == nil || *w.Color2 != "#222222" {
					t.Errorf("absent fields should keep the stored look: %+v", w)
				}
			}
		})
	}
}

func TestEraLook_ExportRoundTrip(t *testing.T) {
	cal := &Calendar{ID: "cal-1", Name: "Reckoning", Mode: ModeFantasy, HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
		Months: []Month{{Name: "One", Days: 30}}, Weekdays: []Weekday{{Name: "Day"}},
		EraLook: EraLook{ColorsOn: false, Feel: EraFeelCustom, Intensity: 2.2, Speed: 0.4},
		Eras: []Era{{Name: "Age", StartYear: 1, StartMonth: 1, StartDay: 1, Color: "#111111", Color2: strp("#222222"),
			Style: EraStyleInk, Feel: strp(EraFeelStill), LoreEntityID: strp("ent-1"), DMNote: strp("note"), HiddenUntilBegins: true}},
	}
	data, err := json.Marshal(BuildExport(cal, nil, false))
	if err != nil {
		t.Fatal(err)
	}
	res, err := parseChronicle(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Eras) != 1 {
		t.Fatalf("eras = %+v", res.Eras)
	}
	e := res.Eras[0]
	if e.Style != EraStyleInk || e.Color2 == nil || *e.Color2 != "#222222" || e.Feel == nil || *e.Feel != EraFeelStill ||
		e.DMNote == nil || *e.DMNote != "note" || !e.HiddenUntilBegins {
		t.Errorf("era look lost in the round trip: %+v", e)
	}
	// A lore page id belongs to one campaign; it never travels in a calendar file.
	if e.LoreEntityID != nil {
		t.Errorf("lore page id should not survive a calendar-format round trip")
	}
	if l := res.Settings.EraLook; l == nil || l.ColorsOn || l.Feel != EraFeelCustom || l.Intensity != 2.2 || l.Speed != 0.4 {
		t.Errorf("calendar look lost in the round trip: %+v", res.Settings.EraLook)
	}
}
