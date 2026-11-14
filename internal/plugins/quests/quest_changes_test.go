package quests

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ChangesSince reads only what the viewer may see: a quest on a page a player
// may not open, and one whose notice the DM hid, count in nothing and name
// nothing, whatever their sheets say.
func TestChangesSince(t *testing.T) {
	ctx := context.Background()
	day := func(d int) time.Time { return time.Date(2026, 11, d, 12, 0, 0, 0, time.UTC) }
	since := day(5)
	build := func() questEnv {
		e := digestEnv(t, onlyCalendar{})
		e.sheet("fresh", "Who is buying the grain", false, day(13), `{"status":"not_started","notice":{"title":"Who is buying the grain?"}}`)
		e.repo.versions["fresh"] = 1
		e.sheet("edited", "The abbey bell", false, day(12), `{"status":"active","notice":{"title":"Find the bell"}}`)
		e.repo.versions["edited"] = 4
		e.sheet("wrapped", "Escort the envoy", false, day(11), `{"status":"done","notice":{"title":"Escort the envoy"}}`)
		e.repo.versions["wrapped"] = 3
		e.sheet("lost", "Pay the ferryman", false, day(10), `{"status":"failed","notice":{"title":"Pay the ferryman"}}`)
		e.repo.versions["lost"] = 2
		e.sheet("old", "Ancient news", false, day(1), `{"status":"done","notice":{"title":"ANCIENT"}}`)
		e.repo.versions["old"] = 2
		e.sheet("secret-new", "The Pretender Plot", true, day(14), `{"status":"active","notice":{"title":"SECRET NEW PLOT"}}`)
		e.repo.versions["secret-new"] = 1
		e.sheet("secret-done", "The Cult", true, day(14), `{"status":"done","notice":{"title":"SECRET FINISHED PLOT"}}`)
		e.repo.versions["secret-done"] = 2
		e.sheet("hidden-notice", "Quiet job", false, day(14), `{"status":"active","notice":{"title":"HIDDEN NOTICE"},"layout":{"notice":{"x":1,"y":1,"w":30,"hidden":true},"map":{"x":1,"y":1,"w":30},"tag":{"x":1,"y":1,"w":30}}}`)
		e.repo.versions["hidden-notice"] = 1
		return e
	}
	titles := func(l []QuestChange) string {
		var out []string
		for _, c := range l {
			out = append(out, c.Title)
		}
		return strings.Join(out, " | ")
	}
	tests := []struct {
		name         string
		viewer       Viewer
		wantNew      string
		wantFinished string
	}{
		{"a player sees only what they may open", player, "Who is buying the grain?", "Escort the envoy | Pay the ferryman"},
		{"a guest reads as a player", guest, "Who is buying the grain?", "Escort the envoy | Pay the ferryman"},
		{"the DM sees the hidden ones too", dm, "HIDDEN NOTICE | SECRET NEW PLOT | Who is buying the grain?", "SECRET FINISHED PLOT | Escort the envoy | Pay the ferryman"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := build().svc.ChangesSince(ctx, camp, tt.viewer, since)
			if err != nil {
				t.Fatal(err)
			}
			if titles(got.New) != tt.wantNew || titles(got.Finished) != tt.wantFinished {
				t.Errorf("new %q finished %q, want %q and %q", titles(got.New), titles(got.Finished), tt.wantNew, tt.wantFinished)
			}
			if got.TypeSlug != "quest" {
				t.Errorf("type slug = %q", got.TypeSlug)
			}
		})
	}
	t.Run("another campaign's sheets are never read", func(t *testing.T) {
		e := build()
		e.ents.add(other, "foreign", "Foreign quest", false)
		e.repo.docs["foreign"], e.repo.camp["foreign"] = []byte(`{"status":"done","notice":{"title":"FOREIGN"}}`), other
		e.repo.saved["foreign"] = day(14)
		got, _ := e.svc.ChangesSince(ctx, camp, dm, since)
		if strings.Contains(titles(got.Finished), "FOREIGN") {
			t.Errorf("reads across campaigns: %q", titles(got.Finished))
		}
	})
	t.Run("nothing changed", func(t *testing.T) {
		got, err := build().svc.ChangesSince(ctx, camp, player, day(20))
		if err != nil || len(got.New) != 0 || len(got.Finished) != 0 || got.TypeSlug != "" {
			t.Errorf("got %+v, %v", got, err)
		}
	})
}
