package campaigns

import (
	"html"

	"github.com/a-h/templ"
)

// leave_onclick.go holds the inline handlers of My campaigns' card menus. The
// list can arrive as an HTMX fragment, where a script tag or a delegated
// listener may never run, so each control carries its own handler. They call
// into Chronicle.CampaignLeave (static/js/widgets/campaign_leave.js), guarded
// in case the script has not loaded yet.

// leaveCall builds a handler that calls one CampaignLeave method with the
// clicked element. method is fixed text from this package, never user input.
func leaveCall(method string) templ.ComponentScript {
	return templ.ComponentScript{
		Name:     "campaign_leave_" + method,
		Function: "",
		Call: html.EscapeString(
			`(function(el){if(window.Chronicle&&Chronicle.CampaignLeave)Chronicle.CampaignLeave.` + method + `(el);})(this)`),
	}
}
