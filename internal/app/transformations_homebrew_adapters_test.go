package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

func TestTransformationPartChecks(t *testing.T) {
	reg := homebrew.NewRegistry()
	if err := registerTransformationParts(reg); err != nil {
		t.Fatal(err)
	}
	svc := homebrew.NewService(&memRecipes{stored: map[string]homebrew.Recipe{}}, reg, nil, nil)
	tests := []struct {
		name string
		data string
		want string // normalised data; empty means refused
	}{
		{"a layer is kept, trimmed and compacted",
			`{"name":" Thrall ","source":"homebrew","note":"","changes":[{"field":"weaknesses","op":"set","value":{ "holy" : 3 }}]}`,
			`{"name":"Thrall","source":"homebrew","note":"","changes":[{"field":"weaknesses","op":"set","value":{"holy":3}}]}`},
		{"no changes refused", `{"name":"Thrall","source":"","note":"","changes":[]}`, ""},
		{"a bad op refused", `{"name":"Thrall","source":"","note":"","changes":[{"field":"speed","op":"explode","value":1}]}`, ""},
		{"a bad field refused", `{"name":"Thrall","source":"","note":"","changes":[{"field":"Speed!","op":"add","value":1}]}`, ""},
		{"an unknown key refused", `{"name":"Thrall","changes":[{"field":"speed","op":"add","value":1}],"script":"x"}`, ""},
		{"no name refused", `{"name":"","changes":[{"field":"speed","op":"add","value":1}]}`, ""},
		{"empty data refused", ``, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := homebrew.Part{Key: "layer", Kind: "transformation", Label: "Thrall", See: homebrew.SeeParty}
			if tt.data != "" {
				p.Data = []byte(tt.data)
			}
			r, err := saveDoc(t, svc, homebrew.Doc{Parts: []homebrew.Part{p}})
			if tt.want == "" {
				if hbCode(err) != 422 {
					t.Fatalf("err = %v, want 422", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := string(r.Doc.Parts[0].Data); got != tt.want {
				t.Errorf("data = %s, want %s", got, tt.want)
			}
			if r.Doc.Parts[0].See != "director" {
				t.Errorf("see = %q, want director: a preset is the Director's", r.Doc.Parts[0].See)
			}
		})
	}
	cat := reg.Catalog()
	found := false
	for _, a := range cat.Actions {
		if a.Name == "put-on-transformation" {
			found = true
			if !a.ClientRun {
				t.Error("putting a preset on must wait for the Director in the browser, never run on the server")
			}
		}
	}
	if !found {
		t.Error("put-on-transformation is not in the catalog")
	}
	if err := registerTransformationParts(reg); err == nil {
		t.Error("registering twice succeeded")
	}
}
