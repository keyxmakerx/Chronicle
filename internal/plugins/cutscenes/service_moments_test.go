package cutscenes

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// momentRig shows a scene with two moments (the second answered by one
// person) to Ana and Bo, and returns the showing.
func momentRig(t *testing.T) (*rig, *Showing) {
	t.Helper()
	r := newRig()
	in := NewScene()
	in.Name = "The Drowned Bell"
	in.Moments = []Moment{
		{At: 2, Ask: "What do the fishermen whisper?", Ways: []Way{{Kind: WayWrite}, {Kind: WayDirector}}, Note: "They lie about the bell."},
		{At: 5, Ask: "Who rings it?", Ways: []Way{{Kind: WayWrite}}, To: ToOne, Note: "Vell does."},
	}
	sc, err := r.svc.Create(ctx, "camp-1", vDir, in)
	if err != nil {
		t.Fatal(err)
	}
	sh, err := r.svc.Show(ctx, "camp-1", sc.ID, vDir, ShowInput{To: []string{"ana", "bo"}})
	if err != nil {
		t.Fatal(err)
	}
	return r, sh
}

func codeOf(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

// passFirst lets the scene's first moment go, so the second is the one it
// holds on.
func passFirst(r *rig, sh *Showing) {
	if _, err := r.svc.Go(ctx, "camp-1", sh.ID, 0, vDir); err != nil {
		panic(err)
	}
}

// Who may answer, and what an answer has to be.
func TestAnswerRules(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(r *rig, sh *Showing)
		v      Viewer
		moment int
		in     AnswerInput
		want   int
	}{
		{name: "a viewer writes", v: vAna, in: AnswerInput{Way: WayWrite, Text: "  The bell\nrang twice. "}, want: 0},
		{name: "a viewer leaves it to the Director", v: vBo, in: AnswerInput{Way: WayDirector}, want: 0},
		{name: "the Director is not a viewer", v: vDir, in: AnswerInput{Way: WayWrite, Text: "x"}, want: 404},
		{name: "a member it was not shown to", v: vCoDM, in: AnswerInput{Way: WayWrite, Text: "x"}, want: 404},
		{name: "nobody signed in", v: Viewer{}, in: AnswerInput{Way: WayWrite, Text: "x"}, want: 404},
		{name: "an empty written answer", v: vAna, in: AnswerInput{Way: WayWrite, Text: " \t\n"}, want: 400},
		{name: "too long", v: vAna, in: AnswerInput{Way: WayWrite, Text: strings.Repeat("a", MaxAnswer+1)}, want: 400},
		{name: "a way the moment does not offer", v: vAna, moment: 1, in: AnswerInput{Way: WayDirector}, want: 400, setup: passFirst},
		{name: "a moment the scene hasn't reached", v: vAna, moment: 1, in: AnswerInput{Way: WayWrite, Text: "Vell"}, want: 409},
		{name: "a way nobody knows", v: vAna, in: AnswerInput{Way: "dance"}, want: 400},
		{name: "no such moment", v: vAna, moment: 7, in: AnswerInput{Way: WayWrite, Text: "x"}, want: 400},
		{name: "a moment let go", v: vAna, in: AnswerInput{Way: WayWrite, Text: "x"}, want: 409,
			setup: func(r *rig, sh *Showing) {
				if _, err := r.svc.Go(ctx, "camp-1", sh.ID, 0, vDir); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "a scene the Director ended", v: vAna, in: AnswerInput{Way: WayWrite, Text: "x"}, want: 409,
			setup: func(r *rig, sh *Showing) {
				if _, err := r.svc.Stop(ctx, "camp-1", sh.ID, vDir); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "after the window closed", v: vAna, in: AnswerInput{Way: WayWrite, Text: "x"}, want: 404,
			setup: func(r *rig, sh *Showing) { r.clock.t = sh.RewatchUntil }},
		{name: "one person's moment someone else started", v: vBo, moment: 1, in: AnswerInput{Way: WayWrite, Text: "Bo"}, want: 409,
			setup: func(r *rig, sh *Showing) {
				passFirst(r, sh)
				if _, err := r.svc.Answer(ctx, "camp-1", sh.ID, 1, vAna, AnswerInput{Way: WayWrite, Text: "Vell"}); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "one person's moment, changed by the one answering", v: vAna, moment: 1, in: AnswerInput{Way: WayWrite, Text: "Vell, at dawn"}, want: 0,
			setup: func(r *rig, sh *Showing) {
				passFirst(r, sh)
				if _, err := r.svc.Answer(ctx, "camp-1", sh.ID, 1, vAna, AnswerInput{Way: WayWrite, Text: "Vell"}); err != nil {
					t.Fatal(err)
				}
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, sh := momentRig(t)
			if tt.setup != nil {
				tt.setup(r, sh)
			}
			before := len(r.pub.answered)
			got, err := r.svc.Answer(ctx, "camp-1", sh.ID, tt.moment, tt.v, tt.in)
			if code := codeOf(err); code != tt.want || (tt.want != 0 && err == nil) {
				t.Fatalf("code %d (%v), want %d", code, err, tt.want)
			}
			if tt.want != 0 {
				if len(r.pub.answered) != before {
					t.Errorf("a refused answer went out live")
				}
				return
			}
			if len(r.pub.answered) != before+1 || r.pub.answered[before].UserID != tt.v.UserID || r.pub.answered[before].Name == "" {
				t.Errorf("live: %+v", r.pub.answered)
			}
			if tt.in.Way == WayWrite && strings.ContainsAny(got.Text, "\n\t") {
				t.Errorf("stored text kept control characters: %q", got.Text)
			}
			if tt.in.Way == WayDirector && got.Text != "" {
				t.Errorf("a left-to-you answer kept words: %q", got.Text)
			}
		})
	}
}

// One answer per viewer per moment: a second replaces the first while it
// waits, and once decided it stays as decided.
func TestAnswerReplacesUntilDecided(t *testing.T) {
	r, sh := momentRig(t)
	for _, text := range []string{"first", "second"} {
		if _, err := r.svc.Answer(ctx, "camp-1", sh.ID, 0, vAna, AnswerInput{Way: WayWrite, Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	live, err := r.svc.LiveMoments(ctx, "camp-1", sh.SceneID, vDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 || len(live[0].Answers) != 1 || live[0].Answers[0].Text != "second" || live[0].Answers[0].Name != "Ana" {
		t.Fatalf("panel: %+v", live)
	}
	aid := live[0].Answers[0].ID
	if _, err := r.svc.Resolve(ctx, "camp-1", sh.ID, aid, vDir, ResolveInput{Action: ResolveDecline}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Answer(ctx, "camp-1", sh.ID, 0, vAna, AnswerInput{Way: WayWrite, Text: "third"}); codeOf(err) != 409 {
		t.Errorf("answer after the Director decided: %v", err)
	}
	if _, err := r.svc.Resolve(ctx, "camp-1", sh.ID, aid, vDir, ResolveInput{Action: ResolveDecline}); codeOf(err) != 409 {
		t.Errorf("decided twice: %v", err)
	}
}

// What the Director does with an answer, and where an added line goes.
func TestResolve(t *testing.T) {
	tests := []struct {
		name      string
		way, text string
		v         Viewer
		in        ResolveInput
		want      int
		status    string
		added     string // page/from/line written to the story, or ""
		storyDown bool
	}{
		{name: "add as written, to a page by name", way: WayWrite, text: "It rang twice.", v: vDir,
			in: ResolveInput{Action: ResolveAdd, Page: " the drowned bell "}, status: AnswerAdded, added: "page-bell/Ana/It rang twice."},
		{name: "add to a page by id", way: WayWrite, text: "It rang twice.", v: vCoDM,
			in: ResolveInput{Action: ResolveAdd, Page: "page-vell"}, status: AnswerAdded, added: "page-vell/Ana/It rang twice."},
		{name: "change it into the Director's words", way: WayWrite, text: "It rang.", v: vDir,
			in: ResolveInput{Action: ResolveChange, Line: "It rang\ttwice, at midnight.", Page: "The Drowned Bell"}, status: AnswerChanged, added: "page-bell/Ana/It rang twice, at midnight."},
		{name: "turn it down", way: WayWrite, text: "It rang.", v: vDir, in: ResolveInput{Action: ResolveDecline}, status: AnswerDeclined},
		{name: "left to the Director: add has nothing to add", way: WayDirector, v: vDir,
			in: ResolveInput{Action: ResolveAdd, Page: "page-bell"}, want: 400},
		{name: "left to the Director: they write it", way: WayDirector, v: vDir,
			in: ResolveInput{Action: ResolveChange, Line: "Vell rang it.", Page: "page-bell"}, status: AnswerChanged, added: "page-bell/Ana/Vell rang it."},
		{name: "no page picked", way: WayWrite, text: "x", v: vDir, in: ResolveInput{Action: ResolveAdd}, want: 400},
		{name: "a page nobody has", way: WayWrite, text: "x", v: vDir, in: ResolveInput{Action: ResolveAdd, Page: "Atlantis"}, want: 400},
		{name: "an empty change", way: WayWrite, text: "x", v: vDir, in: ResolveInput{Action: ResolveChange, Line: "  ", Page: "page-bell"}, want: 400},
		{name: "no such action", way: WayWrite, text: "x", v: vDir, in: ResolveInput{Action: "canonise"}, want: 400},
		{name: "a player cannot decide", way: WayWrite, text: "x", v: vBo, in: ResolveInput{Action: ResolveDecline}, want: 404},
		{name: "the story fails: nothing is marked added", way: WayWrite, text: "x", v: vDir, storyDown: true,
			in: ResolveInput{Action: ResolveAdd, Page: "page-bell"}, want: 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, sh := momentRig(t)
			if tt.storyDown {
				r.story.fail = apperror.NewInternal(errors.New("db down"))
			}
			if _, err := r.svc.Answer(ctx, "camp-1", sh.ID, 0, vAna, AnswerInput{Way: tt.way, Text: tt.text}); err != nil {
				t.Fatal(err)
			}
			list, _ := r.repo.ListAnswers(ctx, "camp-1", sh.ID, "ana")
			a, err := r.svc.Resolve(ctx, "camp-1", sh.ID, list[0].ID, tt.v, tt.in)
			code := codeOf(err)
			if err != nil && code == 0 {
				code = 500
			}
			if code != tt.want {
				t.Fatalf("code %d (%v), want %d", code, err, tt.want)
			}
			stored, _ := r.repo.GetAnswer(ctx, "camp-1", sh.ID, list[0].ID)
			if tt.want != 0 {
				if stored.Status != AnswerWaiting || stored.Line != "" || stored.PageID != "" || len(r.story.added) != 0 {
					t.Errorf("a refusal changed something: %s %q %q, %v", stored.Status, stored.Line, stored.PageID, r.story.added)
				}
				return
			}
			if a.Status != tt.status || stored.Status != tt.status {
				t.Errorf("status %s / stored %s, want %s", a.Status, stored.Status, tt.status)
			}
			if tt.added == "" {
				if len(r.story.added) != 0 || stored.Line != "" {
					t.Errorf("a turned-down answer reached the story: %v %q", r.story.added, stored.Line)
				}
				return
			}
			if len(r.story.added) != 1 || r.story.added[0] != tt.added {
				t.Errorf("story got %v, want %s", r.story.added, tt.added)
			}
			if stored.Text != tt.text {
				t.Errorf("the player's words changed: %q", stored.Text)
			}
		})
	}
}

// A second press on Add, or a second Director, finds the answer already
// claimed: the line joins the page once.
func TestResolveTwiceAddsOnce(t *testing.T) {
	r, sh := momentRig(t)
	if _, err := r.svc.Answer(ctx, "camp-1", sh.ID, 0, vAna, AnswerInput{Way: WayWrite, Text: "It rang."}); err != nil {
		t.Fatal(err)
	}
	list, _ := r.repo.ListAnswers(ctx, "camp-1", sh.ID, "ana")
	in := ResolveInput{Action: ResolveAdd, Page: "page-bell"}
	if _, err := r.svc.Resolve(ctx, "camp-1", sh.ID, list[0].ID, vDir, in); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Resolve(ctx, "camp-1", sh.ID, list[0].ID, vCoDM, in); codeOf(err) != 409 {
		t.Fatalf("second press: %v, want 409", err)
	}
	if len(r.story.added) != 1 {
		t.Errorf("the line joined the page %d times", len(r.story.added))
	}
}

// Continue lets the moment go with only the lines that joined the story,
// each from its player, and the next moment becomes the one held.
func TestGoCarriesOnlyTheStory(t *testing.T) {
	r, sh := momentRig(t)
	for _, v := range []Viewer{vAna, vBo} {
		if _, err := r.svc.Answer(ctx, "camp-1", sh.ID, 0, v, AnswerInput{Way: WayWrite, Text: "from " + v.UserID}); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := r.repo.ListAnswers(ctx, "camp-1", sh.ID, "")
	for _, a := range list {
		in := ResolveInput{Action: ResolveAdd, Page: "page-bell"}
		if a.UserID == "bo" {
			in = ResolveInput{Action: ResolveDecline}
		}
		if _, err := r.svc.Resolve(ctx, "camp-1", sh.ID, a.ID, vDir, in); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.svc.Go(ctx, "camp-1", sh.ID, 0, vAna); codeOf(err) != 404 {
		t.Fatalf("a player let the moment go: %v", err)
	}
	if _, err := r.svc.Go(ctx, "camp-1", sh.ID, 0, vDir); err != nil {
		t.Fatal(err)
	}
	if len(r.pub.gone) != 1 {
		t.Fatalf("went out %d times", len(r.pub.gone))
	}
	g := r.pub.gone[0]
	if g.moment != 0 || len(g.lines) != 1 || g.lines[0] != (ShownLine{Line: "from ana", From: "Ana"}) {
		t.Errorf("lines: %+v", g.lines)
	}
	for _, m := range g.showing.Scene.Moments {
		if m.Note != "" {
			t.Errorf("Go handed over a note")
		}
	}
	// Pressed again (a second tab), nothing goes out twice.
	if _, err := r.svc.Go(ctx, "camp-1", sh.ID, 0, vDir); err != nil || len(r.pub.gone) != 1 {
		t.Errorf("a second Continue: %v, %d", err, len(r.pub.gone))
	}
	live, _ := r.svc.LiveMoments(ctx, "camp-1", sh.SceneID, vDir)
	if len(live) != 1 || live[0].Current != 1 || len(live[0].Answers) != 0 {
		t.Errorf("after Continue the panel holds %+v", live)
	}
	read, err := r.svc.Read(ctx, "camp-1", sh.ID, vAna)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Released) != 1 || read.Released[0] != 0 {
		t.Errorf("a late screen reads released %v", read.Released)
	}
}

// A showing that still holds on a moment is live and caught up after a
// reload, past the few minutes a plain showing is, and the player gets back
// their own answer and nobody else's.
func TestHoldingShowingIsCaughtUp(t *testing.T) {
	r, sh := momentRig(t)
	if _, err := r.svc.Answer(ctx, "camp-1", sh.ID, 0, vAna, AnswerInput{Way: WayWrite, Text: "mine"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Answer(ctx, "camp-1", sh.ID, 0, vBo, AnswerInput{Way: WayWrite, Text: "Bo's"}); err != nil {
		t.Fatal(err)
	}
	r.clock.t = r.clock.t.Add(LiveFor * 3)
	mine, err := r.svc.Mine(ctx, "camp-1", vAna)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || !mine[0].Holding {
		t.Fatalf("mine: %+v", mine)
	}
	read, err := r.svc.Read(ctx, "camp-1", sh.ID, vAna)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Answers) != 1 || read.Answers[0].Text != "mine" {
		t.Errorf("Ana reads answers %+v", read.Answers)
	}
	members, err := r.svc.MemberShowings(ctx, "camp-1", "bo")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || !members[0].Holding || len(members[0].Answers) != 1 || members[0].Answers[0].Text != "Bo's" {
		t.Errorf("Foundry's catch-up for Bo: %+v", members)
	}
	if live, _ := r.svc.LiveFor(ctx, "camp-1", sh.SceneID, vDir); len(live) != 1 {
		t.Errorf("a holding showing dropped off the live list")
	}
	for _, i := range []int{0, 1} {
		if _, err := r.svc.Go(ctx, "camp-1", sh.ID, i, vDir); err != nil {
			t.Fatal(err)
		}
	}
	if mine, _ := r.svc.Mine(ctx, "camp-1", vAna); len(mine) != 1 || mine[0].Holding {
		t.Errorf("after every moment let go: %+v", mine)
	}
	if live, _ := r.svc.LiveFor(ctx, "camp-1", sh.SceneID, vDir); len(live) != 0 {
		t.Errorf("an old showing with nothing held is still live")
	}
}

// No player-facing payload carries the Director's note or their changed
// line before it is let through.
func TestNoDirectorTextReachesPlayers(t *testing.T) {
	r, sh := momentRig(t)
	if _, err := r.svc.Answer(ctx, "camp-1", sh.ID, 0, vAna, AnswerInput{Way: WayWrite, Text: "plain"}); err != nil {
		t.Fatal(err)
	}
	list, _ := r.repo.ListAnswers(ctx, "camp-1", sh.ID, "ana")
	if _, err := r.svc.Resolve(ctx, "camp-1", sh.ID, list[0].ID, vDir, ResolveInput{Action: ResolveChange, Line: "DRAFT-SECRET", Page: "page-bell"}); err != nil {
		t.Fatal(err)
	}
	read, _ := r.svc.Read(ctx, "camp-1", sh.ID, vAna)
	mine, _ := r.svc.Mine(ctx, "camp-1", vAna)
	members, _ := r.svc.MemberShowings(ctx, "camp-1", "ana")
	for name, v := range map[string]any{"show": r.pub.shown, "answered": r.pub.answered, "read": read, "mine": mine, "members": members} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"They lie about the bell.", "Vell does.", "DRAFT-SECRET"} {
			if strings.Contains(string(b), secret) {
				t.Errorf("%s carries %q", name, secret)
			}
		}
	}
	// Once let go, the changed line is the story and reaches every viewer.
	if _, err := r.svc.Go(ctx, "camp-1", sh.ID, 0, vDir); err != nil {
		t.Fatal(err)
	}
	if g := r.pub.gone[0]; len(g.lines) != 1 || g.lines[0].Line != "DRAFT-SECRET" {
		t.Errorf("the line let through: %+v", g.lines)
	}
}
