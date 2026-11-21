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

// DeskPage renders the owner's Campaign settings page. The page draws Find as
// a field on the title's row, so the frame is told not to draw the small
// button in its header: two Finds would also give the "/" shortcut two
// buttons to press.
func DeskPage(cc *CampaignContext, d *Desk) templ.Component {
	page := deskPageBody(cc, d)
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return page.Render(layouts.WithManageFindField(ctx), w)
	})
}

// deskWhy is the reason drawn under a row: the row's own, or the first one a
// blank on it carries (an archived campaign's).
func deskWhy(l DeskLine) string {
	if l.Why != "" {
		return l.Why
	}
	for _, v := range l.Values {
		if v.Blank != nil && v.Blank.Off != "" {
			return v.Blank.Off
		}
	}
	return ""
}

// deskAnyOff reports whether a value on the row is off now, which is when its
// reason is shown.
func deskAnyOff(l DeskLine) bool {
	for _, v := range l.Values {
		if v.Off != "" || (v.Blank != nil && v.Blank.Off != "") {
			return true
		}
	}
	return false
}

// deskValueLabel is a value's accessible name: the row's name, then the value,
// so a screen reader hears "Campaign: Private" or "Members: 5". A warning dot
// is said in words, since the dot itself is hidden from it.
func deskValueLabel(name string, v DeskValue) string {
	if v.Text == "" && v.Sub == "" {
		return name
	}
	s := name + ": "
	if v.State == "warn" {
		s += "warning, "
	}
	s += v.Text
	if v.Sub != "" {
		if v.Joined {
			s += ","
		}
		s += " " + v.Sub
	}
	return s
}

// deskBlankLabel is a blank's accessible name. A long description is cut so a
// screen reader does not read thousands of characters as a button's name.
func deskBlankLabel(b *DeskBlank) string {
	return b.Label + ": " + deskCut(b.Shown, 80)
}

// deskMax is a text blank's length limit for the script.
func deskMax(b *DeskBlank) string { return strconv.Itoa(b.Max) }

// deskOffAttr is aria-disabled for a value that is a word, not a blank.
func deskOffAttr(v DeskValue) string { return strconv.FormatBool(v.Off != "") }

// deskOffGuard stops the view-as form from posting while its word is off. The
// word stays a real, focusable submit button so a screen reader still finds
// it and reads why; only the press is refused. Inline, like the blanks, so it
// works after a boosted swap.
func deskOffGuard() templ.ComponentScript {
	return templ.ComponentScript{
		Name:     "deskOffGuard",
		Function: "",
		Call:     `(function(b,e){if(b.getAttribute('aria-disabled')==='true'){e.preventDefault();}})(this,event)`,
	}
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

// deskHasPopup is aria-haspopup for a blank: a choice opens a menu, a name or
// a description opens a field in place.
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
