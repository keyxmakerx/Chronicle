package records

import "context"

// Writer kinds: who wrote a note, as the owner's switches on the AI page
// name them.
const (
	WriterCoDM   = "co-DM"
	WriterScribe = "Scribe"
	WriterPlayer = "player"
)

// NoteAccess is the owner's choice of which shared Journal entries and jots
// the AI may change, besides the owner's own. Both are off until the owner
// turns them on.
type NoteAccess struct {
	CoDMs   bool
	Scribes bool
}

// Allows reports whether a note by this kind of writer may be changed.
// Players' notes never may.
func (a NoteAccess) Allows(writer string) bool {
	switch writer {
	case WriterCoDM:
		return a.CoDMs
	case WriterScribe:
		return a.Scribes
	}
	return false
}

// Refusal is the review's reason a shared note can't be changed.
func (a NoteAccess) Refusal(writer string) string {
	switch writer {
	case WriterCoDM:
		return "notes your co-DMs share with you are read-only for your AI. Turn on \"Journal entries and jots your co-DMs shared with you\" on the Your AI page to allow it"
	case WriterScribe:
		return "notes your Scribes share with you are read-only for your AI. Turn on \"Journal entries and jots your Scribes shared with you\" on the Your AI page to allow it"
	}
	return "notes players write are read-only for your AI"
}

// NoteWriter is a note's writer: their name and what kind of member they are.
type NoteWriter struct {
	Name string
	Kind string // WriterCoDM, WriterScribe or WriterPlayer
}

// NoteAccessAPI reads the owner's switches and who a writer is. A co-DM is a
// member the owner granted DM access; a Scribe holds the Scribe role
// without that grant.
type NoteAccessAPI interface {
	NoteAccess(ctx context.Context, campaignID string) (NoteAccess, error)
	NoteWriter(ctx context.Context, campaignID, userID string) (NoteWriter, error)
}
