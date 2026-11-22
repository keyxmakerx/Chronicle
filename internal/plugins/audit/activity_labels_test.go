// activity_labels_test.go pins the words the activity log shows for each
// action: a key with words of its own says them, and any other key says what
// area changed, so a raw key such as member.invited never reaches the page.
package audit

import (
	"strings"
	"testing"
)

func TestActionLabel_Words(t *testing.T) {
	tests := []struct {
		action, want string
	}{
		{ActionEntityCreated, "created"},
		{ActionMemberJoined, "joined the campaign"},
		{ActionEntityTypeCreated, "created category"},
		{"member.invited", "invited someone"},
		{"member.allowed_back", "let someone back in"},
		{"campaign.archived", "archived the campaign"},
		{"group.member_added", "added someone to group"},
		{"trash.emptied", "emptied the Trash"},
		{ActionCalendarEventCreated, "changed the calendar"},
		{ActionTimelineEventLinked, "changed timeline"},
		{"entity.place_added", "changed"},
		{"member.something_new", "changed who is in the campaign"},
		{"nothing.known", "made a change"},
		{"", "made a change"},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			if got := actionLabel(tt.action); got != tt.want {
				t.Errorf("actionLabel(%q) = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}

// Every action the code writes reads as words: no dot, no underscore.
func TestActionLabel_NeverRawKey(t *testing.T) {
	keys := []string{
		ActionEntityCreated, ActionEntityUpdated, ActionEntityDeleted, ActionEntityClaimed,
		ActionEntityOwnerChanged, ActionMemberJoined, ActionMemberLeft, ActionMemberRoleChanged,
		ActionCampaignUpdated, ActionEntityTypeCreated, ActionEntityTypeUpdated, ActionEntityTypeDeleted,
		ActionItemShared, ActionTagCreated, ActionTagDeleted, ActionTrashRestored,
		ActionCalendarCreated, ActionCalendarUpdated, ActionCalendarDeleted, ActionCalendarDateAdvanced,
		ActionCalendarEventCreated, ActionCalendarWeatherActiveZoneChanged,
		ActionTimelineCreated, ActionTimelineEntityGroupMemberRemoved, ActionTimelineEventLinkVisibilityChanged,
		"member.invited", "member.allowed_back", "campaign.archived", "campaign.unarchived",
		"campaign.handed_over", "group.created", "group.updated", "group.deleted",
		"group.member_added", "group.member_removed", "entity.place_added", "entity.place_removed",
		"entity.clone", "tag.added", "tag.removed", "trash.campaign_restored", "trash.files_restored",
		"trash.retention_changed", "trash.long_term_changed", "trash.emptied", "guest.joined",
		"addon.changed", "session.terminated",
	}
	for _, k := range keys {
		if got := actionLabel(k); got == "" || strings.ContainsAny(got, "._") {
			t.Errorf("actionLabel(%q) = %q, want plain words", k, got)
		}
	}
}

// The kind beside an entry reads as words, never a stored key.
func TestEntityTypeWords(t *testing.T) {
	for in, want := range map[string]string{
		"page":             "page",
		"calendar_event":   "calendar event",
		"timeline.entity":  "timeline entity",
		"_odd__name-here_": "odd name here",
		"":                 "",
	} {
		if got := entityTypeWords(in); got != want {
			t.Errorf("entityTypeWords(%q) = %q, want %q", in, got, want)
		}
	}
}
