package htmlconv

import "testing"

func TestConvert_Roller(t *testing.T) {
	tests := []struct {
		name, in string
		table    string
		count    float64
	}{
		{"table and count", `<div class="ce-roll ce-roll--t-tavern-names ce-roll--n-5"></div>`, "tavern-names", 5},
		{"no count is 3", `<div class="ce-roll ce-roll--t-rumours"></div>`, "rumours", 3},
		{"a count the editor lacks is 3", `<div class="ce-roll ce-roll--t-rumours ce-roll--n-4"></div>`, "rumours", 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Convert(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			n := firstChildOfDoc(t, out)
			attrs, _ := n["attrs"].(map[string]any)
			if n["type"] != "rollTable" || attrs["table"] != tc.table || attrs["count"] != tc.count {
				t.Errorf("got %s", out)
			}
		})
	}
	for _, in := range []string{`<div class="ce-roll"></div>`, `<div class="ce-roll ce-roll--t-Bad_Id"></div>`, `<div class="other ce-roll--t-x"></div>`} {
		out, err := Convert(in)
		if err != nil {
			t.Fatal(err)
		}
		if n := firstChildOfDoc(t, out); n["type"] == "rollTable" {
			t.Errorf("%s became a roller: %s", in, out)
		}
	}
}
