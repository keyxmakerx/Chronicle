package app

import (
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func TestPurseFields(t *testing.T) {
	num := func(keys ...string) []entities.FieldDefinition {
		var out []entities.FieldDefinition
		for _, k := range keys {
			out = append(out, entities.FieldDefinition{Key: k, Type: "number"})
		}
		return out
	}
	tests := []struct {
		name   string
		fields []entities.FieldDefinition
		want   map[string]string
	}{
		{"gold only has no purse", num("gp", "hp"), nil},
		{"gp with silver and copper", num("gp", "sp", "cp"), map[string]string{"gp": "gp", "sp": "sp", "cp": "cp"}},
		{"all five coins", num("cp", "sp", "ep", "gp", "pp"), map[string]string{"cp": "cp", "sp": "sp", "ep": "ep", "gp": "gp", "pp": "pp"}},
		{"silver without gold has no purse", num("sp", "cp"), nil},
		{"a text coin field does not count", append(num("gp"), entities.FieldDefinition{Key: "sp", Type: "text"}), nil},
		{"wealth sheet", num("wealth"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := purseFields(tt.fields); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("purseFields = %v, want %v", got, tt.want)
			}
		})
	}
}
