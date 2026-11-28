package cutscenes

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// momentRouteRig is the route rig with a showing of a scene that asks
// something, shown to Ana and Bo, with Ana's answer waiting.
func momentRouteRig(t *testing.T, addonOn bool) (*routeRig, *Showing, string) {
	t.Helper()
	r := newRouteRig(t, addonOn)
	in := NewScene()
	in.Name = "Asks"
	in.Moments = []Moment{{At: 1, Ask: "What do they whisper?", Ways: []Way{{Kind: WayWrite}, {Kind: WayDirector}}, Note: "NOTE-SECRET"}}
	sc, err := r.svc.Create(ctx, "camp-1", vDir, in)
	if err != nil {
		t.Fatal(err)
	}
	sh, err := r.svc.Show(ctx, "camp-1", sc.ID, vDir, ShowInput{To: []string{"ana", "bo"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Answer(ctx, "camp-1", sh.ID, 0, vAna, AnswerInput{Way: WayWrite, Text: "It rang."}); err != nil {
		t.Fatal(err)
	}
	list, _ := r.repo.ListAnswers(ctx, "camp-1", sh.ID, "ana")
	return r, sh, list[0].ID
}

func momentDirectorRoutes(sh *Showing, aid string) []route {
	base := "/campaigns/camp-1/cutscenes"
	return []route{
		{"GET", base + "/" + sh.SceneID + "/live", nil},
		{"POST", base + "/showings/" + sh.ID + "/answers/" + aid, url.Values{"action": {"decline"}}},
		{"POST", base + "/showings/" + sh.ID + "/moments/0/go", url.Values{}},
	}
}

// The panel, its decisions and Continue are the Director's: anyone else is
// told they are not there, and nothing changes.
func TestMomentRoutesAreTheDirectors(t *testing.T) {
	r, sh, aid := momentRouteRig(t, true)
	for _, user := range []string{"ana", "bo", "scribe"} {
		for _, rt := range momentDirectorRoutes(sh, aid) {
			rec := r.do(rt.method, rt.path, user, rt.form, "HX-Request", "true")
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s %s: %d, want 404", user, rt.method, rt.path, rec.Code)
			}
			if b := rec.Body.String(); strings.Contains(b, "NOTE-SECRET") || strings.Contains(b, "It rang.") || strings.Contains(b, "csw-") {
				t.Errorf("%s %s %s: the answer holds the panel", user, rt.method, rt.path)
			}
		}
	}
	if a, _ := r.repo.GetAnswer(ctx, "camp-1", sh.ID, aid); a.Status != AnswerWaiting || len(r.pub.gone) != 0 {
		t.Fatalf("a player's press changed something: %s, %d gone", a.Status, len(r.pub.gone))
	}

	for _, user := range []string{"dir", "codm"} {
		r, sh, aid := momentRouteRig(t, true)
		for _, rt := range momentDirectorRoutes(sh, aid) {
			rec := r.do(rt.method, rt.path, user, rt.form, "HX-Request", "true")
			if rec.Code != http.StatusOK {
				t.Errorf("%s %s %s: %d %s", user, rt.method, rt.path, rec.Code, rec.Body.String())
			}
		}
	}
}

// The panel shows the question, the Director's note and the answer, and a
// refused decision redraws it with the reason.
func TestMomentPanelAnswers(t *testing.T) {
	r, sh, aid := momentRouteRig(t, true)
	base := "/campaigns/camp-1/cutscenes"
	rec := r.do("GET", base+"/"+sh.SceneID+"/live", "dir", nil, "HX-Request", "true")
	b := rec.Body.String()
	for _, want := range []string{"What do they whisper?", "NOTE-SECRET", "It rang.", "Ana wrote", "Add to the story", "Continue the scene", "The Drowned Bell"} {
		if !strings.Contains(b, want) {
			t.Errorf("panel lacks %q", want)
		}
	}
	rec = r.do("POST", base+"/showings/"+sh.ID+"/answers/"+aid, "dir", url.Values{"action": {"add"}, "page": {"Atlantis"}}, "HX-Request", "true")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "no page is called that") {
		t.Errorf("a refused add: %d %s", rec.Code, rec.Body.String())
	}
	rec = r.do("POST", base+"/showings/"+sh.ID+"/answers/"+aid, "dir", url.Values{"action": {"add"}, "page": {"The Drowned Bell"}}, "HX-Request", "true")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Added to the story.") {
		t.Errorf("an add: %d %s", rec.Code, rec.Body.String())
	}
	if len(r.story.added) != 1 || r.story.added[0] != "page-bell/Ana/It rang." {
		t.Errorf("story: %v", r.story.added)
	}
	// Show answers with the panel's trigger when the scene asks something.
	rec = r.do("POST", base+"/"+sh.SceneID+"/show", "dir", url.Values{"to": {"bo"}}, "HX-Request", "true")
	if rec.Code != 200 || rec.Header().Get("HX-Trigger") != "cutscene-moment" {
		t.Errorf("show: %d trigger %q", rec.Code, rec.Header().Get("HX-Trigger"))
	}
	// Stop redraws the panel, which then drops the stopped showing.
	rec = r.do("POST", base+"/showings/"+sh.ID+"/stop", "dir", url.Values{}, "HX-Request", "true")
	if rec.Code != 200 || rec.Header().Get("HX-Trigger") != "cutscene-moment" {
		t.Errorf("stop: %d trigger %q", rec.Code, rec.Header().Get("HX-Trigger"))
	}
	// The editor page draws the panel.
	rec = r.do("GET", base+"/"+sh.SceneID, "dir", nil)
	if !strings.Contains(rec.Body.String(), `id="`+momentsID(sh.SceneID)+`"`) {
		t.Errorf("the editor page has no moments panel")
	}
}

// A viewer answers from the scene and gets their own answer back, never the
// Director's note; anyone the scene was not shown to is told it is not
// there; answering is rate-limited.
func TestAnswerRoute(t *testing.T) {
	r, sh, _ := momentRouteRig(t, true)
	path := "/campaigns/camp-1/cutscenes/showings/" + sh.ID + "/moments/0/answer"
	rec := r.do("POST", path, "bo", url.Values{"way": {"write"}, "text": {"The bell is still down there."}})
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 || got["text"] != "The bell is still down there." || got["way"] != "write" {
		t.Fatalf("bo's answer: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "NOTE-SECRET") {
		t.Errorf("the answer carries the note")
	}
	for _, user := range []string{"dir", "codm", "scribe"} {
		if rec := r.do("POST", path, user, url.Values{"way": {"write"}, "text": {"x"}}); rec.Code != 404 {
			t.Errorf("%s answered: %d", user, rec.Code)
		}
	}
	if rec := r.do("POST", path, "ana", url.Values{"way": {"write"}, "text": {""}}); rec.Code != 400 {
		t.Errorf("an empty answer: %d", rec.Code)
	}
	if rec := r.do("POST", "/campaigns/camp-1/cutscenes/showings/"+sh.ID+"/moments/x/answer", "ana", url.Values{"way": {"director"}}); rec.Code != 400 {
		t.Errorf("a moment that is not a number: %d", rec.Code)
	}
	if rec := r.do("POST", path, "", url.Values{"way": {"director"}}); rec.Code != http.StatusSeeOther && rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out: %d", rec.Code)
	}
	if rec := r.do("POST", path, "other", url.Values{"way": {"director"}}); rec.Code != http.StatusForbidden {
		t.Errorf("non-member: %d", rec.Code)
	}
	off, offSh, _ := momentRouteRig(t, false)
	if rec := off.do("POST", "/campaigns/camp-1/cutscenes/showings/"+offSh.ID+"/moments/0/answer", "ana", url.Values{"way": {"director"}}, "Accept", "application/json"); rec.Code != 404 {
		t.Errorf("add-on off: %d", rec.Code)
	}

	limited := 0
	for i := 0; i < answerRateLimit+5; i++ {
		rec := r.do("POST", path, "ana", url.Values{"way": {"write"}, "text": {"try " + strconv.Itoa(i)}})
		if rec.Code == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited == 0 {
		t.Errorf("answering is not rate-limited")
	}
}
