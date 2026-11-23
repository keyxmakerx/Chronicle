package decks

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// post sends a body-less write as a signed-in user.
func (r *workshopRig) post(path, user string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	if user != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: user})
	}
	rec := httptest.NewRecorder()
	r.e.ServeHTTP(rec, req)
	return rec
}

const handBase = "/campaigns/camp-1/decks/hand"

// handWorld is a hero deck with Tam and Ilse chosen and one face-down copy of
// a card in Tam's hand.
type handWorld struct {
	*workshopRig
	deck, tamCopy, card string
}

func newHandWorld(t *testing.T, addonOn bool, settings *SettingsInput) *handWorld {
	t.Helper()
	r := newWorkshopRig(t, addonOn)
	fx := r.fx
	if settings == nil {
		settings = &SettingsInput{Knows: ptr("kinds"), Arrives: ptr("down"), OthersSee: ptr("face")}
	}
	deck := fx.deck("Orrin's omens", KindHero, settings)
	_, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patchOf(1), Look: patchOf(LookInput{
		Kinds: []KindDef{{Key: "boon", Name: "Boon", Colour: "teal"}, {Key: "bane", Name: "Bane", Colour: "rose"}}})})
	fx.must(err)
	fx.choose(deck, PlayerInput{UserID: "u-tam"}, PlayerInput{UserID: "u-ilse"})
	card := fx.card(deck, CreateCardInput{Name: "Moonwell", KindKey: "bane", Words: "MOONWELL-WORDS", Corner: "7", Picture: "pic-moon", Copies: ptr(1)})
	fx.card(deck, CreateCardInput{Name: "Filler", KindKey: "boon", Words: "FILLER-WORDS", Copies: ptr(3)})
	res, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: card, To: "hand", HeroID: "h-tam"})
	fx.must(err)
	return &handWorld{workshopRig: r, deck: deck, tamCopy: res.CopyID, card: card}
}

func (w *handWorld) turnPath(copyID string) string {
	return handBase + "/" + w.deck + "/copies/" + copyID + "/turn"
}

func (w *handWorld) openPath(copyID string) string {
	return handBase + "/" + w.deck + "/copies/" + copyID
}

// Everything a face-down card is to its holder before the tap: the deck's
// back, its id and the deck's name, and none of what the card is.
var cardSecrets = []string{"Moonwell", "MOONWELL-WORDS", "pic-moon", "Boon", "Bane", "bane", "askFirst", "directorNote"}

func TestHandFragmentHoldsNothingOfAFaceDownCard(t *testing.T) {
	w := newHandWorld(t, true, nil)
	rec := w.get(handBase, "u-tam")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %.200s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, bad := range append(cardSecrets, w.card) {
		if bad == "Boon" || bad == "Bane" {
			// Kinds are known to this deck's players by their counts, not here; the
			// hand names no kind for a face-down card.
			continue
		}
		if strings.Contains(body, bad) {
			t.Errorf("the hand holds %q before the card is turned", bad)
		}
	}
	if !strings.Contains(body, w.tamCopy) || !strings.Contains(body, "dk-turn") || !strings.Contains(body, "A new card from Orrin&#39;s omens. Tap it to turn it over.") {
		t.Errorf("the hand lacks the face-down copy, its turn button or the new-card line: %.600s", body)
	}
	if strings.Contains(body, "<template") || strings.Contains(body, `data-face-src="/`) || strings.Contains(body, `data-face-src="h`) {
		t.Error("the hand carries a template or a face address before the tap")
	}
	// Another player and a Scribe read no copy ids of Tam's hand.
	for _, user := range []string{"u-ilse", "u-scribe"} {
		other := w.get(handBase, user).Body.String()
		if strings.Contains(other, w.tamCopy) {
			t.Errorf("%s was sent Tam's copy id", user)
		}
		for _, bad := range []string{"Moonwell", "MOONWELL-WORDS", "pic-moon"} {
			if strings.Contains(other, bad) {
				t.Errorf("%s was sent %q", user, bad)
			}
		}
	}
}

func TestTurningBringsTheFaceOnceAndFlipsTheCopyOnce(t *testing.T) {
	w := newHandWorld(t, true, nil)
	rec := w.post(w.turnPath(w.tamCopy), "u-tam", "Accept", "text/html, application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("turn: %d %.200s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"Moonwell", "dk-card"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("the face lacks %q: %.300s", want, rec.Body.String())
		}
	}
	if !w.fx.copy(w.tamCopy).FaceUp {
		t.Error("the copy is still face down")
	}
	lines := len(w.fx.repo.logs)
	again := w.post(w.turnPath(w.tamCopy), "u-tam", "Accept", "text/html, application/json")
	if again.Code != http.StatusConflict || strings.Contains(again.Body.String(), "Moonwell") || strings.Contains(again.Body.String(), "MOONWELL-WORDS") {
		t.Errorf("a repeat turn: %d %.200s", again.Code, again.Body.String())
	}
	if !w.fx.copy(w.tamCopy).FaceUp || len(w.fx.repo.logs) != lines {
		t.Error("a repeat flipped the copy back or wrote a line")
	}
}

// The draw overlay asks for the turned face at open size, with the card's
// words on it; the Hand's own strip keeps the small face.
func TestTurningAtOpenSizeForTheOverlay(t *testing.T) {
	for _, tc := range []struct {
		query, size string
	}{
		{"", "dk-sz-hand"},
		{"?size=open", "dk-sz-open"},
	} {
		t.Run("query "+tc.query, func(t *testing.T) {
			w := newHandWorld(t, true, nil)
			rec := w.post(w.turnPath(w.tamCopy)+tc.query, "u-tam", "Accept", "text/html, application/json")
			if rec.Code != http.StatusOK {
				t.Fatalf("turn: %d %.200s", rec.Code, rec.Body.String())
			}
			if body := rec.Body.String(); !strings.Contains(body, tc.size) || !strings.Contains(body, "Moonwell") {
				t.Errorf("the face is not %s: %.300s", tc.size, body)
			}
		})
	}
}

// A deck whose cards arrive face up gives a player a card that is already
// turned; the overlay's open-size turn reads it without flipping it, while the
// Hand's own turn still answers 409. A card not theirs stays hidden.
func TestOpenSizeTurnReadsACardThatArrivedFaceUp(t *testing.T) {
	up := "up"
	w := newHandWorld(t, true, &SettingsInput{Arrives: &up})
	if !w.fx.copy(w.tamCopy).FaceUp {
		t.Fatal("the dealt copy did not arrive face up")
	}
	hdr := []string{"Accept", "text/html, application/json"}
	lines := len(w.fx.repo.logs)
	rec := w.post(w.turnPath(w.tamCopy)+"?size=open", "u-tam", hdr...)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Moonwell") || !strings.Contains(rec.Body.String(), "dk-sz-open") {
		t.Fatalf("open-size turn of a face-up card: %d %.300s", rec.Code, rec.Body.String())
	}
	if len(w.fx.repo.logs) != lines || !w.fx.copy(w.tamCopy).FaceUp {
		t.Error("reading the face moved the card or wrote a line")
	}
	if plain := w.post(w.turnPath(w.tamCopy), "u-tam", hdr...); plain.Code != http.StatusConflict {
		t.Errorf("the Hand's turn of a face-up card: %d", plain.Code)
	}
	if other := w.post(w.turnPath(w.tamCopy)+"?size=open", "u-ilse", hdr...); other.Code != http.StatusNotFound || strings.Contains(other.Body.String(), "Moonwell") {
		t.Errorf("another player read the card: %d %.200s", other.Code, other.Body.String())
	}
}

// A copy that is not theirs, one that does not exist, a bad id and another
// deck's id answer the same 404, for the turn request and the open request.
func TestHandRoutesHideWhatIsNotTheirs(t *testing.T) {
	w := newHandWorld(t, true, nil)
	// Make the face readable so the open route has something to refuse.
	w.fx.must(func() error {
		_, err := w.fx.svc.Act(bg, camp, w.deck, w.tamCopy, "turn", w.fx.dir, ActInput{})
		return err
	}())
	hdr := []string{"Accept", "text/html, application/json"}
	var turnBodies, openBodies []string
	for _, copyID := range []string{w.tamCopy, "00000000-0000-0000-0000-000000000000", "not-an-id", "..%2fx"} {
		for _, user := range []string{"u-ilse", "u-scribe"} {
			rt := w.post(w.turnPath(copyID), user, hdr...)
			ro := w.get(w.openPath(copyID), user, hdr...)
			if rt.Code != http.StatusNotFound || ro.Code != http.StatusNotFound {
				t.Errorf("%s on %q: turn %d, open %d, want 404", user, copyID, rt.Code, ro.Code)
			}
			turnBodies = append(turnBodies, rt.Body.String())
			openBodies = append(openBodies, ro.Body.String())
		}
	}
	for _, set := range [][]string{turnBodies, openBodies} {
		for _, b := range set {
			if b != set[0] {
				t.Errorf("a hidden copy and a missing one answer differently:\n %s\n %s", set[0], b)
			}
			if strings.Contains(b, "Moonwell") || strings.Contains(b, "MOONWELL-WORDS") {
				t.Error("a refusal names the card")
			}
		}
	}
	if rec := w.get(w.openPath(w.tamCopy), "u-tam", hdr...); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Moonwell") {
		t.Errorf("the owner could not open it: %d", rec.Code)
	}
	// A deck the player is not on is a 404 as well, even for a copy id that exists.
	hidden := w.fx.deck("Vault", KindEvent, nil)
	hc := w.fx.card(hidden, CreateCardInput{Name: "VaultCard", Words: "VAULT-WORDS"})
	res, err := w.fx.svc.DealCard(bg, camp, hidden, w.fx.dir, DealCardInput{CardID: hc, To: "hand", HeroID: "h-npc"})
	w.fx.must(err)
	for _, user := range []string{"u-tam", "u-ilse"} {
		for _, rec := range []*httptest.ResponseRecorder{
			w.post(handBase+"/"+hidden+"/copies/"+res.CopyID+"/turn", user, hdr...),
			w.get(handBase+"/"+hidden+"/copies/"+res.CopyID, user, hdr...),
		} {
			if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "VaultCard") {
				t.Errorf("%s on a hidden deck: %d", user, rec.Code)
			}
		}
	}
}

// Ilse reads Tam's hero only as the deck lets her: here faces of shown
// cards and backs, never a face-down card's face, and never through the strip.
func TestAnotherHeroIsSeenOnlyAsTheDeckAllows(t *testing.T) {
	w := newHandWorld(t, true, nil)
	strip := handBase + "/hero/h-tam"
	rec := w.get(strip, "u-ilse")
	if rec.Code != http.StatusOK {
		t.Fatalf("strip: %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, w.tamCopy) || strings.Contains(body, "Moonwell") || strings.Contains(body, "MOONWELL-WORDS") {
		t.Errorf("Ilse can read Tam's face-down card: %.400s", body)
	}
	if !strings.Contains(body, "dk-back") {
		t.Errorf("Ilse is shown no back: %.400s", body)
	}
	// Turned but not shown: the same bytes.
	_, err := w.fx.svc.Act(bg, camp, w.deck, w.tamCopy, "turn", w.fx.tam, ActInput{})
	w.fx.must(err)
	if after := w.get(strip, "u-ilse").Body.String(); after != body {
		t.Error("turning a card changed what Ilse reads")
	}
	// Shown: now the face.
	_, err = w.fx.svc.Act(bg, camp, w.deck, w.tamCopy, "show", w.fx.tam, ActInput{})
	w.fx.must(err)
	shown := w.get(strip, "u-ilse").Body.String()
	if !strings.Contains(shown, "Moonwell") || strings.Contains(shown, w.tamCopy) || strings.Contains(shown, "data-dkh-open") {
		t.Errorf("a shown card is not read plainly, or Ilse is offered controls: %.500s", shown)
	}
	// A hero page she may not open is a 404.
	w.fx.heroes.hidden["h-tam"] = true
	if rec := w.get(strip, "u-ilse"); rec.Code != http.StatusNotFound {
		t.Errorf("a hidden hero's strip: %d, want 404", rec.Code)
	}
	if rec := w.get(strip, "u-tam"); rec.Code != http.StatusNotFound && rec.Code != http.StatusOK {
		t.Errorf("Tam's own strip: %d", rec.Code)
	}
	// The player's own strip carries controls. A Director reads the hand as
	// its holder does and is offered none: turning a card is its holder's.
	w.fx.heroes.hidden["h-tam"] = false
	if b := w.get(strip, "u-tam").Body.String(); !strings.Contains(b, "data-dkh-open") {
		t.Error("Tam was offered no controls on their own strip")
	}
	dir := w.get(strip, "u-dir").Body.String()
	if !strings.Contains(dir, "Moonwell") {
		t.Errorf("a Director cannot read the hero's turned card: %.400s", dir)
	}
	if strings.Contains(dir, "data-dkh-open") || strings.Contains(dir, `class="dk-turn"`) || strings.Contains(dir, "dkh-chip-new") {
		t.Errorf("a Director is offered controls on a player's hand: %.600s", dir)
	}
}

// Every new route answers the site-wide gates as the workshop's do.
func TestHandRoutesAnswerTheSiteGates(t *testing.T) {
	r := newHandWorld(t, true, nil)
	type route struct{ method, path string }
	routes := []route{
		{"GET", handBase}, {"GET", handBase + "/hero/h-tam"},
		{"GET", r.openPath(r.tamCopy)}, {"POST", r.turnPath(r.tamCopy)},
	}
	do := func(rg route, user string, hdr ...string) *httptest.ResponseRecorder {
		if rg.method == "POST" {
			return r.post(rg.path, user, hdr...)
		}
		return r.get(rg.path, user, hdr...)
	}
	for _, rg := range routes {
		for _, h := range [][]string{nil, {"HX-Request", "true"}, {"Accept", "application/json"}} {
			if rec := do(rg, "u-other", h...); rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "dk-") {
				t.Errorf("a signed-in non-member at %s %s %v: %d, want 403", rg.method, rg.path, h, rec.Code)
			}
		}
		if page := do(rg, ""); page.Code != http.StatusSeeOther || !strings.HasPrefix(page.Header().Get("Location"), "/login") {
			t.Errorf("signed out at %s (page): %d", rg.path, page.Code)
		}
		if frag := do(rg, "", "HX-Request", "true"); frag.Code != http.StatusNoContent || !strings.HasPrefix(frag.Header().Get("HX-Redirect"), "/login") {
			t.Errorf("signed out at %s (HTMX): %d", rg.path, frag.Code)
		}
	}
	off := newHandWorld(t, false, nil)
	offRoutes := []route{{"GET", handBase}, {"GET", handBase + "/hero/h-tam"}, {"GET", off.openPath(off.tamCopy)}, {"POST", off.turnPath(off.tamCopy)}}
	for _, rg := range offRoutes {
		for _, h := range [][]string{{"Accept", "application/json"}, {"HX-Request", "true"}} {
			var rec *httptest.ResponseRecorder
			if rg.method == "POST" {
				rec = off.post(rg.path, "u-tam", h...)
			} else {
				rec = off.get(rg.path, "u-tam", h...)
			}
			if rec.Code != http.StatusNotFound {
				t.Errorf("add-on off at %s %s %v: %d, want 404", rg.method, rg.path, h, rec.Code)
			}
		}
		var page *httptest.ResponseRecorder
		if rg.method == "POST" {
			page = off.post(rg.path, "u-tam")
		} else {
			page = off.get(rg.path, "u-tam")
		}
		if page.Code != http.StatusSeeOther {
			t.Errorf("add-on off at %s (page): %d, want 303", rg.path, page.Code)
		}
	}
	// A Director asking for another campaign's copy under their own address.
	if rec := r.get("/campaigns/camp-2/decks/hand/"+r.deck+"/copies/"+r.tamCopy, "u-other"); rec.Code != http.StatusNotFound {
		t.Errorf("camp-2's Director asking for camp-1's copy: %d, want 404", rec.Code)
	}
}

var copyID = regexp.MustCompile(`data-copy="[^"]*"`)

// scrub removes the one thing that is meant to differ between worlds: ids.
func scrub(s string, ids ...string) string {
	s = copyID.ReplaceAllString(s, `data-copy="COPY"`)
	for _, id := range ids {
		s = strings.ReplaceAll(s, id, "ID")
	}
	return s
}

// The hand as the fragment is drawn is the same bytes whichever card lies
// face down under it, and whatever a Director has done out of sight.
func TestTwoWorldsTheHandFragmentIsTheSameBytes(t *testing.T) {
	type world func(w *handWorld)
	run := func(t *testing.T, which string, extra world) map[string]string {
		r := newWorkshopRig(t, true)
		fx := r.fx
		deck := fx.deck("Omens", KindHero, &SettingsInput{Knows: ptr("kinds"), Arrives: ptr("down"), OthersSee: ptr("face"), Draws: ptr("players")})
		_, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patchOf(1), Look: patchOf(LookInput{
			Kinds: []KindDef{{Key: "boon", Name: "Boon", Colour: "teal"}, {Key: "bane", Name: "Bane", Colour: "rose"}}})})
		fx.must(err)
		fx.choose(deck, PlayerInput{UserID: "u-tam", DrawsLeft: ptr(3)}, PlayerInput{UserID: "u-ilse"})
		x := fx.card(deck, CreateCardInput{Name: "Xcard", KindKey: "boon", Words: "XWORDS", Copies: ptr(2), Picture: "pic-xx", AskFirst: false, Fate: FateUsed})
		y := fx.card(deck, CreateCardInput{Name: "Ycard", KindKey: "bane", Words: "YWORDS", Copies: ptr(2), Picture: "pic-yy", AskFirst: true, Fate: FateBack})
		z := fx.card(deck, CreateCardInput{Name: "Zcard", KindKey: "boon", Words: "ZWORDS", Copies: ptr(3), Picture: "pic-zz"})
		_, err = fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: z, To: "hand", HeroID: "h-tam"})
		fx.must(err)
		dealt, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: map[string]string{"X": x, "Y": y}[which], To: "hand", HeroID: "h-tam"})
		fx.must(err)
		w := &handWorld{workshopRig: r, deck: deck, tamCopy: dealt.CopyID}
		if extra != nil {
			extra(w)
		}
		out := map[string]string{}
		for _, user := range []string{"u-tam", "u-ilse", "u-scribe"} {
			out[user+"/hand"] = scrub(w.get(handBase, user).Body.String(), deck)
			out[user+"/strip"] = scrub(w.get(handBase+"/hero/h-tam", user).Body.String(), deck)
		}
		return out
	}
	hidden := func(back int) world {
		return func(w *handWorld) {
			fx := w.fx
			res, err := fx.svc.Draw(bg, camp, w.deck, fx.dir, DrawInput{Count: 5})
			fx.must(err)
			for _, id := range res.CopyIDs[:back] {
				_, err := fx.svc.Act(bg, camp, w.deck, id, "put-back", fx.dir, ActInput{})
				fx.must(err)
			}
		}
	}
	drewOne := func(w *handWorld) {
		_, err := w.fx.svc.Draw(bg, camp, w.deck, w.fx.dir, DrawInput{})
		w.fx.must(err)
	}
	scenarios := []struct {
		name string
		a, b world
	}{
		{"dealt down", nil, nil},
		{"drew five and put all back", nil, hidden(5)},
		{"drew five, put four back", drewOne, hidden(4)},
		{"next and last set", nil, func(w *handWorld) {
			cards := w.fx.repo.cardsOf(w.deck)
			w.fx.must(w.fx.nudge(w.deck, patchOf(cards[1]), patchOf(cards[0])))
		}},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			// "Which card is under it" is the difference between the two worlds in
			// the first scenario; in the others the same card with hidden acts.
			ax, by := run(t, "X", sc.a), run(t, "Y", sc.b)
			if sc.a != nil || sc.b != nil {
				by = run(t, "X", sc.b)
			}
			for k := range ax {
				if ax[k] != by[k] {
					t.Errorf("%s differs between worlds:\n A: %s\n B: %s", k, ax[k], by[k])
				}
				for _, bad := range []string{"XWORDS", "YWORDS", "ZWORDS", "Xcard", "Ycard", "pic-xx", "pic-yy", "pic-zz", "nextCardId", "directorNote"} {
					if strings.Contains(ax[k], bad) || strings.Contains(by[k], bad) {
						t.Errorf("%s names %q", k, bad)
					}
				}
			}
			if len(ax) != 6 {
				t.Errorf("only %d fragments compared", len(ax))
			}
		})
	}
}

// A card on the table opens for every member, whatever decks they may list:
// the Hand leaf draws it with an open button, so the button has to work. The
// rest of such a deck stays as hidden as the deck is.
func TestATableCardOpensForAMemberWhoCannotListItsDeck(t *testing.T) {
	r := newWorkshopRig(t, true)
	fx := r.fx
	vault := fx.deck("Vault", KindEvent, &SettingsInput{Fate: ptr(FatePlay)})
	_, err := fx.svc.Update(bg, camp, vault, fx.dir, UpdateDeckInput{Version: patchOf(1), Look: patchOf(LookInput{
		Kinds: []KindDef{{Key: "boon", Name: "Boon", Colour: "teal"}, {Key: "bane", Name: "VAULTKIND", Colour: "rose"}}})})
	fx.must(err)
	onTable := fx.card(vault, CreateCardInput{Name: "Stormfront", KindKey: "boon", Words: "STORM-WORDS"})
	hidden := fx.card(vault, CreateCardInput{Name: "Buried", KindKey: "bane", Words: "BURIED-WORDS", Copies: ptr(2)})
	res, err := fx.svc.DealCard(bg, camp, vault, fx.dir, DealCardInput{CardID: onTable, To: "table"})
	fx.must(err)
	open := func(copyID string) string { return handBase + "/" + vault + "/copies/" + copyID }

	got := r.get(open(res.CopyID), "u-ilse")
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "STORM-WORDS") {
		t.Fatalf("a table card of a deck Ilse cannot list: %d %.300s", got.Code, got.Body.String())
	}
	for _, bad := range []string{"VAULTKIND", "BURIED-WORDS", "data-dkh-act"} {
		if strings.Contains(got.Body.String(), bad) {
			t.Errorf("the opened table card carries %q", bad)
		}
	}
	// A copy of the same deck that is not on the table, and one that does not
	// exist, answer alike.
	inPile := fx.copiesOf(hidden)[0]
	missing := r.get(open("00000000-0000-0000-0000-00000000dead"), "u-ilse")
	pile := r.get(open(inPile), "u-ilse")
	if pile.Code != http.StatusNotFound || missing.Code != http.StatusNotFound || pile.Body.String() != missing.Body.String() {
		t.Errorf("a pile copy of a hidden deck: %d %q; a missing copy: %d %q", pile.Code, pile.Body.String(), missing.Code, missing.Body.String())
	}
	// The turn request never works on it.
	if rec := r.post(open(res.CopyID)+"/turn", "u-ilse"); rec.Code == http.StatusOK {
		t.Errorf("a table card of a hidden deck was turned: %d", rec.Code)
	}
}
