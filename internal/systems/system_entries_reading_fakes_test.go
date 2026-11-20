package systems

import (
	"context"
	"sort"
	"strings"
)

// The in-memory half of fakeEntryRepo for ties, left-out entries, old names
// and notes, with the database's unique keys and cascades.

func (f *fakeEntryRepo) standIn(camp, sys, field, slug string) *SystemEntry {
	for i := range f.rows {
		r := f.rows[i]
		if r.CampaignID == camp && r.SystemID == sys && r.FieldKey == field && r.StandsFor == slug {
			return &r
		}
	}
	return nil
}

func (f *fakeEntryRepo) cascade(entryID int64) {
	names := f.oldNames[:0]
	for _, n := range f.oldNames {
		if n.EntryID != entryID {
			names = append(names, n)
		}
	}
	f.oldNames = names
	notes := f.notes[:0]
	for _, n := range f.notes {
		if n.EntryID != entryID {
			notes = append(notes, n)
		}
	}
	f.notes = notes
}

func (f *fakeEntryRepo) SetStandsFor(_ context.Context, camp string, id int64, standsFor, seen string) error {
	for i := range f.rows {
		r := &f.rows[i]
		if r.ID != id || r.CampaignID != camp {
			continue
		}
		if standsFor != "" {
			if o := f.standIn(camp, r.SystemID, r.FieldKey, standsFor); o != nil && o.ID != id {
				return errStandInTaken
			}
		}
		r.StandsFor, r.StandsForSeen = standsFor, seen
		if standsFor == "" {
			r.StandsForSeen = ""
		}
	}
	return nil
}

func (f *fakeEntryRepo) SetStandsForSeen(_ context.Context, camp string, id int64, seen string) error {
	for i := range f.rows {
		if f.rows[i].ID == id && f.rows[i].CampaignID == camp && f.rows[i].StandsFor != "" {
			f.rows[i].StandsForSeen = seen
		}
	}
	return nil
}

func (f *fakeEntryRepo) FindStandIn(_ context.Context, camp, sys, field, slug string) (*SystemEntry, error) {
	return f.standIn(camp, sys, field, slug), nil
}

func (f *fakeEntryRepo) ListLeftOut(_ context.Context, camp, sys, field string) ([]LeftOut, error) {
	out := []LeftOut{}
	for _, l := range f.leftOut {
		if l.CampaignID == camp && l.SystemID == sys && (field == "" || l.FieldKey == field) {
			out = append(out, l)
		}
	}
	return out, nil
}

func (f *fakeEntryRepo) PutLeftOut(_ context.Context, l *LeftOut) error {
	for i := range f.leftOut {
		o := &f.leftOut[i]
		if o.CampaignID == l.CampaignID && o.SystemID == l.SystemID && o.FieldKey == l.FieldKey && o.Slug == l.Slug {
			o.Sentence = l.Sentence
			return nil
		}
	}
	f.leftOut = append(f.leftOut, *l)
	return nil
}

func (f *fakeEntryRepo) DeleteLeftOut(_ context.Context, camp, sys, field, slug string) (bool, error) {
	for i, l := range f.leftOut {
		if l.CampaignID == camp && l.SystemID == sys && l.FieldKey == field && l.Slug == slug {
			f.leftOut = append(f.leftOut[:i], f.leftOut[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeEntryRepo) ListOldNames(_ context.Context, camp, sys, field string) ([]EntryOldName, error) {
	out := []EntryOldName{}
	for _, n := range f.oldNames {
		if n.CampaignID == camp && n.SystemID == sys && (field == "" || n.FieldKey == field) {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *fakeEntryRepo) FindOldName(_ context.Context, camp, sys, field, name string) (*EntryOldName, error) {
	for _, n := range f.oldNames {
		if n.CampaignID == camp && n.SystemID == sys && n.FieldKey == field && strings.EqualFold(n.Name, name) {
			c := n
			return &c, nil
		}
	}
	return nil, nil
}

func (f *fakeEntryRepo) AddOldName(ctx context.Context, n *EntryOldName) error {
	if o, _ := f.FindOldName(ctx, n.CampaignID, n.SystemID, n.FieldKey, n.Name); o != nil {
		return nil
	}
	f.nextID++
	n.ID = f.nextID
	f.oldNames = append(f.oldNames, *n)
	return nil
}

func (f *fakeEntryRepo) DeleteOldName(_ context.Context, camp string, id int64) (bool, error) {
	for i, n := range f.oldNames {
		if n.ID == id && n.CampaignID == camp {
			f.oldNames = append(f.oldNames[:i], f.oldNames[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeEntryRepo) AddNote(_ context.Context, n *EntryNote) error {
	f.nextID++
	n.ID = f.nextID
	f.notes = append(f.notes, *n)
	return nil
}

func (f *fakeEntryRepo) ListNotes(_ context.Context, camp string, entryID int64, limit int) ([]EntryNote, error) {
	out := []EntryNote{}
	for i := len(f.notes) - 1; i >= 0 && len(out) < limit; i-- {
		if n := f.notes[i]; n.CampaignID == camp && n.EntryID == entryID {
			out = append(out, n)
		}
	}
	return out, nil
}

// fakeData is an in-memory DataProvider.
type fakeData map[string][]ReferenceItem

func (d fakeData) List(cat string) ([]ReferenceItem, error) {
	return append([]ReferenceItem(nil), d[cat]...), nil
}
func (d fakeData) Get(cat, id string) (*ReferenceItem, error) {
	for _, it := range d[cat] {
		if it.ID == id {
			c := it
			return &c, nil
		}
	}
	return nil, nil
}
func (d fakeData) Search(string) ([]ReferenceItem, error) { return nil, nil }
func (d fakeData) Categories() []string                   { return nil }

// fakeSystem is a game system package with its data.
type fakeSystem struct {
	m    *SystemManifest
	data fakeData
}

func (s fakeSystem) Info() *SystemManifest            { return s.m }
func (s fakeSystem) DataProvider() DataProvider       { return s.data }
func (s fakeSystem) TooltipRenderer() TooltipRenderer { return nil }
