package records

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

// RulesLookup answers `what: rules`: the campaign's sheet-field rules and
// local words, each written as the `field-rule` or `local-word` block that
// sends it back changed. Directors only, as changing them is: a local word
// can be kept to Directors, and the owner's AI in Safe privacy reads as a
// player. It reads through the homebrew service, which checks the Actor again.
type RulesLookup struct {
	Svc HomebrewAPI
}

func (RulesLookup) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{
		What: "rules", Aliases: []string{"sheet-rules", "field-rules", "local-words"}, Keys: "none",
		Brief: "the campaign's sheet-field rules (a field worked out from another) and local words (what a place or era calls a word), each written as the `field-rule` or `local-word` block that changes it (owner and co-DM only)",
		Tool: aiplug.Tool{
			Name: "sheet_rules", Title: "Sheet rules", Did: "Looked at the sheet rules", Owner: true,
			Desc: "The campaign's sheet-field rules (a field worked out from another, such as a guild rank from an echelon) and its local words (what a place or era calls a word, such as a coin), each as a block to send back changed with ask_for_changes.",
		},
	}
}

func (l RulesLookup) Answer(ctx context.Context, campaignID string, a Actor, _ Record) (string, string, error) {
	chip := "Sheet rules"
	if l.Svc == nil {
		return chip, "", badRequestf("sheet rules can't be looked up on this server")
	}
	if !a.CanAuthorDmOnly() {
		return chip, "", badRequestf("sheet rules are left out in Safe privacy; choose Permitted to include them")
	}
	rules, err := l.Svc.Rules(ctx, campaignID, a)
	if err != nil {
		return chip, "", err
	}
	words, err := l.Svc.Words(ctx, campaignID, a)
	if err != nil {
		return chip, "", err
	}
	var b strings.Builder
	b.WriteString("## Sheet rules\n\nSend a block back, changed, to change it.\n\n### Field rules\n\n")
	if len(rules) == 0 {
		b.WriteString("None yet.\n")
	}
	for _, r := range rules {
		b.WriteString("```\n" + ruleBlock(r) + "```\n\n")
	}
	b.WriteString("### Local words\n\n")
	if len(words) == 0 {
		b.WriteString("None yet.\n")
	}
	for _, w := range words {
		b.WriteString("```\n" + wordBlock(w) + "```\n\n")
	}
	return fmt.Sprintf("Sheet rules · %d rules, %d words", len(rules), len(words)), b.String(), nil
}

func ruleBlock(r HomebrewRule) string {
	var b strings.Builder
	b.WriteString("---\nkind: field-rule\nname: " + yamlStr(r.Name) + "\nfollows: " + yamlStr(r.Follows) + "\nrows:\n")
	num := func(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
	for _, row := range r.Rows {
		lead := "  - "
		put := func(k, v string) {
			b.WriteString(lead + k + ": " + v + "\n")
			lead = "    "
		}
		if row.Is != "" {
			put("is", yamlStr(row.Is))
		}
		if row.From != nil {
			put("from", num(*row.From))
		}
		if row.To != nil {
			put("to", num(*row.To))
		}
		put("gives", yamlStr(row.Gives))
	}
	b.WriteString("---\n")
	return b.String()
}

func wordBlock(w HomebrewWord) string {
	var b strings.Builder
	b.WriteString("---\nkind: local-word\nname: " + yamlStr(w.Word) + "\nlocal: " + yamlStr(w.Local) + "\n")
	if w.EraID != 0 {
		b.WriteString("era: " + yamlStr(w.EraName) + "\n")
	} else {
		b.WriteString("place: " + yamlStr(w.PlaceName) + "\n")
	}
	if w.DMOnly {
		b.WriteString("director: true\n")
	}
	b.WriteString("---\n")
	return b.String()
}
