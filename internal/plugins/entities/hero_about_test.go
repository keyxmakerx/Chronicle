package entities

import (
	"strings"
	"testing"
)

func TestHeroAbout(t *testing.T) {
	tests := []struct {
		name   string
		fields []FieldDefinition
		want   []HeroAboutField
	}{
		{"no type fields, no step", nil, nil},
		{
			"asks in its own order, long boxes for the stories",
			[]FieldDefinition{
				{Key: "backstory", Label: "Backstory"},
				{Key: "pronouns", Label: "Pronouns"},
				{Key: "level", Label: "Level"},
				{Key: "appearance", Label: "What they look like"},
			},
			[]HeroAboutField{
				{Key: "pronouns", Label: "Pronouns"},
				{Key: "appearance", Label: "What they look like", Long: true},
				{Key: "backstory", Label: "Backstory", Long: true},
			},
		},
		{
			"a GM-only field is never asked",
			[]FieldDefinition{{Key: "gender", Label: "Gender", GMOnly: true}, {Key: "pronouns"}},
			[]HeroAboutField{{Key: "pronouns", Label: "pronouns"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := heroAbout(&EntityType{Fields: tt.fields})
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("box %d: got %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
	if heroAbout(nil) != nil {
		t.Fatal("nil type should have no step")
	}
}

func TestHeroAboutValues(t *testing.T) {
	offered := []HeroAboutField{
		{Key: "pronouns", Label: "Pronouns"},
		{Key: "backstory", Label: "Backstory", Long: true},
	}
	tests := []struct {
		name    string
		sent    map[string]string
		want    map[string]any
		wantErr string
	}{
		{"nothing sent", nil, nil, ""},
		{"trims and skips empty boxes", map[string]string{"pronouns": "  they/them ", "backstory": "   "}, map[string]any{"pronouns": "they/them"}, ""},
		{"a box not offered is refused", map[string]string{"level": "20"}, nil, "not asked for"},
		{"a short box over 200 is refused", map[string]string{"pronouns": strings.Repeat("a", 201)}, nil, "too long"},
		{"a long box takes a long story", map[string]string{"backstory": strings.Repeat("a", 5000)}, map[string]any{"backstory": strings.Repeat("a", 5000)}, ""},
		{"a long box over 20000 is refused", map[string]string{"backstory": strings.Repeat("a", 20001)}, nil, "too long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := heroAboutValues(offered, tt.sent)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Fatalf("%s: got %v, want %v", k, got[k], v)
				}
			}
		})
	}
}
