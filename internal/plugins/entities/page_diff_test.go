package entities

import (
	"strings"
	"testing"
	"time"
)

func TestHTMLToPlainLines(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"paragraphs become lines", "<p>One</p><p>Two <b>bold</b></p>", "One\nTwo bold"},
		{"list items and headings", "<h2>Inn</h2><ul><li>Ale</li><li>Bread</li></ul>", "Inn\nAle\nBread"},
		{"br splits", "<p>a<br>b</p>", "a\nb"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HTMLToPlainLines(tt.in); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDiffWords(t *testing.T) {
	tests := []struct {
		name, older, newer string
		added, removed     string
	}{
		{"word swap", "The door is open.", "The door is barred.", "barred.", "open."},
		{"pure add", "A", "A and B", " and B", ""},
		{"from nothing", "", "New page", "New page", ""},
		{"no change", "same", "same", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var add, del, joined strings.Builder
			for _, p := range DiffWords(tt.older, tt.newer) {
				switch p.Op {
				case '+':
					add.WriteString(p.Text)
					joined.WriteString(p.Text)
				case '-':
					del.WriteString(p.Text)
				default:
					joined.WriteString(p.Text)
				}
			}
			if add.String() != tt.added || del.String() != tt.removed {
				t.Errorf("added %q removed %q, want %q / %q", add.String(), del.String(), tt.added, tt.removed)
			}
			if joined.String() != tt.newer {
				t.Errorf("kept+added must rebuild the newer text: %q", joined.String())
			}
		})
	}
}

func TestDiffWords_LongTextFallsBack(t *testing.T) {
	older := strings.Repeat("word ", 3000)
	newer := older + "\nend"
	parts := DiffWords(older, newer)
	if len(parts) == 0 {
		t.Fatal("no diff")
	}
}

func TestCountLineChanges(t *testing.T) {
	tests := []struct {
		name, older, newer string
		add, rem           int
	}{
		{"changed line", "a\nb", "a\nc", 1, 1},
		{"two added", "a", "a\nb\nc", 2, 0},
		{"created", "", "a\nb", 2, 0},
		{"same", "a", "a", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			add, rem := CountLineChanges(tt.older, tt.newer)
			if add != tt.add || rem != tt.rem {
				t.Errorf("got +%d -%d, want +%d -%d", add, rem, tt.add, tt.rem)
			}
		})
	}
}

func TestTrashRetentionAndDaysLeft(t *testing.T) {
	for in, want := range map[int]int{30: 30, 365: 365, 7: 30, 0: 30, -1: 30, 90: 90} {
		if got := NormalizeTrashRetention(in); got != want {
			t.Errorf("NormalizeTrashRetention(%d) = %d, want %d", in, got, want)
		}
	}
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		deleted time.Time
		want    int
	}{
		{now, 30},
		{now.Add(-3 * 24 * time.Hour), 27},
		{now.Add(-29*24*time.Hour - time.Hour), 1},
		{now.Add(-31 * 24 * time.Hour), 0},
	}
	for _, tt := range tests {
		if got := DaysLeft(tt.deleted, 30, now); got != tt.want {
			t.Errorf("DaysLeft(%v) = %d, want %d", tt.deleted, got, tt.want)
		}
	}
}

func TestHistoryLabels(t *testing.T) {
	now := time.Date(2026, 10, 3, 21, 30, 0, 0, time.UTC)
	for in, want := range map[time.Time]string{
		time.Date(2026, 10, 3, 21, 14, 0, 0, time.UTC): "Today 21:14",
		time.Date(2026, 10, 2, 18, 2, 0, 0, time.UTC):  "Yesterday 18:02",
		time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC):   "28 Sep",
		time.Date(2025, 9, 28, 9, 0, 0, 0, time.UTC):   "28 Sep 2025",
	} {
		if got := whenLabel(in, now); got != want {
			t.Errorf("whenLabel(%v) = %q, want %q", in, got, want)
		}
	}

	sam, me := "u-sam", "u-me"
	d := HistoryPanelData{ViewerID: me, Roles: map[string]string{sam: "scribe"}}
	cases := []struct {
		v    VersionView
		who  string
		what string
	}{
		{VersionView{EntityVersion: EntityVersion{UserID: &sam, UserName: "Sam", Kind: VersionEdit}, Added: 2, Removed: 1}, "Sam (scribe)", "+2 lines −1"},
		{VersionView{EntityVersion: EntityVersion{UserID: &me, UserName: "Me", Kind: VersionEdit}, Added: 6}, "You", "+6 lines"},
		{VersionView{EntityVersion: EntityVersion{UserID: &me, Kind: VersionCreated}, First: true}, "You", "created"},
		{VersionView{EntityVersion: EntityVersion{Kind: VersionBaseline}, First: true}, "Earlier version", ""},
		{VersionView{EntityVersion: EntityVersion{UserID: &sam, UserName: "Sam", Kind: VersionRestore}, Removed: 1}, "Sam (scribe)", "restored · −1 line"},
	}
	for _, c := range cases {
		if got := versionAuthor(c.v, d); got != c.who {
			t.Errorf("author = %q, want %q", got, c.who)
		}
		if got := versionChange(c.v); got != c.what {
			t.Errorf("change = %q, want %q", got, c.what)
		}
	}
}
