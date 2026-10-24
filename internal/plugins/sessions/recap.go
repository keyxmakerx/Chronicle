package sessions

import (
	"encoding/json"
	"strings"
)

// isEditorDoc reports whether a stored recap is the page editor's
// ProseMirror JSON. Recaps saved from the calendar's plain box hold the
// typed text instead, and must never be handed to the editor as a document.
func isEditorDoc(recap string) bool {
	var d struct {
		Type string `json:"type"`
	}
	return json.Unmarshal([]byte(recap), &d) == nil && d.Type == "doc"
}

// hasRecap reports whether anyone has written the night up.
func hasRecap(s *Session) bool {
	return s.RecapHTML != nil && strings.TrimSpace(*s.RecapHTML) != ""
}

// recapEditorBody is the recap in the page editor's load contract: the
// document when there is one, else the HTML, which the editor opens as is.
func recapEditorBody(s *Session) map[string]any {
	out := map[string]any{"entry_html": strPtrVal(s.RecapHTML)}
	if s.Recap != nil && isEditorDoc(*s.Recap) {
		out["entry"] = *s.Recap
	}
	return out
}

// recapOpen reports whether a night's page shows its recap. A recap needs
// no Mark Complete: it opens on the night's own day, so the table can write
// it up that evening, and stays once written. A cancelled night has none.
func recapOpen(s *Session, today string) bool {
	if s.Status == StatusCancelled {
		return false
	}
	if s.Status == StatusCompleted || hasRecap(s) {
		return true
	}
	return s.ScheduledDate != nil && *s.ScheduledDate != "" && *s.ScheduledDate <= today
}
