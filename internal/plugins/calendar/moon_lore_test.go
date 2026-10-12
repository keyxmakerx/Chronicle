package calendar

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// moonLoreFixture is two calendars in one campaign whose moons point at
// pages. Both calendars have a single 20-day month and sit in year 0, where
// the day counter is just the day of the month, so a 20-day cycle with no
// offset reads day 10 as 0.5 (Full Moon), day 5 as 0.25 (First Quarter) and
// day 20 as a whole turn (New Moon).
func moonLoreFixture() *calendarService {
	months := []Month{{Name: "Only", Days: 20}}
	cals := []Calendar{
		{ID: "cal-a", CampaignID: "camp", Name: "Common reckoning", Mode: ModeFantasy, Visibility: "everyone",
			CurrentYear: 0, CurrentMonth: 1, CurrentDay: 10},
		{ID: "cal-b", CampaignID: "camp", Name: "Secret reckoning", Mode: ModeFantasy, Visibility: "dm_only",
			CurrentYear: 0, CurrentMonth: 1, CurrentDay: 5},
		{ID: "cal-c", CampaignID: "camp", Name: "Quiet reckoning", Mode: ModeFantasy, Visibility: "everyone",
			CurrentYear: 0, CurrentMonth: 1, CurrentDay: 20},
	}
	moons := map[string][]Moon{
		"cal-a": {
			{ID: 1, CalendarID: "cal-a", Name: "Luna", CycleDays: 20, LoreEntityID: strp("ent-moon")},
			{ID: 2, CalendarID: "cal-a", Name: "Veiled", CycleDays: 20, HiddenFromPlayers: true, LoreEntityID: strp("ent-moon")},
			{ID: 3, CalendarID: "cal-a", Name: "Other", CycleDays: 20, LoreEntityID: strp("ent-other")},
			{ID: 4, CalendarID: "cal-a", Name: "Unlinked", CycleDays: 20},
		},
		"cal-b": {
			{ID: 5, CalendarID: "cal-b", Name: "Dusk", CycleDays: 20, LoreEntityID: strp("ent-moon")},
		},
		"cal-c": {
			{ID: 6, CalendarID: "cal-c", Name: "Elsewhere", CycleDays: 20, LoreEntityID: strp("ent-other")},
		},
	}
	repo := &fakeCalendarRepo{
		listByCampaignFn: func(context.Context, string) ([]Calendar, error) {
			return append([]Calendar(nil), cals...), nil
		},
		getMoonsFn: func(_ context.Context, id string) ([]Moon, error) {
			return append([]Moon(nil), moons[id]...), nil
		},
		getMonthsFn: func(context.Context, string) ([]Month, error) { return months, nil },
	}
	return &calendarService{calRepo: repo}
}

func TestMoonsForEntity(t *testing.T) {
	tests := []struct {
		name   string
		role   int
		entity string
		// want is "moon|calendar|phase" per chip, in calendar order.
		want []string
	}{
		{"player sees only the open moon on the open calendar", permissions.RolePlayer, "ent-moon",
			[]string{"Luna|Common reckoning|Full Moon"}},
		{"owner also sees the hidden moon and the Directors-only calendar", permissions.RoleOwner, "ent-moon",
			[]string{"Luna|Common reckoning|Full Moon", "Veiled|Common reckoning|Full Moon", "Dusk|Secret reckoning|First Quarter"}},
		{"phase is read on each calendar's own date", permissions.RolePlayer, "ent-other",
			[]string{"Other|Common reckoning|Full Moon", "Elsewhere|Quiet reckoning|New Moon"}},
		{"a page that is no moon's lore has no chips", permissions.RolePlayer, "ent-nothing", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := moonLoreFixture().MoonsForEntity(context.Background(), "camp", tt.entity, permissions.RequestViewer(tt.role, "u1"))
			if err != nil {
				t.Fatal(err)
			}
			var lines []string
			for _, ref := range got {
				lines = append(lines, ref.MoonName+"|"+ref.CalendarName+"|"+ref.PhaseName)
			}
			if strings.Join(lines, ";") != strings.Join(tt.want, ";") {
				t.Errorf("chips = %v, want %v", lines, tt.want)
			}
		})
	}
}

func TestMoonsForEntity_CarriesIDs(t *testing.T) {
	got, err := moonLoreFixture().MoonsForEntity(context.Background(), "camp", "ent-moon", permissions.RequestViewer(permissions.RolePlayer, "u1"))
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
	if got[0].MoonID != 1 || got[0].CalendarID != "cal-a" {
		t.Errorf("ids = moon %d calendar %s, want 1 and cal-a", got[0].MoonID, got[0].CalendarID)
	}
}

func TestRedactMoonLore(t *testing.T) {
	mk := func() []Moon {
		return []Moon{
			{ID: 1, LoreEntityID: strp("ent-open"), LoreEntityName: "Open page"},
			{ID: 2, LoreEntityID: strp("ent-hidden"), LoreEntityName: "Hidden page"},
			{ID: 3},
		}
	}
	tests := []struct {
		name     string
		gate     EntityVisibilityGate
		role     int
		wantKept []bool
	}{
		{"player: only the page they may see", fakeEntityGate{allowed: map[string]bool{"ent-open": true}}, permissions.RolePlayer, []bool{true, false, false}},
		{"no gate wired: fail closed", nil, permissions.RolePlayer, []bool{false, false, false}},
		{"owner: untouched", nil, permissions.RoleOwner, []bool{true, true, false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &calendarService{entityGate: tt.gate}
			moons := mk()
			if err := s.redactMoonLore(context.Background(), "c", moons, permissions.RequestViewer(tt.role, "u1")); err != nil {
				t.Fatal(err)
			}
			for i, keep := range tt.wantKept {
				if (moons[i].LoreEntityID != nil) != keep || (moons[i].LoreEntityName != "") != keep {
					t.Errorf("moon %d lore kept = %v, want %v", i+1, moons[i].LoreEntityID != nil, keep)
				}
			}
		})
	}
}

// A structure save may only point a moon at a page in this campaign, and
// only a wizard-style round trip is stripped of pages.
func TestCheckMoonLore(t *testing.T) {
	tests := []struct {
		name    string
		gate    EntityVisibilityGate
		moons   []MoonInput
		wantErr bool
	}{
		{"no page named", fakeEntityGate{}, []MoonInput{{Name: "Luna"}}, false},
		{"cleared page needs no check", fakeEntityGate{}, []MoonInput{{Name: "Luna", LoreSet: true}}, false},
		{"a page in the campaign", fakeEntityGate{allowed: map[string]bool{"ent-ok": true}},
			[]MoonInput{{Name: "Luna", LoreSet: true, LoreEntityID: strp("ent-ok")}}, false},
		{"a page elsewhere", fakeEntityGate{allowed: map[string]bool{"ent-ok": true}},
			[]MoonInput{{Name: "Luna", LoreSet: true, LoreEntityID: strp("ent-else")}}, true},
		{"a new moon's page is checked too", fakeEntityGate{},
			[]MoonInput{{Name: "New", LoreEntityID: strp("ent-else")}}, true},
		{"no gate wired: refuses rather than store an unchecked id", nil,
			[]MoonInput{{Name: "Luna", LoreSet: true, LoreEntityID: strp("ent-ok")}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &calendarService{entityGate: tt.gate}
			err := s.checkMoonLore(context.Background(), "camp", tt.moons)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMoonLoreImportJSON(t *testing.T) {
	raw, err := json.Marshal(ImportResult{
		Format:   FormatBuilt,
		Months:   []MonthInput{{Name: "Only", Days: 20}},
		Weekdays: []WeekdayInput{{Name: "Day"}},
		Moons: []MoonInput{
			{Name: "Luna", CycleDays: 20, LoreEntityID: strp("ent-1"), LoreSet: true},
			{Name: "Plain", CycleDays: 20},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("the structure editor keeps a moon's page", func(t *testing.T) {
		ir, err := parseImportJSON(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		if ir.Moons[0].LoreEntityID == nil || !ir.Moons[0].LoreSet || ir.Moons[1].LoreSet {
			t.Errorf("moons = %+v", ir.Moons)
		}
		edit := StructureEditFromImport(ir)
		if edit.Moons[0].LoreEntityID == nil || *edit.Moons[0].LoreEntityID != "ent-1" {
			t.Errorf("edit lost the page: %+v", edit.Moons[0])
		}
	})
	t.Run("a new calendar's wizard round trip drops it", func(t *testing.T) {
		ir, err := parseWizardImportJSON(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ir.Moons {
			if m.LoreEntityID != nil || m.LoreSet {
				t.Errorf("moon %s kept a page: %+v", m.Name, m)
			}
		}
	})
}

// The editor sends a moon's page only once the owner touched the picker (or
// for a moon it has just added), so a save that never touched it cannot
// overwrite a page changed elsewhere, and the row prefills from the stored one.
func TestStructureEditorMoonLore(t *testing.T) {
	cal := &Calendar{CampaignID: "camp", Moons: []Moon{
		{ID: 7, Name: "Luna", CycleDays: 29.5, LoreEntityID: strp("ent-1"), LoreEntityName: "Luna, mother of tides"},
		{ID: 8, Name: "Bare", CycleDays: 10},
	}}
	st := calendarEditorState(cal)
	if st.CampaignID != "camp" || st.Moons[0].Lore != "ent-1" || st.Moons[0].LoreName != "Luna, mother of tides" || st.Moons[1].Lore != "" {
		t.Fatalf("state = %+v", st)
	}
	js := structureEditorXData(st, "slot")
	for _, want := range []string{`"lore":"ent-1"`, `"loreName":"Luna, mother of tides"`, `campaignId: "camp"`, `mo.loreDirty||(!mo.id&&mo.lore)`, `o.lore_set=true`} {
		if !strings.Contains(js, want) {
			t.Errorf("editor state lacks %q", want)
		}
	}
}
