package syncapi

import (
	"fmt"
	"strconv"
	"strings"
)

// What the call flow says about a failed step: what happened, why, what
// Foundry does next and how to fix it. Known failures get their own story;
// the rest are explained by their answer code.

// isDatePush reports whether a row is Foundry setting the calendar's date.
func isDatePush(ev SyncEvent) bool {
	return ev.Call == "PUT /calendar/date"
}

func statusCode(ev SyncEvent) int {
	n, _ := strconv.Atoi(ev.Status)
	return n
}

// shortProblem is the label on a failed answer's arrow.
func shortProblem(ev SyncEvent) string {
	code := statusCode(ev)
	switch {
	case isDatePush(ev) && code == 403:
		return "Refused: owner only"
	case code == 401:
		return "Refused: key not accepted"
	case code == 403:
		return "Refused"
	case code == 404:
		return "Not found"
	case code == 409:
		return "Conflict"
	case code == 429:
		return "Too many calls"
	case code >= 500:
		return "Server problem"
	case code >= 400:
		return "Rejected"
	}
	return "Failed"
}

// keyOwner is who a key belongs to, for sentences.
func keyOwner(ev SyncEvent, key *FlowKey) string {
	if key != nil && key.Owner != "" {
		return key.Owner
	}
	if ev.UserName != "" {
		return ev.UserName
	}
	return "this"
}

func possessive(name string) string {
	if name == "this" {
		return "this"
	}
	return name + "’s"
}

// explainProblem tells the story of one failed row.
func explainProblem(ev SyncEvent, key *FlowKey, in flowInput) *FlowProblem {
	owner := keyOwner(ev, key)
	p := &FlowProblem{Was: ev.Was}
	code := statusCode(ev)
	clientSide := ev.ReportedBy == reportedByClient
	reason := ev.Message
	if in.Admin && clientSide {
		// Foundry's own words can carry page names, which admin pages hide.
		reason = ""
	}

	switch {
	case isDatePush(ev) && code == 403:
		p.Headline = "Chronicle refused " + possessive(owner) + " date"
		if ev.ResourceName != "" {
			p.Asked = ev.ResourceName
			p.What = fmt.Sprintf("Foundry sent the date %s to Chronicle with %s key, and Chronicle said no.", ev.ResourceName, possessive(owner))
		} else {
			p.What = fmt.Sprintf("Foundry sent a new date to Chronicle with %s key, and Chronicle said no.", possessive(owner))
		}
		role := "isn’t the owner’s"
		if key != nil && key.Role != "" {
			role = "belongs to a " + key.Role
		}
		p.Why = fmt.Sprintf("Changing the date from Foundry needs the campaign owner’s key. %s key %s. DM access wouldn’t help here: date changes are owner only.", capitalise(possessive(owner)), role)
		p.Next = []string{
			"Stops sending dates and tells the GM once.",
			"Starts sending again on its next pull (reconnect, or Pull on its dashboard).",
		}
		p.Fix = []string{
			"In Foundry, open Game Settings › Chronicle Sync and paste the owner’s key, or",
			"set the date in Chronicle; Foundry follows.",
		}
	case isDatePush(ev) && (code == 400 || code == 422):
		p.Headline = "Chronicle’s calendar couldn’t take that date"
		p.Asked = ev.ResourceName
		p.What = "Foundry sent a date that Chronicle’s calendar can’t hold, or the calendar follows real time and its date can’t be set."
		p.Why = answerWhy(ev, reason)
		p.Next = []string{
			"Stops sending dates and tells the GM once.",
			"Starts sending again on its next pull (reconnect, or Pull on its dashboard).",
		}
		p.Fix = []string{
			"Make the two calendars match (same months and days), or",
			"set the date in Chronicle; Foundry follows.",
		}
	case clientSide && ev.Direction == DirToFoundry:
		p.Headline = "Foundry couldn’t apply a change"
		p.What = "Chronicle told Foundry about a change, and Foundry couldn’t apply it."
		p.Why = foundryWhy(reason)
		p.Fix = []string{
			"In Foundry, open the Chronicle Sync dashboard and press Pull to try again.",
		}
	case clientSide:
		p.Headline = "Foundry reported a problem"
		p.What = "Foundry hit a problem on its side; it never reached Chronicle as a call."
		p.Why = foundryWhy(reason)
		p.Fix = []string{"Open the Chronicle Sync dashboard in Foundry; its log has the details."}
	case code == 401:
		p.Headline = "Chronicle didn’t accept the key"
		p.What = "Foundry called Chronicle with a key Chronicle no longer accepts."
		p.Why = "The key was turned off, revoked or has expired, or it is being used from a place it isn’t allowed."
		p.Fix = []string{"In Chronicle, open Manage › Foundry and make a new connect line, then paste it into Foundry."}
	case code == 403:
		p.Headline = "Chronicle refused " + possessive(owner) + " change"
		p.What = fmt.Sprintf("Foundry sent a change with %s key, and Chronicle said no.", possessive(owner))
		p.Why = answerWhy(ev, reason)
		p.Fix = []string{"Use a key whose owner may make this change, or make it in Chronicle."}
	case code == 404:
		p.Headline = "Chronicle couldn’t find it"
		p.What = "Foundry asked about something Chronicle doesn’t have, or this key can’t see."
		p.Why = "It was deleted in Chronicle, or it is hidden from the key’s owner."
		p.Fix = []string{"In Foundry, press Pull on the Chronicle Sync dashboard so it catches up."}
	case code == 409:
		p.Headline = "The change clashed with another"
		p.What = "Foundry’s change arrived after a newer one."
		p.Why = answerWhy(ev, reason)
		p.Fix = []string{"In Foundry, press Pull to get Chronicle’s copy, then make the change again."}
	case code == 429:
		p.Headline = "Foundry called too often"
		p.What = "Chronicle slowed Foundry down because the key went over its calls per minute."
		p.Why = "Each key has a rate limit; a big catch-up can go over it."
		p.Next = []string{"Tries again on its own."}
		p.Fix = []string{"Nothing, unless it keeps happening; then tell the site admin."}
	case code >= 500:
		p.Headline = "Chronicle hit a problem"
		p.What = "Chronicle failed while handling Foundry’s call."
		p.Why = "Something went wrong on the server, not in Foundry."
		p.Fix = []string{"Try again. If it keeps happening, the server’s log has the details."}
	default:
		p.Headline = "Chronicle rejected the change"
		p.What = "Chronicle couldn’t accept what Foundry sent."
		p.Why = answerWhy(ev, reason)
		p.Fix = []string{"Make the change in Chronicle, or check the module is up to date."}
	}

	if reps := in.Repeats[ev.ID]; len(reps) > 0 {
		p.RepeatCount = len(reps)
		sameKey := reps[0].APIKeyID
		for i, r := range reps {
			if i < flowRepeatTimes {
				p.Repeats = append(p.Repeats, r.OccurredAt)
			}
			if sameKey != nil && (r.APIKeyID == nil || *r.APIKeyID != *sameKey) {
				sameKey = nil
			}
		}
		if sameKey != nil && len(reps) > 1 {
			if k, ok := in.Keys[*sameKey]; ok && k.Name != "" {
				p.RepeatKey = k.Name
			}
		}
		// Oldest first reads as a timeline.
		for i, j := 0, len(p.Repeats)-1; i < j; i, j = i+1, j-1 {
			p.Repeats[i], p.Repeats[j] = p.Repeats[j], p.Repeats[i]
		}
	}
	if in.OnlyThese != nil {
		p.OnlyThese = in.OnlyThese(ev)
	}
	p.OpenHistory = in.OpenHistory
	return p
}

// answerWhy is Chronicle's own answer, in a sentence.
func answerWhy(ev SyncEvent, reason string) string {
	if reason == "" {
		return "Chronicle answered " + ev.Status + "."
	}
	return "Chronicle answered: “" + strings.TrimSuffix(reason, ".") + ".”"
}

func foundryWhy(reason string) string {
	if reason == "" {
		return "Foundry didn’t say why."
	}
	return "Foundry said: “" + strings.TrimSuffix(reason, ".") + ".”"
}
