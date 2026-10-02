package calendar

import (
	"context"
	"errors"
	"testing"
)

// fakeHarptosStore is an in-memory HarptosSeasonStore with the same
// compare-then-write behaviour as the repository.
type fakeHarptosStore struct {
	months  map[string]int
	seasons map[string][]Season
	failGet map[string]bool
}

func (f *fakeHarptosStore) ListCalendarIDsWithSeasonNamed(_ context.Context, name string) ([]string, error) {
	var ids []string
	for id, ss := range f.seasons {
		for _, s := range ss {
			if s.Name == name {
				ids = append(ids, id)
				break
			}
		}
	}
	return ids, nil
}

func (f *fakeHarptosStore) GetMonths(_ context.Context, id string) ([]Month, error) {
	if f.failGet[id] {
		return nil, errors.New("boom")
	}
	return make([]Month, f.months[id]), nil
}

func (f *fakeHarptosStore) GetSeasons(_ context.Context, id string) ([]Season, error) {
	return append([]Season(nil), f.seasons[id]...), nil
}

func (f *fakeHarptosStore) RewriteSeasonsIfUnchanged(_ context.Context, id string, expected, replacement []Season) (bool, error) {
	if !seasonsMatchPreset(f.seasons[id], expected) {
		return false, nil
	}
	byID := map[int]Season{}
	for _, r := range replacement {
		byID[r.ID] = r
	}
	cur := f.seasons[id]
	for i := range cur {
		r := byID[cur[i].ID]
		cur[i].StartMonth, cur[i].StartDay, cur[i].EndMonth, cur[i].EndDay = r.StartMonth, r.StartDay, r.EndMonth, r.EndDay
	}
	return true, nil
}

func storedSeasons(src []Season) []Season {
	out := make([]Season, len(src))
	for i, s := range src {
		s.ID = i + 1
		out[i] = s
	}
	return out
}

func TestReconcileHarptosSeasons(t *testing.T) {
	preset, err := LoadPreset("harptos")
	if err != nil {
		t.Fatal(err)
	}
	nMonths := len(preset.Months)
	newSeasons := storedSeasons(preset.Seasons)

	edited := storedSeasons(oldHarptosSeasons)
	edited[1].EndDay = 29
	described := storedSeasons(oldHarptosSeasons)
	note := "owner note"
	described[0].Description = &note
	recolored := storedSeasons(oldHarptosSeasons)
	recolored[2].Color = "#ff0000"
	dropped := storedSeasons(oldHarptosSeasons)[:3]

	tests := []struct {
		name     string
		months   int
		seasons  []Season
		wantFix  int
		wantDone []Season // seasons after the run
	}{
		{"exact old is fixed", nMonths, storedSeasons(oldHarptosSeasons), 1, newSeasons},
		{"already new is untouched", nMonths, newSeasons, 0, newSeasons},
		{"edited bound is untouched", nMonths, edited, 0, edited},
		{"owner description is untouched", nMonths, described, 0, described},
		{"owner color is untouched", nMonths, recolored, 0, recolored},
		{"removed season is untouched", nMonths, dropped, 0, dropped},
		{"different month layout is untouched", 12, storedSeasons(oldHarptosSeasons), 0, storedSeasons(oldHarptosSeasons)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeHarptosStore{
				months:  map[string]int{"c1": tt.months},
				seasons: map[string][]Season{"c1": append([]Season(nil), tt.seasons...)},
			}
			n, err := ReconcileHarptosSeasons(context.Background(), store)
			if err != nil {
				t.Fatalf("first run: %v", err)
			}
			if n != tt.wantFix {
				t.Errorf("repaired = %d, want %d", n, tt.wantFix)
			}
			assertSeasonsEqual(t, store.seasons["c1"], tt.wantDone)

			n, err = ReconcileHarptosSeasons(context.Background(), store)
			if err != nil || n != 0 {
				t.Errorf("second run = (%d, %v), want (0, nil)", n, err)
			}
			assertSeasonsEqual(t, store.seasons["c1"], tt.wantDone)
		})
	}
}

// TestReconcileHarptosSeasonsOneFailureDoesNotStopTheRest pins that a
// calendar whose read fails is skipped and reported while the others are
// still repaired.
func TestReconcileHarptosSeasonsOneFailureDoesNotStopTheRest(t *testing.T) {
	preset, _ := LoadPreset("harptos")
	n := len(preset.Months)
	store := &fakeHarptosStore{
		months:  map[string]int{"bad": n, "good": n},
		seasons: map[string][]Season{"bad": storedSeasons(oldHarptosSeasons), "good": storedSeasons(oldHarptosSeasons)},
		failGet: map[string]bool{"bad": true},
	}
	fixed, err := ReconcileHarptosSeasons(context.Background(), store)
	if err == nil {
		t.Error("want the failure reported")
	}
	if fixed != 1 {
		t.Errorf("repaired = %d, want 1", fixed)
	}
}

// TestOldHarptosSeasonsDifferFromThePreset guards the frozen constant: if it
// ever equals the live preset the reconciler would be a no-op that looks
// like it works.
func TestOldHarptosSeasonsDifferFromThePreset(t *testing.T) {
	preset, err := LoadPreset("harptos")
	if err != nil {
		t.Fatal(err)
	}
	if seasonsMatchPreset(storedSeasons(preset.Seasons), oldHarptosSeasons) {
		t.Fatal("oldHarptosSeasons equals the current preset's seasons")
	}
}

func assertSeasonsEqual(t *testing.T, got, want []Season) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d seasons, want %d", len(got), len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Name != w.Name || g.StartMonth != w.StartMonth || g.StartDay != w.StartDay ||
			g.EndMonth != w.EndMonth || g.EndDay != w.EndDay || g.Color != w.Color {
			t.Errorf("season %d = %+v, want %+v", i, g, w)
		}
	}
}
