package entities

import "testing"

// The layout example's Attributes block shows the category's own fields with a
// value of each field's kind, so a Character layout is not drawn with a town's.
func TestExampleFields(t *testing.T) {
	fields := []FieldDefinition{
		{Label: "Age", Type: "number"},
		{Label: "Alignment", Type: "select", Options: []string{"Neutral", "Evil"}},
		{Label: "", Type: "text"},
		{Label: "Notes", Type: "textarea"},
		{Label: "Home", Type: "link"},
		{Label: "Alive", Type: "checkbox"},
		{Label: "Born", Type: "calendar_date"},
		{Label: "Site", Type: "url"},
	}
	got := exampleFields(fields)
	if len(got) != maxExampleFields {
		t.Fatalf("shows %d rows, want %d", len(got), maxExampleFields)
	}
	want := [][2]string{{"Age", "12"}, {"Alignment", "Neutral"}, {"Notes", "A few lines of notes"}, {"Home", "A linked page"}, {"Alive", "Yes"}, {"Born", "14 Frost, 1492"}}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("row %d = %v, want %v", i, got[i], w)
		}
	}
	if n := len(exampleFields(nil)); n != 0 {
		t.Errorf("a category with no fields shows %d rows", n)
	}
}
