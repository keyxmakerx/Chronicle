package notes

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// Pages link to notes the way notes do: a [[link]] anchor with data-note-id
// in the page's body, which never carries the note's title (see links.go).

// PageLinker lists the pages that link a note, only those the viewer may
// see. seesSecrets says whether the viewer reads a page's inline GM
// secrets; for anyone else a page whose only link sits inside one is left
// out, or the list would say what the secret mentions. An adapter over the
// entities service satisfies it, so this widget never imports a plugin.
type PageLinker interface {
	PagesLinkingNote(ctx context.Context, campaignID string, v permissions.Viewer, seesSecrets bool, noteID string) ([]PageRef, error)
}

// PageRef names a page in a backlink list, and nothing of its body.
type PageRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	TypeName string `json:"typeName,omitempty"`
}

// LinksIn is everything the viewer can see that links to a note.
type LinksIn struct {
	Notes []NoteRef `json:"notes"`
	Pages []PageRef `json:"pages"`
}
