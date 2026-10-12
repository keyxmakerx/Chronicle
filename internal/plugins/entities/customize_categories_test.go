package entities

import (
	"context"
	"net/http"
	"reflect"
	"strconv"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// categoryWorld is an in-memory set of entity types across two campaigns,
// wired into the service through the mock repos, with a log of every write.
type categoryWorld struct {
	types   map[int]*EntityType
	nextID  int
	log     []string
	pages   map[int]int
	layouts map[int]EntityTypeLayout
	failOn  string // a log entry prefix whose write should fail
}

func newCategoryWorld(types ...EntityType) (*categoryWorld, EntityService) {
	w := &categoryWorld{types: map[int]*EntityType{}, nextID: 100, pages: map[int]int{}, layouts: map[int]EntityTypeLayout{}}
	for i := range types {
		t := types[i]
		w.types[t.ID] = &t
	}
	typeRepo := &mockEntityTypeRepo{
		listByCampaignFn: func(_ context.Context, campaignID string) ([]EntityType, error) {
			var out []EntityType
			for _, t := range w.types {
				if t.CampaignID == campaignID {
					out = append(out, *t)
				}
			}
			return out, nil
		},
		findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
			if t, ok := w.types[id]; ok {
				c := *t
				return &c, nil
			}
			return nil, apperror.NewNotFound("entity type not found")
		},
		createFn: func(_ context.Context, et *EntityType) error {
			if w.failOn == "create:"+et.Name {
				return apperror.NewInternal(nil)
			}
			w.nextID++
			et.ID = w.nextID
			c := *et
			w.types[et.ID] = &c
			w.log = append(w.log, "create:"+et.Name)
			return nil
		},
		updateFn: func(_ context.Context, et *EntityType) error {
			if w.failOn == "update:"+strconv.Itoa(et.ID) {
				return apperror.NewInternal(nil)
			}
			c := *et
			w.types[et.ID] = &c
			w.log = append(w.log, "update:"+strconv.Itoa(et.ID))
			return nil
		},
		updateLayoutFn: func(_ context.Context, id int, _ string) error {
			if w.failOn == "layout" {
				return apperror.NewInternal(nil)
			}
			w.log = append(w.log, "layout:"+strconv.Itoa(id))
			return nil
		},
		deleteFn: func(_ context.Context, id int) error {
			delete(w.types, id)
			w.log = append(w.log, "delete:"+strconv.Itoa(id))
			return nil
		},
		slugExistsFn: func(_ context.Context, campaignID, slug string) (bool, error) {
			for _, t := range w.types {
				if t.CampaignID == campaignID && t.Slug == slug {
					return true, nil
				}
			}
			return false, nil
		},
	}
	entityRepo := &mockEntityRepo{
		countByTypeFn: func(context.Context, string, int, string) (map[int]int, error) { return w.pages, nil },
	}
	return w, newTestService(entityRepo, typeRepo)
}

func ptrInt(i int) *int { return &i }

func typ(id int, campaign, name string, parent *int) EntityType {
	return EntityType{ID: id, CampaignID: campaign, Slug: Slugify(name), Name: name, NamePlural: name + "s", Icon: "fa-circle", Color: "#112233", ParentTypeID: parent, Enabled: true}
}

func patchOf(items map[string]CustomizeCategoryItem) CustomizeCategoriesPatch {
	return CustomizeCategoriesPatch{Items: items}
}

func TestApplyCustomizeCategories_OrderAndIDs(t *testing.T) {
	w, svc := newCategoryWorld(
		typ(1, "c1", "Places", nil),
		typ(2, "c1", "Towns", ptrInt(1)),
		typ(3, "c1", "Gods", nil),
		typ(4, "c1", "Old", nil),
	)
	res, err := svc.ApplyCustomizeCategories(context.Background(), "c1", patchOf(map[string]CustomizeCategoryItem{
		// Listed child first on purpose: the parent must still be created first.
		"new-b": {New: true, Name: "Ship", NamePlural: "Ships", Parent: "new-a"},
		"new-a": {New: true, Name: "Vehicle", NamePlural: "Vehicles"},
		"new-c": {New: true, Name: "Cult", NamePlural: "Cults", Parent: "3"},
		"3":     {Name: "Deities"},
		"2":     {ClearParent: true},
		"4":     {Delete: true},
	}))
	if err != nil {
		t.Fatal(err)
	}
	wantLog := []string{"create:Vehicle", "create:Ship", "create:Cult", "update:2", "update:3", "delete:4"}
	if !reflect.DeepEqual(w.log, wantLog) {
		t.Errorf("writes = %v, want %v", w.log, wantLog)
	}
	if len(res.Created) != 3 {
		t.Fatalf("created = %v", res.Created)
	}
	vehicle, ship := res.Created["new-a"], res.Created["new-b"]
	cult := res.Created["new-c"]
	if w.types[cult].ParentTypeID == nil || *w.types[cult].ParentTypeID != 3 {
		t.Errorf("Cult parent = %v, want 3", w.types[cult].ParentTypeID)
	}
	if w.types[ship].ParentTypeID == nil || *w.types[ship].ParentTypeID != vehicle {
		t.Errorf("Ship parent = %v, want the new Vehicle %d", w.types[ship].ParentTypeID, vehicle)
	}
	if w.types[2].ParentTypeID != nil {
		t.Error("Towns kept its parent")
	}
	if w.types[3].Name != "Deities" {
		t.Errorf("Gods renamed to %q", w.types[3].Name)
	}
	if _, ok := w.types[4]; ok {
		t.Error("Old not deleted")
	}
	if !reflect.DeepEqual(res.Applied, []string{"new-a", "new-b", "new-c", "2", "3", "4"}) {
		t.Errorf("applied = %v", res.Applied)
	}
}

func TestApplyCustomizeCategories_DeletesChildrenBeforeParents(t *testing.T) {
	w, svc := newCategoryWorld(
		typ(1, "c1", "Parent", nil),
		typ(2, "c1", "Child", ptrInt(1)),
	)
	if _, err := svc.ApplyCustomizeCategories(context.Background(), "c1", patchOf(map[string]CustomizeCategoryItem{
		"1": {Delete: true}, "2": {Delete: true},
	})); err != nil {
		t.Fatal(err)
	}
	if want := []string{"delete:2", "delete:1"}; !reflect.DeepEqual(w.log, want) {
		t.Errorf("writes = %v, want %v", w.log, want)
	}
}

func TestApplyCustomizeCategories_Refusals(t *testing.T) {
	tests := []struct {
		name     string
		items    map[string]CustomizeCategoryItem
		wantCode int
	}{
		{"another campaign's id is not found", map[string]CustomizeCategoryItem{"50": {Name: "Hijack"}}, http.StatusNotFound},
		{"delete of another campaign's id is not found", map[string]CustomizeCategoryItem{"50": {Delete: true}}, http.StatusNotFound},
		{"id that does not exist", map[string]CustomizeCategoryItem{"999": {Name: "x"}}, http.StatusNotFound},
		{"parent in another campaign", map[string]CustomizeCategoryItem{"2": {Parent: "50"}}, http.StatusBadRequest},
		{"new category under another campaign's parent", map[string]CustomizeCategoryItem{"new-a": {New: true, Name: "A", NamePlural: "As", Parent: "50"}}, http.StatusBadRequest},
		{"copy_from another campaign", map[string]CustomizeCategoryItem{"new-a": {New: true, Name: "A", NamePlural: "As", CopyFrom: 50}}, http.StatusNotFound},
		{"copy_from on an existing category", map[string]CustomizeCategoryItem{"1": {CopyFrom: 2}}, http.StatusBadRequest},
		{"new cycle", map[string]CustomizeCategoryItem{
			"new-a": {New: true, Name: "A", NamePlural: "As", Parent: "new-b"},
			"new-b": {New: true, Name: "B", NamePlural: "Bs", Parent: "new-a"}}, http.StatusBadRequest},
		{"self parent", map[string]CustomizeCategoryItem{"new-a": {New: true, Name: "A", NamePlural: "As", Parent: "new-a"}}, http.StatusBadRequest},
		{"existing cycle", map[string]CustomizeCategoryItem{"1": {Parent: "2"}}, http.StatusBadRequest},
		{"unknown parent key", map[string]CustomizeCategoryItem{"new-a": {New: true, Name: "A", NamePlural: "As", Parent: "new-zzz"}}, http.StatusBadRequest},
		{"junk parent", map[string]CustomizeCategoryItem{"2": {Parent: "../etc"}}, http.StatusBadRequest},
		{"junk key", map[string]CustomizeCategoryItem{"abc": {Name: "x"}}, http.StatusBadRequest},
		{"new key not marked new", map[string]CustomizeCategoryItem{"new-a": {Name: "A", NamePlural: "As"}}, http.StatusBadRequest},
		{"new without a name", map[string]CustomizeCategoryItem{"new-a": {New: true, NamePlural: "As"}}, http.StatusBadRequest},
		{"existing marked new", map[string]CustomizeCategoryItem{"1": {New: true, Name: "x", NamePlural: "xs"}}, http.StatusBadRequest},
		{"delete a parent and keep its child", map[string]CustomizeCategoryItem{"1": {Delete: true}}, http.StatusBadRequest},
		{"parent being deleted", map[string]CustomizeCategoryItem{"3": {Delete: true}, "2": {Parent: "3"}}, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, svc := newCategoryWorld(
				typ(1, "c1", "Places", nil),
				typ(2, "c1", "Towns", ptrInt(1)),
				typ(3, "c1", "Gods", nil),
				typ(50, "other", "Secret", nil),
			)
			res, err := svc.ApplyCustomizeCategories(context.Background(), "c1", patchOf(tt.items))
			assertAppError(t, err, tt.wantCode)
			if len(w.log) != 0 || len(res.Created) != 0 || len(res.Applied) != 0 {
				t.Errorf("a refused patch still wrote: %v, %+v", w.log, res)
			}
			if w.types[50].Name != "Secret" {
				t.Error("another campaign's category was touched")
			}
		})
	}
}

func TestApplyCustomizeCategories_StopsAtFirstFailure(t *testing.T) {
	w, svc := newCategoryWorld(
		typ(1, "c1", "Places", nil),
		typ(2, "c1", "Gods", nil),
		typ(3, "c1", "Old", nil),
	)
	w.failOn = "update:2"
	res, err := svc.ApplyCustomizeCategories(context.Background(), "c1", patchOf(map[string]CustomizeCategoryItem{
		"new-a": {New: true, Name: "Ship", NamePlural: "Ships"},
		"1":     {Name: "Realms"},
		"2":     {Name: "Deities"},
		"3":     {Delete: true},
	}))
	if err == nil {
		t.Fatal("expected the failing update to surface")
	}
	if !reflect.DeepEqual(res.Applied, []string{"new-a", "1"}) {
		t.Errorf("applied = %v, want [new-a 1]", res.Applied)
	}
	if _, ok := res.Created["new-a"]; !ok || len(res.Created) != 1 {
		t.Errorf("created = %v", res.Created)
	}
	if _, ok := w.types[3]; !ok {
		t.Error("the delete after the failure still ran")
	}
	if w.types[1].Name != "Realms" {
		t.Error("the update before the failure was not kept")
	}
}

func TestApplyCustomizeCategories_DeleteKeepsExistingGuards(t *testing.T) {
	w, svc := newCategoryWorld(typ(1, "c1", "Places", nil))
	w.pages[1] = 4
	res, err := svc.ApplyCustomizeCategories(context.Background(), "c1", patchOf(map[string]CustomizeCategoryItem{"1": {Delete: true}}))
	assertAppError(t, err, http.StatusConflict)
	if len(res.Applied) != 0 {
		t.Errorf("applied = %v", res.Applied)
	}
	if _, ok := w.types[1]; !ok {
		t.Error("a category with pages was deleted")
	}
}

func TestApplyCustomizeCategories_CopyFrom(t *testing.T) {
	src := typ(1, "c1", "Places", nil)
	src.Fields = []FieldDefinition{{Key: "region", Label: "Region", Type: "text"}}
	src.Layout = DefaultLayout()

	t.Run("copies fields and layout", func(t *testing.T) {
		w, svc := newCategoryWorld(src)
		res, err := svc.ApplyCustomizeCategories(context.Background(), "c1", patchOf(map[string]CustomizeCategoryItem{
			"new-a": {New: true, Name: "Haunts", NamePlural: "Haunts", CopyFrom: 1}}))
		if err != nil {
			t.Fatal(err)
		}
		id := res.Created["new-a"]
		if got := w.types[id].Fields; len(got) != 1 || got[0].Key != "region" {
			t.Errorf("fields = %+v", got)
		}
		if want := []string{"create:Haunts", "layout:" + strconv.Itoa(id)}; !reflect.DeepEqual(w.log, want) {
			t.Errorf("writes = %v, want %v", w.log, want)
		}
	})

	t.Run("a failed layout copy removes the empty category", func(t *testing.T) {
		w, svc := newCategoryWorld(src)
		w.failOn = "layout"
		res, err := svc.ApplyCustomizeCategories(context.Background(), "c1", patchOf(map[string]CustomizeCategoryItem{
			"new-a": {New: true, Name: "Haunts", NamePlural: "Haunts", CopyFrom: 1}}))
		if err == nil {
			t.Fatal("expected an error")
		}
		if len(res.Created) != 0 || len(res.Applied) != 0 {
			t.Errorf("result = %+v", res)
		}
		if len(w.types) != 1 {
			t.Errorf("types left = %d, want only the source", len(w.types))
		}
	})
}

func TestApplyCustomizeCategories_MenuOnlyItemIsApplied(t *testing.T) {
	w, svc := newCategoryWorld(typ(1, "c1", "Places", nil))
	off := false
	res, err := svc.ApplyCustomizeCategories(context.Background(), "c1", patchOf(map[string]CustomizeCategoryItem{"1": {Menu: &off}}))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Applied, []string{"1"}) || len(w.log) != 0 {
		t.Errorf("applied %v, writes %v", res.Applied, w.log)
	}
}

func TestCustomizeCategories_Info(t *testing.T) {
	yes, no := true, false
	explicitNo := typ(2, "c1", "Heroes", nil)
	explicitNo.Claimable = &no
	explicitYes := typ(3, "c1", "Cast", nil)
	explicitYes.Claimable = &yes
	heuristic := typ(4, "c1", "Crew", nil)
	heuristic.Slug = "crew-character"
	w, svc := newCategoryWorld(typ(1, "c1", "Places", nil), explicitNo, explicitYes, heuristic)
	w.pages[1] = 7

	infos, err := svc.CustomizeCategories(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]CustomizeCategoryInfo{}
	for _, in := range infos {
		got[in.Type.ID] = in
	}
	if got[1].Pages != 7 || got[1].Claimable {
		t.Errorf("Places = %+v", got[1])
	}
	if got[2].Claimable || !got[3].Claimable || !got[4].Claimable {
		t.Errorf("claimable: explicit-no=%v explicit-yes=%v heuristic=%v", got[2].Claimable, got[3].Claimable, got[4].Claimable)
	}
}
