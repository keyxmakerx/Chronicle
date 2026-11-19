package campaigns

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// DeskPage renders the owner's Campaign settings page. The page draws its own
// full-width Find field, so the frame is told not to draw the small one in its
// header: two Finds would also give the "/" shortcut two buttons to press.
func DeskPage(cc *CampaignContext, d *Desk) templ.Component {
	page := deskPageBody(cc, d)
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return page.Render(layouts.WithManageFindField(ctx), w)
	})
}

// deskNodeKind is how the template draws one piece of a sentence.
type deskNodeKind int

const (
	deskNodeText deskNodeKind = iota
	deskNodeBold
	deskNodeGM
	deskNodeSwatch
)

// deskNodeView is a DeskPart ready to draw.
type deskNodeView struct {
	Kind deskNodeKind
	Text string
}

// deskNodes turns a sentence's parts into what the template draws.
func deskNodes(parts []DeskPart) []deskNodeView {
	out := make([]deskNodeView, 0, len(parts))
	for _, p := range parts {
		switch {
		case p.GM:
			out = append(out, deskNodeView{Kind: deskNodeGM})
		case p.Swatch:
			out = append(out, deskNodeView{Kind: deskNodeSwatch})
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
func deskFoldActCall() templ.ComponentScript    { return deskFoldCall("deskFoldAct", "act", "") }
func deskFoldInviteCall() templ.ComponentScript { return deskFoldCall("deskFoldInvite", "invite", "") }

// deskFoldInviteKey sends the invite on Enter, like pressing its button.
func deskFoldInviteKey() templ.ComponentScript {
	return deskFoldCall("deskFoldInviteKey", "inviteKey", `if(e.key!=='Enter')return;e.preventDefault();`)
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
