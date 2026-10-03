package packages

import (
	"strings"
	"text/template"

	"github.com/a-h/templ"
)

// sanitizeForID returns a string safe to use in HTML element IDs.
// Version strings can contain dots and dashes that some CSS selectors
// or HTMX hx-target attributes won't escape correctly. Replace dots
// with hyphens; pass-through everything else.
//
// Used by packages.templ to build per-version DOM IDs for foundry_vtt's
// "campaigns using v0.1.5" expandable target divs.
func sanitizeForID(s string) string {
	return strings.NewReplacer(".", "-", "+", "-", "/", "-").Replace(s)
}

// campaignActionURL is a per-campaign admin action on a package.
func campaignActionURL(packageID, campaignID, action string) string {
	return "/admin/packages/" + packageID + "/campaigns/" + campaignID + "/" + action
}

// moveDialogID names one campaign's "Move to a version" dialog.
func moveDialogID(campaignID string) string {
	return "move-dlg-" + sanitizeForID(campaignID)
}

// openDialogScript opens the native dialog with the given id and folds the
// menu it was clicked from. It is an inline handler rather than a templ script
// helper so it needs no companion script tag, the same rule the HTMX-swapped
// fragments follow.
func openDialogScript(id string) templ.ComponentScript {
	return templ.ComponentScript{
		Name: "pkgs_openDialog",
		Call: "(function(b){var d=document.getElementById('" + template.JSEscapeString(id) + "');" +
			"var m=b.closest('details');if(m)m.open=false;if(d&&d.showModal)d.showModal();})(this)",
	}
}

// closeDialogScript closes the dialog the clicked button sits in.
func closeDialogScript() templ.ComponentScript {
	return templ.ComponentScript{
		Name: "pkgs_closeDialog",
		Call: "(function(b){var d=b.closest('dialog');if(d)d.close();})(this)",
	}
}
