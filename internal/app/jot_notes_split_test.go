package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
)

type fakeJotAddons struct {
	journal []string
	rows    map[string]bool // campaign id -> has a jot-notes row
	enabled []string
}

func (f *fakeJotAddons) ListCampaignsUsingAddon(_ context.Context, slug string) ([]string, error) {
	if slug != "notes" {
		return nil, nil
	}
	return f.journal, nil
}

func (f *fakeJotAddons) HasCampaignAddonRecord(_ context.Context, id, slug string) (bool, error) {
	return slug == addons.JotNotesAddonSlug && f.rows[id], nil
}

func (f *fakeJotAddons) EnableForCampaignBySlug(_ context.Context, id, slug, _ string) error {
	if slug == addons.JotNotesAddonSlug {
		f.enabled = append(f.enabled, id)
		f.rows[id] = true
	}
	return nil
}

func TestSplitJotNotesOnce(t *testing.T) {
	tests := []struct {
		name    string
		ran     bool
		journal []string
		rows    map[string]bool
		want    []string
	}{
		{"journal campaigns get jots", false, []string{"a", "b"}, map[string]bool{}, []string{"a", "b"}},
		{"an existing jot row is a decision", false, []string{"a", "b"}, map[string]bool{"b": true}, []string{"a"}},
		{"no journal, nothing to do", false, nil, map[string]bool{}, nil},
		{"already ran", true, []string{"a"}, map[string]bool{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeExtrasSettings{vals: map[string]string{}}
			if tt.ran {
				st.vals[jotNotesSplitKey] = "1"
			}
			svc := &fakeJotAddons{journal: tt.journal, rows: tt.rows}
			n, err := splitJotNotesOnce(context.Background(), st, svc)
			if err != nil {
				t.Fatal(err)
			}
			if n != len(tt.want) || len(svc.enabled) != len(tt.want) {
				t.Fatalf("enabled %v, want %v", svc.enabled, tt.want)
			}
			for i := range tt.want {
				if svc.enabled[i] != tt.want[i] {
					t.Fatalf("enabled %v, want %v", svc.enabled, tt.want)
				}
			}
			if st.vals[jotNotesSplitKey] != "1" {
				t.Fatal("run not recorded")
			}
			// A second run changes nothing.
			if n, _ := splitJotNotesOnce(context.Background(), st, svc); n != 0 {
				t.Fatalf("second run enabled %d", n)
			}
		})
	}
}
