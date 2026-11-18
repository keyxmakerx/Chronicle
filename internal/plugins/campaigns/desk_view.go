package campaigns

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// deskNodeKind is how the template draws one piece of a sentence.
type deskNodeKind int

const (
	deskNodeText deskNodeKind = iota
	deskNodeBold
	deskNodeGM
	deskNodeSwatch
	deskNodeAfter
	deskNodeBlank
)

// deskNodeView is a DeskPart ready to draw: a blank carries the punctuation
// that follows it (Glue) so the two can be kept on one line.
type deskNodeView struct {
	Kind  deskNodeKind
	Text  string
	Blank *DeskBlank
	Glue  string
}

// deskPunct is what may not start a line after a blank.
const deskPunct = ".,:;!?"

// deskNodes turns a line's parts into what the template draws, moving the
// punctuation that directly follows a blank into the blank's own no-wrap
// span. A blank is plain inline text for wrapping, so without this a full
// stop could be left alone at the start of the next line.
func deskNodes(parts []DeskPart) []deskNodeView {
	var out []deskNodeView
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		switch {
		case p.Blank != nil:
			n := deskNodeView{Kind: deskNodeBlank, Blank: p.Blank}
			if i+1 < len(parts) && parts[i+1].Blank == nil && !parts[i+1].Bold && !parts[i+1].GM {
				next := parts[i+1].Text
				rest := strings.TrimLeft(next, deskPunct)
				n.Glue = next[:len(next)-len(rest)]
				if rest == "" {
					i++
				} else if n.Glue != "" {
					parts = append([]DeskPart(nil), parts...)
					parts[i+1].Text = rest
				}
			}
			out = append(out, n)
		case p.GM:
			out = append(out, deskNodeView{Kind: deskNodeGM})
		case p.Swatch:
			out = append(out, deskNodeView{Kind: deskNodeSwatch})
		case p.After:
			out = append(out, deskNodeView{Kind: deskNodeAfter, Text: p.Text})
		case p.Bold:
			out = append(out, deskNodeView{Kind: deskNodeBold, Text: p.Text})
		default:
			out = append(out, deskNodeView{Kind: deskNodeText, Text: p.Text})
		}
	}
	return out
}

// deskOptionsJSON is a blank's menu as the script reads it.
func deskOptionsJSON(o []DeskOption) string {
	if len(o) == 0 {
		return "[]"
	}
	b, err := json.Marshal(o)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// deskHasPopup is aria-haspopup for a blank: a choice opens a menu, a name
// opens a field in place.
func deskHasPopup(b *DeskBlank) string {
	if b.Kind == DeskBlankChoice {
		return "menu"
	}
	return "false"
}

// deskBlankCall is the blank's click handler. It hands the word and the event
// to static/js/desk_blank.js, loading that file on first use. Loading it from
// the handler, not from a script tag or the layout, means the desk works after
// a boosted swap (where a tag may never run) and every other page stays free
// of a script only this one needs.
func deskBlankCall() templ.ComponentScript { return deskBlankScript("deskBlank", "") }

// deskBlankKey is the same handler for the keyboard. A blank is a span with a
// button role, not a button: a real button is always an atomic box and cannot
// wrap across lines like the words around it. A span has no key behaviour of
// its own, so Enter and Space are answered here.
func deskBlankKey() templ.ComponentScript {
	return deskBlankScript("deskBlankKey", `if(e.key!=='Enter'&&e.key!==' ')return;e.preventDefault();`)
}

func deskBlankScript(name, guard string) templ.ComponentScript {
	src := layouts.AssetURL("/static/js/desk_blank.js")
	return templ.ComponentScript{
		Name:     name,
		Function: "",
		Call: `(function(b,e){` + guard + `var C=window.Chronicle=window.Chronicle||{};` +
			`function go(){if(C.deskBlank)C.deskBlank.activate(b,e);}` +
			`if(C.deskBlank){go();return;}` +
			`var s=document.querySelector('script[data-desk-blank-js]');` +
			`if(!s){s=document.createElement('script');s.src=` + jsString(src) + `;s.setAttribute('data-desk-blank-js','');document.head.appendChild(s);}` +
			`s.addEventListener('load',go);})(this,event)`,
	}
}

// deskTabindex takes an off blank out of the tab order.
func deskTabindex(b *DeskBlank) string {
	if b.Off != "" {
		return "-1"
	}
	return "0"
}

// jsString quotes s as a single-quoted JavaScript string. Asset URLs hold only
// path characters, but the quote and backslash are escaped anyway so a future
// URL cannot break out of the handler.
func jsString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", ``, "\r", ``, `<`, `\x3c`)
	return "'" + r.Replace(s) + "'"
}

// deskDisabled is the aria-disabled value for a blank.
func deskDisabled(b *DeskBlank) string { return strconv.FormatBool(b.Off != "") }

// deskJoin keeps the punctuation that follows a blank on the blank's line. A
// word joiner forbids a break between the blank's last letter and the full
// stop, while the blank itself still wraps like the words around it.
func deskJoin(glue string) string {
	if glue == "" {
		return ""
	}
	return "\u2060" + glue
}
