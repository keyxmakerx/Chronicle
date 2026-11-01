// Package email builds the HTML and plain-text bodies of Chronicle's
// campaign emails (game night invites, reminders, date polls, availability
// asks, campaign invites) from one Message, so every email shares a layout,
// escapes what people typed, and carries a plain-text twin. Sending stays
// with the smtp plugin; this package only writes bodies.
//
// A layout is its own template with its own type, colours and button shape,
// not one frame with a swapped header. Note is the layout every campaign
// starts with.
package email

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/colour"
)

// Layout names one email template. The value is stored per campaign, so
// renaming one loses owners' choices.
type Layout string

const (
	// Note is plain text in a narrow column, with the answers as links.
	Note Layout = "note"
)

// DefaultLayout is what a campaign uses until its owner picks another.
const DefaultLayout = Note

// DefaultAccent is the link colour when a campaign has none, or one too
// light to read on white.
const DefaultAccent = "#4f46e5"

// ParseLayout returns the named layout, or the default for anything unknown,
// so a stored value from a newer or older version never breaks a send.
func ParseLayout(s string) Layout {
	switch Layout(strings.TrimSpace(s)) {
	case Note:
		return Note
	}
	return DefaultLayout
}

// Answer is one link the reader can press: a one-press answer such as
// Going, or a quieter link such as "Suggest another time".
type Answer struct {
	Label string
	URL   string
}

// Group is a titled set of answers, such as one time in a date poll with
// its own Yes, Maybe and No.
type Group struct {
	Title   string
	Detail  string
	Answers []Answer
}

// Message is everything an email says. Every string is plain text; the
// builder escapes it. URLs must be absolute http(s) links Chronicle made.
type Message struct {
	// Campaign is the campaign's name, shown above the title.
	Campaign string
	// Accent is the campaign's colour as #RRGGBB. Anything else, or a colour
	// too light to read as a link on white, falls back to DefaultAccent.
	Accent string
	// Title is the email's heading, usually the game night's name.
	Title string
	// Lines are the paragraphs under the title.
	Lines []string
	// Answers are the one-press answers, in order.
	Answers []Answer
	// Groups are titled sets of answers, for an email that asks several
	// things at once.
	Groups []Group
	// Links are quieter links shown after the answers.
	Links []Answer
	// Small is small print such as when the links expire.
	Small string
	// Why says why this person got the email. Empty leaves the footer out.
	Why string
	// PrefsURL opens the notification choices. Empty leaves the link out,
	// as on safety mail that can't be switched off.
	PrefsURL string
}

// Render returns the plain-text and HTML bodies of m in the given layout.
func Render(layout Layout, m Message) (plain, html string, err error) {
	m.Accent = linkAccent(m.Accent)
	var buf bytes.Buffer
	if err := templates[ParseLayout(string(layout))].Execute(&buf, m); err != nil {
		return "", "", fmt.Errorf("rendering %s email: %w", layout, err)
	}
	return Plain(m), buf.String(), nil
}

// linkAccent keeps a campaign colour only when it is valid and reads on
// white at the WCAG AA ratio for text.
func linkAccent(hex string) string {
	if !colour.ValidHex(hex) || colour.Contrast(hex, "#ffffff") < 4.5 {
		return DefaultAccent
	}
	return colour.Lower(hex)
}

// Plain is the plain-text twin: the same words in the same order, with each
// link written out after its label.
func Plain(m Message) string {
	var b strings.Builder
	para := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			b.WriteString(s)
			b.WriteString("\n\n")
		}
	}
	answers := func(as []Answer, indent string) {
		for _, a := range as {
			fmt.Fprintf(&b, "%s%s: %s\n", indent, a.Label, a.URL)
		}
		if len(as) > 0 {
			b.WriteString("\n")
		}
	}
	para(m.Campaign)
	para(m.Title)
	for _, l := range m.Lines {
		para(l)
	}
	for _, g := range m.Groups {
		b.WriteString(strings.TrimSpace(g.Title))
		b.WriteString("\n")
		if d := strings.TrimSpace(g.Detail); d != "" {
			b.WriteString(d)
			b.WriteString("\n")
		}
		answers(g.Answers, "  ")
	}
	answers(m.Answers, "")
	answers(m.Links, "")
	para(m.Small)
	switch {
	case m.Why != "" && m.PrefsURL != "":
		fmt.Fprintf(&b, "%s Change what reaches you: %s\n", m.Why, m.PrefsURL)
	case m.Why != "":
		b.WriteString(m.Why + "\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

var templates = map[Layout]*template.Template{
	Note: template.Must(template.New("note").Parse(noteHTML)),
}
