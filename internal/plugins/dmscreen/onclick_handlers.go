package dmscreen

import (
	"github.com/a-h/templ"
)

// onclick_handlers.go holds the Adventures tab's inline handlers. The panel is
// swapped in by HTMX whenever the screen refreshes, and a script tag or a
// delegated listener may not run in a swapped fragment, so each control
// carries its own self-contained handler. The bodies are fixed text: every
// value they use (urls, ids) is read from the control's own data attributes,
// which templ escapes, so nothing user-supplied is built into JavaScript. A
// body starts with `(function(`, holds no double quote (it sits inside a
// double-quoted attribute) and never references a templ script helper.

// inlineHandler wraps a JS body in a ComponentScript with an empty Function so
// templ writes only the call into the attribute and emits no script tag.
func inlineHandler(name, body string) templ.ComponentScript {
	return templ.ComponentScript{Name: name, Function: "", Call: body}
}

// refreshScreen asks the panel to redraw itself, which keeps the open tab.
const refreshScreen = `if(window.htmx)htmx.trigger(document.body,'dms-refresh');`

// openSheetJS opens the pressed card's page over the screen. The sheet script
// owns the overlay; with it missing the card does nothing rather than leave.
const openSheetJS = `(function(b){` +
	`if(window.Chronicle&&Chronicle.dmsSheet)Chronicle.dmsSheet.open(b);` +
	`})(this)`

// removeItemJS deletes the card's board item, then redraws the screen.
const removeItemJS = `(function(b){` +
	`b.disabled=true;` +
	`Chronicle.apiFetch(b.getAttribute('data-url'),{method:'DELETE'}).then(function(r){` +
	`if(!r.ok)throw new Error('refused');` + refreshScreen +
	`}).catch(function(){b.disabled=false;Chronicle.notify('That could not be removed. Try again.','error');});` +
	`})(this)`

// addNoteJS posts the form's text as a new loose note, then redraws.
const addNoteJS = `(function(b){` +
	`var f=b.closest('[data-dms-adv-form]'),i=f.querySelector('[data-dms-adv-text]'),t=i.value.trim();` +
	`if(!t){i.focus();return;}` +
	`b.disabled=true;` +
	`Chronicle.apiFetch(b.getAttribute('data-url'),{method:'POST',body:{text:t}}).then(function(r){` +
	`if(!r.ok)throw new Error('refused');i.value='';` + refreshScreen +
	`}).catch(function(){Chronicle.notify('That note could not be added. Try again.','error');}).then(function(){b.disabled=false;});` +
	`})(this)`

// noteEnterJS sends the note when Enter is pressed in its box.
const noteEnterJS = `(function(e,i){` +
	`if(e.key==='Enter'&&!e.shiftKey&&!e.isComposing){e.preventDefault();i.closest('[data-dms-adv-form]').querySelector('button').click();}` +
	`})(event,this)`

// pickPageJS opens the pin picker under the button.
const pickPageJS = `(function(b){` +
	`if(window.Chronicle&&Chronicle.dmsSheet)Chronicle.dmsSheet.pick(b);` +
	`})(this)`

func openSheetOnClick() templ.ComponentScript  { return inlineHandler("dms_open_sheet", openSheetJS) }
func removeItemOnClick() templ.ComponentScript { return inlineHandler("dms_remove_item", removeItemJS) }
func addNoteOnClick() templ.ComponentScript    { return inlineHandler("dms_add_note", addNoteJS) }
func noteEnterOnKeyDown() templ.ComponentScript {
	return inlineHandler("dms_note_enter", noteEnterJS)
}
func pickPageOnClick() templ.ComponentScript { return inlineHandler("dms_pick_page", pickPageJS) }
