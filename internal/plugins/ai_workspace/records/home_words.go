package records

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// markupTag matches a tag opening or closing, the same test the home's
// written blocks use to tell typed words from stored markup.
var markupTag = regexp.MustCompile(`<[a-zA-Z/!]`)

// writtenMarkdown is a Table agreement or Making a hero block's stored words as
// markdown an AI can edit and send back. Words the owner typed (plain text)
// come back as typed; stored markup keeps its headings, paragraphs, lists,
// bold, italic and links, and loses any other tag, so sending it back never
// flattens a heading into a run-on line.
func writtenMarkdown(content string) string {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
	if !markupTag.MatchString(content) {
		return content
	}
	var w mdWriter
	z := html.NewTokenizer(strings.NewReader(content))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.TextToken:
			w.text(string(z.Text()))
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			var href string
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				if string(k) == "href" {
					href = string(v)
				}
			}
			w.open(string(name), href)
		case html.EndTagToken:
			name, _ := z.TagName()
			w.close(string(name))
		}
	}
	return strings.TrimSpace(w.out.String())
}

// mdWriter turns the tokens of a small HTML subset into markdown.
type mdWriter struct {
	out strings.Builder
	// lists is the open lists, innermost last: "ul", or "ol" with its counter.
	lists []*mdList
	links []string
	// gap is whether the source had whitespace after the last words.
	gap bool
}

type mdList struct {
	ordered bool
	n       int
}

// block starts a new block, one blank line after the last.
func (w *mdWriter) block() {
	if w.out.Len() == 0 {
		return
	}
	s := w.out.String()
	switch {
	case strings.HasSuffix(s, "\n\n"):
	case strings.HasSuffix(s, "\n"):
		w.out.WriteString("\n")
	default:
		w.out.WriteString("\n\n")
	}
}

// line starts a new line without a blank one, as list items do.
func (w *mdWriter) line() {
	if w.out.Len() > 0 && !strings.HasSuffix(w.out.String(), "\n") {
		w.out.WriteString("\n")
	}
}

// flushGap writes the one space the source had between words, unless a line
// has just begun.
func (w *mdWriter) flushGap() {
	cur := w.out.String()
	if w.gap && cur != "" && !strings.HasSuffix(cur, "\n") && !strings.HasSuffix(cur, " ") {
		w.out.WriteString(" ")
	}
	w.gap = false
}

func (w *mdWriter) text(s string) {
	words := strings.Fields(s)
	if len(words) == 0 {
		// Whitespace between two tags is a word gap only when words sit on
		// both sides of it, which the next words decide.
		w.gap = w.gap || s != ""
		return
	}
	if r, _ := utf8.DecodeRuneInString(s); unicode.IsSpace(r) {
		w.gap = true
	}
	w.flushGap()
	w.out.WriteString(strings.Join(words, " "))
	r, _ := utf8.DecodeLastRuneInString(s)
	w.gap = unicode.IsSpace(r)
}

func (w *mdWriter) open(tag, href string) {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		w.block()
		w.out.WriteString(strings.Repeat("#", int(tag[1]-'0')) + " ")
	case "p", "blockquote", "div":
		w.block()
		if tag == "blockquote" {
			w.out.WriteString("> ")
		}
	case "ul", "ol":
		if len(w.lists) > 0 {
			w.line() // a nested list continues its item
		} else {
			w.block()
		}
		w.lists = append(w.lists, &mdList{ordered: tag == "ol"})
	case "li":
		w.line()
		indent := ""
		if len(w.lists) > 1 {
			indent = strings.Repeat("  ", len(w.lists)-1)
		}
		marker := "- "
		if n := len(w.lists); n > 0 && w.lists[n-1].ordered {
			w.lists[n-1].n++
			marker = strconv.Itoa(w.lists[n-1].n) + ". "
		}
		w.out.WriteString(indent + marker)
	case "br":
		w.out.WriteString("  \n")
	case "strong", "b":
		w.flushGap()
		w.out.WriteString("**")
	case "em", "i":
		w.flushGap()
		w.out.WriteString("*")
	case "a":
		w.flushGap()
		w.links = append(w.links, href)
		w.out.WriteString("[")
	}
}

func (w *mdWriter) close(tag string) {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6", "p", "blockquote", "div":
		w.block()
	case "ul", "ol":
		if n := len(w.lists); n > 0 {
			w.lists = w.lists[:n-1]
		}
		if len(w.lists) == 0 {
			w.block()
		}
	case "li":
		w.line()
	case "strong", "b":
		w.out.WriteString("**")
	case "em", "i":
		w.out.WriteString("*")
	case "a":
		n := len(w.links)
		if n == 0 {
			return
		}
		href := w.links[n-1]
		w.links = w.links[:n-1]
		if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "/") {
			w.out.WriteString("](" + href + ")")
		} else {
			w.out.WriteString("]") // an unsafe or empty href is not carried
		}
	}
}
