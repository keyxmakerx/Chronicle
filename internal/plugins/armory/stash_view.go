// stash_view.go holds the small presentation helpers the stash templates use:
// plain-English history lines, relative times and URLs. They live in Go, not
// in the templates, so they can be tested and so no template needs a script.
package armory

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/a-h/templ"
)

// thingText names what moved: "2 × Healing Potion" or "12 money".
func thingText(l MoveLine) string {
	if l.Kind == MoveKindMoney {
		return l.Amount.String() + " money"
	}
	return fmt.Sprintf("%d × %s", l.Quantity, l.ItemName)
}

// MoveSummary renders one history row as a sentence a player can read.
func MoveSummary(l MoveLine) string {
	if l.Summary != "" {
		return l.Summary
	}
	// A money row from a character to itself is not a move: it records an edit
	// of the sheet, and its stored reason already reads as a sentence.
	if l.IsMoneyEdit() && l.Reason != "" {
		return l.Reason
	}
	if l.IsGive() {
		return giveSummary(l, true, false, "", l.RequesterName)
	}
	route := fmt.Sprintf("%s from %s to %s", thingText(l), l.FromName, l.ToName)
	switch l.Status {
	case MovePending:
		return fmt.Sprintf("%s asked to move %s. Waiting for the GM.", l.RequesterName, route)
	case MoveDeclined:
		return fmt.Sprintf("The GM turned down %s's request to move %s.", l.RequesterName, route)
	case MoveFailed:
		return fmt.Sprintf("%s's move of %s didn't go through. %s", l.RequesterName, route, l.Reason)
	default:
		switch {
		case l.Reason != "":
			return fmt.Sprintf("%s moved %s. %s", l.RequesterName, route, l.Reason)
		case l.DecidedBy != "" && l.DecidedBy != l.RequestedBy:
			return fmt.Sprintf("The GM approved %s's request to move %s.", l.RequesterName, route)
		}
		return fmt.Sprintf("%s moved %s.", l.RequesterName, route)
	}
}

// agoText renders a past time as "5 minutes ago". now is a parameter so the
// output is testable.
func agoText(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minute") + " ago"
	case d < 48*time.Hour:
		return plural(int(d/time.Hour), "hour") + " ago"
	case d < 60*24*time.Hour:
		return plural(int(d/(24*time.Hour)), "day") + " ago"
	}
	return t.Format("2 Jan 2006")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

func moveAgo(t time.Time) string { return agoText(t, time.Now()) }

func stashesURL(campaignID string) string {
	return "/campaigns/" + campaignID + "/armory/stashes"
}

func stashURL(campaignID string, stashID int) string {
	return stashesURL(campaignID) + "/" + strconv.Itoa(stashID)
}

func movesURL(campaignID string) string { return "/campaigns/" + campaignID + "/armory/moves" }

func purchaseRequestsURL(campaignID string) string {
	return "/campaigns/" + campaignID + "/armory/purchase-requests"
}

// purchaseRequestText is the tail of a waiting purchase row after the
// requester's name: "wants to buy 2 items at The Gilded Cup for 35 gp". The
// price is omitted when the basket can no longer be priced from the listings.
func purchaseRequestText(p PurchaseRequestLine) string {
	text := fmt.Sprintf("wants to buy %s at %s", plural(p.ItemCount(), "item"), p.ShopName)
	if p.Total != nil {
		text += fmt.Sprintf(" for %s %s", p.Total.String(), p.Currency)
		if p.PriceRose {
			text += ", but the price has gone up since they asked"
		}
	}
	return text
}

func downtimeURL(campaignID string) string { return "/campaigns/" + campaignID + "/armory/downtime" }

func panelURL(campaignID, characterID string) string {
	return "/campaigns/" + campaignID + "/armory/characters/" + url.PathEscape(characterID) + "/panel"
}

func characterHistoryURL(campaignID, characterID string) string {
	return "/campaigns/" + campaignID + "/armory/characters/" + url.PathEscape(characterID) + "/history"
}

// moveDialogURL is the GET that loads the move dialog for one holding.
func moveDialogURL(campaignID, kind string, from Endpoint, itemID string) string {
	q := url.Values{}
	q.Set("kind", kind)
	q.Set("from_kind", from.Kind)
	q.Set("from_id", from.ID)
	if itemID != "" {
		q.Set("item_id", itemID)
	}
	return "/campaigns/" + campaignID + "/armory/move?" + q.Encode()
}

// closeMoveDialogOnClick empties the dialog container. It is an inline
// expression rather than a script helper so it still runs after the fragment
// is swapped in.
func closeMoveDialogOnClick() templ.ComponentScript {
	return inlineOnClick("armory_closeMoveDialog",
		`(function(){var m=document.getElementById('armory-move-modal');if(m){m.innerHTML='';}})()`)
}

// closeMoveDialogOnKey closes the dialog on Escape.
func closeMoveDialogOnKey() templ.ComponentScript {
	return inlineOnClick("armory_closeMoveDialogKey",
		`(function(e){if(e.key==='Escape'){var m=document.getElementById('armory-move-modal');if(m){m.innerHTML='';}}})(event)`)
}

// giveDialogURL is the GET that loads the "Give to" dialog, opened for a
// character (pick an item or map) or for an item (pick a character).
func giveDialogURL(campaignID, param, id string) string {
	return "/campaigns/" + campaignID + "/armory/give?" + url.Values{param: {id}}.Encode()
}

func giveURL(campaignID string) string { return "/campaigns/" + campaignID + "/armory/give" }

func destValue(e Endpoint) string { return e.Kind + ":" + e.ID }

func moveStatusLabel(status string) string {
	switch status {
	case MovePending:
		return "Waiting"
	case MoveDeclined:
		return "Turned down"
	case MoveFailed:
		return "Failed"
	}
	return "Done"
}

func moveStatusClass(status string) string {
	switch status {
	case MovePending:
		return "badge-amber"
	case MoveDeclined:
		return "badge-gray"
	case MoveFailed:
		return "badge-red"
	}
	return "badge-green"
}

// giveTabOnClick shows one tab of the give dialog and hides the other.
func giveTabOnClick(tab string) templ.ComponentScript {
	return inlineOnClick("armory_giveTab",
		`(function(btn){var root=btn.closest('[data-give-root]');if(!root)return;var want=`+jsStr(tab)+`;`+
			`root.querySelectorAll('[data-give-panel]').forEach(function(p){p.hidden=p.getAttribute('data-give-panel')!==want;});`+
			`root.querySelectorAll('[data-give-tab]').forEach(function(t){var on=t.getAttribute('data-give-tab')===want;`+
			`t.setAttribute('aria-selected',on?'true':'false');`+
			`t.classList.toggle('border-accent',on);t.classList.toggle('text-fg',on);`+
			`t.classList.toggle('border-transparent',!on);t.classList.toggle('text-fg-muted',!on);});})(this)`)
}

// giveFilterOnInput hides the rows of the list below the search box whose
// name does not contain what was typed.
func giveFilterOnInput() templ.ComponentScript {
	return inlineOnClick("armory_giveFilter",
		`(function(inp){var form=inp.closest('form');if(!form)return;var q=inp.value.trim().toLowerCase();`+
			`form.querySelectorAll('[data-give-row]').forEach(function(r){r.style.display=r.getAttribute('data-name').indexOf(q)<0?'none':'';});})(this)`)
}

// giveHintOnChange writes the picked map's sentence under the list.
func giveHintOnChange() templ.ComponentScript {
	return inlineOnClick("armory_giveHint",
		`(function(r){var p=r.closest('form').querySelector('[data-give-hint]');if(p)p.textContent=r.getAttribute('data-hint')||'';})(this)`)
}

// mapHint says what a character gets for the picked map.
func mapHint(who string) func(NamedRef) string {
	return func(m NamedRef) string {
		return fmt.Sprintf("%s gets an item called \"%s\". Opening it shows the map.", who, handoutName(m.Name))
	}
}

// itemHint warns, for a private item, that giving it lets the character's
// player see its page. It is nil (no hint) when the character is not known.
func itemHint(who *NamedRef) func(NamedRef) string {
	if who == nil {
		return nil
	}
	return func(it NamedRef) string {
		if !it.Restricted {
			return ""
		}
		return who.Name + "'s player will be able to see this item's page."
	}
}

// restrictedHint is the same warning for the item flow, where the item is
// fixed and the character is picked.
func restrictedHint(restricted bool) func(NamedRef) string {
	if !restricted {
		return nil
	}
	return func(c NamedRef) string {
		return c.Name + "'s player will be able to see this item's page."
	}
}
