package records

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func answerOne(l *Lookups, a Actor, what string, fields map[string]any) LookupAnswer {
	return l.Answer(context.Background(), camp, a, []Record{lookup(what, fields)})[0]
}

func TestLookupPins_SayWhoSeesThem(t *testing.T) {
	maps := &fakeMaps{
		maps: []MapRef{{ID: "m1", CampaignID: camp, Name: "World"}},
		pins: []PinRef{
			{ID: "p1", Name: "Old Mill", X: 1, Y: 2, Category: "location", PageName: "The Mill", Description: "Abandoned since the flood."},
			{ID: "p2", Name: "Cult Lair", X: 3, Y: 4, DMOnly: true, Category: "danger", Description: "Strong\nwords   here"},
			{ID: "p3", Name: "Smuggler Cove", X: 5, Y: 6, OnlyFor: 2, HiddenFrom: 1},
		},
	}
	l := &Lookups{Maps: maps}

	tests := []struct {
		name    string
		actor   Actor
		want    []string
		notWant []string
	}{
		{"owner sees hidden and the rest", owner,
			[]string{
				"Cult Lair (x 3.0, y 4.0) · category: danger · hidden from players (visibility: dm_only) — Strong words here",
				"Old Mill (x 1.0, y 2.0) · category: location · page: The Mill · visibility: everyone — Abandoned since the flood.",
				"only 2 chosen players see it", "hidden from 1 chosen player",
			}, nil},
		{"player never gets a hidden pin, even if handed one", player,
			[]string{"Old Mill", "category: location", "page: The Mill", "Abandoned since the flood."},
			[]string{"Cult Lair", "dm_only", "hidden from", "visibility:", "chosen player", "Strong"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := answerOne(l, tc.actor, "pins", map[string]any{"map": "World"})
			if got.Error != "" {
				t.Fatalf("error: %s", got.Error)
			}
			for _, w := range tc.want {
				if !strings.Contains(got.Text, w) {
					t.Errorf("missing %q in:\n%s", w, got.Text)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(got.Text, w) {
					t.Errorf("a player's answer holds %q:\n%s", w, got.Text)
				}
			}
		})
	}
}

func TestLookupPins_LongNoteIsTrimmed(t *testing.T) {
	long := strings.Repeat("word ", 200)
	l := &Lookups{Maps: &fakeMaps{
		maps: []MapRef{{ID: "m1", CampaignID: camp, Name: "World"}},
		pins: []PinRef{{ID: "p1", Name: "Big", Description: long}},
	}}
	got := answerOne(l, owner, "pins", map[string]any{"map": "World"})
	if len(got.Text) > 600 || !strings.Contains(got.Text, "…") {
		t.Errorf("note not trimmed (%d bytes): %s", len(got.Text), got.Text)
	}
}

func TestPageAnswer(t *testing.T) {
	line := strings.Repeat("x", 99) + "\n"
	long := strings.Repeat(line, 600) // about 60 KB: three pieces
	tests := []struct {
		name      string
		text      string
		piece     int
		wantErr   bool
		want      []string
		wantEqual bool
	}{
		{name: "short answer is untouched", text: "## A\n\nshort\n", piece: 1, wantEqual: true},
		{name: "short answer has no piece 2", text: "## A\n\nshort\n", piece: 2, wantErr: true},
		{name: "first piece points on", text: long, piece: 1, want: []string{"Piece 1 of 3", "`piece: 2`"}},
		{name: "last piece ends clean", text: long, piece: 3, want: []string{"Piece 3 of 3"}},
		{name: "past the end is refused", text: long, piece: 4, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pageAnswer(tc.text, tc.piece)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantEqual && got != tc.text {
				t.Errorf("answer changed")
			}
			if len(got) > maxAnswerBytes {
				t.Errorf("piece is %d bytes, over the cap", len(got))
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q", w)
				}
			}
			if tc.piece == 3 && strings.Contains(got, "`piece: 4`") {
				t.Errorf("last piece points past itself")
			}
		})
	}
}

func TestPageAnswer_PiecesJoinBackToTheWhole(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 900; i++ {
		fmt.Fprintf(&b, "line %d with some words in it é\n", i)
	}
	text := b.String()
	var joined strings.Builder
	_, of := pieceOf(text, 1, maxAnswerBytes-pieceRoom)
	for p := 1; p <= of; p++ {
		piece, _ := pieceOf(text, p, maxAnswerBytes-pieceRoom)
		joined.WriteString(piece)
	}
	if joined.String() != text || of < 2 {
		t.Errorf("pieces do not rebuild the answer (%d pieces)", of)
	}
}

func TestPageAnswer_OneHugeLineIsStillReadable(t *testing.T) {
	text := strings.Repeat("é", maxAnswerBytes) // 2 bytes each, no newline
	var joined strings.Builder
	_, of := pieceOf(text, 1, maxAnswerBytes-pieceRoom)
	for p := 1; p <= of; p++ {
		piece, _ := pieceOf(text, p, maxAnswerBytes-pieceRoom)
		joined.WriteString(piece)
	}
	if joined.String() != text {
		t.Errorf("a long line lost or split characters")
	}
}

func TestLookups_PieceStepsThroughALongAnswer(t *testing.T) {
	p := &fakePlug{text: "## Widgets\n\n" + strings.Repeat("- a widget with a long name\n", maxAnswerBytes/20)}
	l := &Lookups{Plugged: []aiplug.Lookup{p}}
	first := answerOne(l, owner, "widgets", nil)
	second := answerOne(l, owner, "widgets", map[string]any{"piece": "2"})
	if !strings.Contains(first.Text, "Piece 1 of") || !strings.Contains(second.Text, "Piece 2 of") {
		t.Fatalf("pieces not numbered:\n%.120s\n%.120s", first.Text, second.Text)
	}
	if strings.Contains(first.Text, "cut here") || first.Text == second.Text {
		t.Errorf("piece 2 repeats piece 1 or the answer was cut")
	}
	for _, bad := range []string{"0", "two", "-1"} {
		if got := answerOne(l, owner, "widgets", map[string]any{"piece": bad}); got.Error == "" {
			t.Errorf("piece %q was accepted", bad)
		}
	}
	if got := answerOne(l, owner, "widgets", map[string]any{"piece": "99"}); got.Error == "" {
		t.Errorf("a piece past the end was accepted")
	}
}

func TestLookups_ListPiecesReadTheRest(t *testing.T) {
	var all []entities.Entity
	for i := 0; i < 700; i++ {
		all = append(all, page(fmt.Sprintf("p%03d", i), fmt.Sprintf("Page %03d", i), "Place", false))
	}
	l := &Lookups{Pages: &fakePages{all: all}}

	tests := []struct {
		piece   string
		want    []string
		notWant []string
		wantErr bool
	}{
		{"", []string{"Page 000", "Page 299", "`piece: 2`"}, []string{"Page 300"}, false},
		{"2", []string{"Lines 301 to 600 of 700", "Page 300", "Page 599", "### Places"}, []string{"Page 299", "Page 600"}, false},
		{"3", []string{"Lines 601 to 700 of 700", "Page 699"}, []string{"Page 599", "`piece: 4`"}, false},
		{"4", nil, nil, true},
	}
	for _, tc := range tests {
		t.Run("piece "+tc.piece, func(t *testing.T) {
			fields := map[string]any{}
			if tc.piece != "" {
				fields["piece"] = tc.piece
			}
			got := answerOne(l, owner, "pages", fields)
			if (got.Error != "") != tc.wantErr {
				t.Fatalf("error = %q, wantErr %v", got.Error, tc.wantErr)
			}
			for _, w := range tc.want {
				if !strings.Contains(got.Text, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(got.Text, w) {
					t.Errorf("unexpected %q", w)
				}
			}
		})
	}
}

func TestLookups_PinListPiecesReadTheRest(t *testing.T) {
	var pins []PinRef
	for i := 0; i < 650; i++ {
		pins = append(pins, PinRef{ID: fmt.Sprint(i), Name: fmt.Sprintf("Pin %03d", i)})
	}
	l := &Lookups{Maps: &fakeMaps{maps: []MapRef{{ID: "m1", CampaignID: camp, Name: "World"}}, pins: pins}}
	got := answerOne(l, owner, "pins", map[string]any{"map": "World", "piece": "3"})
	if got.Error != "" || !strings.Contains(got.Text, "Pin 600") || !strings.Contains(got.Text, "Pin 649") || strings.Contains(got.Text, "Pin 599") {
		t.Errorf("piece 3 of the pins: %q", got.Error+got.Text)
	}
}

func TestTellChaptersApart(t *testing.T) {
	chapters := []houseChapter{
		{title: "Combat", text: "### Combat\n\nA\n"},
		{title: "Travel", text: "### Travel\n\nB\n"},
		{title: "combat", text: "### combat\n\nC\n"},
	}
	shared := tellChaptersApart(chapters)
	if chapters[0].title != "Combat (1 of 2)" || chapters[2].title != "combat (2 of 2)" || chapters[1].title != "Travel" {
		t.Errorf("titles = %q, %q, %q", chapters[0].title, chapters[1].title, chapters[2].title)
	}
	if !strings.HasPrefix(chapters[2].text, "### combat (2 of 2)\n") || !strings.HasPrefix(chapters[1].text, "### Travel\n") {
		t.Errorf("headings not renamed with their chapters: %q", chapters[2].text)
	}
	if len(shared["combat"]) != 2 || len(shared["travel"]) != 0 {
		t.Errorf("shared = %v", shared)
	}
}

// searchPages is a page search that hides private pages from anyone below a
// co-DM, as the real service's visibility filter does.
type searchPages struct {
	fakePages
	gotQuery string
	gotPage  int
}

func (s *searchPages) Search(_ context.Context, _, query string, typeID int, role int, _ string, o entities.ListOptions) ([]entities.Entity, int, error) {
	s.gotQuery, s.gotPage = query, o.Page
	var hits []entities.Entity
	for _, e := range s.all {
		if (e.IsPrivate && role < 3) || (typeID != 0 && e.EntityTypeID != typeID) {
			continue
		}
		if strings.Contains(strings.ToLower(e.Name), strings.ToLower(query)) {
			hits = append(hits, e)
		}
	}
	start := min((o.Page-1)*o.PerPage, len(hits))
	end := min(start+o.PerPage, len(hits))
	return hits[start:end], len(hits), nil
}

func (s *searchPages) SearchSnippets(_ context.Context, _, query string, ids []string, role int, _ string) map[string][]entities.SnippetPart {
	out := map[string][]entities.SnippetPart{}
	for _, id := range ids {
		out[id] = []entities.SnippetPart{{Text: "met the "}, {Text: query, Mark: true}, {Text: " at dusk\nin the rain"}}
	}
	return out
}

func TestLookupSearch(t *testing.T) {
	pages := &searchPages{fakePages: fakePages{
		all: []entities.Entity{
			page("a", "Dragon Hoard", "Item", false),
			page("b", "Dragon Cult Plans", "Note", true), // GM-only
			page("c", "Dragonfly Inn", "Place", false),
		},
		types: []entities.EntityType{{ID: 7, Name: "Place", NamePlural: "Places", Slug: "place"}},
	}}
	pages.all[2].EntityTypeID = 7
	l := &Lookups{Pages: pages}

	tests := []struct {
		name    string
		actor   Actor
		fields  map[string]any
		want    []string
		notWant []string
		wantErr string
	}{
		{"owner finds the GM-only page too", owner, map[string]any{"query": "dragon"},
			[]string{"Dragon Hoard (Item)", "Dragon Cult Plans (Note)", "Dragonfly Inn (Place)", "met the **dragon** at dusk in the rain", "read_page"}, nil, ""},
		{"player never finds or learns of a GM-only page", player, map[string]any{"query": "dragon"},
			[]string{"Dragon Hoard", "Dragonfly Inn", "(2)"}, []string{"Cult", "Plans"}, ""},
		{"player searching for the secret page's name finds nothing", player, map[string]any{"query": "cult plans"},
			[]string{"No page you can see matches"}, []string{"Cult Plans"}, ""},
		{"kind narrows it", owner, map[string]any{"query": "dragon", "type": "Places"},
			[]string{"Dragonfly Inn"}, []string{"Hoard"}, ""},
		{"unknown kind is refused", owner, map[string]any{"query": "dragon", "type": "Starship"}, nil, nil, "no kind of page called"},
		{"one letter is too short", owner, map[string]any{"query": "d"}, nil, nil, "at least two characters"},
		{"no query is refused", owner, nil, nil, nil, "it needs `query`"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := answerOne(l, tc.actor, "search", tc.fields)
			if tc.wantErr != "" {
				if !strings.Contains(got.Error, tc.wantErr) {
					t.Fatalf("error = %q, want %q", got.Error, tc.wantErr)
				}
				return
			}
			if got.Error != "" {
				t.Fatalf("error: %s", got.Error)
			}
			for _, w := range tc.want {
				if !strings.Contains(got.Text, w) {
					t.Errorf("missing %q in:\n%s", w, got.Text)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(got.Text, w) {
					t.Errorf("unexpected %q in:\n%s", w, got.Text)
				}
			}
		})
	}
}

// A Director's search hits say who sees each page, as the page index does;
// a player's never carry a mark or the legend.
func TestLookupSearch_SightMarks(t *testing.T) {
	shared := page("s", "Wolf Den", "Place", false)
	shared.Visibility = entities.VisibilityCustom
	adventure := page("a", "Wolf Hunt", "Adventure", false)
	adventure.TypeDMOnly = true
	pages := &searchPages{fakePages: fakePages{all: []entities.Entity{
		page("p", "Wolf Pack", "Creature", false),
		page("g", "Wolf Secret", "Note", true),
		shared, adventure,
	}}}
	l := &Lookups{Pages: pages}
	tests := []struct {
		name    string
		actor   Actor
		query   string
		want    []string
		notWant []string
	}{
		{"director sees the marks and the legend", owner, "wolf",
			[]string{sightLegend, "- Wolf Secret (Note) (GM only):", "- Wolf Den (Place) (shared with some):", "- Wolf Hunt (Adventure) (GM only):", "- Wolf Pack (Creature):"},
			[]string{"Wolf Pack (Creature) ("}},
		{"no mark, no legend", owner, "pack", []string{"- Wolf Pack (Creature):"}, []string{"(GM only)", "Neither is part of the name"}},
		{"a player gets no marks", player, "wolf", []string{"- Wolf Den (Place):", "- Wolf Pack (Creature):"},
			[]string{"(GM only)", "(shared with some)", "Neither is part of the name", "Wolf Secret"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := answerOne(l, tc.actor, "search", map[string]any{"query": tc.query})
			if got.Error != "" {
				t.Fatalf("error: %s", got.Error)
			}
			for _, w := range tc.want {
				if !strings.Contains(got.Text, w) {
					t.Errorf("missing %q in:\n%s", w, got.Text)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(got.Text, w) {
					t.Errorf("unexpected %q in:\n%s", w, got.Text)
				}
			}
		})
	}
}

func TestLookupSearch_PiecesAndCaps(t *testing.T) {
	var all []entities.Entity
	for i := 0; i < 60; i++ {
		all = append(all, page(fmt.Sprintf("p%02d", i), fmt.Sprintf("Wolf %02d", i), "Creature", false))
	}
	pages := &searchPages{fakePages: fakePages{all: all}}
	l := &Lookups{Pages: pages}

	first := answerOne(l, owner, "search", map[string]any{"query": "wolf"})
	if got := strings.Count(first.Text, "\n- "); got != searchPerPiece || !strings.Contains(first.Text, "`piece: 2`") {
		t.Errorf("first piece has %d hits:\n%s", got, first.Text)
	}
	last := answerOne(l, owner, "search", map[string]any{"query": "wolf", "piece": "3"})
	if pages.gotPage != 3 || strings.Count(last.Text, "\n- ") != 10 || strings.Contains(last.Text, "`piece: 4`") {
		t.Errorf("piece 3: page %d\n%s", pages.gotPage, last.Text)
	}
	if got := answerOne(l, owner, "search", map[string]any{"query": "wolf", "piece": "9"}); got.Error == "" {
		t.Errorf("a piece past the end was accepted")
	}
}

func TestLookupSearch_NeedsASearchableServer(t *testing.T) {
	// fakePages lists pages but cannot search them.
	l := &Lookups{Pages: &fakePages{}}
	if got := answerOne(l, owner, "search", map[string]any{"query": "dragon"}); !strings.Contains(got.Error, "can't be used") {
		t.Errorf("error = %q", got.Error)
	}
	if got := answerOne(&Lookups{}, owner, "search", map[string]any{"query": "dragon"}); got.Error == "" {
		t.Errorf("an unwired server answered a search")
	}
}

// piecedPlug is a plugged lookup that cuts its own list, as game nights do.
type piecedPlug struct{ lines []string }

func (piecedPlug) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{What: "long-list", Keys: "none", Brief: "a long list"}
}

func (p piecedPlug) Answer(_ context.Context, _ string, _ Actor, r Record) (string, string, error) {
	var b strings.Builder
	b.WriteString("## Long list\n\n")
	aiplug.WriteLinesPiece(&b, p.lines, aiplug.PieceOf(r))
	return "Long list", b.String(), nil
}

func (piecedPlug) Pieced() {}

// longName is a unique name padded to about n bytes, so a list of them
// outgrows the answer cap well before it reaches the line cap.
func longName(prefix string, i, n int) string {
	head := fmt.Sprintf("%s %04d ", prefix, i)
	return head + strings.Repeat("w", max(n-len(head), 0))
}

// Every line of a long-lined list is reachable by stepping through `piece`:
// no piece is cut by the answer cap with its tail out of reach.
func TestLookups_LongLinesAreAllReachable(t *testing.T) {
	pagesOf := func(n int) *Lookups {
		var all []entities.Entity
		for i := 0; i < n; i++ {
			all = append(all, page(fmt.Sprintf("p%04d", i), longName("Page", i, 120), "Place", false))
		}
		return &Lookups{Pages: &fakePages{all: all}}
	}
	pinsOf := func(n int) *Lookups {
		var pins []PinRef
		for i := 0; i < n; i++ {
			pins = append(pins, PinRef{ID: fmt.Sprint(i), Name: longName("Pin", i, 140), Category: "location"})
		}
		return &Lookups{Maps: &fakeMaps{maps: []MapRef{{ID: "m1", CampaignID: camp, Name: "World"}}, pins: pins}}
	}
	plugOf := func(n int) *Lookups {
		var lines []string
		for i := 0; i < n; i++ {
			lines = append(lines, "- "+longName("Line", i, 100))
		}
		return &Lookups{Plugged: []aiplug.Lookup{piecedPlug{lines: lines}}}
	}
	tests := []struct {
		name   string
		l      *Lookups
		what   string
		fields map[string]any
		prefix string
		n      int
	}{
		{"pages index under the line cap", pagesOf(250), "pages", nil, "Page", 250},
		{"pages index over the line cap", pagesOf(700), "pages", nil, "Page", 700},
		{"pins under the line cap", pinsOf(250), "pins", map[string]any{"map": "World"}, "Pin", 250},
		{"pins over the line cap", pinsOf(650), "pins", map[string]any{"map": "World"}, "Pin", 650},
		{"a plugged list that cuts itself", plugOf(650), "long-list", nil, "Line", 650},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var seen strings.Builder
			pieces := 0
			for p := 1; p <= 50; p++ {
				fields := map[string]any{"piece": fmt.Sprint(p)}
				for k, v := range tc.fields {
					fields[k] = v
				}
				got := answerOne(tc.l, owner, tc.what, fields)
				if got.Error != "" {
					t.Fatalf("piece %d: %s", p, got.Error)
				}
				if len(got.Text) > maxAnswerBytes || strings.Contains(got.Text, "cut here") {
					t.Fatalf("piece %d was cut (%d bytes)", p, len(got.Text))
				}
				seen.WriteString(got.Text)
				pieces = p
				if !strings.Contains(got.Text, fmt.Sprintf("`piece: %d`", p+1)) {
					break
				}
			}
			if pieces < 2 {
				t.Fatalf("the list was not cut into pieces")
			}
			all := seen.String()
			for i := 0; i < tc.n; i++ {
				if w := fmt.Sprintf("%s %04d ", tc.prefix, i); strings.Count(all, w) != 1 {
					t.Fatalf("%q appears %d times across %d pieces", w, strings.Count(all, w), pieces)
				}
			}
		})
	}
}
