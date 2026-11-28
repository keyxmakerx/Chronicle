package cutscenes

import (
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/a-h/templ"
)

// What the Director's pages draw. The pages are server rendered and every
// write is a plain form post (HTMX where it helps); the small script is only
// for Preview, which hands the checked scene to the engine.

type listPage struct {
	CampaignID string
	Scenes     []Scene
	Recent     []ShowingReport
	Now        time.Time
}

type editorPage struct {
	CampaignID string
	Scene      Scene
	New        bool
	Live       []Showing
	// Moments are the live showings holding on a moment, for the panel.
	Moments []LiveMoment
	Saved   bool
	Error   string
	// Foundry offers Preview in Foundry: the campaign uses the module.
	Foundry bool
}

func (p editorPage) base() string { return "/campaigns/" + p.CampaignID + "/cutscenes" }

// action is where the form posts: create for a new scene, else save.
func (p editorPage) action() string {
	if p.New {
		return p.base()
	}
	return p.base() + "/" + p.Scene.ID
}

func (p editorPage) title() string {
	if p.New {
		return "New scene"
	}
	return p.Scene.Name
}

type showSheet struct {
	CampaignID string
	Scene      Scene
	Rows       []AudienceRow
	Rewatch    string
	Picked     map[string]bool
	Error      string
}

func (s showSheet) rewatch() string {
	if s.Rewatch == "" {
		return DefaultRewatch
	}
	return s.Rewatch
}

// everyoneHere says whether "Everyone here" ticks a row: a player who has
// Chronicle or Foundry open. Directors, the asker among them, are left out:
// their screens are where the table is run from, and a Director's Foundry
// screen is the GM's. A Director still ticks themselves by hand to see it.
func everyoneHere(r AudienceRow) bool {
	return r.Place.Here() && !r.Director && !r.You
}

// anyHere is true when "Everyone here" has someone to pick.
func (s showSheet) anyHere() bool {
	for _, r := range s.Rows {
		if everyoneHere(r) {
			return true
		}
	}
	return false
}

// anyoneElse is true when the sheet lists someone besides the asker.
func (s showSheet) anyoneElse() bool {
	for _, r := range s.Rows {
		if !r.You {
			return true
		}
	}
	return false
}

// Element ids, one set per scene so the list page can hold a sheet and a live
// list under every row.
func sheetID(sceneID string) string { return "csw-sheet-" + sceneID }
func liveID(sceneID string) string  { return "csw-live-" + sceneID }

// recentRowID is a showing's row on the "Shown lately" list, apart from its
// row on a scene's live list.
func recentRowID(showingID string) string { return showingRowID(showingID) + "-recent" }
func showBtnID(sceneID string) string {
	return "csw-showbtn-" + sceneID
}
func showingRowID(showingID string) string { return "csw-showing-" + showingID }

// EngineSrc is where the cut scene engine is loaded from; the workshop script
// loads it the first time Preview is pressed.
const EngineSrc = "/static/js/cutscene_engine.js"

// ---- inline handlers ------------------------------------------------------
//
// Every control inside a fragment HTMX swaps in carries its own inline IIFE,
// never a templ script helper: a helper's <script> sibling is not reliably run
// on an innerHTML swap, so the button would call a function that does not
// exist. The bodies use single-quoted JS strings only and escape what they
// interpolate, because templ writes them into a double-quoted attribute.

func inline(name, body string) templ.ComponentScript {
	return templ.ComponentScript{Name: name, Call: body}
}

func jsStr(s string) string { return "'" + template.JSEscapeString(s) + "'" }

// everyoneHereOnClick ticks everyone the sheet marks as here; it leaves
// anyone already ticked as they are.
func everyoneHereOnClick() templ.ComponentScript {
	return inline("cswEveryoneHere", `(function(b){var f=b.closest('form');if(!f){return;}f.querySelectorAll('input[name=to][data-here=y]').forEach(function(i){i.checked=true;});})(this)`)
}

// closeSheetBody empties the sheet's host and gives focus back to the button
// that opened it.
func closeSheetBody(sceneID string) string {
	return fmt.Sprintf(`var h=document.getElementById(%s);if(h){h.innerHTML='';}var o=document.getElementById(%s);if(o){o.focus();}`,
		jsStr(sheetID(sceneID)), jsStr(showBtnID(sceneID)))
}

func closeSheetOnClick(sceneID string) templ.ComponentScript {
	return inline("cswCloseSheet", `(function(){`+closeSheetBody(sceneID)+`})()`)
}

// sheetKeyDown closes the sheet on Escape, as every open thing does.
func sheetKeyDown(sceneID string) templ.ComponentScript {
	return inline("cswSheetKey", `(function(e){if(e.key==='Escape'){e.stopPropagation();`+closeSheetBody(sceneID)+`}})(event)`)
}

// addRowOnClick copies the list's spare row from its template, up to max
// rows, and moves focus into the new row.
func addRowOnClick(listID, tplID string, max int) templ.ComponentScript {
	return inline("cswAddRow", fmt.Sprintf(`(function(){var l=document.getElementById(%s),t=document.getElementById(%s);if(!l||!t){return;}if(l.querySelectorAll('[data-csw-row]').length>=%d){return;}l.appendChild(t.content.cloneNode(true));var r=l.lastElementChild;var i=r&&r.querySelector('select,input');if(i){i.focus();}})()`,
		jsStr(listID), jsStr(tplID), max))
}

func removeRowOnClick() templ.ComponentScript {
	return inline("cswRemoveRow", `(function(b){var r=b.closest('[data-csw-row]');if(r){r.remove();}})(this)`)
}

// previewOnClick hands the form to the workshop script, which asks the
// server for the checked scene and plays it with the engine.
func previewOnClick() templ.ComponentScript {
	return inline("cswPreview", `(function(b){var w=window.ChronicleCutsceneWorkshop;if(w){w.preview(b);}})(this)`)
}

// foundryPreviewOnClick hands the form to the workshop script, which asks
// the server to play the checked scene on the Director's Foundry screen.
func foundryPreviewOnClick() templ.ComponentScript {
	return inline("cswFoundryPreview", `(function(b){var w=window.ChronicleCutsceneWorkshop;if(w){w.previewInFoundry(b);}})(this)`)
}

// endingAutoOnChange shows the wait field only for "By itself".
func endingOnChange() templ.ComponentScript {
	return inline("cswEnding", `(function(b){var f=b.closest('form');if(!f){return;}var a=f.querySelector('[data-csw-after]');var on=f.querySelector('input[name=end_kind][value=auto]');if(a&&on){a.hidden=!on.checked;}})(this)`)
}

// bgKindOnChange shows the background's Look picker only for the kind that
// has looks. The hidden picker still posts; the server drops a look on a kind
// without looks, so a hidden choice never reaches the scene.
// The background is two pickers, what it is and how it arrives: the arrival
// and its time only mean something on black.
func bgKindOnChange() templ.ComponentScript {
	return inline("cswBgKind", `(function(s){var f=s.closest('form');if(!f){return;}var k=f.querySelector('[name=bg_kind]'),a=f.querySelector('[name=bg_in]');if(!k||!a){return;}var black=k.value!==`+jsStr(BackgroundNone)+`;f.querySelectorAll('[data-csw-bg-in]').forEach(function(e){e.hidden=!black;});var l=f.querySelector('[data-csw-bg-look]');if(l){l.hidden=!black||a.value!==`+jsStr(ArriveFallAsleep)+`;}var t=f.querySelector('[data-csw-bg-sec]');if(t){t.hidden=!black||a.value===`+jsStr(ArriveCut)+`;}})(this)`)
}

// ovKindOnChange does the same for one layer on top: only an eye has looks
// and a pupil.
func ovKindOnChange() templ.ComponentScript {
	return inline("cswOvKind", `(function(s){var r=s.closest('[data-csw-row]');if(!r){return;}r.querySelectorAll('[data-csw-ov-look],[data-csw-ov-pupil]').forEach(function(l){l.hidden=s.value!==`+jsStr(OverlayEye)+`;});var c=r.querySelector('[data-csw-ov-color]');if(c){c.hidden=s.value===`+jsStr(OverlayLetterbox)+`;}})(this)`)
}

// wayChoices are the editor's ways to answer a moment, as the Director
// reads them: the value is the kinds it stores, space-separated.
func wayChoices() []Kind {
	return []Kind{
		{Key: WayWrite, Name: "They write it"},
		{Key: WayDirector, Name: "You answer it"},
		{Key: WayWrite + " " + WayDirector, Name: "They write it, or leave it to you"},
	}
}

// waysValue is a moment's ways as the editor's select value.
func waysValue(ws []Way) string {
	ks := make([]string, 0, len(ws))
	for _, w := range ws {
		ks = append(ks, w.Kind)
	}
	if len(ks) == 0 {
		return WayWrite
	}
	return strings.Join(ks, " ")
}

func afterValue(e Ending) string {
	if e.After != nil {
		return num(*e.After)
	}
	return num(DefaultAfter)
}

func colorOr(c, def string) string {
	if colorPattern.MatchString(c) {
		return c
	}
	return def
}

// viewersOf is a showing's people for the live row.
func viewersOf(s Showing) string { return names(s.Viewers) }

// momentsID is the scene's live moments panel; pagesListID the page names a
// showing's answers can join.
func momentsID(sceneID string) string     { return "csw-moments-" + sceneID }
func pagesListID(showingID string) string { return "csw-pages-" + showingID }

// pageChoice is one entry of the page picker: what picking it fills in, and
// what the list shows.
type pageChoice struct{ Value, Label string }

// pageChoices offers each page by its name, or by its id where two pages
// share a name, so every page can be picked; the service takes either.
func pageChoices(pages []PageRef) []pageChoice {
	seen := map[string]int{}
	for _, p := range pages {
		seen[strings.ToLower(strings.TrimSpace(p.Name))]++
	}
	out := make([]pageChoice, 0, len(pages))
	for _, p := range pages {
		if seen[strings.ToLower(strings.TrimSpace(p.Name))] > 1 {
			out = append(out, pageChoice{Value: p.ID, Label: p.Name + " (one of several pages with this name)"})
			continue
		}
		out = append(out, pageChoice{Value: p.Name, Label: p.Name})
	}
	return out
}

// answerBy says who answered and how, in the panel.
func answerBy(a Answer) string {
	if a.Way == WayDirector {
		return a.Name + " left it to you"
	}
	return a.Name + " wrote"
}

// Clock is a time on the scene's clock as the panel says it: 0:12, 1:05.
func Clock(secs float64) string {
	s := int(secs + 0.5)
	return strconv.Itoa(s/60) + ":" + fmt.Sprintf("%02d", s%60)
}
