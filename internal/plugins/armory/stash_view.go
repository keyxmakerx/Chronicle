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

// moveOpenOnClick opens (or shuts) the Move box under the line it belongs to,
// loading it from url the first time.
func moveOpenOnClick(url string) templ.ComponentScript {
	return inlineOnClick("armory_moveOpen",
		`(function(btn){if(window.Chronicle&&Chronicle.GiveBox)Chronicle.GiveBox.open(btn,`+jsStr(url)+`);})(this)`)
}

// moveEnter sends the Move box on Enter from its number or amount field.
func moveEnter() templ.ComponentScript {
	return inlineOnClick("armory_moveEnter",
		`(function(el,e){if(e.key==='Enter'&&window.Chronicle&&Chronicle.GiveBox){e.preventDefault();Chronicle.GiveBox.move(el);}})(this,event)`)
}

// moveGoLabel is the Move box's main button: a player outside downtime asks
// the GM instead of moving.
func moveGoLabel(immediate bool) string {
	if immediate {
		return "Move"
	}
	return "Ask the GM"
}

// moveBusyLabel is what that button says while the answer is awaited.
func moveBusyLabel(immediate bool) string {
	if immediate {
		return "Moving…"
	}
	return "Asking…"
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

// giveBoxCall is the inline handler of a Give or Share box control: it hands
// the element to Chronicle.GiveBox's method of that name. The method name is
// one of a fixed set chosen here, never user text.
func giveBoxCall(method string) templ.ComponentScript {
	return inlineOnClick("armory_giveBox_"+method,
		`(function(el){if(window.Chronicle&&Chronicle.GiveBox)Chronicle.GiveBox.`+method+`(el);})(this)`)
}

// giveOpenOnClick opens (or shuts) the character panel's Give box, loading it
// from url the first time.
func giveOpenOnClick(url string) templ.ComponentScript {
	return inlineOnClick("armory_giveOpen",
		`(function(btn){if(window.Chronicle&&Chronicle.GiveBox)Chronicle.GiveBox.open(btn,`+jsStr(url)+`);})(this)`)
}

// itemPickHint is the line under the list once an item (or, on a card, a
// character) is picked. A hidden item becomes visible to the character's
// player, so that is what it says; otherwise it says who hears about it.
// With nobody playing the character there is nobody to tell.
func itemPickHint(restricted bool, character, player string) string {
	switch {
	case player == "":
		return "It shows on " + character + " in Foundry too."
	case restricted:
		return player + ", " + character + "’s player, will be able to see this item’s page. Nobody else will."
	default:
		return player + " gets a notification. It shows on " + character + " in Foundry too."
	}
}

// itemPickIcon is the Font Awesome icon in front of itemPickHint.
func itemPickIcon(restricted bool, player string) string {
	if restricted && player != "" {
		return "fa-eye"
	}
	return "fa-bell"
}

// mapPickHint says what a character gets for the picked map.
func mapPickHint(character, mapName string) string {
	return fmt.Sprintf("%s gets “%s”. Opening it shows the map. Only players holding it can see its page.", character, handoutName(mapName))
}

// shareURL is the Share box's address (GET) and where it saves (POST).
func shareURL(campaignID, characterID, itemID string) string {
	return "/campaigns/" + campaignID + "/armory/characters/" + url.PathEscape(characterID) + "/items/" + url.PathEscape(itemID) + "/share"
}

// shareOpenOnClick opens (or shuts) a panel line's Share box.
func shareOpenOnClick(url string) templ.ComponentScript {
	return inlineOnClick("armory_shareOpen",
		`(function(btn){if(window.Chronicle&&Chronicle.GiveBox)Chronicle.GiveBox.open(btn,`+jsStr(url)+`);})(this)`)
}

// shareHint says what sharing lets the others do; a map handout's page opens
// the map too.
func shareHint(v *ShareBoxView) string {
	if v.IsMap {
		return "They can open its page and the map. It stays on " + v.Character.Name + "."
	}
	return "They can open its page. It stays on " + v.Character.Name + "."
}
