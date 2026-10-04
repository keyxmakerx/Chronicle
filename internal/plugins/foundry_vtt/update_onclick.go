package foundry_vtt

import (
	"fmt"

	"github.com/a-h/templ"
)

// Dialog openers for the owner's update screens. Both screens are swapped in
// by HTMX, so the handlers are inline IIFEs built here rather than templ
// script helpers (see onclick_handlers.go for why).

// openDialogOnClick opens the native <dialog> with the given element id.
func openDialogOnClick(id string) templ.ComponentScript {
	body := fmt.Sprintf(
		`(function(){var d=document.getElementById(%s);if(d&&d.showModal)d.showModal();})()`,
		jsStr(id))
	return inlineOnClick("fvtt_openDialog", body)
}

// closeDialogOnClick closes the dialog the clicked button sits in.
func closeDialogOnClick() templ.ComponentScript {
	return inlineOnClick("fvtt_closeDialog",
		`(function(b){var d=b.closest('dialog');if(d)d.close();})(this)`)
}
