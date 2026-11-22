package systemstate

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

// SystemResolver names the game system a campaign has on, "" for none.
// Implemented in app over the systems handler.
type SystemResolver interface {
	EnabledSystemID(ctx context.Context, campaignID string) string
}

// AIPageState is this plugin's part of the AI engine (aiplug): the
// `page-state` lookup, which reads one key of the game system's per-page
// state across the campaign, both halves as stored. Chronicle never
// interprets the documents, so the answer gives them as the system's
// widgets keep them. It is the Director team's: the `gm` half is theirs,
// and the `public` half a widget derives is only safe to show per page,
// which the campaign listing does not check, so anyone else is refused.
// It only reads; the widget on each page is what changes the state.
type AIPageState struct {
	svc     Service
	systems SystemResolver
}

// NewAIPageState builds the lookup over the service.
func NewAIPageState(svc Service, systems SystemResolver) *AIPageState {
	return &AIPageState{svc: svc, systems: systems}
}

var _ aiplug.Lookup = (*AIPageState)(nil)

// LookupInfo describes `what: page-state` and its tool.
func (*AIPageState) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{
		What: "page-state", Aliases: []string{"system-state"},
		Keys:  "`key` (the state's name, such as negotiation); optional `page` (one page's name) and `system` (a game system's id; the campaign's own by default)",
		Brief: "what the game system's widgets keep on pages under one key, such as the negotiation tracker on NPC pages: each page's Director half and players' half as stored (owner and co-DM only)",
		Tool: aiplug.Tool{
			Name: "page_state", Title: "Game system state on pages", Did: "Looked at the game system's state on pages", Owner: true,
			Desc: "What the campaign's game system keeps on pages under one key, read as stored: for each page carrying it, the Director's half and the half players who can open the page see. The negotiation tracker on NPC pages keeps its negotiation under `negotiation` (attitude, interest, patience, motivations, pitfalls, which of them the heroes have found, whether the meters are shown to players, and a log). Read only: the widget on the page changes it, so describe a change for the Director rather than asking for one.",
			Args: []aiplug.Arg{
				{Key: "key", Desc: "The state's name, such as negotiation", Required: true},
				{Key: "page", Desc: "Only this page"},
				{Key: "system", Desc: "A game system's id; leave out for the campaign's own"},
			},
		},
	}
}

// Answer lists the pages carrying the key, with both halves.
func (l *AIPageState) Answer(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) (string, string, error) {
	chip := "Page state"
	if !a.CanAuthorDmOnly() {
		return chip, "", aiplug.BadRequestf("a game system's page state is left out in Safe privacy; choose Permitted to include it")
	}
	key := strings.ToLower(firstSet(r.Str("key"), r.Name))
	if key == "" {
		return chip, "", aiplug.BadRequestf("it needs `key`, the state's name, such as negotiation")
	}
	system := strings.ToLower(r.Str("system"))
	if system == "" && l.systems != nil {
		system = l.systems.EnabledSystemID(ctx, campaignID)
	}
	if system == "" {
		return chip, "", aiplug.BadRequestf("this campaign has no game system on; name one with `system`")
	}
	if !idPattern.MatchString(key) || !idPattern.MatchString(system) {
		return chip, "", aiplug.BadRequestf("`key` and `system` are lower-case letters, digits, - and _")
	}
	// A named page is read directly: picked out of the capped listing, a
	// page past its end would read as carrying nothing.
	page := strings.TrimSpace(r.Str("page"))
	var items []ListItem
	var truncated bool
	var err error
	if page != "" {
		items, truncated, err = l.svc.ListByKeyForPage(ctx, campaignID, system, key, page)
	} else {
		items, truncated, err = l.svc.ListByKey(ctx, campaignID, system, key)
	}
	if err != nil {
		return chip, "", err
	}
	chip = fmt.Sprintf("Page state · %s · %d", key, len(items))
	var b strings.Builder
	fmt.Fprintf(&b, "## %s on pages (%d)\n\n", key, len(items))
	fmt.Fprintf(&b, "As the %s game system's widgets keep it. The Director's half is never shown to players; the players' half is what anyone who can open the page reads.\n\n", system)
	if truncated {
		if page != "" {
			fmt.Fprintf(&b, "Only the first %d pages called %s are listed.\n\n", MaxListItems, aiplug.Quote(page))
		} else {
			fmt.Fprintf(&b, "Only the first %d pages are listed; name one with `page`.\n\n", MaxListItems)
		}
	}
	if len(items) == 0 {
		if page != "" {
			b.WriteString("None on " + aiplug.Quote(page) + ".\n")
		} else {
			b.WriteString("No page carries it.\n")
		}
	}
	for _, it := range items {
		fmt.Fprintf(&b, "### %s\n\nUpdated %s.\n\nDirector's half:\n```json\n%s\n```\n\nPlayers' half:\n```json\n%s\n```\n\n",
			it.EntityName, it.UpdatedAt.UTC().Format("2006-01-02 15:04 UTC"), half(it.GM), half(it.Public))
	}
	return chip, b.String(), nil
}

// half is a stored half as text; an absent half reads as an empty object,
// as the service's own reads do.
func half(raw []byte) string {
	if len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}

func firstSet(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
