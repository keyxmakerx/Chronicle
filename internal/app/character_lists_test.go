package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fixedTypeLists is an entities.CharacterListReader that answers with the
// page types a test names, standing in for the owner's saved choice.
type fixedTypeLists struct {
	char, npc []int
	err       error
}

func (f fixedTypeLists) CharacterTypeIDs(context.Context, string) ([]int, error) {
	return f.char, f.err
}

func (f fixedTypeLists) NPCTypeIDs(context.Context, string) ([]int, error) {
	return f.npc, f.err
}

// fakeListCampaigns is the campaign service as characterListStore sees it.
type fakeListCampaigns struct {
	settings map[string]string
	saved    map[string][2][]int
}

func (f *fakeListCampaigns) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	s, ok := f.settings[id]
	if !ok {
		return nil, nil
	}
	return &campaigns.Campaign{ID: id, Settings: s}, nil
}

func (f *fakeListCampaigns) UpdateCharacterLists(_ context.Context, id string, chars, npcs []int) error {
	f.saved[id] = [2][]int{chars, npcs}
	return nil
}

func TestCharacterListStore_Get(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		want     entities.CharacterLists
		wantOK   bool
	}{
		{"never chosen", `{}`, entities.CharacterLists{}, false},
		{"unrelated settings only", `{"accent_color":"#fff"}`, entities.CharacterLists{}, false},
		{"both chosen", `{"character_type_ids":[1,2],"npc_type_ids":[3]}`,
			entities.CharacterLists{CharacterTypeIDs: []int{1, 2}, NPCTypeIDs: []int{3}}, true},
		{"chose none is still a choice", `{"character_type_ids":[],"npc_type_ids":[]}`,
			entities.CharacterLists{CharacterTypeIDs: []int{}, NPCTypeIDs: []int{}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &characterListStore{camps: &fakeListCampaigns{settings: map[string]string{"c1": tt.settings}}}
			got, ok, err := st.Get(context.Background(), "c1")
			if err != nil {
				t.Fatal(err)
			}
			if ok != tt.wantOK || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Get = %+v, %v; want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}

	t.Run("missing campaign is not found", func(t *testing.T) {
		st := &characterListStore{camps: &fakeListCampaigns{settings: map[string]string{}}}
		_, _, err := st.Get(context.Background(), "nope")
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != 404 {
			t.Errorf("err = %v, want a 404", err)
		}
	})
}

// seedRecorder notes which campaigns it was asked to seed.
type seedRecorder struct {
	seeded []string
	fail   map[string]bool
}

func (r *seedRecorder) Seed(_ context.Context, id string) error {
	if r.fail[id] {
		return errors.New("boom")
	}
	r.seeded = append(r.seeded, id)
	return nil
}

func TestSeedCharacterListsOnce(t *testing.T) {
	ctx := context.Background()
	camps := fakeExtrasCampaigns{ids: []string{"c1", "c2"}}

	t.Run("seeds every campaign once and records the run", func(t *testing.T) {
		st := &fakeExtrasSettings{vals: map[string]string{}}
		rec := &seedRecorder{}
		n, err := seedCharacterListsOnce(ctx, st, camps, rec)
		if err != nil || n != 2 || !reflect.DeepEqual(rec.seeded, []string{"c1", "c2"}) {
			t.Fatalf("first run: n=%d err=%v seeded=%v", n, err, rec.seeded)
		}
		if st.vals[characterListsSeededKey] != "1" {
			t.Fatal("the run was not recorded")
		}
		rec.seeded = nil
		if n, err := seedCharacterListsOnce(ctx, st, camps, rec); err != nil || n != 0 || len(rec.seeded) != 0 {
			t.Errorf("second run: n=%d err=%v seeded=%v, want a no-op", n, err, rec.seeded)
		}
	})

	t.Run("a failed campaign leaves the run to be retried", func(t *testing.T) {
		st := &fakeExtrasSettings{vals: map[string]string{}}
		rec := &seedRecorder{fail: map[string]bool{"c1": true}}
		if _, err := seedCharacterListsOnce(ctx, st, camps, rec); err == nil {
			t.Fatal("want an error when a campaign could not be seeded")
		}
		if _, recorded := st.vals[characterListsSeededKey]; recorded {
			t.Error("a partial run must not be recorded as done")
		}
		if !reflect.DeepEqual(rec.seeded, []string{"c2"}) {
			t.Errorf("the other campaign should still be seeded, got %v", rec.seeded)
		}
	})
}
