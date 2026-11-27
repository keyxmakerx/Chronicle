package audit

import (
	"sort"
	"strings"
)

// campaignWords are the words for the campaign-wide actions other plugins
// log under the campaign area. Without them every one read "changed the
// campaign", so a run of settings changes was a wall of identical lines.
var campaignWords = map[string]string{
	"campaign.trashed":                        "moved the campaign to the Trash",
	"campaign.details.updated":                "changed the campaign's details",
	"campaign.branding.updated":               "changed the campaign's name and logo in the menu",
	"campaign.appearance.saved":               "changed the campaign's look",
	"campaign.appearance.picture_uploaded":    "added a picture to the campaign's look",
	"campaign.appearance.picture_discarded":   "took a picture out of the campaign's look",
	"campaign.customize.published":            "published changes from the Customizer",
	"campaign.default_visibility.updated":     "changed who sees new pages",
	"campaign.dm_grants.updated":              "changed what co-Directors can see",
	"campaign.event_tier_definitions.updated": "changed the calendar's event tiers",
	"campaign.handover_setting.updated":       "changed who can be handed a page",
	"campaign.hero_changes.updated":           "changed what players may change on their heroes",
	"campaign.system.updated":                 "changed the game system",
	"campaign.welcome_message.updated":        "changed the welcome message",
	"campaign.join_code.generated":            "made a new join code",
	"campaign.join_code.revoked":              "turned off the join code",
	"campaign.vault_import.finished":          "imported notes from a vault",
	"campaign.vault_import.failed":            "tried to import notes from a vault",
	"campaign.ai_connection.created":          "connected an AI",
	"campaign.ai_connection.changed":          "changed what the AI may do",
	"campaign.ai_connection.paused":           "paused the AI",
	"campaign.ai_connection.resumed":          "turned the AI back on",
	"campaign.ai_connection.disconnected":     "disconnected the AI",
	"campaign.ai_note_access.changed":         "changed which notes the AI may read",
	"campaign.ai_lookup.answered":             "answered the AI's question",
	"campaign.ai_alert.answered":              "answered an AI alert",
	"campaign.ai_request.turned_down":         "turned down the AI's changes",
	"campaign.ai_request.undone":              "undid the AI's changes",
	"campaign.ai_player_change.applied":       "let a player's AI change their hero",
	"campaign.ai_players.turned_on":           "let players connect their own AI",
	"campaign.ai_players.turned_off":          "stopped players connecting their own AI",
	"campaign.ai_player_key.pause":            "paused a player's AI",
	"campaign.ai_player_key.resume":           "turned a player's AI back on",
	"campaign.ai_player_key.end":              "disconnected a player's AI",
	"campaign.ai_prompt.generated":            "made a prompt for an AI",
	"campaign.ai_prompt.copied":               "copied a prompt for an AI",
	"campaign.ai_prompt.instructions_copied":  "copied the AI instructions",
	"campaign.ai_import.parsed":               "read an AI's answer to import",
	"campaign.ai_import.committed":            "imported an AI's answer",
	"campaign.ai_export.generated":            "exported the campaign for an AI",
	"campaign.ai.cut_off":                     "had the AI cut off for too many calls",
	"campaign.ai.back_on":                     "had the AI turned back on",
}

// detailFields names the settings a change touched, from the field names
// its details list (never their values), in a stable order.
func detailFields(d map[string]any) []string {
	var out []string
	switch v := d["fields"].(type) {
	case []string:
		out = append(out, v...)
	case []any:
		for _, f := range v {
			if s, ok := f.(string); ok {
				out = append(out, s)
			}
		}
	}
	// The details save names each part it was sent rather than a list.
	for _, k := range []string{"name", "description", "is_public"} {
		if sent, _ := d[k+"_sent"].(bool); sent {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// fieldWords are the campaign settings a change can name.
var fieldWords = map[string]string{
	"name":        "name",
	"description": "description",
	"is_public":   "public setting",
}

// entryLabel is actionLabel with what an entry's details add: a change to
// the campaign's details says which ones.
func entryLabel(e AuditEntry) string {
	if e.Action == ActionCampaignUpdated || e.Action == "campaign.details.updated" {
		var words []string
		for _, f := range detailFields(e.Details) {
			if w, ok := fieldWords[f]; ok {
				words = append(words, w)
			}
		}
		if len(words) > 0 {
			return "changed the campaign's " + listWords(words)
		}
	}
	return actionLabel(e.Action)
}

// listWords joins words as "a", "a and b" or "a, b and c".
func listWords(w []string) string {
	switch len(w) {
	case 0:
		return ""
	case 1:
		return w[0]
	}
	return strings.Join(w[:len(w)-1], ", ") + " and " + w[len(w)-1]
}
