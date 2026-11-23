package entities

import (
	"context"
	"net/http"
	"reflect"
	"testing"
)

// dmOnlyTyp is a category only the DM team sees.
func dmOnlyTyp(id int, campaign, name string) EntityType {
	t := typ(id, campaign, name, nil)
	p := PresetCategoryAdventures
	t.PresetCategory = &p
	return t
}

func TestMergeEntityType(t *testing.T) {
	tests := []struct {
		name     string
		from, to int
		wantCode int // 0 for success
		wantLog  []string
	}{
		{"moves the pages and deletes the category", 4, 1, 0, []string{"merge:4>1"}},
		{"into a kind", 4, 2, 0, []string{"merge:4>2"}},
		{"a visible category into a DM-only one", 4, 5, 0, []string{"merge:4>5"}},
		{"a DM-only category into another DM-only one", 5, 6, 0, []string{"merge:5>6"}},
		{"a DM-only category into a visible one is refused", 5, 1, http.StatusBadRequest, nil},
		{"into itself is refused", 4, 4, http.StatusBadRequest, nil},
		{"a category with kinds is refused", 1, 4, http.StatusBadRequest, nil},
		{"into another campaign's category reads as missing", 4, 50, http.StatusNotFound, nil},
		{"another campaign's category reads as missing", 50, 4, http.StatusNotFound, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, svc := newCategoryWorld(
				typ(1, "c1", "Places", nil),
				typ(2, "c1", "Towns", ptrInt(1)),
				typ(4, "c1", "Adventures", nil),
				dmOnlyTyp(5, "c1", "Secrets"),
				dmOnlyTyp(6, "c1", "Plots"),
				typ(50, "other", "Theirs", nil),
			)
			w.pages[tt.from] = 3
			moved, err := svc.MergeEntityType(context.Background(), "c1", tt.from, tt.to)
			if tt.wantCode != 0 {
				assertAppError(t, err, tt.wantCode)
				if len(w.log) != 0 {
					t.Errorf("a refused merge still wrote: %v", w.log)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if moved != 3 {
				t.Errorf("moved = %d, want 3", moved)
			}
			if !reflect.DeepEqual(w.log, tt.wantLog) {
				t.Errorf("writes = %v, want %v", w.log, tt.wantLog)
			}
			if _, ok := w.types[tt.from]; ok {
				t.Error("the merged category still stands")
			}
		})
	}
}

func TestApplyCustomizeCategories_DeleteWithMoveTo(t *testing.T) {
	w, svc := newCategoryWorld(
		typ(1, "c1", "Places", nil),
		typ(4, "c1", "Adventures", nil),
		typ(7, "c1", "Empty", nil),
	)
	w.pages[4] = 11
	res, err := svc.ApplyCustomizeCategories(context.Background(), "c1", patchOf(map[string]CustomizeCategoryItem{
		"4": {Delete: true, MoveTo: "1"},
		"7": {Delete: true},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"merge:4>1", "delete:7"}; !reflect.DeepEqual(w.log, want) {
		t.Errorf("writes = %v, want %v", w.log, want)
	}
	if w.pages[1] != 11 {
		t.Errorf("Places has %d pages, want 11", w.pages[1])
	}
	if !reflect.DeepEqual(res.Applied, []string{"4", "7"}) {
		t.Errorf("applied = %v", res.Applied)
	}
}

func TestApplyCustomizeCategories_MoveToRefusals(t *testing.T) {
	tests := []struct {
		name     string
		items    map[string]CustomizeCategoryItem
		wantCode int
	}{
		{"move_to without delete", map[string]CustomizeCategoryItem{"4": {MoveTo: "1"}}, http.StatusBadRequest},
		{"into itself", map[string]CustomizeCategoryItem{"4": {Delete: true, MoveTo: "4"}}, http.StatusBadRequest},
		{"into a category being deleted", map[string]CustomizeCategoryItem{"4": {Delete: true, MoveTo: "3"}, "3": {Delete: true}}, http.StatusBadRequest},
		{"into a draft category", map[string]CustomizeCategoryItem{"4": {Delete: true, MoveTo: "new-a"}, "new-a": {New: true, Name: "A", NamePlural: "As"}}, http.StatusBadRequest},
		{"into another campaign's category", map[string]CustomizeCategoryItem{"4": {Delete: true, MoveTo: "50"}}, http.StatusNotFound},
		{"into a missing category", map[string]CustomizeCategoryItem{"4": {Delete: true, MoveTo: "999"}}, http.StatusNotFound},
		{"junk target", map[string]CustomizeCategoryItem{"4": {Delete: true, MoveTo: "../x"}}, http.StatusBadRequest},
		{"DM-only pages into a visible category", map[string]CustomizeCategoryItem{"5": {Delete: true, MoveTo: "1"}}, http.StatusBadRequest},
		{"a draft category with move_to", map[string]CustomizeCategoryItem{"new-a": {New: true, Name: "A", NamePlural: "As", MoveTo: "1"}}, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, svc := newCategoryWorld(
				typ(1, "c1", "Places", nil),
				typ(3, "c1", "Gods", nil),
				typ(4, "c1", "Adventures", nil),
				dmOnlyTyp(5, "c1", "Secrets"),
				typ(50, "other", "Theirs", nil),
			)
			w.pages[4], w.pages[5] = 2, 2
			res, err := svc.ApplyCustomizeCategories(context.Background(), "c1", patchOf(tt.items))
			assertAppError(t, err, tt.wantCode)
			if len(w.log) != 0 || len(res.Applied) != 0 {
				t.Errorf("a refused patch still wrote: %v, %+v", w.log, res)
			}
		})
	}
}
