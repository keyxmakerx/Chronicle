package cutscenes

import (
	"context"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

var ctx = context.Background()

func code(err error) int {
	if err == nil {
		return 0
	}
	return apperror.SafeCode(err)
}

// Only a Director reads, writes or shows scenes; everyone else is told the
// page is not there.
func TestDirectorOnly(t *testing.T) {
	r := newRig()
	sc := r.scene(t, "One")
	calls := []struct {
		name string
		call func(v Viewer) error
	}{
		{"list", func(v Viewer) error { _, err := r.svc.List(ctx, "camp-1", v); return err }},
		{"get", func(v Viewer) error { _, err := r.svc.Get(ctx, "camp-1", sc.ID, v); return err }},
		{"create", func(v Viewer) error {
			in := NewScene()
			in.Name = "x-" + v.UserID
			_, err := r.svc.Create(ctx, "camp-1", v, in)
			return err
		}},
		{"save", func(v Viewer) error { _, err := r.svc.Save(ctx, "camp-1", sc.ID, v, *sc); return err }},
		{"check", func(v Viewer) error { _, err := r.svc.Check(ctx, "camp-1", v, *sc); return err }},
		{"audience", func(v Viewer) error { _, err := r.svc.Audience(ctx, "camp-1", v); return err }},
		{"show", func(v Viewer) error {
			_, err := r.svc.Show(ctx, "camp-1", sc.ID, v, ShowInput{To: []string{"ana"}})
			return err
		}},
		{"recent", func(v Viewer) error { _, err := r.svc.Recent(ctx, "camp-1", v); return err }},
		{"live", func(v Viewer) error { _, err := r.svc.LiveFor(ctx, "camp-1", sc.ID, v); return err }},
		{"delete", func(v Viewer) error { return r.svc.Delete(ctx, "camp-1", "missing", v) }},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			for _, v := range []Viewer{vAna, {UserID: ""}, {UserID: "", Director: true}} {
				if got := code(c.call(v)); got != 404 {
					t.Errorf("%+v: code %d, want 404", v, got)
				}
			}
			if err := c.call(vCoDM); err != nil && code(err) != 404 {
				t.Errorf("a co-DM was refused: %v", err)
			}
		})
	}
	if len(r.pub.shown) != 1 {
		t.Errorf("only the co-DM's show went out, got %d", len(r.pub.shown))
	}
}

func TestScenesStayInTheirCampaign(t *testing.T) {
	r := newRig()
	sc := r.scene(t, "One")
	if _, err := r.svc.Get(ctx, "camp-2", sc.ID, vOther); code(err) != 404 {
		t.Errorf("camp-2 read camp-1's scene: %v", err)
	}
	if _, err := r.svc.Show(ctx, "camp-2", sc.ID, vOther, ShowInput{To: []string{"other"}}); code(err) != 404 {
		t.Errorf("camp-2 showed camp-1's scene: %v", err)
	}
	if list, _ := r.svc.List(ctx, "camp-2", vOther); len(list) != 0 {
		t.Errorf("camp-2 lists %d scenes", len(list))
	}
}

func TestCreateAndSave(t *testing.T) {
	r := newRig()
	sc := r.scene(t, "One")
	if _, err := r.svc.Create(ctx, "camp-1", vDir, Scene{Name: "one"}); code(err) != 409 {
		t.Errorf("a second scene of the same name: %v", err)
	}
	edit := *sc
	edit.Background = Background{Kind: BackgroundBlack}
	edit.Overlays = nil
	got, err := r.svc.Save(ctx, "camp-1", sc.ID, vDir, edit)
	if err != nil {
		t.Fatal(err)
	}
	if got.Background.Kind != BackgroundBlack || len(got.Overlays) != 0 || got.ID != sc.ID {
		t.Errorf("saved %+v", got)
	}
	bad := *sc
	bad.Overlays = []Overlay{{Kind: "dragon"}}
	if _, err := r.svc.Save(ctx, "camp-1", sc.ID, vDir, bad); code(err) != 400 {
		t.Errorf("a bad save: %v", err)
	}
	if _, err := r.svc.Save(ctx, "camp-1", "missing", vDir, edit); code(err) != 404 {
		t.Errorf("saving a missing scene: %v", err)
	}
}

func TestShow(t *testing.T) {
	r := newRig()
	sc := r.scene(t, "One")
	tests := []struct {
		name     string
		in       ShowInput
		wantCode int
		viewers  int
		hours    int
	}{
		{"to one player, tonight by default", ShowInput{To: []string{"ana"}}, 0, 1, 12},
		{"to several, one hour", ShowInput{To: []string{"ana", "bo", "ana", " "}, Rewatch: "1h"}, 0, 2, 1},
		{"to the Director too, three days", ShowInput{To: []string{"ana", "dir"}, Rewatch: "72h"}, 0, 2, 72},
		{"nobody picked", ShowInput{}, 400, 0, 0},
		{"a stranger picked", ShowInput{To: []string{"ana", "stranger"}}, 400, 0, 0},
		{"an unknown window", ShowInput{To: []string{"ana"}, Rewatch: "forever"}, 400, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := len(r.pub.shown)
			sh, err := r.svc.Show(ctx, "camp-1", sc.ID, vDir, tt.in)
			if tt.wantCode != 0 {
				if code(err) != tt.wantCode {
					t.Fatalf("err %v, want %d", err, tt.wantCode)
				}
				if len(r.pub.shown) != before {
					t.Errorf("a refused show went out live")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(sh.Viewers) != tt.viewers || sh.RewatchUntil.Sub(sh.ShownAt) != time.Duration(tt.hours)*time.Hour {
				t.Errorf("showing %+v", sh)
			}
			if len(r.pub.shown) != before+1 || r.pub.shown[len(r.pub.shown)-1].ID != sh.ID {
				t.Errorf("not published")
			}
			if sh.Scene.Name != "One" || sh.SceneID != sc.ID {
				t.Errorf("snapshot %+v", sh.Scene)
			}
		})
	}
}

// A showing keeps the scene as it was shown: a later edit or delete does not
// change what its people can watch again.
func TestShowingIsASnapshot(t *testing.T) {
	r := newRig()
	sc := r.scene(t, "One")
	sh, err := r.svc.Show(ctx, "camp-1", sc.ID, vDir, ShowInput{To: []string{"ana"}})
	if err != nil {
		t.Fatal(err)
	}
	edit := *sc
	edit.Name = "Renamed"
	edit.Text = []TextLine{{Say: "Different"}}
	if _, err := r.svc.Save(ctx, "camp-1", sc.ID, vDir, edit); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Delete(ctx, "camp-1", sc.ID, vDir); err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Read(ctx, "camp-1", sh.ID, vAna)
	if err != nil {
		t.Fatal(err)
	}
	if got.Scene.Name != "One" || got.Scene.Text[0].Say != "Something has noticed you." {
		t.Errorf("snapshot changed: %+v", got.Scene)
	}
}

// Only the people a showing was for read it, and only inside the window; a
// Director reads any; everyone else gets the same 404 as a missing id.
func TestReadShowing(t *testing.T) {
	r := newRig()
	sc := r.scene(t, "One")
	sh, err := r.svc.Show(ctx, "camp-1", sc.ID, vDir, ShowInput{To: []string{"ana"}, Rewatch: "1h"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		v     Viewer
		camp  string
		id    string
		after time.Duration
		want  int
	}{
		{"the viewer, inside the window", vAna, "camp-1", sh.ID, 30 * time.Minute, 0},
		{"another player", vBo, "camp-1", sh.ID, 0, 404},
		{"anonymous", Viewer{}, "camp-1", sh.ID, 0, 404},
		{"the viewer, as the window closes", vAna, "camp-1", sh.ID, time.Hour, 404},
		{"the viewer, after the window", vAna, "camp-1", sh.ID, 2 * time.Hour, 404},
		{"the Director, after the window", vDir, "camp-1", sh.ID, 48 * time.Hour, 0},
		{"a co-DM not picked", vCoDM, "camp-1", sh.ID, 0, 0},
		{"another campaign's Director", vOther, "camp-2", sh.ID, 0, 404},
		{"a missing id", vAna, "camp-1", "missing", 0, 404},
	}
	start := r.clock.t
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r.clock.t = start.Add(tt.after)
			got, err := r.svc.Read(ctx, tt.camp, tt.id, tt.v)
			if code(err) != tt.want {
				t.Fatalf("err %v, want %d", err, tt.want)
			}
			if tt.want == 404 {
				if err.Error() != notFound().Error() {
					t.Errorf("a refusal says more than a missing id: %v", err)
				}
				return
			}
			if got.ShowingID != sh.ID || got.Scene.Name != "One" {
				t.Errorf("read %+v", got)
			}
		})
	}
}

func TestMineAndWatched(t *testing.T) {
	r := newRig()
	sc := r.scene(t, "One")
	first, _ := r.svc.Show(ctx, "camp-1", sc.ID, vDir, ShowInput{To: []string{"ana", "bo"}, Rewatch: "1h"})
	r.clock.t = r.clock.t.Add(10 * time.Minute)
	second, _ := r.svc.Show(ctx, "camp-1", sc.ID, vDir, ShowInput{To: []string{"ana"}, Rewatch: "72h"})

	mine, err := r.svc.Mine(ctx, "camp-1", vAna)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 2 || mine[0].ShowingID != second.ID || mine[1].ShowingID != first.ID {
		t.Fatalf("mine %+v, want newest first", mine)
	}
	if bo, _ := r.svc.Mine(ctx, "camp-1", vBo); len(bo) != 1 {
		t.Errorf("bo's list %+v", bo)
	}
	if _, err := r.svc.Mine(ctx, "camp-1", Viewer{}); code(err) != 404 {
		t.Errorf("anonymous mine: %v", err)
	}

	// Watched once: the first time is kept.
	if err := r.svc.Watched(ctx, "camp-1", first.ID, vAna); err != nil {
		t.Fatal(err)
	}
	stamp := *r.repo.showings[first.ID].Viewers[0].WatchedAt
	r.clock.t = r.clock.t.Add(time.Minute)
	if err := r.svc.Watched(ctx, "camp-1", first.ID, vAna); err != nil {
		t.Fatal(err)
	}
	if got := *r.repo.showings[first.ID].Viewers[0].WatchedAt; !got.Equal(stamp) {
		t.Errorf("watched moved from %v to %v", stamp, got)
	}
	if err := r.svc.Watched(ctx, "camp-1", second.ID, vBo); code(err) != 404 {
		t.Errorf("bo marked a showing he was not shown: %v", err)
	}
	mine, _ = r.svc.Mine(ctx, "camp-1", vAna)
	if mine[0].Watched || !mine[1].Watched {
		t.Errorf("watched flags %+v", mine)
	}

	// Stopped is sent so the stage never plays a called-off showing on
	// opening; the showing stays in the list and stays readable.
	if mine[0].Stopped || mine[1].Stopped {
		t.Errorf("stopped before any stop: %+v", mine)
	}
	if _, err := r.svc.Stop(ctx, "camp-1", second.ID, vDir); err != nil {
		t.Fatal(err)
	}
	mine, _ = r.svc.Mine(ctx, "camp-1", vAna)
	if len(mine) != 2 || !mine[0].Stopped || mine[1].Stopped {
		t.Errorf("stopped flags %+v", mine)
	}
	if _, err := r.svc.Read(ctx, "camp-1", second.ID, vAna); err != nil {
		t.Errorf("a stopped showing could not be watched again: %v", err)
	}

	// The hour passes: the first showing leaves the list and can no longer be read.
	r.clock.t = first.RewatchUntil.Add(time.Second)
	mine, _ = r.svc.Mine(ctx, "camp-1", vAna)
	if len(mine) != 1 || mine[0].ShowingID != second.ID {
		t.Errorf("after the window: %+v", mine)
	}
	if err := r.svc.Watched(ctx, "camp-1", first.ID, vBo); code(err) != 404 {
		t.Errorf("marked after the window: %v", err)
	}
}

func TestStop(t *testing.T) {
	r := newRig()
	sc := r.scene(t, "One")
	sh, _ := r.svc.Show(ctx, "camp-1", sc.ID, vDir, ShowInput{To: []string{"ana", "bo"}})
	if _, err := r.svc.Stop(ctx, "camp-1", sh.ID, vAna); code(err) != 404 {
		t.Errorf("a player stopped it: %v", err)
	}
	got, err := r.svc.Stop(ctx, "camp-1", sh.ID, vCoDM)
	if err != nil {
		t.Fatal(err)
	}
	if got.StoppedAt == nil || len(r.pub.stops) != 1 || got.Viewers[0].Name == "" {
		t.Errorf("stop %+v, %d published", got, len(r.pub.stops))
	}
	if _, err := r.svc.Stop(ctx, "camp-1", sh.ID, vDir); err != nil || len(r.pub.stops) != 1 {
		t.Errorf("a second stop went out again: %v, %d", err, len(r.pub.stops))
	}
	// Stopping leaves the window: the viewer can still watch it again.
	if _, err := r.svc.Read(ctx, "camp-1", sh.ID, vAna); err != nil {
		t.Errorf("rewatch after stop: %v", err)
	}
	if _, err := r.svc.Stop(ctx, "camp-2", sh.ID, vOther); code(err) != 404 {
		t.Errorf("another campaign stopped it: %v", err)
	}
	if live, _ := r.svc.LiveFor(ctx, "camp-1", sc.ID, vDir); len(live) != 0 {
		t.Errorf("a stopped showing is still live")
	}
}

func TestLiveAndRecent(t *testing.T) {
	r := newRig()
	sc := r.scene(t, "One")
	sh, _ := r.svc.Show(ctx, "camp-1", sc.ID, vDir, ShowInput{To: []string{"ana"}})
	if live, _ := r.svc.LiveFor(ctx, "camp-1", sc.ID, vDir); len(live) != 1 || live[0].Viewers[0].Name != "Ana" {
		t.Errorf("live %+v", live)
	}
	_ = r.svc.Watched(ctx, "camp-1", sh.ID, vAna)
	rec, err := r.svc.Recent(ctx, "camp-1", vDir)
	if err != nil || len(rec) != 1 || !rec[0].Live || !rec[0].Viewers[0].Watched || rec[0].Viewers[0].Name != "Ana" {
		t.Errorf("recent %+v %v", rec, err)
	}
	r.clock.t = r.clock.t.Add(LiveFor)
	if live, _ := r.svc.LiveFor(ctx, "camp-1", sc.ID, vDir); len(live) != 0 {
		t.Errorf("an old showing is still live")
	}
	// A scene waiting for a tap can still be up: Stop stays past Live, until
	// the showing is stopped or its window closes.
	if rec, _ := r.svc.Recent(ctx, "camp-1", vDir); rec[0].Live || !rec[0].CanStop {
		t.Errorf("past live: %+v", rec[0])
	}
	r.clock.t = sh.RewatchUntil
	if rec, _ := r.svc.Recent(ctx, "camp-1", vDir); rec[0].CanStop {
		t.Errorf("stoppable after its window: %+v", rec[0])
	}
	sh2, _ := r.svc.Show(ctx, "camp-1", sc.ID, vDir, ShowInput{To: []string{"ana"}})
	_, _ = r.svc.Stop(ctx, "camp-1", sh2.ID, vDir)
	if rec, _ := r.svc.Recent(ctx, "camp-1", vDir); rec[0].ShowingID != sh2.ID || rec[0].CanStop || !rec[0].Stopped {
		t.Errorf("stoppable once stopped: %+v", rec[0])
	}
}

func TestAudience(t *testing.T) {
	r := newRig()
	rows, err := r.svc.Audience(ctx, "camp-1", vDir)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	words := map[string]string{}
	for _, row := range rows {
		order = append(order, row.UserID)
		words[row.UserID] = row.Place.Words()
	}
	want := []string{"ana", "bo", "codm", "dir"}
	for i := range want {
		if i >= len(order) || order[i] != want[i] {
			t.Fatalf("order %v, want players, then Directors, then you: %v", order, want)
		}
	}
	if words["ana"] != "In Chronicle" || words["bo"] != "In Foundry" || words["dir"] != "Both" || words["codm"] != "Away" {
		t.Errorf("places %v", words)
	}
	if !rows[3].You {
		t.Errorf("the asker is not marked")
	}
}

func TestMemberShowings(t *testing.T) {
	r := newRig()
	sc := r.scene(t, "One")
	sh, _ := r.svc.Show(ctx, "camp-1", sc.ID, vDir, ShowInput{To: []string{"bo"}})
	got, err := r.svc.MemberShowings(ctx, "camp-1", "bo")
	if err != nil || len(got) != 1 || got[0].ShowingID != sh.ID || got[0].Scene.Name != "One" {
		t.Errorf("bo's showings %+v %v", got, err)
	}
	if got, _ := r.svc.MemberShowings(ctx, "camp-1", "ana"); len(got) != 0 {
		t.Errorf("ana sees bo's showing")
	}
	if got, _ := r.svc.MemberShowings(ctx, "camp-2", "bo"); len(got) != 0 {
		t.Errorf("another campaign lists it")
	}
	if _, err := r.svc.MemberShowings(ctx, "camp-1", " "); code(err) != 400 {
		t.Errorf("no member named: %v", err)
	}
}
