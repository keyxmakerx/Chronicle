package dmscreen

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

type fakeNotes struct {
	notes      map[string]*ScreenNote // by night key
	savedTitle string
	savedKey   string
	savedVer   string
	saveErr    error
}

func (f *fakeNotes) Find(_ context.Context, _, key string, _ Viewer) (*ScreenNote, error) {
	return f.notes[key], nil
}

func (f *fakeNotes) Save(_ context.Context, _, key string, _ Viewer, title, entry, _, version string) (*ScreenNote, error) {
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	f.savedTitle, f.savedKey, f.savedVer = title, key, version
	if f.notes == nil {
		f.notes = map[string]*ScreenNote{}
	}
	f.notes[key] = &ScreenNote{ID: "n1", Title: title, Entry: entry}
	return f.notes[key], nil
}

type fakeNight struct{ n *NightView }

func (f fakeNight) NextNight(context.Context, string, Viewer) (*NightView, error) { return f.n, nil }

func appErrCode(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

func TestSaveNote(t *testing.T) {
	night := &NightView{Key: "sess-1:2026-10-16", Name: "Session 12"}
	tests := []struct {
		name      string
		role      int
		night     *NightView
		text      string
		wantErr   bool
		wantTitle string
		wantLabel string
		wantKey   string
	}{
		{"with a night", 3, night, "plan\nmore", false, "Session 12: DM notes", "Kept with Session 12", "sess-1:2026-10-16"},
		{"no night is the standing note", 3, nil, "plan", false, "DM Screen notes", "DM Screen notes", ""},
		{"player refused", 1, nil, "plan", true, "", "", ""},
		{"scribe refused", 2, nil, "plan", true, "", "", ""},
		{"too long", 3, nil, strings.Repeat("x", maxNoteChars+1), true, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := &fakeNotes{}
			svc := NewService(Sources{Notes: fn, Nights: fakeNight{tt.night}})
			got, err := svc.SaveNote(context.Background(), "c1", Viewer{UserID: "u", Role: tt.role}, tt.text, "")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if err != nil {
				if len(fn.notes) != 0 {
					t.Error("a refused save wrote the note")
				}
				return
			}
			if fn.savedTitle != tt.wantTitle || got.Label != tt.wantLabel || fn.savedKey != tt.wantKey {
				t.Errorf("title %q label %q key %q", fn.savedTitle, got.Label, fn.savedKey)
			}
			if got.Text != tt.text || got.Link != "/campaigns/c1/journal/n1" || got.Version == "" {
				t.Errorf("view = %+v", got)
			}
		})
	}
}

// Each night keeps its own note, and a night with no key shares the standing one.
func TestNotesFollowTheNight(t *testing.T) {
	fn := &fakeNotes{}
	nights := []*NightView{{Key: "a:1", Name: "One"}, {Key: "a:2", Name: "Two"}, nil}
	for i, n := range nights {
		svc := NewService(Sources{Notes: fn, Nights: fakeNight{n}})
		if _, err := svc.SaveNote(context.Background(), "c1", Viewer{Role: 3}, strings.Repeat("x", i+1), ""); err != nil {
			t.Fatal(err)
		}
	}
	if len(fn.notes) != 3 {
		t.Fatalf("notes = %d, want one per night plus the standing note", len(fn.notes))
	}
}

func TestSaveNote_RefusesRichAndStale(t *testing.T) {
	rich := `{"type":"doc","content":[{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"Plan"}]}]}`
	plain, _ := proseFromPlain("old")
	tests := []struct {
		name     string
		stored   *ScreenNote
		version  string
		saveErr  error
		wantCode int
	}{
		{"formatted note is never overwritten", &ScreenNote{ID: "n", Entry: rich}, BodyVersion(rich), nil, http.StatusUnprocessableEntity},
		{"legacy block note is never overwritten", &ScreenNote{ID: "n", Legacy: "old"}, BodyVersion(""), nil, http.StatusUnprocessableEntity},
		{"plain note saves with its version", &ScreenNote{ID: "n", Entry: plain}, BodyVersion(plain), nil, 0},
		{"source reports a stale version", &ScreenNote{ID: "n", Entry: plain}, "stale", ErrNoteChanged, http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := &fakeNotes{notes: map[string]*ScreenNote{"": tt.stored}, saveErr: tt.saveErr}
			svc := NewService(Sources{Notes: fn})
			_, err := svc.SaveNote(context.Background(), "c1", Viewer{Role: 3}, "new", tt.version)
			if got := appErrCode(err); got != tt.wantCode {
				t.Fatalf("code = %d (%v), want %d", got, err, tt.wantCode)
			}
			if tt.wantCode == 0 && fn.savedVer != tt.version {
				t.Errorf("version passed through = %q", fn.savedVer)
			}
			if tt.wantCode == http.StatusUnprocessableEntity && fn.savedKey != "" {
				t.Error("a refused save reached the source")
			}
		})
	}
}

func TestBuild_NotesTab(t *testing.T) {
	plain, _ := proseFromPlain("saved line")
	rich := `{"type":"doc","content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"item"}]}]}]}]}`
	tests := []struct {
		name     string
		stored   *ScreenNote
		wantText string
		wantLink bool
		readOnly bool
	}{
		{"nothing saved yet", nil, "", false, false},
		{"saved plain note", &ScreenNote{ID: "n9", Entry: plain}, "saved line", true, false},
		{"formatted note shows read-only", &ScreenNote{ID: "n9", Entry: rich}, "item", true, true},
		{"legacy note shows read-only", &ScreenNote{ID: "n9", Legacy: "old text"}, "old text", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := &fakeNotes{notes: map[string]*ScreenNote{"a:1": tt.stored}}
			v, err := NewService(Sources{Notes: fn, Nights: fakeNight{&NightView{Key: "a:1", Name: "Fri"}}}).Build(context.Background(), "c1", Viewer{Role: 3})
			if err != nil || v.Notes == nil {
				t.Fatalf("err %v notes %+v", err, v.Notes)
			}
			n := v.Notes
			if n.Text != tt.wantText || (n.Link != "") != tt.wantLink || n.ReadOnly != tt.readOnly || n.Label != "Kept with Fri" {
				t.Errorf("notes = %+v", n)
			}
			if tt.stored != nil && n.Version == "" {
				t.Error("a stored note needs a version")
			}
		})
	}
	if v, _ := NewService(Sources{}).Build(context.Background(), "c1", Viewer{Role: 3}); v.Notes != nil {
		t.Error("no source should hide the tab")
	}
}

func TestIsPlainProse(t *testing.T) {
	plain, _ := proseFromPlain("a\n\nb")
	tests := []struct {
		name  string
		entry string
		want  bool
	}{
		{"empty", "", true},
		{"our own output", plain, true},
		{"bold mark", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"bold"}]}]}]}`, false},
		{"heading", `{"type":"doc","content":[{"type":"heading","content":[{"type":"text","text":"x"}]}]}`, false},
		{"image", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"image","attrs":{"src":"/a.png"}}]}]}`, false},
		{"mention", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"id":"e1"}}]}]}`, false},
		{"hard break", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x"},{"type":"hardBreak"}]}]}`, false},
		{"aligned paragraph", `{"type":"doc","content":[{"type":"paragraph","attrs":{"textAlign":"center"},"content":[{"type":"text","text":"x"}]}]}`, false},
		{"not json", "garbage", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPlainProse(tt.entry); got != tt.want {
				t.Errorf("isPlainProse = %v, want %v", got, tt.want)
			}
		})
	}
}

// Scribes may open the screen but never see or write the DM note.
func TestScribeHasNoNotes(t *testing.T) {
	fn := &fakeNotes{notes: map[string]*ScreenNote{"": {ID: "n"}}}
	svc := NewService(Sources{Notes: fn})
	scribe := Viewer{UserID: "s", Role: 2}
	if _, err := svc.Note(context.Background(), "c1", scribe); appErrCode(err) != http.StatusForbidden {
		t.Errorf("Note: %v, want 403", err)
	}
	if _, err := svc.SaveNote(context.Background(), "c1", scribe, "x", ""); appErrCode(err) != http.StatusForbidden {
		t.Errorf("SaveNote: %v, want 403", err)
	}
	if fn.savedKey != "" || fn.savedVer != "" {
		t.Error("a scribe's save reached the source")
	}
	v, err := svc.Build(context.Background(), "c1", scribe)
	if err != nil || v.Notes != nil {
		t.Errorf("Build: notes = %+v err %v, want no notes section", v.Notes, err)
	}
}
