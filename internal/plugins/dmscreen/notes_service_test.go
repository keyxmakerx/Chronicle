package dmscreen

import (
	"context"
	"strings"
	"testing"
)

type fakeNotes struct {
	note       *ScreenNote
	savedTitle string
}

func (f *fakeNotes) Find(context.Context, string, Viewer) (*ScreenNote, error) { return f.note, nil }
func (f *fakeNotes) Save(_ context.Context, _ string, _ Viewer, title, entry, _ string) (*ScreenNote, error) {
	f.savedTitle = title
	f.note = &ScreenNote{ID: "n1", Title: title, Entry: entry}
	return f.note, nil
}

type fakeNight struct{ n *NightView }

func (f fakeNight) NextNight(context.Context, string, Viewer) (*NightView, error) { return f.n, nil }

func TestSaveNote(t *testing.T) {
	tests := []struct {
		name      string
		role      int
		night     *NightView
		text      string
		wantErr   bool
		wantTitle string
		wantLabel string
	}{
		{"with a night", 3, &NightView{Name: "Session 12"}, "plan\nmore", false, "Session 12: DM notes", "Kept with Session 12"},
		{"no night", 2, nil, "plan", false, "DM Screen notes", "DM Screen notes"},
		{"player refused", 1, nil, "plan", true, "", ""},
		{"too long", 3, nil, strings.Repeat("x", maxNoteChars+1), true, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := &fakeNotes{}
			svc := NewService(Sources{Notes: fn, Nights: fakeNight{tt.night}})
			got, err := svc.SaveNote(context.Background(), "c1", Viewer{UserID: "u", Role: tt.role}, tt.text)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if err != nil {
				if fn.note != nil {
					t.Error("a refused save wrote the note")
				}
				return
			}
			if fn.savedTitle != tt.wantTitle || got.Label != tt.wantLabel {
				t.Errorf("title %q label %q", fn.savedTitle, got.Label)
			}
			if got.Text != tt.text || got.Link != "/campaigns/c1/journal/n1" {
				t.Errorf("view = %+v", got)
			}
		})
	}
}

func TestBuild_NotesTab(t *testing.T) {
	entry, _ := proseFromPlain("saved line")
	tests := []struct {
		name     string
		src      *fakeNotes
		wantText string
		wantLink bool
	}{
		{"nothing saved yet", &fakeNotes{}, "", false},
		{"saved note", &fakeNotes{note: &ScreenNote{ID: "n9", Entry: entry}}, "saved line", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := NewService(Sources{Notes: tt.src, Nights: fakeNight{&NightView{Name: "Fri"}}}).Build(context.Background(), "c1", Viewer{Role: 3})
			if err != nil || v.Notes == nil {
				t.Fatalf("err %v notes %+v", err, v.Notes)
			}
			if v.Notes.Text != tt.wantText || (v.Notes.Link != "") != tt.wantLink || v.Notes.Label != "Kept with Fri" {
				t.Errorf("notes = %+v", v.Notes)
			}
		})
	}
	if v, _ := NewService(Sources{}).Build(context.Background(), "c1", Viewer{Role: 3}); v.Notes != nil {
		t.Error("no source should hide the tab")
	}
}
