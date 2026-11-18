package campaigns

import (
	"context"
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
	return deskScript(name, "desk_blank.js", "data-desk-blank-js", "deskBlank", "activate", guard, "b,e", "this,event")
}

// deskScript builds an inline handler that loads one of the desk's scripts on
// first use and then calls Chronicle.<global>.<method>(args). The desk and its
// fragments arrive by boosted swaps and innerHTML, where a script tag may
// never run, so the handler brings its own script. Loading it from the handler
// keeps every other page free of a script only the desk needs.
func deskScript(name, file, attr, global, method, guard, params, args string) templ.ComponentScript {
	src := layouts.AssetURL("/static/js/" + file)
	return templ.ComponentScript{
		Name:     name,
		Function: "",
		Call: `(function(` + params + `){` + guard + `var C=window.Chronicle=window.Chronicle||{};` +
			`function go(){if(C.` + global + `)C.` + global + `.` + method + `(` + params + `);}` +
			`if(C.` + global + `){go();return;}` +
			`var s=document.querySelector('script[` + attr + `]');` +
			`if(!s){s=document.createElement('script');s.src=` + jsString(src) + `;s.setAttribute('` + attr + `','');document.head.appendChild(s);}` +
			`s.addEventListener('load',go);})(` + args + `)`,
	}
}

func deskFoldCall(name, method, guard string) templ.ComponentScript {
	return deskScript(name, "desk_fold.js", "data-desk-fold-js", "deskFold", method, guard, "b,e", "this,event")
}

// The fold's handlers. Each is a plain inline attribute, so they work in a
// fragment swapped in by innerHTML.
func deskFoldToggleCall() templ.ComponentScript { return deskFoldCall("deskFoldToggle", "toggle", "") }
func deskFoldRowCall() templ.ComponentScript    { return deskFoldCall("deskFoldRow", "row", "") }
func deskFoldActCall() templ.ComponentScript    { return deskFoldCall("deskFoldAct", "act", "") }
func deskFoldInviteCall() templ.ComponentScript { return deskFoldCall("deskFoldInvite", "invite", "") }

// deskFoldKey answers Escape, which closes the fold from its chevron or from
// anywhere inside it.
func deskFoldKey() templ.ComponentScript {
	return deskFoldCall("deskFoldKey", "key", `if(e.key!=='Escape')return;`)
}

// deskFoldInviteKey sends the invite on Enter, like pressing its button.
func deskFoldInviteKey() templ.ComponentScript {
	return deskFoldCall("deskFoldInviteKey", "inviteKey", `if(e.key!=='Enter')return;e.preventDefault();`)
}

// deskCarryCall is the click handler of a line's "open the page" word. It
// hands the press to static/js/desk_carry.js, which lifts the line's sentence
// and carries it to the page it opens. A plain click would navigate before
// the script has loaded, so the first press is held and handed over with
// "late" set: the script then plays the move, or goes to the link itself when
// it cannot.
func deskCarryCall() templ.ComponentScript {
	src := layouts.AssetURL("/static/js/desk_carry.js")
	return templ.ComponentScript{
		Name:     "deskCarry",
		Function: "",
		Call: `(function(a,e){var C=window.Chronicle=window.Chronicle||{};var late=0;` +
			`if(!C.deskCarry&&e.button===0&&!e.metaKey&&!e.ctrlKey&&!e.shiftKey&&!e.altKey&&!a.target){e.preventDefault();late=1;}` +
			`function go(){if(C.deskCarry)C.deskCarry.go(a,e,late);}` +
			`if(C.deskCarry){go();return;}` +
			`var s=document.querySelector('script[data-desk-carry-js]');` +
			`if(!s){s=document.createElement('script');s.src=` + jsString(src) + `;s.setAttribute('data-desk-carry-js','');document.head.appendChild(s);}` +
			`s.addEventListener('load',go);})(this,event)`,
	}
}

// deskFoldTitle is the tool a fold belongs to, for its screen-reader name and
// the chevron's label.
func deskFoldTitle(id string) string {
	switch id {
	case "people":
		return "People"
	case "trash":
		return "Trash"
	case "foundry":
		return "Foundry"
	}
	return id
}

// deskFoldURL is where a line's fold is fetched from.
func deskFoldURL(ctx context.Context, id string) string {
	return "/campaigns/" + layouts.GetCampaignID(ctx) + "/dashboard/lines/" + id + "/fold"
}

// deskMore words the rows a fold leaves out.
func deskMore(n int) string { return "and " + strconv.Itoa(n) + " more" }

// deskInviteRole is the invite's role, a dashed word that sends nothing by
// itself: the Send button reads its value.
func deskInviteRole(i *DeskInvite) *DeskBlank {
	return &DeskBlank{
		Kind: DeskBlankChoice, ID: "invite-role", Label: "Role for the invite",
		Value: "player", Shown: "player", Options: i.Roles, Local: true,
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
