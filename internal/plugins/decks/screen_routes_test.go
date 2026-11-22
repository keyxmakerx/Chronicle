package decks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The Card decks section and the draw overlay's routes through the real route
// registration and middleware, with the real service over the in-memory
// repositories: who may read each, what each holds, and what a player is never
// sent.

const decksBase = "/campaigns/camp-1/decks"

func (r *workshopRig) put(path, user, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: user})
	}
	rec := httptest.NewRecorder()
	r.e.ServeHTTP(rec, req)
	return rec
}

// screenWorld is the workshop rig plus an event deck Tam may draw from, with a
// secret card (name, words, picture) and a shown copy a Director has drawn.
type screenWorld struct {
	*workshopRig
	omens, secretCard string
	shownCopy         string
}

var screenSecrets = []string{"Drowned Bell", "DROWNED-WORDS", "pic-bell"}

func newScreenWorld(t *testing.T, addonOn bool) *screenWorld {
	t.Helper()
	r := newWorkshopRig(t, addonOn)
	fx := r.fx
	omens := fx.deck("Omens", KindEvent, &SettingsInput{Draws: ptr("players"), Knows: ptr("count"), Arrives: ptr("down")})
	fx.choose(omens, PlayerInput{UserID: "u-tam"})
	card := fx.card(omens, CreateCardInput{Name: "Drowned Bell", Words: "DROWNED-WORDS", Picture: "pic-bell", Copies: ptr(1)})
	fx.card(omens, CreateCardInput{Name: "Plain", Words: "PLAIN", Copies: ptr(3)})
	return &screenWorld{workshopRig: r, omens: omens, secretCard: card}
}

func (w *screenWorld) shownBell() string {
	w.fx.t.Helper()
	res, err := w.fx.svc.Draw(bg, camp, w.omens, w.fx.dir, DrawInput{Count: 5})
	w.fx.must(err)
	for _, id := range res.CopyIDs {
		if w.fx.copy(id).CardID == w.secretCard {
			return id
		}
	}
	w.fx.t.Fatal("the bell was not drawn")
	return ""
}

func TestScreenRoutesGate(t *testing.T) {
	type route struct {
		name, method, path, body string
		director                 bool // answers 200/204 to a Director, 404 to everyone else
	}
	w := newScreenWorld(t, true)
	copyID := w.shownBell()
	routes := []route{
		{"section", "GET", decksBase + "/screen", "", true},
		{"picks", "PUT", decksBase + "/screen/picks", `{"deckIds":[]}`, true},
		{"shown face", "GET", decksBase + "/" + w.omens + "/copies/" + copyID + "/face", "", true},
	}
	do := func(w *screenWorld, rt route, user string, hdr ...string) *httptest.ResponseRecorder {
		if rt.method == "PUT" {
			return w.put(rt.path, user, rt.body)
		}
		return w.get(rt.path, user, hdr...)
	}
	for _, rt := range routes {
		for _, c := range []struct {
			user string
			want int
		}{
			{"u-dir", http.StatusOK}, {"u-codm", http.StatusOK},
			{"u-tam", http.StatusNotFound}, {"u-ilse", http.StatusNotFound}, {"u-scribe", http.StatusNotFound},
			{"u-other", http.StatusForbidden},
		} {
			rec := do(w, rt, c.user)
			want := c.want
			if want == http.StatusOK && rt.method == "PUT" {
				want = http.StatusNoContent
			}
			if rec.Code != want {
				t.Errorf("%s as %s: %d, want %d: %.160s", rt.name, c.user, rec.Code, want, rec.Body.String())
			}
			if c.want == http.StatusNotFound {
				for _, secret := range screenSecrets {
					if strings.Contains(rec.Body.String(), secret) {
						t.Errorf("%s as %s: the refusal holds %q", rt.name, c.user, secret)
					}
				}
			}
		}
		// Signed out is sent to sign in, like the rest of the plugin.
		if rec := do(w, rt, ""); rec.Code != http.StatusSeeOther {
			t.Errorf("%s signed out: %d, want 303", rt.name, rec.Code)
		}
		// With the add-on off even a Director is told 404.
		off := newScreenWorld(t, false)
		if rec := do(off, route{rt.name, rt.method, strings.ReplaceAll(rt.path, w.omens, off.omens), rt.body, true}, "u-dir", "HX-Request", "true"); rec.Code != http.StatusNotFound {
			t.Errorf("%s with the add-on off: %d, want 404", rt.name, rec.Code)
		}
	}
}

func TestPlayerRoutesGate(t *testing.T) {
	w := newScreenWorld(t, true)
	w.fx.must(func() error { _, err := w.fx.svc.Draw(bg, camp, w.omens, w.fx.dir, DrawInput{}); return err }())
	// The draw view: a Director reads any deck, Tam (chosen to draw) reads this
	// one, and everyone else is told 404.
	view := decksBase + "/" + w.omens + "/draw-view"
	for _, c := range []struct {
		user string
		want int
	}{
		{"u-dir", 200}, {"u-codm", 200}, {"u-tam", 200}, {"u-ilse", 404}, {"u-scribe", 404}, {"u-other", 403},
	} {
		if rec := w.get(view, c.user); rec.Code != c.want {
			t.Errorf("draw view as %s: %d, want %d", c.user, rec.Code, c.want)
		}
	}
	if rec := w.get(view, ""); rec.Code != http.StatusSeeOther {
		t.Errorf("draw view signed out: %d", rec.Code)
	}
	off := newScreenWorld(t, false)
	if rec := off.get(decksBase+"/"+off.omens+"/draw-view", "u-dir", "Accept", "application/json"); rec.Code != http.StatusNotFound {
		t.Errorf("draw view with the add-on off: %d", rec.Code)
	}
	// Fresh and seen are a player's; a Director is told no cards.
	for _, user := range []string{"u-tam", "u-ilse", "u-scribe", "u-dir", "u-codm"} {
		rec := w.get(decksBase+"/hand/fresh", user)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"cards":[]`) {
			t.Errorf("fresh as %s: %d %.120s", user, rec.Code, rec.Body.String())
		}
	}
	if rec := w.get(decksBase+"/hand/fresh", "u-other"); rec.Code != http.StatusForbidden {
		t.Errorf("fresh as a stranger: %d", rec.Code)
	}
	if rec := w.get(decksBase+"/hand/fresh", ""); rec.Code != http.StatusSeeOther {
		t.Errorf("fresh signed out: %d", rec.Code)
	}
	if rec := off.get(decksBase+"/hand/fresh", "u-tam", "Accept", "application/json"); rec.Code != http.StatusNotFound {
		t.Errorf("fresh with the add-on off: %d", rec.Code)
	}
	if rec := off.post(decksBase+"/hand/fresh/x/seen", "u-tam", "Accept", "application/json"); rec.Code != http.StatusNotFound {
		t.Errorf("seen with the add-on off: %d", rec.Code)
	}
	if rec := w.post(decksBase+"/hand/fresh/x/seen", ""); rec.Code != http.StatusSeeOther {
		t.Errorf("seen signed out: %d", rec.Code)
	}
}

// A deck a player may not read, one that is missing and a bad id answer the
// very same 404 on the draw view.
func TestDrawViewHiddenIsMissing(t *testing.T) {
	w := newScreenWorld(t, true)
	hidden := w.fx.deck("Vault", KindEvent, &SettingsInput{Draws: ptr("directors")})
	directorsOnly := w.omens // Ilse is not chosen
	var bodies []string
	for _, c := range []struct{ user, deck string }{
		{"u-tam", hidden}, {"u-ilse", directorsOnly}, {"u-tam", "00000000-0000-0000-0000-000000000000"}, {"u-tam", "not-an-id"},
	} {
		rec := w.get(decksBase+"/"+c.deck+"/draw-view", c.user)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s on %s: %d, want 404", c.user, c.deck, rec.Code)
		}
		bodies = append(bodies, rec.Body.String())
	}
	// A deck Tam sees but may not draw from (draws are the Director's) reads the same.
	seen := w.fx.deck("Seen", KindEvent, &SettingsInput{Draws: ptr("directors")})
	w.fx.choose(seen, PlayerInput{UserID: "u-tam"})
	bodies = append(bodies, w.get(decksBase+"/"+seen+"/draw-view", "u-tam").Body.String())
	for _, b := range bodies {
		if b != bodies[0] {
			t.Errorf("a hidden deck and a missing one answer differently:\n %s\n %s", bodies[0], b)
		}
	}
}

func TestScreenDefaultsToTheFiveNewestMoves(t *testing.T) {
	r := newWorkshopRig(t, true)
	fx := r.fx
	ids := []string{}
	for i := 0; i < 7; i++ {
		d := fx.deck(fmt.Sprintf("Deck %d", i), KindEvent, nil)
		fx.card(d, CreateCardInput{Name: "Card", Copies: ptr(2)})
		ids = append(ids, d)
	}
	// Move decks 2, 5, 0, 6, 3 in that order; 1 and 4 never move.
	for _, i := range []int{2, 5, 0, 6, 3} {
		_, err := fx.svc.Draw(bg, camp, ids[i], fx.dir, DrawInput{})
		fx.must(err)
	}
	v, err := fx.svc.Screen(bg, camp, fx.dir)
	fx.must(err)
	var got []string
	for _, d := range v.Shown {
		got = append(got, d.Name)
	}
	want := []string{"Deck 3", "Deck 6", "Deck 0", "Deck 5", "Deck 2"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("default decks %v, want %v", got, want)
	}
	// The rig's own "Iron omens" is a deck too.
	if len(v.Rest) != 3 || v.Total != 8 {
		t.Errorf("rest %d total %d, want 3 and 8", len(v.Rest), v.Total)
	}
	body := r.get(decksBase+"/screen", "u-dir").Body.String()
	for _, want := range []string{"data-dkd-screen", `data-dkd-deck="` + ids[3] + `"`, "data-dkd-row", "data-dkd-acts", "data-dkd-draw", "data-dkd-quick",
		"data-dkd-shown", "data-dkd-err", "data-dkd-more", "data-dkd-rest", "data-dkd-choose", "data-dkd-picker", "data-dkd-picks-save", "data-dkd-heroes", "data-hero=\"h-tam\"",
		"Events · 1 left · 1 drawn", "Events · 2 left", "All decks · 8", "dk-back"} {
		if !strings.Contains(body, want) {
			t.Errorf("the section lacks %q", want)
		}
	}
	if strings.Contains(body, "<html") {
		t.Error("the section is a whole page")
	}
}

func TestScreenPicks(t *testing.T) {
	r := newWorkshopRig(t, true)
	fx := r.fx
	var ids []string
	for i := 0; i < 14; i++ {
		ids = append(ids, fx.deck(fmt.Sprintf("D%02d", i), KindEvent, nil))
	}
	other, err := fx.svc.Create(bg, "camp-2", Viewer{UserID: "u-other", MemberRole: 3, VisibilityRole: 3, Director: true}, CreateDeckInput{Name: "Foreign", Kind: KindEvent})
	fx.must(err)
	foreign := other.(DirectorDeck).ID
	quote := func(l []string) string { b, _ := json.Marshal(map[string][]string{"deckIds": l}); return string(b) }

	tests := []struct {
		name string
		ids  []string
		code int
		want []string // the section's decks, by name, when it saved
	}{
		{"order kept", []string{ids[3], ids[1], ids[2]}, 204, []string{"D03", "D01", "D02"}},
		{"repeats keep their first place", []string{ids[4], ids[5], ids[4]}, 204, []string{"D04", "D05"}},
		{"twelve are fine", ids[:12], 204, nil},
		{"thirteen are refused", ids[:13], 400, nil},
		{"a foreign deck is refused", []string{ids[0], foreign}, 400, nil},
		{"an unknown id is refused", []string{"00000000-0000-0000-0000-000000000000"}, 400, nil},
		{"a bad id is refused", []string{"../x"}, 400, nil},
	}
	for _, tc := range tests {
		rec := r.put(decksBase+"/screen/picks", "u-dir", quote(tc.ids))
		if rec.Code != tc.code {
			t.Errorf("%s: %d, want %d: %s", tc.name, rec.Code, tc.code, rec.Body.String())
			continue
		}
		if tc.code == 400 {
			if !strings.Contains(rec.Body.String(), "message") || strings.Contains(rec.Body.String(), foreign) {
				t.Errorf("%s: the refusal lacks words or names the foreign deck: %s", tc.name, rec.Body.String())
			}
			continue
		}
		if tc.want == nil {
			continue
		}
		v, err := fx.svc.Screen(bg, camp, fx.dir)
		fx.must(err)
		var got []string
		for _, d := range v.Shown {
			got = append(got, d.Name)
		}
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("%s: shown %v, want %v", tc.name, got, tc.want)
		}
		if len(v.Rest) != v.Total-len(tc.want) {
			t.Errorf("%s: rest %d of %d", tc.name, len(v.Rest), v.Total)
		}
	}
	// A refused save leaves the last good picks.
	r.put(decksBase+"/screen/picks", "u-dir", quote([]string{ids[7]}))
	r.put(decksBase+"/screen/picks", "u-dir", quote([]string{ids[8], foreign}))
	if v, _ := fx.svc.Screen(bg, camp, fx.dir); len(v.Shown) != 1 || v.Shown[0].Name != "D07" {
		t.Errorf("a refused save changed the picks: %+v", v.Shown)
	}
	// Picks are per Director.
	if v, _ := fx.svc.Screen(bg, camp, Viewer{UserID: "u-codm", MemberRole: 1, VisibilityRole: 3, Director: true}); len(v.Shown) == 1 && v.Shown[0].Name == "D07" {
		t.Error("one Director's picks showed for another")
	}
	// Empty goes back to the recent decks.
	if rec := r.put(decksBase+"/screen/picks", "u-dir", `{"deckIds":[]}`); rec.Code != 204 {
		t.Fatalf("clear: %d", rec.Code)
	}
	if v, _ := fx.svc.Screen(bg, camp, fx.dir); len(v.Shown) != screenRecent {
		t.Errorf("after clearing, %d decks shown, want %d", len(v.Shown), screenRecent)
	}
	// A pick follows its deck out.
	r.put(decksBase+"/screen/picks", "u-dir", quote([]string{ids[9]}))
	fx.must(fx.svc.Delete(bg, camp, ids[9], fx.dir))
	if v, _ := fx.svc.Screen(bg, camp, fx.dir); len(v.Shown) != screenRecent {
		t.Errorf("a deleted deck's pick still counts: %d shown", len(v.Shown))
	}
	// A body that is not JSON is a 400.
	if rec := r.put(decksBase+"/screen/picks", "u-dir", `nope`); rec.Code != 400 {
		t.Errorf("bad body: %d", rec.Code)
	}
}

func TestScreenSummaryAndEmpty(t *testing.T) {
	r := newWorkshopRig(t, true)
	fx := r.fx
	v, err := fx.svc.Screen(bg, camp, fx.dir)
	fx.must(err)
	if got := v.Summary(); got != "1 deck" {
		t.Errorf("summary %q", got)
	}
	d := fx.deck("Omens", KindEvent, nil)
	fx.card(d, CreateCardInput{Name: "A", Copies: ptr(3)})
	_, err = fx.svc.Draw(bg, camp, d, fx.dir, DrawInput{Count: 2})
	fx.must(err)
	v, _ = fx.svc.Screen(bg, camp, fx.dir)
	if got := v.Summary(); got != "2 decks · 2 drawn" {
		t.Errorf("summary %q", got)
	}
	empty := &ScreenView{CampaignID: camp}
	if empty.Summary() != "No decks yet" {
		t.Errorf("empty summary %q", empty.Summary())
	}
}

type drawViewBody struct {
	DeckID    string `json:"deckId"`
	Name      string `json:"name"`
	Sub       string `json:"sub"`
	Shape     string `json:"shape"`
	Turn      string `json:"turn"`
	Pile      *int   `json:"pile"`
	Fan       int    `json:"fan"`
	BackHTML  string `json:"backHtml"`
	CanDraw   bool   `json:"canDraw"`
	Refusal   string `json:"refusal"`
	DrawsLeft *int   `json:"drawsLeft"`
	HeroID    string `json:"heroId"`
	Shown     []struct {
		CopyID string `json:"copyId"`
		HTML   string `json:"html"`
	} `json:"shown"`
}

func (w *screenWorld) drawView(user, deck string) (drawViewBody, string) {
	w.fx.t.Helper()
	rec := w.get(decksBase+"/"+deck+"/draw-view", user)
	if rec.Code != http.StatusOK {
		w.fx.t.Fatalf("draw view as %s: %d %s", user, rec.Code, rec.Body.String())
	}
	var out drawViewBody
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		w.fx.t.Fatal(err)
	}
	return out, rec.Body.String()
}

func TestDrawViewForADirector(t *testing.T) {
	w := newScreenWorld(t, true)
	shown := w.shownBell()
	v, raw := w.drawView("u-dir", w.omens)
	if v.DeckID != w.omens || v.Name != "Omens" || v.Sub != "Events · 0 left" || v.Pile == nil || *v.Pile != 0 || v.Fan != 0 || v.Shape != "tall" {
		t.Errorf("view %+v", v)
	}
	if v.CanDraw || v.Refusal != "Nothing left in this deck." {
		t.Errorf("an empty pile: canDraw %v refusal %q", v.CanDraw, v.Refusal)
	}
	if len(v.Shown) != 4 || v.HeroID != "" || v.DrawsLeft != nil {
		t.Errorf("shown %d hero %q", len(v.Shown), v.HeroID)
	}
	found := false
	for _, s := range v.Shown {
		if s.CopyID == shown {
			found = strings.Contains(s.HTML, "Drowned Bell") && strings.Contains(s.HTML, "dk-card")
		}
	}
	if !found {
		t.Errorf("the shown bell is not drawn face up: %.300s", raw)
	}
	if !strings.Contains(v.BackHTML, "dk-back") || strings.Contains(v.BackHTML, "Drowned") {
		t.Errorf("back %.200s", v.BackHTML)
	}

	// Five shown refuses with the server's words, and a pile of nine fans seven.
	big := w.fx.deck("Big", KindEvent, nil)
	w.fx.card(big, CreateCardInput{Name: "Many", Copies: ptr(9)})
	if v, _ := w.drawView("u-dir", big); v.Fan != 7 || *v.Pile != 9 || !v.CanDraw || v.Refusal != "" {
		t.Errorf("a pile of nine: %+v", v)
	}
	_, err := w.fx.svc.Draw(bg, camp, big, w.fx.dir, DrawInput{Count: 5})
	w.fx.must(err)
	if v, _ := w.drawView("u-dir", big); v.CanDraw || v.Refusal != shownFullWords() || v.Fan != 4 {
		t.Errorf("five shown: %+v", v)
	}
}

func TestDrawViewForAPlayerIsFromThePlayerTypes(t *testing.T) {
	w := newScreenWorld(t, true)
	w.shownBell() // a Director has drawn every copy to the shown spot
	v, raw := w.drawView("u-tam", w.omens)
	if v.HeroID != "h-tam" || len(v.Shown) != 0 {
		t.Errorf("hero %q shown %d", v.HeroID, len(v.Shown))
	}
	// Shown copies count as the pile to a player, as everywhere else.
	if v.Pile == nil || *v.Pile != 4 || v.Fan != 4 || v.Sub != "Events · 4 left" || !v.CanDraw {
		t.Errorf("player view %+v", v)
	}
	for _, secret := range append(screenSecrets, w.secretCard) {
		if strings.Contains(raw, secret) {
			t.Errorf("the player's draw view holds %q: %s", secret, raw)
		}
	}
	if strings.Contains(raw, "kindKey") || strings.Contains(raw, "directorNote") {
		t.Errorf("the player's draw view holds card fields: %s", raw)
	}
	// What the deck tells players of its pile is all a player is told.
	d := w.fx.repo.decks[w.omens]
	_, err := w.fx.svc.Update(bg, camp, w.omens, w.fx.dir, UpdateDeckInput{Version: patchOf(d.Version), Settings: patchOf(SettingsInput{Knows: ptr("nothing")})})
	w.fx.must(err)
	hidden, raw := w.drawView("u-tam", w.omens)
	if hidden.Pile != nil || hidden.Fan != 7 || hidden.Sub != "Events" || !strings.Contains(raw, `"pile":null`) {
		t.Errorf("a pile players may not count: %+v %s", hidden, raw)
	}
	// Draws run out.
	d = w.fx.repo.decks[w.omens]
	w.fx.choose(w.omens, PlayerInput{UserID: "u-tam", DrawsLeft: ptr(0)})
	if out, _ := w.drawView("u-tam", w.omens); out.CanDraw || out.Refusal != "You have no draws left." || out.DrawsLeft == nil || *out.DrawsLeft != 0 {
		t.Errorf("no draws left: %+v", out)
	}
	w.fx.choose(w.omens, PlayerInput{UserID: "u-tam", DrawsLeft: ptr(2)})
	if out, _ := w.drawView("u-tam", w.omens); !out.CanDraw || out.DrawsLeft == nil || *out.DrawsLeft != 2 {
		t.Errorf("two draws left: %+v", out)
	}
}

func TestShownFaceIsForShownCopiesOnly(t *testing.T) {
	w := newScreenWorld(t, true)
	shown := w.shownBell()
	rec := w.get(decksBase+"/"+w.omens+"/copies/"+shown+"/face", "u-dir")
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Drowned Bell") || !strings.Contains(body, "DROWNED-WORDS") || !strings.Contains(body, "dk-leaf") {
		t.Errorf("the shown face: %d %.300s", rec.Code, body)
	}
	if strings.Contains(body, "<html") {
		t.Error("the face is a whole page")
	}
	// A copy in a hand, in the pile, a bad id and another deck's copy are 404.
	_, err := w.fx.svc.Act(bg, camp, w.omens, shown, "deal", w.fx.dir, ActInput{To: "hand", HeroID: "h-tam"})
	w.fx.must(err)
	other := w.fx.deck("Other", KindEvent, nil)
	w.fx.card(other, CreateCardInput{Name: "Piled", Copies: ptr(1)})
	pile := w.fx.copiesOf(w.fx.repo.cardsOf(other)[0])[0]
	var bodies []string
	for _, c := range []struct{ deck, copy string }{
		{w.omens, shown}, {other, pile}, {w.omens, "not-an-id"}, {w.omens, pile}, {other, shown},
	} {
		rec := w.get(decksBase+"/"+c.deck+"/copies/"+c.copy+"/face", "u-dir")
		if rec.Code != 404 {
			t.Errorf("%v: %d, want 404", c, rec.Code)
		}
		bodies = append(bodies, rec.Body.String())
	}
	for _, b := range bodies {
		if b != bodies[0] || strings.Contains(b, "Drowned") {
			t.Errorf("a refused face differs or names the card: %s", b)
		}
	}
}

func TestFreshRules(t *testing.T) {
	deal := func(w *screenWorld, hero string) string {
		w.fx.t.Helper()
		res, err := w.fx.svc.Draw(bg, camp, w.omens, w.fx.dir, DrawInput{})
		w.fx.must(err)
		moved, err := w.fx.svc.Act(bg, camp, w.omens, res.CopyID, "deal", w.fx.dir, ActInput{To: "hand", HeroID: hero})
		w.fx.must(err)
		return moved.CopyID
	}
	age := func(w *screenWorld, copyID string, by time.Duration) {
		i := w.fx.repo.index(copyID)
		at := time.Now().Add(-by)
		w.fx.repo.copies[i].DealtAt = &at
	}
	fresh := func(w *screenWorld, user string) []string {
		w.fx.t.Helper()
		rec := w.get(decksBase+"/hand/fresh", user)
		if rec.Code != 200 {
			w.fx.t.Fatalf("fresh: %d", rec.Code)
		}
		var out struct {
			Cards []struct {
				DeckID, CopyID, DeckName, Sub, BackHTML, Turn, Shape string
			} `json:"cards"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			w.fx.t.Fatal(err)
		}
		var ids []string
		for _, c := range out.Cards {
			ids = append(ids, c.CopyID)
			if c.DeckID != w.omens || c.DeckName != "Omens" || !strings.Contains(c.BackHTML, "dk-back") || c.Turn == "" || c.Shape == "" || c.Sub == "" {
				w.fx.t.Errorf("a fresh card lacks its parts: %+v", c)
			}
		}
		return ids
	}

	t.Run("only my heroes' face-down copies dealt by someone else", func(t *testing.T) {
		w := newScreenWorld(t, true)
		mine := deal(w, "h-tam")
		theirs := deal(w, "h-ilse")
		if got := fresh(w, "u-tam"); len(got) != 1 || got[0] != mine {
			t.Errorf("Tam: %v, want [%s]", got, mine)
		}
		if got := fresh(w, "u-ilse"); len(got) != 1 || got[0] != theirs {
			t.Errorf("Ilse: %v, want [%s]", got, theirs)
		}
		if got := fresh(w, "u-scribe"); len(got) != 0 {
			t.Errorf("a Scribe with no hero: %v", got)
		}
	})
	t.Run("a card I drew myself is not fresh", func(t *testing.T) {
		w := newScreenWorld(t, true)
		_, err := w.fx.svc.Draw(bg, camp, w.omens, w.fx.tam, DrawInput{HeroID: "h-tam"})
		w.fx.must(err)
		if got := fresh(w, "u-tam"); len(got) != 0 {
			t.Errorf("own draw: %v", got)
		}
	})
	t.Run("ten minutes", func(t *testing.T) {
		w := newScreenWorld(t, true)
		old, recent := deal(w, "h-tam"), deal(w, "h-tam")
		age(w, old, freshWindow+time.Second)
		age(w, recent, freshWindow-time.Minute)
		if got := fresh(w, "u-tam"); len(got) != 1 || got[0] != recent {
			t.Errorf("window: %v, want [%s]", got, recent)
		}
	})
	t.Run("face up and turned cards are not fresh", func(t *testing.T) {
		w := newScreenWorld(t, true)
		id := deal(w, "h-tam")
		_, err := w.fx.svc.Act(bg, camp, w.omens, id, "turn", w.fx.tam, ActInput{})
		w.fx.must(err)
		if got := fresh(w, "u-tam"); len(got) != 0 {
			t.Errorf("a turned card: %v", got)
		}
		up := w.fx.deck("Up", KindEvent, &SettingsInput{Arrives: ptr("up")})
		w.fx.choose(up, PlayerInput{UserID: "u-tam"})
		w.fx.card(up, CreateCardInput{Name: "Open", Copies: ptr(1)})
		res, _ := w.fx.svc.Draw(bg, camp, up, w.fx.dir, DrawInput{})
		_, err = w.fx.svc.Act(bg, camp, up, res.CopyID, "deal", w.fx.dir, ActInput{To: "hand", HeroID: "h-tam"})
		w.fx.must(err)
		if got := fresh(w, "u-tam"); len(got) != 0 {
			t.Errorf("a card dealt face up: %v", got)
		}
	})
	t.Run("at most three, newest first", func(t *testing.T) {
		w := newScreenWorld(t, true)
		var ids []string
		for i := 0; i < 4; i++ {
			id := deal(w, "h-tam")
			age(w, id, time.Duration(4-i)*time.Minute) // later deals are newer
			ids = append(ids, id)
		}
		got := fresh(w, "u-tam")
		want := []string{ids[3], ids[2], ids[1]}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("seen once, idempotent, and only my own", func(t *testing.T) {
		w := newScreenWorld(t, true)
		mine, theirs := deal(w, "h-tam"), deal(w, "h-ilse")
		path := decksBase + "/hand/fresh/" + mine + "/seen"
		for i := 0; i < 2; i++ {
			if rec := w.post(path, "u-tam"); rec.Code != 204 {
				t.Errorf("seen #%d: %d", i, rec.Code)
			}
		}
		if got := fresh(w, "u-tam"); len(got) != 0 {
			t.Errorf("after seen: %v", got)
		}
		// Seen is per person: Ilse's own card is still hers to be shown.
		if got := fresh(w, "u-ilse"); len(got) != 1 || got[0] != theirs {
			t.Errorf("Ilse: %v", got)
		}
		var bodies []string
		for _, c := range []struct{ user, copy string }{{"u-ilse", mine}, {"u-tam", theirs}, {"u-tam", "00000000-0000-0000-0000-000000000000"}, {"u-tam", "not-an-id"}, {"u-dir", mine}, {"u-scribe", mine}} {
			rec := w.post(decksBase+"/hand/fresh/"+c.copy+"/seen", c.user)
			if rec.Code != 404 {
				t.Errorf("%v: %d, want 404", c, rec.Code)
			}
			bodies = append(bodies, rec.Body.String())
		}
		for _, b := range bodies {
			if b != bodies[0] {
				t.Errorf("refusals differ: %s / %s", bodies[0], b)
			}
		}
		if got := fresh(w, "u-ilse"); len(got) != 1 {
			t.Error("someone else's seen mark hid Ilse's card")
		}
	})
}

// A face-down copy never gives a player its card: not in the draw view, not in
// the fresh list, not in the hand's markup.
func TestFaceDownNeverLeaksOnTheDrawRoutes(t *testing.T) {
	w := newScreenWorld(t, true)
	res, err := w.fx.svc.Draw(bg, camp, w.omens, w.fx.dir, DrawInput{Count: 5})
	w.fx.must(err)
	var copyID string
	for _, id := range res.CopyIDs {
		if w.fx.copy(id).CardID == w.secretCard {
			copyID = id
		}
	}
	_, err = w.fx.svc.Act(bg, camp, w.omens, copyID, "deal", w.fx.dir, ActInput{To: "hand", HeroID: "h-tam"})
	w.fx.must(err)
	secrets := append(append([]string{}, screenSecrets...), w.secretCard, "kindKey", "directorNote", "askFirst")
	for _, user := range []string{"u-tam", "u-ilse", "u-scribe"} {
		for _, p := range []string{decksBase + "/" + w.omens + "/draw-view", decksBase + "/hand/fresh", decksBase + "/hand"} {
			rec := w.get(p, user)
			if rec.Code == http.StatusNotFound {
				continue
			}
			for _, s := range secrets {
				if strings.Contains(rec.Body.String(), s) {
					t.Errorf("%s at %s was sent %q", user, p, s)
				}
			}
			// Ilse and the Scribe are never told Tam's copy id either.
			if user != "u-tam" && strings.Contains(rec.Body.String(), copyID) {
				t.Errorf("%s at %s was sent Tam's copy id", user, p)
			}
		}
	}
	// The hand carries the two buttons the overlay needs.
	hand := w.get(decksBase+"/hand", "u-tam").Body.String()
	for _, want := range []string{`data-dkd-open`, `data-deck="` + w.omens + `"`, `data-hero="h-tam"`, "Quick draw", "data-dkh-draw"} {
		if !strings.Contains(hand, want) {
			t.Errorf("the hand lacks %q", want)
		}
	}
	// A Director's own face route stays out of a player's reach even with the copy id.
	if rec := w.get(decksBase+"/"+w.omens+"/copies/"+copyID+"/face", "u-tam"); rec.Code != 404 || strings.Contains(rec.Body.String(), "Drowned") {
		t.Errorf("a player asked for a face: %d", rec.Code)
	}
}
