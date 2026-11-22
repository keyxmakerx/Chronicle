package decks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// leakWorld builds decks holding things no player may be sent, with a copy in
// every zone, then each test marshals what players are sent and searches it.
type leakWorld struct {
	*fixture
	secrets, party, vault string
	partyUsedCopy         string
	secretCopies          map[string]string // card name -> copy id
}

func newLeakWorld(t *testing.T) *leakWorld {
	fx := newFixture(t)
	w := &leakWorld{fixture: fx, secretCopies: map[string]string{}}

	// An event deck Tam is chosen for: knows only the count, others see backs.
	w.secrets = fx.deck("Secrets", KindEvent, &SettingsInput{Draws: ptr("players"), Knows: ptr("count"), OthersSee: ptr("back")})
	for _, c := range []struct{ name, words, note string }{
		{"PileCard", "PILEWORDS", "PILENOTE"},
		{"ShownCard", "SHOWNWORDS", "SHOWNNOTE"},
		{"DownCard", "TAMFACEDOWN", "DOWNNOTE"},
		{"UpCard", "TAMFACEUP", "UPNOTE"},
		{"IlseDown", "ILSEFACEDOWN", "ILSEDOWNNOTE"},
		{"IlseUp", "ILSEFACEUP", "ILSEUPNOTE"},
		{"UsedCard", "USEDWORDS", "USEDNOTE"},
		{"GoneCard", "GONEWORDS", "GONENOTE"},
		{"TableCard", "TABLEWORDS", "TABLENOTE"},
	} {
		id := fx.card(w.secrets, CreateCardInput{Name: c.name, Words: c.words, DirectorNote: c.note})
		w.secretCopies[c.name] = fx.copiesOf(id)[0]
	}
	fx.choose(w.secrets, PlayerInput{UserID: "u-tam", DrawsLeft: ptr(2)})
	fx.place(w.secretCopies["ShownCard"], ZoneShown, "", false)
	fx.place(w.secretCopies["DownCard"], ZoneHand, "h-tam", false)
	fx.place(w.secretCopies["UpCard"], ZoneHand, "h-tam", true)
	fx.place(w.secretCopies["IlseDown"], ZoneHand, "h-ilse", false)
	fx.place(w.secretCopies["IlseUp"], ZonePlay, "h-ilse", true)
	fx.place(w.secretCopies["UsedCard"], ZoneUsed, "", true)
	fx.place(w.secretCopies["GoneCard"], ZoneGone, "", true)
	fx.place(w.secretCopies["TableCard"], ZonePlay, "", true)

	// A party deck: every member sees it and its used pile, not its pile.
	w.party = fx.deck("Fortune", KindParty, nil)
	for _, c := range []struct{ name, words, note string }{
		{"PartyPile", "FORTUNEPILETEXT", "PARTYPILENOTE"},
		{"PartyUsed", "FORTUNEUSEDTEXT", "PARTYUSEDNOTE"},
	} {
		id := fx.card(w.party, CreateCardInput{Name: c.name, Words: c.words, DirectorNote: c.note})
		if c.name == "PartyUsed" {
			w.partyUsedCopy = fx.copiesOf(id)[0]
		}
	}
	fx.place(w.partyUsedCopy, ZoneUsed, "", true)

	// A deck no player is on: it must not exist for them.
	w.vault = fx.deck("VaultOfSecrets", KindEvent, nil)
	id := fx.card(w.vault, CreateCardInput{Name: "VaultCard", Words: "VAULTWORDS", DirectorNote: "VAULTNOTE"})
	fx.place(fx.copiesOf(id)[0], ZoneHand, "h-vault", false)
	return w
}

// everyDirectorOnly is text that must never reach a player of this world.
var everyDirectorOnly = []string{
	"PILENOTE", "SHOWNNOTE", "DOWNNOTE", "UPNOTE", "ILSEDOWNNOTE", "ILSEUPNOTE", "USEDNOTE", "GONENOTE", "TABLENOTE",
	"PARTYPILENOTE", "PARTYUSEDNOTE", "VAULTNOTE",
	"directorNote", "whenJson", `"when"`, "whenOpen", "askFirst",
	"VaultOfSecrets", "VaultCard", "VAULTWORDS", "h-vault",
}

// responses marshals everything a player can ask for.
func (w *leakWorld) responses(v Viewer) (out []string, errs map[string]int) {
	out, _, errs = w.labelled(v)
	return out, errs
}

// labelled is responses with each body under the request that produced it.
func (w *leakWorld) labelled(v Viewer) (out []string, by map[string]string, errs map[string]int) {
	errs = map[string]int{}
	by = map[string]string{}
	add := func(label string, val any, err error) {
		if err != nil {
			errs[label] = code(err)
			return
		}
		raw, e := json.Marshal(val)
		if e != nil {
			w.t.Fatal(e)
		}
		out = append(out, string(raw))
		by[label] = string(raw)
	}
	list, err := w.svc.List(bg, camp, v)
	add("list", list, err)
	for name, id := range map[string]string{"secrets": w.secrets, "party": w.party, "vault": w.vault} {
		d, err := w.svc.Get(bg, camp, id, v)
		add("get "+name, d, err)
	}
	table, err := w.svc.Table(bg, camp, v)
	add("table", table, err)
	return out, by, errs
}

func TestLeak_OwningPlayer(t *testing.T) {
	w := newLeakWorld(t)
	got, errs := w.responses(w.tam)
	all := strings.Join(got, "\n")
	for _, bad := range append(append([]string{}, everyDirectorOnly...),
		// face-down, pile, shown, used (of a non-party deck) and gone contents
		"DownCard", "TAMFACEDOWN", "PileCard", "PILEWORDS", "ShownCard", "SHOWNWORDS", "UsedCard", "USEDWORDS",
		"GoneCard", "GONEWORDS", "PartyPile", "FORTUNEPILETEXT",
		// another hero's face-down card, and (othersSee is back) even their face-up one
		"IlseDown", "ILSEFACEDOWN", "IlseUp", "ILSEFACEUP") {
		if strings.Contains(all, bad) {
			t.Errorf("owning player is sent %q", bad)
		}
	}
	for _, want := range []string{"UpCard", "TAMFACEUP", "TableCard", "TABLEWORDS", "PartyUsed", "FORTUNEUSEDTEXT", "Secrets", "Fortune"} {
		if !strings.Contains(all, want) {
			t.Errorf("owning player should be sent %q", want)
		}
	}
	if errs["get vault"] != http.StatusNotFound {
		t.Errorf("an unlisted deck answered %d, want 404", errs["get vault"])
	}
	// The face-down copy is its id, deck and that it is face down, nothing else.
	var list PlayerList
	raw, _ := json.Marshal(mustList(t, w.svc, w.tam))
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	for _, d := range list.Decks {
		for _, h := range d.Hands {
			for _, c := range h.Hand {
				if c.Face != nil && !c.FaceUp {
					t.Errorf("a face-down copy carries a face: %+v", c)
				}
			}
		}
	}
	var seenDown bool
	for _, d := range list.Decks {
		for _, h := range d.Hands {
			for _, c := range h.Hand {
				if !c.FaceUp {
					seenDown = true
					b, _ := json.Marshal(c)
					if string(b) != `{"id":"`+c.ID+`","deckId":"`+w.secrets+`","faceUp":false,"waiting":false}` {
						t.Errorf("face-down copy JSON = %s", b)
					}
				}
			}
		}
	}
	if !seenDown {
		t.Error("the owner should still hold their face-down card")
	}
}

func mustList(t *testing.T, s Service, v Viewer) DeckListView {
	t.Helper()
	l, err := s.List(bg, camp, v)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestLeak_OtherPlayerAndScribe(t *testing.T) {
	for _, who := range []string{"ilse", "scribe"} {
		t.Run(who, func(t *testing.T) {
			w := newLeakWorld(t)
			v := w.ilse
			if who == "scribe" {
				v = w.scribe
			}
			got, errs := w.responses(v)
			all := strings.Join(got, "\n")
			for _, bad := range append(append([]string{}, everyDirectorOnly...),
				"TAMFACEDOWN", "TAMFACEUP", "PileCard", "PILEWORDS", "SHOWNWORDS", "GONEWORDS", "FORTUNEPILETEXT", "ILSEFACEDOWN") {
				if who == "ilse" && (bad == "ILSEFACEDOWN") {
					continue
				}
				if strings.Contains(all, bad) {
					t.Errorf("%s is sent %q", who, bad)
				}
			}
			if !strings.Contains(all, "TABLEWORDS") || !strings.Contains(all, "FORTUNEUSEDTEXT") {
				t.Errorf("%s should read the table and the party's used pile", who)
			}
			if errs["get secrets"] != http.StatusNotFound && who == "scribe" {
				t.Errorf("an event deck the Scribe is not on answered %d, want 404", errs["get secrets"])
			}
			if errs["get vault"] != http.StatusNotFound {
				t.Errorf("an unlisted deck answered %d, want 404", errs["get vault"])
			}
			if who == "scribe" {
				// Not in the list either: event decks are never listed to a stranger.
				// (A card lying on the table names its deck, by design.)
				_, by, _ := w.labelled(v)
				if strings.Contains(by["list"], "Secrets") || strings.Contains(by["get secrets"], "Secrets") {
					t.Error("the Scribe is told an event deck exists")
				}
			}
		})
	}
}

func TestLeak_FormerMemberAndVisitorSeeNothing(t *testing.T) {
	for name, pick := range map[string]func(*fixture) Viewer{"former": formerOf, "visitor": visitorOf} {
		t.Run(name, func(t *testing.T) {
			w := newLeakWorld(t)
			got, errs := w.responses(pick(w.fixture))
			if len(got) != 0 {
				t.Errorf("sent %d responses: %v", len(got), got)
			}
			for _, label := range []string{"list", "get secrets", "get party", "get vault", "table"} {
				if errs[label] != http.StatusNotFound {
					t.Errorf("%s answered %d, want 404", label, errs[label])
				}
			}
			// Nor can they reach a copy by id.
			for _, id := range w.secretCopies {
				if _, err := w.svc.Act(bg, camp, w.secrets, id, "turn", pick(w.fixture), ActInput{}); code(err) != http.StatusNotFound {
					t.Fatalf("act answered %d, want 404", code(err))
				}
			}
		})
	}
}

// A former member who was chosen once still sees nothing even though the
// grant row remains.
func TestLeak_FormerChosenPlayer(t *testing.T) {
	w := newLeakWorld(t)
	former := Viewer{UserID: "u-tam", MemberRole: 0, VisibilityRole: 0}
	if _, errs := w.responses(former); errs["list"] != http.StatusNotFound || errs["get secrets"] != http.StatusNotFound {
		t.Errorf("former chosen player: %v", errs)
	}
}

// Knows and othersSee decide what else a listed player is sent.
func TestLeak_KnowsAndOthersSee(t *testing.T) {
	tests := []struct {
		name      string
		knows     string
		others    string
		wantPile  bool
		wantKinds bool
		wantCards bool
		wantOther string // "", "count" or "faces"
	}{
		{name: "nothing and nothing", knows: "nothing", others: "nothing"},
		{name: "count and back", knows: "count", others: "back", wantPile: true, wantOther: "count"},
		{name: "kinds and face", knows: "kinds", others: "face", wantPile: true, wantKinds: true, wantOther: "faces"},
		{name: "faces", knows: "faces", others: "back", wantPile: true, wantKinds: true, wantCards: true, wantOther: "count"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newLeakWorld(t)
			d := w.repo.decks[w.secrets]
			d.Settings.Knows, d.Settings.OthersSee = tt.knows, tt.others
			w.repo.decks[w.secrets] = d
			v, err := w.svc.Get(bg, camp, w.secrets, w.tam)
			w.must(err)
			pd := v.(PlayerDeck)
			if (pd.Pile != nil) != tt.wantPile || (pd.Kinds != nil) != tt.wantKinds || (pd.Cards != nil) != tt.wantCards {
				t.Errorf("pile=%v kinds=%v cards=%v", pd.Pile != nil, pd.Kinds != nil, pd.Cards != nil)
			}
			// One copy lies in the pile and one in the shown spot; a player
			// counts both, so a Director looking at a card changes no count.
			if tt.wantPile && *pd.Pile != 2 {
				t.Errorf("pile count = %d, want 2", *pd.Pile)
			}
			raw, _ := json.Marshal(pd)
			s := string(raw)
			if tt.wantCards != strings.Contains(s, "PILEWORDS") {
				t.Errorf("card faces present = %v, want %v", strings.Contains(s, "PILEWORDS"), tt.wantCards)
			}
			// Even when every face is known, the Director's notes and the hidden
			// copies' positions are not.
			for _, bad := range []string{"PILENOTE", "directorNote", "SHOWNNOTE"} {
				if strings.Contains(s, bad) {
					t.Errorf("sent %q", bad)
				}
			}
			switch tt.wantOther {
			case "":
				if len(pd.Others) != 0 {
					t.Errorf("others = %+v, want none", pd.Others)
				}
			case "count":
				if len(pd.Others) != 1 || pd.Others[0].Count != 2 || len(pd.Others[0].Faces) != 0 || (!tt.wantCards && strings.Contains(s, "ILSEFACEUP")) {
					t.Errorf("others = %+v", pd.Others)
				}
			case "faces":
				if len(pd.Others) != 1 || pd.Others[0].Count != 2 || len(pd.Others[0].Faces) != 1 || pd.Others[0].Faces[0].Words != "ILSEFACEUP" || strings.Contains(s, "ILSEFACEDOWN") {
					t.Errorf("others = %+v", pd.Others)
				}
			}
		})
	}
}

// A hero page a player cannot open is never named to them, even as a count.
func TestLeak_HiddenHeroIsNotNamed(t *testing.T) {
	w := newLeakWorld(t)
	id := w.card(w.secrets, CreateCardInput{Name: "VaultHeld"})
	w.place(w.copiesOf(id)[0], ZoneHand, "h-vault", true)
	v, err := w.svc.Get(bg, camp, w.secrets, w.tam)
	w.must(err)
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "h-vault") || strings.Contains(string(raw), "VaultHeld") {
		t.Errorf("a hidden hero is named: %s", raw)
	}
}

// The player types carry no field a Director type holds.
func TestPlayerTypesHaveNoHiddenFields(t *testing.T) {
	raw, _ := json.Marshal(PlayerDeck{Hands: []PlayerHand{{Hand: []PlayerCopy{{Face: &Face{}}}}}, Cards: []Face{{}}, Used: []PlayerCopy{{}}})
	for _, bad := range []string{"directorNote", "when", "askFirst", "copies", "fate", "settings", "log", "zone", "holder", "players", "version"} {
		if strings.Contains(string(raw), `"`+bad) {
			t.Errorf("PlayerDeck JSON has a %q key: %s", bad, raw)
		}
	}
}

func TestDirectorSeesEverything(t *testing.T) {
	w := newLeakWorld(t)
	got, errs := w.responses(w.dir)
	all := strings.Join(got, "\n")
	for _, want := range []string{"PILENOTE", "VAULTNOTE", "VaultOfSecrets", "TAMFACEDOWN", "directorNote"} {
		if !strings.Contains(all, want) {
			t.Errorf("the Director should be sent %q", want)
		}
	}
	if len(errs) != 0 {
		t.Errorf("errors for the Director: %v", errs)
	}
}
