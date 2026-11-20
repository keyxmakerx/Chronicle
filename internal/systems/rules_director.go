package systems

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// What the Director may do from an entry's own page: the quiet amber words
// of the signed design. Only a Director's view ever carries them (the service
// refuses everyone else's writes as well); the page draws them in the slot
// beside the whose-words line and the script opens their small dialogs.

// Which amber words an entry's page offers the Director.
const (
	// ToolsStandard: an untouched standard entry of a kind ("Rewrite for
	// <campaign>", "Not in <campaign>").
	ToolsStandard = "standard"
	// ToolsLeftOut: a standard entry the campaign left out ("Bring back").
	ToolsLeftOut = "left_out"
	// ToolsRewritten: a campaign entry that takes a standard entry's place.
	ToolsRewritten = "rewritten"
	// ToolsCampaign: the campaign's own entry, tied to nothing.
	ToolsCampaign = "campaign"
)

// directorTools is the Director's part of an entry's page.
type directorTools struct {
	Kind         string
	CampaignName string
	SystemName   string
	FieldKey     string
	// Slug is the standard entry's slug (standard, left out); Name its name,
	// or the campaign entry's.
	Slug string
	Name string
	// KindPhrase names one of the kind in a sentence ("a people").
	KindPhrase string
	// Rewritable is true when the standard entry copies without loss.
	Rewritable bool
	// SameName is the campaign's own untied entry of the same name, which
	// "Rewrite" offers to use instead of a copy.
	SameName *sameNameEntry
	// Sentence is a left-out entry's sentence.
	Sentence string
	// EntryID and EditURL are the campaign entry's, for the pen.
	EntryID int64
	EditURL string
	// WhoReads says who reads a rewritten entry now, when the amber mark
	// above does not already say it.
	WhoReads string
	// PrintedName is the standard entry a rewritten one replaces.
	PrintedName string
	// CanSetAside is true while players read the rewritten entry.
	CanSetAside bool
	// Heroes are the heroes of this entry the Director can see, for the
	// dialogs that say who is affected; MoreHeroes counts the rest.
	Heroes     []string
	MoreHeroes int
	// APIBase is the campaign's entry API, PageURL where to land afterwards.
	APIBase string
	PageURL string
}

// sameNameEntry is an untied campaign entry a rewrite could use instead.
type sameNameEntry struct {
	ID   int64
	Name string
}

// EntryEditorURL opens the entry editor straight on one campaign entry;
// back is the page the editor returns to when it closes.
func EntryEditorURL(campaignID, fieldKey string, entryID int64, back string) string {
	q := url.Values{}
	q.Set("field", fieldKey)
	q.Set("entry", strconv.FormatInt(entryID, 10))
	if back != "" {
		q.Set("return", back)
	}
	return "/campaigns/" + url.PathEscape(campaignID) + "/characters/entries?" + q.Encode()
}

func entryAPIBase(campaignID string) string {
	return "/campaigns/" + url.PathEscape(campaignID) + "/system-entries"
}

// fillTools builds the Director's words for a page already drawn for them.
// Categories that are not kinds, the printed original and a player's draft
// get none: the printed page already says whose it is, and a draft is
// reviewed in the editor.
func (p *RulesPages) fillTools(ctx context.Context, sys System, v *EntryView, r RulesReader) error {
	e := v.Entry
	if !r.Actor.IsDirector || e.FieldKey == "" || v.Printed || !p.overlay(ctx, sys, r.CampaignID) {
		return nil
	}
	t := &directorTools{CampaignName: r.CampaignName, SystemName: sys.Info().Name, FieldKey: e.FieldKey,
		Name: e.Name, KindPhrase: KindPhrase(e.FieldKey), APIBase: entryAPIBase(r.CampaignID), PageURL: v.URL}
	if e.Credit != "" {
		t.SystemName = e.Credit
	}
	if v.InCampaign != nil {
		t.Heroes = names(v.InCampaign.Heroes)
		t.MoreHeroes = v.InCampaign.MoreHeroes
	}
	switch {
	case v.LeftOut:
		t.Kind, t.Slug, t.Sentence = ToolsLeftOut, e.Slug, e.Sentence
	case e.Origin == OriginStandard:
		t.Kind, t.Slug = ToolsStandard, e.Slug
		ok, err := p.catalog.Rewritable(ctx, r.CampaignID, e.FieldKey, e.Slug)
		if err != nil {
			return err
		}
		t.Rewritable = ok
		if ok {
			same, err := p.sameNameEntry(ctx, sys, v.Category.Slug, e, r)
			if err != nil {
				return err
			}
			t.SameName = same
		}
		// A standard entry's heroes resolve to it only when nothing of the
		// campaign's answers their name first; "Not in" names exactly those.
	case e.Origin == OriginRewritten && e.Status == "":
		t.Kind, t.EntryID = ToolsRewritten, e.EntryID
		t.EditURL = EntryEditorURL(r.CampaignID, e.FieldKey, e.EntryID, v.URL)
		t.PrintedName = e.StandsForName
		if t.PrintedName == "" {
			t.PrintedName = e.Name
		}
		t.CanSetAside = !e.DirectorsOnly
		switch {
		case e.DirectorsOnly:
			// The amber mark above already says only Directors read it.
		case e.KeptFor != "":
			t.WhoReads = "Only " + e.KeptFor + "’s player and the Directors read " + r.CampaignName + "’s " + e.Name + "."
		default:
			t.WhoReads = "Everyone in " + r.CampaignName + " reads this " + e.Name + "."
		}
	case e.Origin == OriginCampaign:
		t.Kind, t.EntryID = ToolsCampaign, e.EntryID
		t.EditURL = EntryEditorURL(r.CampaignID, e.FieldKey, e.EntryID, v.URL)
	default:
		return nil
	}
	v.Tools = t
	return nil
}

// sameNameEntry finds the campaign's own accepted, untied entry called what
// the standard entry is called, the one "Use the Dwarf I already wrote" ties.
func (p *RulesPages) sameNameEntry(ctx context.Context, sys System, category string, e CatalogEntry, r RulesReader) (*sameNameEntry, error) {
	l, err := p.catalogList(ctx, sys, category, r)
	if err != nil {
		return nil, err
	}
	for _, c := range l.Entries {
		if c.Origin == OriginCampaign && c.Status == "" && c.EntryID != 0 && strings.EqualFold(c.Name, e.Name) {
			return &sameNameEntry{ID: c.EntryID, Name: c.Name}, nil
		}
	}
	return nil, nil
}

func names(ls []personLink) []string {
	out := make([]string, 0, len(ls))
	for _, l := range ls {
		out = append(out, l.Name)
	}
	return out
}

// heroesLine says who a change reaches ("Sable Varr is a Polder ...", or
// none's words when no hero is), with the verb the dialog needs.
func (t *directorTools) heroesLine(none, one, many string) string {
	if len(t.Heroes) == 0 {
		return none
	}
	who := joinNames(t.Heroes)
	if t.MoreHeroes > 0 {
		who = strings.Join(t.Heroes, ", ") + " and " + strconv.Itoa(t.MoreHeroes) + " more"
	}
	if len(t.Heroes)+t.MoreHeroes == 1 {
		return who + " " + one
	}
	return who + " " + many
}

// NotInHeroes is the "Not in" dialog's line about the heroes who are of it.
func (t *directorTools) NotInHeroes() string {
	return t.heroesLine("No hero in "+t.CampaignName+" is "+aOr(t.Name)+".",
		"is "+aOr(t.Name)+" and keeps reading its traits, marked no longer offered.",
		"are "+t.Name+" and keep reading its traits, marked no longer offered.")
}

// AsideHeroes is the "Set aside" dialog's line about who goes back.
func (t *directorTools) AsideHeroes() string {
	return t.heroesLine("No hero is "+aOr(t.Name)+" yet.",
		"goes back to the printed "+t.PrintedName+".",
		"go back to the printed "+t.PrintedName+".")
}

// aOr puts "a" or "an" before a name: "a Polder", "an Elf".
func aOr(name string) string {
	if name == "" {
		return name
	}
	if strings.ContainsRune("AEIOUaeiou", rune(name[0])) {
		return "an " + name
	}
	return "a " + name
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

// editorBase is the entry editor's address for a kind; the script adds the
// entry it lands on.
func editorBase(campaignID, fieldKey string) string {
	return "/campaigns/" + url.PathEscape(campaignID) + "/characters/entries?field=" + url.QueryEscape(fieldKey)
}
