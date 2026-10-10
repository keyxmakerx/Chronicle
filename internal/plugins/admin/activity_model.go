package admin

import (
	"fmt"
	"strings"
	"time"
)

// ActivityEntry is one row of the site-wide admin change log. Unlike
// SecurityEvent (sign-in and session security) it records what an admin
// changed, so the Home page can answer "who changed what".
type ActivityEntry struct {
	ID          int64          `json:"id"`
	ActorUserID string         `json:"actorUserId,omitempty"`
	Action      string         `json:"action"` // "resource.verb", e.g. "user.admin_granted".
	TargetType  string         `json:"targetType,omitempty"`
	TargetID    string         `json:"targetId,omitempty"`
	TargetLabel string         `json:"targetLabel,omitempty"` // Name at the time, so the log survives deletes.
	Detail      map[string]any `json:"detail,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`

	// ActorName is joined from users for display; empty when the account is gone.
	ActorName string `json:"actorName,omitempty"`
}

// activityPhrases maps an action to the sentence tail shown after the actor's
// name. %s is the target label. Phrases are plain language on purpose: this
// list is read by site owners, not developers.
var activityPhrases = map[string]string{
	"user.admin_granted":              "made %s an admin",
	"user.admin_revoked":              "removed admin rights from %s",
	"user.disabled":                   "disabled the account of %s",
	"user.enabled":                    "re-enabled the account of %s",
	"user.force_logout":               "signed %s out everywhere",
	"user.two_factor_off":             "switched off two-factor sign-in for %s",
	"session.terminated":              "ended a signed-in session",
	"campaign.deleted":                "deleted the campaign %s",
	"campaign.joined":                 "joined the campaign %s",
	"campaign.left":                   "left the campaign %s",
	"media.deleted":                   "deleted the file %s",
	"registration.mode_changed":       "changed who can sign up to %s",
	"hygiene.purged":                  "cleaned up %s",
	"migrations.applied":              "applied pending database updates",
	"package.added":                   "added the package source %s",
	"package.removed":                 "removed the package %s",
	"package.version_installed":       "installed a version of %s",
	"package.version_pinned":          "pinned the version of %s",
	"package.auto_update":             "changed automatic updates for %s",
	"package.reviewed":                "reviewed the package submission %s",
	"package.repo_changed":            "changed the package source %s",
	"package.deprecated":              "marked %s as deprecated",
	"package.archived":                "archived %s",
	"package.pruned":                  "removed old package versions of %s",
	"backup.run":                      "started a backup",
	"restore.run":                     "restored the site from a backup",
	"smtp.saved":                      "changed the email settings",
	"sitelook.saved":                  "changed the site look",
	"addon.changed":                   "changed the feature %s",
	"extension.installed":             "installed the extension %s",
	"extension.uninstalled":           "removed the extension %s",
	"extension.updated":               "updated the extension %s",
	"extension.rescanned":             "rescanned for extensions (%s)",
	"extension.plugin_reloaded":       "reloaded the extension plugin %s",
	"extension.plugin_stopped":        "stopped the extension plugin %s",
	"storage.user_limit_set":          "set a storage limit for a person",
	"storage.user_limit_removed":      "removed a person's storage limit",
	"storage.campaign_limit_set":      "set a storage limit for a campaign",
	"storage.campaign_limit_removed":  "removed a campaign's storage limit",
	"storage.user_bypass_set":         "allowed a person to exceed their storage limit for a while",
	"storage.user_bypass_cleared":     "ended a person's temporary storage allowance",
	"storage.campaign_bypass_set":     "allowed a campaign to exceed its storage limit for a while",
	"storage.campaign_bypass_cleared": "ended a campaign's temporary storage allowance",
	"apialert.resolved":               "marked an API security alert as resolved",
	"foundry.campaign_notified":       "told a campaign about Foundry module version %s",
	"foundry.campaign_force_pinned":   "moved a campaign to Foundry module version %s",
	"foundry.older_notified":          "told campaigns on older Foundry module versions about %s",
	"foundry.older_force_pinned":      "moved campaigns on older Foundry module versions to %s",
	"restore.started":                 "began restoring the site from a backup",
	"storage.settings_changed":        "changed the storage limits",
	"cors.changed":                    "changed the allowed websites for the API",
	"apikey.changed":                  "changed the API key %s",
	"ipblock.changed":                 "changed the blocked address %s",
}

// Sentence renders "Mara made Theo an admin". An unknown action still reads
// acceptably so a recorder added later never shows a blank row.
func (e ActivityEntry) Sentence() string {
	actor := e.ActorName
	if actor == "" {
		actor = "A former admin"
	}
	label := e.TargetLabel
	if label == "" {
		label = "an item"
	}
	phrase, ok := activityPhrases[e.Action]
	if !ok {
		return fmt.Sprintf("%s made a change (%s)", actor, strings.ReplaceAll(e.Action, "_", " "))
	}
	if strings.Contains(phrase, "%s") {
		phrase = fmt.Sprintf(phrase, label)
	}
	return actor + " " + phrase
}
