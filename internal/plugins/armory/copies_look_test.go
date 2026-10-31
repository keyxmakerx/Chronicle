package armory

import (
	"strings"
	"testing"
)

func TestCraftLookClean(t *testing.T) {
	good := &CraftLook{Shape: "potion", Body: "#C3223A", Trim: "#9a7650", Detail: "#f4ecd6", Mark: &CraftMark{Picture: "leaf", Tint: "#9fc7e8"}}
	cases := []struct {
		name     string
		in       *CraftLook
		wantNil  bool
		wantMark bool
	}{
		{"good look kept, colours lower-cased", good, false, true},
		{"nil", nil, true, false},
		{"unknown shape", &CraftLook{Shape: "castle", Body: "#000000", Trim: "#000000", Detail: "#000000"}, true, false},
		{"bad colour", &CraftLook{Shape: "ring", Body: "red", Trim: "#000000", Detail: "#000000"}, true, false},
		{"markup in a colour", &CraftLook{Shape: "ring", Body: `#000000" onload="x`, Trim: "#000000", Detail: "#000000"}, true, false},
		{"bad mark dropped on its own", &CraftLook{Shape: "ring", Body: "#000000", Trim: "#000000", Detail: "#000000", Mark: &CraftMark{Picture: `"><x`, Tint: "#ffffff"}}, false, false},
	}
	for _, c := range cases {
		got := c.in.clean()
		if (got == nil) != c.wantNil {
			t.Errorf("%s: clean() = %+v, want nil %v", c.name, got, c.wantNil)
			continue
		}
		if got != nil && (got.Mark != nil) != c.wantMark {
			t.Errorf("%s: mark %+v, want one %v", c.name, got.Mark, c.wantMark)
		}
	}
	if g := good.clean(); g.Body != "#c3223a" {
		t.Errorf("colours are stored lower-case, got %q", g.Body)
	}
}

func TestMemoryKeepsALookAcrossStorage(t *testing.T) {
	m := Memory{Look: &CraftLook{Shape: "blade", Body: "#8c96a3", Trim: "#c9a24a", Detail: "#7a4a2a"}}
	if m.Empty() {
		t.Fatal("a copy with only a look is not plain")
	}
	back := decodeMemory(encodeMemory(m))
	if back.Look == nil || back.Look.Shape != "blade" {
		t.Fatalf("look lost in storage: %+v", back.Look)
	}
	if LookJSON(nil) != "" || LookJSON(&CraftLook{Shape: "x"}) != "" {
		t.Error("no look prints no attribute")
	}
	j := FirstLookJSON([]Copy{{Memory: Memory{Quality: "fine"}}, {Memory: m}})
	if !strings.Contains(j, `"shape":"blade"`) {
		t.Errorf("FirstLookJSON = %s", j)
	}
}
