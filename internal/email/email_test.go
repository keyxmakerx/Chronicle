package email

import (
	"strings"
	"testing"
)

func sample() Message {
	return Message{
		Campaign: "Therin",
		Accent:   "#2563eb",
		Title:    "Session 15: The Ferryman's Toll",
		Lines:    []string{"Friday 7 November, 7pm to 10pm, your time. Can you make it?"},
		Answers: []Answer{
			{Label: "Going", URL: "https://example.test/rsvp/aaa"},
			{Label: "Maybe", URL: "https://example.test/rsvp/bbb"},
			{Label: "Can't make it", URL: "https://example.test/rsvp/ccc"},
		},
		Links:    []Answer{{Label: "Suggest another time", URL: "https://example.test/rsvp/ddd/suggest"}},
		Small:    "These links expire in 7 days.",
		Why:      `You get this because "A game night is planned" is on for email.`,
		PrefsURL: "https://example.test/account#notifications",
	}
}

func TestRender_NoteCarriesEveryPartInBothBodies(t *testing.T) {
	plain, html, err := Render(Note, sample())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Therin", "Session 15: The Ferryman&#39;s Toll", "Can you make it?",
		`href="https://example.test/rsvp/aaa"`, ">Going<", ">Maybe<", ">Can&#39;t make it<",
		">Suggest another time<", "These links expire in 7 days.",
		`href="https://example.test/account#notifications"`, "Change what reaches you",
		"color:#2563eb", `class="n-ans"`, "max-width:600px", "min-height:46px",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML is missing %q", want)
		}
	}
	for _, want := range []string{
		"Therin\n\nSession 15: The Ferryman's Toll\n\n",
		"Going: https://example.test/rsvp/aaa\n",
		"Maybe: https://example.test/rsvp/bbb\n",
		"Can't make it: https://example.test/rsvp/ccc\n",
		"Suggest another time: https://example.test/rsvp/ddd/suggest\n",
		"These links expire in 7 days.",
		"Change what reaches you: https://example.test/account#notifications\n",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("plain text is missing %q\n%s", want, plain)
		}
	}
	if strings.ContainsAny(plain, "<>") {
		t.Errorf("plain text carries markup:\n%s", plain)
	}
}

func TestRender_EscapesWhatPeopleTyped(t *testing.T) {
	m := sample()
	m.Campaign = `<img src=x onerror=alert(1)>`
	m.Title = `</h1><script>alert(1)</script>`
	m.Lines = []string{`<b>bold</b>`}
	m.Groups = []Group{{Title: `<i>t</i>`, Detail: `"quoted"`, Answers: []Answer{{Label: `<u>Yes</u>`, URL: "https://example.test/y"}}}}
	_, html, err := Render(Note, m)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<img", "<script", "<b>bold", "<i>t", "<u>Yes"} {
		if strings.Contains(html, bad) {
			t.Errorf("HTML carries unescaped %q", bad)
		}
	}
}

func TestRender_RefusesNonWebLinks(t *testing.T) {
	m := sample()
	m.Answers = []Answer{{Label: "Going", URL: "javascript:alert(1)"}}
	_, html, err := Render(Note, m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "javascript:") {
		t.Error("a javascript: link reached the HTML")
	}
}

func TestLinkAccent(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"#2563EB", "#2563eb"},
		{"", DefaultAccent},
		{"red", DefaultAccent},
		{"#fde68a", DefaultAccent}, // too light to read as a link on white
		{"#123", DefaultAccent},
		{"#2563eb;background:url(x)", DefaultAccent},
	} {
		if got := linkAccent(tc.in); got != tc.want {
			t.Errorf("linkAccent(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPlain_LeavesOutEmptyParts(t *testing.T) {
	plain := Plain(Message{Title: "You're invited", Answers: []Answer{{Label: "Accept", URL: "https://example.test/a"}}})
	if plain != "You're invited\n\nAccept: https://example.test/a\n" {
		t.Errorf("got %q", plain)
	}
	plain = Plain(Message{Title: "T", Why: "Because."})
	if !strings.HasSuffix(plain, "Because.\n") || strings.Contains(plain, "Change what reaches you") {
		t.Errorf("a footer with no preferences link: %q", plain)
	}
}

func TestRender_GroupsKeepTheirOwnAnswers(t *testing.T) {
	m := Message{Title: "When can you play?", Groups: []Group{
		{Title: "Friday 7 November", Detail: "7pm to 10pm", Answers: []Answer{{"Yes", "https://e.test/1y"}, {"No", "https://e.test/1n"}}},
		{Title: "Saturday 8 November", Answers: []Answer{{"Yes", "https://e.test/2y"}}},
	}}
	plain, html, err := Render(Note, m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain, "Friday 7 November\n7pm to 10pm\n  Yes: https://e.test/1y\n  No: https://e.test/1n\n\nSaturday 8 November\n  Yes: https://e.test/2y\n") {
		t.Errorf("plain groups:\n%s", plain)
	}
	if strings.Index(html, "https://e.test/1n") > strings.Index(html, "Saturday 8 November") {
		t.Error("a group's answers came after the next group")
	}
}

func TestParseLayout_UnknownFallsBackToNote(t *testing.T) {
	for _, s := range []string{"", "note", " note ", "poster-v9", "<x>"} {
		if got := ParseLayout(s); got != Note {
			t.Errorf("ParseLayout(%q) = %q", s, got)
		}
	}
}

func TestRender_NoEmojiAndNoOldWording(t *testing.T) {
	plain, html, _ := Render(Note, sample())
	for _, body := range []string{plain, html} {
		for _, bad := range []string{"🎲", "📅", "✓", "✗", "Session Invite"} {
			if strings.Contains(body, bad) {
				t.Errorf("body carries %q", bad)
			}
		}
	}
}

func TestRender_EveryLayoutCarriesEveryPart(t *testing.T) {
	m := sample()
	m.Stub = &Stub{Weekday: "FRI", Day: "7", Month: "NOV", Time: "7pm"}
	m.Sender = &Sender{Name: "Kara", Role: "your DM"}
	m.Groups = []Group{{Title: "Saturday 8 November", Detail: "7pm to 10pm", Answers: []Answer{{"Yes", "https://example.test/g/y"}}}}
	for _, l := range Layouts {
		t.Run(string(l), func(t *testing.T) {
			plain, html, err := Render(l, m)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{
				"Therin", "Session 15: The Ferryman&#39;s Toll", "Can you make it?",
				`href="https://example.test/rsvp/aaa"`, ">Maybe<", ">Can&#39;t make it<", ">Suggest another time<",
				"Saturday 8 November", `href="https://example.test/g/y"`,
				"These links expire in 7 days.", `href="https://example.test/account#notifications"`, "Change what reaches you",
				"#2563eb", "max-width:600px",
			} {
				if !strings.Contains(html, want) {
					t.Errorf("HTML is missing %q", want)
				}
			}
			if plain != Plain(m) {
				t.Error("the plain-text twin depends on the layout")
			}
		})
	}
}

func TestRender_EveryLayoutEscapesAndRefusesNonWebLinks(t *testing.T) {
	m := sample()
	m.Campaign = `<img src=x onerror=alert(1)>`
	m.Title = `</h1><script>alert(1)</script>`
	m.Answers = []Answer{{Label: `<u>Going</u>`, URL: "javascript:alert(1)"}}
	m.Stub = &Stub{Weekday: `<b>`, Day: "7", Month: "NOV"}
	m.Sender = &Sender{Name: `<i>Kara</i>`, Role: `"DM"`}
	for _, l := range Layouts {
		_, html, err := Render(l, m)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"<img", "<script", "<u>Going", "<b>", "<i>Kara", "javascript:"} {
			if strings.Contains(html, bad) {
				t.Errorf("%s: HTML carries %q", l, bad)
			}
		}
	}
}

func TestRender_TicketStubAndChatSender(t *testing.T) {
	m := sample()
	_, html, _ := Render(Ticket, m)
	if strings.Contains(html, "t-day\" style") {
		t.Error("a Ticket with no date drew a stub")
	}
	m.Stub = &Stub{Weekday: "FRI", Day: "7", Month: "NOV", Time: "7pm"}
	_, html, _ = Render(Ticket, m)
	for _, want := range []string{">FRI<", ">7<", ">NOV<", ">7pm<", "background:#2563eb"} {
		if !strings.Contains(html, want) {
			t.Errorf("Ticket stub is missing %q", want)
		}
	}

	_, html, _ = Render(Chat, m)
	if !strings.Contains(html, ">T<") || !strings.Contains(html, "<b style=\"color:#111111;font-size:14px\">Therin</b>") {
		t.Errorf("a Chat with no sender should read as from the campaign:\n%s", html)
	}
	m.Sender = &Sender{Name: "kara", Role: "your DM"}
	_, html, _ = Render(Chat, m)
	if !strings.Contains(html, ">K<") || !strings.Contains(html, "kara, your DM</b> in Therin") {
		t.Errorf("Chat sender line:\n%s", html)
	}
}

func TestParseLayout_KnowsEveryLayout(t *testing.T) {
	for _, l := range Layouts {
		if got := ParseLayout(" " + string(l)); got != l {
			t.Errorf("ParseLayout(%q) = %q", l, got)
		}
	}
}
