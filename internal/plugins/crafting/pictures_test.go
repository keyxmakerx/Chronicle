package crafting

import (
	"context"
	"net/http"
	"os"
	"regexp"
	"testing"
)

// The picture keys the server accepts must be exactly the drawings the
// browser has, or a saved choice would draw nothing.
func TestPicturesMatchTheDrawings(t *testing.T) {
	src, err := os.ReadFile("../../../static/js/widgets/craft_pictures.js")
	if err != nil {
		t.Fatal(err)
	}
	drawn := regexp.MustCompile(`(?m)^\s+\['([a-z]+)', '([^']+)', '(none|sway|slosh|glint|pulse|beat|flicker|drip|look|writhe)'`).FindAllStringSubmatch(string(src), -1)
	if len(drawn) != len(Pictures) {
		t.Fatalf("craft_pictures.js draws %d pictures, library.go lists %d", len(drawn), len(Pictures))
	}
	for i, d := range drawn {
		if d[1] != Pictures[i].Key || d[2] != Pictures[i].Label {
			t.Errorf("picture %d: drawn %q %q, listed %q %q", i, d[1], d[2], Pictures[i].Key, Pictures[i].Label)
		}
	}
	for _, tg := range Tags {
		if !pictureKnown(tg.Picture) || !tintPattern.MatchString(tg.Tint) || !choiceKnown(TagGroups, tg.Group) {
			t.Errorf("kind %s: picture %q tint %q group %q", tg.Key, tg.Picture, tg.Tint, tg.Group)
		}
	}
}

// The first nine kinds keep their keys and order: an item's first tag picks
// its picture, and recipes store the keys.
func TestKindOrderIsAppendOnly(t *testing.T) {
	first := []string{"metal", "remnant_part", "body_part", "bone", "hide", "liquid", "herb", "magic_item", "magic_essence"}
	for i, k := range first {
		if Tags[i].Key != k {
			t.Fatalf("kind %d is %q, want %q", i, Tags[i].Key, k)
		}
	}
}

func TestCleanPicture(t *testing.T) {
	cases := []struct {
		name string
		in   *MaterialPicture
		want *MaterialPicture
		bad  bool
	}{
		{"nil follows", nil, nil, false},
		{"empty follows", &MaterialPicture{}, nil, false},
		{"tint lowered", &MaterialPicture{Picture: "leaf", Tint: " #6A9C3A "}, &MaterialPicture{Picture: "leaf", Tint: "#6a9c3a"}, false},
		{"unknown picture", &MaterialPicture{Picture: "bloodmoss", Tint: "#6a9c3a"}, nil, true},
		{"bad tint", &MaterialPicture{Picture: "leaf", Tint: "red"}, nil, true},
		{"short tint", &MaterialPicture{Picture: "leaf", Tint: "#fff"}, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := cleanPicture(c.in)
			if (err != nil) != c.bad {
				t.Fatalf("err = %v", err)
			}
			if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestMaterialPictures(t *testing.T) {
	s, repo, _ := newTestService()
	ctx := context.Background()
	repo.tags["moss"] = []string{"herb"}
	repo.tags["water"] = []string{"liquid", "magic_essence"}

	// Swapping two kinds is one save, and Library reports both.
	tags, err := s.SetKindPictures(ctx, "c", director, KindPicturesInput{Kinds: map[string]*MaterialPicture{
		"herb":   {Picture: "mushroom", Tint: "#c98a3a"},
		"fungus": {Picture: "leaf", Tint: "#6a9c3a"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]Tag{}
	for _, tg := range tags {
		byKey[tg.Key] = tg
	}
	if byKey["herb"].Picture != "mushroom" || byKey["herb"].DefaultPicture != "leaf" || byKey["fungus"].Picture != "leaf" || byKey["liquid"].Picture != "flask" {
		t.Fatalf("tags after swap = %+v", byKey)
	}

	// An item follows its first kind until it gets its own picture.
	if _, err := s.SetItemPicture(ctx, "c", "water", director, &MaterialPicture{Picture: "vial", Tint: "#9FC6E6"}); err != nil {
		t.Fatal(err)
	}
	cat, err := s.Catalog(ctx, "c", director)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range cat.Items {
		switch it.ID {
		case "moss":
			if it.Picture != nil {
				t.Errorf("moss has its own picture %+v", it.Picture)
			}
		case "water":
			if it.Picture == nil || *it.Picture != (MaterialPicture{Picture: "vial", Tint: "#9fc6e6"}) {
				t.Errorf("water picture = %+v", it.Picture)
			}
		}
	}
	book, _ := s.pictureBook(ctx, "c")
	if got := book.item("moss", []string{"herb"}); got.Picture != "mushroom" {
		t.Errorf("moss wears %+v, want herb's mushroom", got)
	}
	if got := book.item("rope", nil); got.Picture != "" {
		t.Errorf("an item with no kind wears %+v", got)
	}

	// Null puts a kind back on its default and an item back on its kind.
	if _, err := s.SetKindPictures(ctx, "c", director, KindPicturesInput{Kinds: map[string]*MaterialPicture{"herb": nil}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetItemPicture(ctx, "c", "water", director, nil); err != nil {
		t.Fatal(err)
	}
	if len(repo.itemPics) != 0 || repo.kindPics["herb"] != (MaterialPicture{}) {
		t.Fatalf("after reset kinds=%v items=%v", repo.kindPics, repo.itemPics)
	}

	// A stored picture the library no longer has falls back to the default.
	repo.kindPics["bone"] = MaterialPicture{Picture: "retired", Tint: "#000000"}
	book, _ = s.pictureBook(ctx, "c")
	if got := book.kind("bone"); got.Picture != "bone" {
		t.Errorf("retired picture read as %+v", got)
	}
}

func TestMaterialPictureRefusals(t *testing.T) {
	s, repo, _ := newTestService()
	ctx := context.Background()
	cases := []struct {
		name string
		call func() error
		want int
	}{
		{"unknown kind", func() error {
			_, err := s.SetKindPictures(ctx, "c", director, KindPicturesInput{Kinds: map[string]*MaterialPicture{"dragon": nil}})
			return err
		}, http.StatusUnprocessableEntity},
		{"no kinds", func() error { _, err := s.SetKindPictures(ctx, "c", director, KindPicturesInput{}); return err }, http.StatusUnprocessableEntity},
		{"bad picture", func() error {
			_, err := s.SetKindPictures(ctx, "c", director, KindPicturesInput{Kinds: map[string]*MaterialPicture{"herb": {Picture: "x", Tint: "#000000"}}})
			return err
		}, http.StatusUnprocessableEntity},
		{"not an item", func() error {
			_, err := s.SetItemPicture(ctx, "c", "mira", director, &MaterialPicture{Picture: "leaf", Tint: "#6a9c3a"})
			return err
		}, http.StatusNotFound},
		{"no such item", func() error {
			_, err := s.SetItemPicture(ctx, "c", "gone", director, &MaterialPicture{Picture: "leaf", Tint: "#6a9c3a"})
			return err
		}, http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := code(c.call()); got != c.want {
				t.Fatalf("code = %d, want %d", got, c.want)
			}
		})
	}
	if len(repo.kindPics) != 0 || len(repo.itemPics) != 0 {
		t.Fatal("a refused call still saved")
	}
}
