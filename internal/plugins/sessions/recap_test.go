package sessions

import (
	"context"
	"strings"
	"testing"
)

func TestIsEditorDoc(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{`{"type":"doc","content":[]}`, true},
		{`We rowed out to the lighthouse.`, false},
		{`{"type":"paragraph"}`, false},
		{``, false},
	}
	for _, tt := range tests {
		if got := isEditorDoc(tt.in); got != tt.want {
			t.Errorf("isEditorDoc(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestRecapOpen(t *testing.T) {
	d := func(s string) *string { return &s }
	written := "<p>We rowed out.</p>"
	tests := []struct {
		name string
		s    Session
		want bool
	}{
		{"planned, date passed: no Mark Complete needed", Session{Status: StatusPlanned, ScheduledDate: d("2026-10-23")}, true},
		{"planned, tonight: open for writing up after the game", Session{Status: StatusPlanned, ScheduledDate: d("2026-10-24")}, true},
		{"planned, next week", Session{Status: StatusPlanned, ScheduledDate: d("2026-10-31")}, false},
		{"planned, undated", Session{Status: StatusPlanned}, false},
		{"completed, undated", Session{Status: StatusCompleted}, true},
		{"written early stays shown", Session{Status: StatusPlanned, ScheduledDate: d("2026-10-31"), RecapHTML: &written}, true},
		{"cancelled", Session{Status: StatusCancelled, ScheduledDate: d("2026-10-23")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := recapOpen(&tt.s, "2026-10-24"); got != tt.want {
				t.Errorf("recapOpen = %v, want %v", got, tt.want)
			}
		})
	}
}

// A recap typed in the calendar's plain box is text, not editor JSON, so
// the editor must be given only the HTML or it would fail to parse it.
func TestRecapEditorBody(t *testing.T) {
	plain, plainHTML := "We rowed out.", "<p>We rowed out.</p>"
	doc := `{"type":"doc","content":[]}`
	if got := recapEditorBody(&Session{Recap: &plain, RecapHTML: &plainHTML}); got["entry"] != nil || got["entry_html"] != plainHTML {
		t.Errorf("plain recap body = %v", got)
	}
	if got := recapEditorBody(&Session{Recap: &doc, RecapHTML: &plainHTML}); got["entry"] != doc {
		t.Errorf("editor recap body = %v", got)
	}
}

// Every member reads the recap as stored, so GM-only text never survives a
// save, whichever path sent it.
func TestUpdateSessionRecap_DropsSecrets(t *testing.T) {
	var gotRecap, gotHTML string
	repo := &mockSessionRepo{updateRecapFn: func(_ context.Context, _ string, recap, html *string) error {
		gotRecap, gotHTML = *recap, *html
		return nil
	}}
	svc := NewSessionService(repo, nil, nil)
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Open. "},{"type":"text","text":"The mayor is the cultist.","marks":[{"type":"secret"}]}]}]}`
	html := `<p>Open. <span data-secret="true">The mayor is the cultist.</span></p>`
	if err := svc.UpdateSessionRecap(context.Background(), "s1", &doc, &html); err != nil {
		t.Fatal(err)
	}
	for _, got := range []string{gotRecap, gotHTML} {
		if strings.Contains(got, "cultist") {
			t.Errorf("secret survived the save: %s", got)
		}
		if !strings.Contains(got, "Open.") {
			t.Errorf("open text lost: %s", got)
		}
	}
}

func TestLastNightToWriteUp(t *testing.T) {
	d := func(s string) *string { return &s }
	written := "<p>done</p>"
	list := []Session{
		{ID: "oct9", Status: StatusPlanned, ScheduledDate: d("2026-10-09")},
		{ID: "oct20", Status: StatusCompleted, ScheduledDate: d("2026-10-20")},
		{ID: "oct23", Status: StatusPlanned, ScheduledDate: d("2026-10-23")},
		{ID: "tonight", Status: StatusPlanned, ScheduledDate: d("2026-10-24")},
		{ID: "weekly", Status: StatusPlanned, ScheduledDate: d("2026-10-22"), IsRecurring: true},
		{ID: "off", Status: StatusCancelled, ScheduledDate: d("2026-10-23")},
	}
	tests := []struct {
		name    string
		written map[string]bool
		want    string
	}{
		{"latest past one-off with no recap", nil, "oct23"},
		{"latest already written: nothing, not an older one", map[string]bool{"oct23": true}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockSessionRepo{
				listByCampaignFn: func(context.Context, string) ([]Session, error) { return list, nil },
				findByIDFn: func(_ context.Context, id string) (*Session, error) {
					for _, s := range list {
						if s.ID == id {
							if tt.written[id] {
								s.RecapHTML = &written
							}
							return &s, nil
						}
					}
					return nil, nil
				},
			}
			got, err := NewSessionService(repo, nil, nil).LastNightToWriteUp(context.Background(), "c1", "2026-10-24")
			if err != nil {
				t.Fatal(err)
			}
			id := ""
			if got != nil {
				id = got.ID
			}
			if id != tt.want {
				t.Errorf("got %q, want %q", id, tt.want)
			}
		})
	}
}
