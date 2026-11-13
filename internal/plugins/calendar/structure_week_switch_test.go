// structure_week_switch_test.go covers the "each month starts a new week"
// switch on the structure save: previewed in words, part of the fingerprint,
// written with the rest of the structure, and left alone when the submission
// does not mention it.
package calendar

import (
	"context"
	"strings"
	"testing"
)

func TestStructureEdit_WeekSwitch(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name     string
		stored   bool
		sent     *bool
		wantSave bool
		wantNote string
	}{
		{"turned on", false, &yes, true, "Weekdays will restart at the start of every month"},
		{"turned off", true, &no, false, "run on through month and festival boundaries"},
		{"on stays on, no note", true, &yes, true, ""},
		{"off stays off, no note", false, &no, false, ""},
		{"not mentioned keeps off", false, nil, false, ""},
		{"not mentioned keeps on", true, nil, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cal := structureFixture()
			cal.MonthStartsNewWeek = tt.stored
			edit := editFrom(cal)
			edit.MonthStartsNewWeek = tt.sent

			plan := planStructureEdit(cal, nil, nil, nil, nil, edit)
			if plan.monthStartsNewWeek != tt.wantSave {
				t.Errorf("stored switch = %v, want %v", plan.monthStartsNewWeek, tt.wantSave)
			}
			joined := strings.Join(plan.preview.OtherNotes, "\n")
			if tt.wantNote == "" && strings.Contains(joined, "Weekdays will") {
				t.Errorf("an unchanged switch must not be previewed: %q", joined)
			}
			if tt.wantNote != "" && !strings.Contains(joined, tt.wantNote) {
				t.Errorf("preview notes %q missing %q", joined, tt.wantNote)
			}
			if got := structureWriteFor(edit, plan).MonthStartsNewWeek; got != tt.wantSave {
				t.Errorf("write carries %v, want %v", got, tt.wantSave)
			}
		})
	}
}

// A calendar that flips the switch between preview and save must be a
// conflict, because every weekly repeat would land somewhere else.
func TestStructureFingerprint_FollowsWeekSwitch(t *testing.T) {
	cal := structureFixture()
	a := structureFingerprint(cal, nil, nil, nil, nil)
	cal.MonthStartsNewWeek = true
	if b := structureFingerprint(cal, nil, nil, nil, nil); a == b {
		t.Error("fingerprint ignores the week switch")
	}
}

func TestApplyStructureEdit_WritesWeekSwitch(t *testing.T) {
	cal := structureFixture()
	var wrote StructureWrite
	calls := 0
	svc := structureService(cal, nil, &wrote, &calls)
	on := true
	edit := editFrom(cal)
	edit.MonthStartsNewWeek = &on

	ctx := context.Background()
	preview, err := svc.PreviewStructureEdit(ctx, cal.ID, cal.CampaignID, edit)
	if err != nil {
		t.Fatalf("PreviewStructureEdit: %v", err)
	}
	if _, err := svc.ApplyStructureEdit(ctx, cal.ID, cal.CampaignID, preview.Fingerprint, edit); err != nil {
		t.Fatalf("ApplyStructureEdit: %v", err)
	}
	if !wrote.MonthStartsNewWeek {
		t.Error("the save did not carry the switch")
	}
}

func TestEditorStateCarriesWeekSwitch(t *testing.T) {
	cal := structureFixture()
	cal.MonthStartsNewWeek = true
	st := calendarEditorState(cal)
	if !st.NewWeek {
		t.Fatal("editor state lost the stored switch")
	}
	x := structureEditorXData(st, structurePreviewSlot)
	for _, want := range []string{"newWeek: true", "month_starts_new_week:!!this.newWeek"} {
		if !strings.Contains(x, want) {
			t.Errorf("x-data missing %q", want)
		}
	}
	if x := structureEditorXData(newCalendarEditorState(), ""); !strings.Contains(x, "newWeek: false") {
		t.Error("a new calendar's editor must start with the switch off")
	}
}

// What the editor submits survives the same parse a wizard build and a
// structure save both use: an explicit off is still "off", a missing key is
// still "not said".
func TestParseImportJSON_KeepsWeekSwitch(t *testing.T) {
	base := func(settings string) string {
		return `{"format":"built","calendar_name":"x","months":[{"name":"A","days":30}],"weekdays":[{"name":"D"}],"settings":{` + settings + `}}`
	}
	tests := []struct {
		name     string
		settings string
		want     *bool
	}{
		{"on", `"month_starts_new_week":true`, ptr(true)},
		{"explicit off", `"month_starts_new_week":false`, ptr(false)},
		{"absent", `"mode":"fantasy"`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ir, err := parseWizardImportJSON(base(tt.settings))
			if err != nil {
				t.Fatal(err)
			}
			got := ir.Settings.MonthStartsNewWeek
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Fatalf("settings flag = %v, want %v", got, tt.want)
			}
			if edit := StructureEditFromImport(ir); (edit.MonthStartsNewWeek == nil) != (tt.want == nil) {
				t.Errorf("structure edit flag = %v, want %v", edit.MonthStartsNewWeek, tt.want)
			}
		})
	}
}

// The preset is the one calendar that ships with the switch on.
func TestHarptosPresetStartsMonthsOnANewWeek(t *testing.T) {
	ir, err := LoadPreset("harptos")
	if err != nil {
		t.Fatal(err)
	}
	if ir.Settings.MonthStartsNewWeek == nil || !*ir.Settings.MonthStartsNewWeek {
		t.Fatal("harptos preset must carry month_starts_new_week: true")
	}
	for _, name := range []string{"blank", "dwarven", "elven"} {
		other, err := LoadPreset(name)
		if err != nil {
			t.Fatal(err)
		}
		if other.Settings.MonthStartsNewWeek != nil && *other.Settings.MonthStartsNewWeek {
			t.Errorf("only Harptos starts with the switch on, %s does too", name)
		}
	}
}
