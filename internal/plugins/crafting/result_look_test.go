package crafting

import "testing"

func TestLookShape(t *testing.T) {
	cases := []struct {
		name string
		own  *MaterialPicture
		item string
		want string
	}{
		{"own picture wins", &MaterialPicture{Picture: "vial", Tint: "#123456"}, "Iron Sword", "vial"},
		{"own picture with no shape falls to the name", &MaterialPicture{Picture: "leaf", Tint: "#123456"}, "Healing Potion", "potion"},
		{"shield before blade", nil, "Swordbreaker Buckler", "shield"},
		{"blade", nil, "Ember Shortsword", "blade"},
		{"ring", nil, "Ring of Clear Water", "ring"},
		{"vial", nil, "Frost Tonic", "vial"},
		{"case-blind", nil, "ELIXIR OF LIFE", "potion"},
		{"anything else", nil, "Ranger's Kit", "bundle"},
	}
	for _, c := range cases {
		if got := lookShape(c.own, c.item); got != c.want {
			t.Errorf("%s: lookShape(%q) = %q, want %q", c.name, c.item, got, c.want)
		}
	}
}

func TestComposeLook(t *testing.T) {
	moss := MaterialPicture{Picture: "moss", Tint: "#c3223a"}
	leaf := MaterialPicture{Picture: "leaf", Tint: "#9fc7e8"}
	salt := MaterialPicture{Picture: "salt", Tint: "#e8f2f6"}
	root := MaterialPicture{Picture: "root", Tint: "#000000"}
	cases := []struct {
		name  string
		shape string
		mats  []MaterialPicture
		want  ItemLook
	}{
		{"recipe order, second is the mark", "potion", []MaterialPicture{moss, leaf, salt, root},
			ItemLook{Shape: "potion", Body: "#c3223a", Trim: "#9fc7e8", Detail: "#e8f2f6", Mark: &LookMark{Picture: "leaf", Tint: "#9fc7e8"}}},
		{"one material is its own mark", "blade", []MaterialPicture{moss},
			ItemLook{Shape: "blade", Body: "#c3223a", Trim: "#c9a24a", Detail: "#7a4a2a", Mark: &LookMark{Picture: "moss", Tint: "#c3223a"}}},
		{"unusable pictures are skipped", "ring", []MaterialPicture{{Picture: "nope", Tint: "#ffffff"}, {}, leaf},
			ItemLook{Shape: "ring", Body: "#9fc7e8", Trim: "#d1ad52", Detail: "#d1ad52", Mark: &LookMark{Picture: "leaf", Tint: "#9fc7e8"}}},
		{"no materials, defaults", "shield", nil, ItemLook{Shape: "shield", Body: "#3f9a84", Trim: "#8c96a3", Detail: "#a8743e"}},
		{"unknown shape is a bundle", "castle", nil, ItemLook{Shape: "bundle", Body: "#7a6a9a", Trim: "#c9a54a", Detail: "#efe6d2"}},
	}
	for _, c := range cases {
		got := composeLook(c.shape, c.mats)
		if got.Shape != c.want.Shape || got.Body != c.want.Body || got.Trim != c.want.Trim || got.Detail != c.want.Detail {
			t.Errorf("%s: got %+v, want %+v", c.name, *got, c.want)
		}
		if (got.Mark == nil) != (c.want.Mark == nil) || got.Mark != nil && *got.Mark != *c.want.Mark {
			t.Errorf("%s: mark %+v, want %+v", c.name, got.Mark, c.want.Mark)
		}
	}
}

func TestKeptWithOnlyALookIsRemembered(t *testing.T) {
	if (Kept{Look: &ItemLook{Shape: "potion"}}).Empty() {
		t.Fatal("a made copy with only a look must still be stored, or it loses its picture")
	}
}
