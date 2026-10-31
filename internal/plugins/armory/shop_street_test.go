package armory

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestNormalizeShopFront(t *testing.T) {
	ok := `"style":"stone","colour":"#8D9098","awning":"none","sign":"board","wares":["sword"],"lamp":true,"closed":"board"`
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{"minimal, colour lowered", `{` + ok + `}`, `{"style":"stone","colour":"#8d9098","awning":"none","sign":"board","wares":["sword"],"lamp":true,"closed":"board"}`, false},
		{"text trimmed", `{` + ok + `,"words":"  Iron  ","closedNote":" At the forge "}`, `{"style":"stone","colour":"#8d9098","awning":"none","sign":"board","words":"Iron","wares":["sword"],"lamp":true,"closed":"board","closedNote":"At the forge"}`, false},
		{"no wares is an empty list", `{"style":"timber","colour":"#d7b98f","awning":"stripe","sign":"hang","closed":"dark"}`, `{"style":"timber","colour":"#d7b98f","awning":"stripe","sign":"hang","wares":[],"lamp":false,"closed":"dark"}`, false},
		{"unknown style", `{"style":"glass","colour":"#d7b98f","awning":"none","sign":"hang","closed":"dark"}`, ``, true},
		{"unknown awning", `{"style":"stone","colour":"#d7b98f","awning":"tent","sign":"hang","closed":"dark"}`, ``, true},
		{"unknown sign", `{"style":"stone","colour":"#d7b98f","awning":"none","sign":"neon","closed":"dark"}`, ``, true},
		{"unknown closed look", `{"style":"stone","colour":"#d7b98f","awning":"none","sign":"hang","closed":"bricked"}`, ``, true},
		{"unknown picture", `{` + ok + `,"icon":"skull"}`, ``, true},
		{"css in a colour", `{"style":"stone","colour":"red;x:url(y)","awning":"none","sign":"hang","closed":"dark"}`, ``, true},
		{"markup in a colour", `{` + ok + `,"trim":"\"/><script>"}`, ``, true},
		{"short colour", `{` + ok + `,"awningColour":"#fff"}`, ``, true},
		{"missing colour", `{"style":"stone","awning":"none","sign":"hang","closed":"dark"}`, ``, true},
		{"unknown ware", `{"style":"stone","colour":"#8d9098","awning":"none","sign":"hang","wares":["dragon"],"closed":"dark"}`, ``, true},
		{"four wares", `{"style":"stone","colour":"#8d9098","awning":"none","sign":"hang","wares":["gem","herb","bread","sword"],"closed":"dark"}`, ``, true},
		{"words too long", `{` + ok + `,"words":"` + strings.Repeat("é", 25) + `"}`, ``, true},
		{"note at the limit", `{` + ok + `,"closedNote":"` + strings.Repeat("é", 26) + `"}`, `{"style":"stone","colour":"#8d9098","awning":"none","sign":"board","wares":["sword"],"lamp":true,"closed":"board","closedNote":"` + strings.Repeat("é", 26) + `"}`, false},
		{"control characters", `{` + ok + `,"words":"a\u0007b"}`, ``, true},
		{"unknown field", `{` + ok + `,"script":"x"}`, ``, true},
		{"trailing data", `{` + ok + `}{}`, ``, true},
		{"too large", `{` + ok + `,"words":"` + strings.Repeat(" ", maxShopFrontBytes) + `"}`, ``, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeShopFront([]byte(tc.raw))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
			if tc.wantErr {
				if !isStatus(err, http.StatusBadRequest) {
					t.Fatalf("err = %v, want 400", err)
				}
				return
			}
			if string(got) != tc.want {
				t.Fatalf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

// The default storefront must pass the same checks a saved one does.
func TestDefaultShopFrontIsValid(t *testing.T) {
	raw, _ := json.Marshal(DefaultShopFront())
	if _, err := NormalizeShopFront(raw); err != nil {
		t.Fatal(err)
	}
}

type fakeFrontRepo struct {
	fronts  map[string]json.RawMessage
	asked   []string
	upserts int
}

func (f *fakeFrontRepo) Get(_ context.Context, _, id string) (json.RawMessage, error) {
	return f.fronts[id], nil
}

func (f *fakeFrontRepo) ListFor(_ context.Context, _ string, ids []string) (map[string]json.RawMessage, error) {
	f.asked = ids
	out := map[string]json.RawMessage{}
	for _, id := range ids {
		if v, ok := f.fronts[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func (f *fakeFrontRepo) Upsert(_ context.Context, _, id, _ string, front json.RawMessage) error {
	f.upserts++
	f.fronts[id] = front
	return nil
}

type fakeLister struct {
	shops     []StreetShop
	truncated bool
}

func (f fakeLister) ListShops(context.Context, string, int, string) ([]StreetShop, bool, error) {
	return f.shops, f.truncated, nil
}

type fakeDowntime bool

func (f fakeDowntime) IsDowntimeOpen(context.Context, string) (bool, error) { return bool(f), nil }

func TestShopStreetService(t *testing.T) {
	saved := json.RawMessage(`{"style":"stone"}`)
	repo := &fakeFrontRepo{fronts: map[string]json.RawMessage{"s2": saved}}
	lister := fakeLister{shops: []StreetShop{{ID: "s1", Name: "Lantern"}, {ID: "s2", Name: "Smithy", TownID: "t", TownName: "Thornbury"}}, truncated: true}
	svc := NewShopStreetService(repo, fakeShops{shops: map[string]string{"s1": "c1", "s2": "c1"}}, lister, fakeDowntime(false))

	t.Run("street pairs each listed shop with its storefront", func(t *testing.T) {
		v, err := svc.Street(context.Background(), "c1", 1, "u")
		if err != nil {
			t.Fatal(err)
		}
		if v.Open || !v.Truncated || len(v.Shops) != 2 {
			t.Fatalf("view = %+v", v)
		}
		if v.Shops[0].Saved || v.Shops[1].Saved != true || string(v.Shops[1].Front) != string(saved) {
			t.Fatalf("fronts = %+v", v.Shops)
		}
		var def ShopFront
		if err := json.Unmarshal(v.Shops[0].Front, &def); err != nil || def.Style != DefaultShopFront().Style {
			t.Fatalf("default front = %s", v.Shops[0].Front)
		}
		if strings.Join(repo.asked, ",") != "s1,s2" {
			t.Fatalf("read fronts for %v; only listed shops may be read", repo.asked)
		}
	})

	t.Run("saving checks the shop and the storefront", func(t *testing.T) {
		good := []byte(`{"style":"stone","colour":"#8d9098","awning":"none","sign":"board","closed":"dark"}`)
		if _, err := svc.SaveFront(context.Background(), "c2", "s1", "u", good); !isStatus(err, http.StatusNotFound) {
			t.Fatalf("shop in another campaign: err = %v, want 404", err)
		}
		if _, err := svc.SaveFront(context.Background(), "c1", "s1", "u", []byte(`{"style":"x"}`)); !isStatus(err, http.StatusBadRequest) {
			t.Fatalf("bad front: err = %v, want 400", err)
		}
		if repo.upserts != 0 {
			t.Fatal("a refused save was stored")
		}
		if _, err := svc.SaveFront(context.Background(), "c1", "s1", "u", good); err != nil || repo.upserts != 1 {
			t.Fatalf("err = %v, upserts = %d", err, repo.upserts)
		}
	})
}
