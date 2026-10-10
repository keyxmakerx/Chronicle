package vault_import

import (
	"fmt"
	"strings"
)

// plural picks the singular word for exactly one, the plural otherwise.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func joinNames(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = "“" + n + "”"
	}
	return strings.Join(q, ", ")
}

// folderLine says how the folders become pages.
func folderLine(s Stats) string {
	switch {
	case s.FolderPages == 0 && s.FolderNotes == 0:
		return "The notes sit at the top level of the Imported folder."
	case s.FolderNotes == 0:
		return fmt.Sprintf("%d %s become pages that hold the notes inside them.", s.FolderPages, plural(s.FolderPages, "folder", "folders"))
	case s.FolderPages == 0:
		return fmt.Sprintf("%d %s use the note of the same name as the folder's page.", s.FolderNotes, plural(s.FolderNotes, "folder", "folders"))
	}
	return fmt.Sprintf("%d %s become pages that hold the notes inside them, and %d use the note of the same name.",
		s.FolderPages, plural(s.FolderPages, "folder", "folders"), s.FolderNotes)
}

// notesLine gathers the small things the import does not carry over.
func notesLine(s Stats) string {
	var parts []string
	if s.TagPages > 0 {
		parts = append(parts, fmt.Sprintf("Tags on %d %s are not carried over.", s.TagPages, plural(s.TagPages, "page", "pages")))
	}
	if s.VisibilityKeys > 0 {
		parts = append(parts, "A visibility setting in a note is ignored: every page starts GM only.")
	}
	if s.BadFrontMatter > 0 {
		parts = append(parts, fmt.Sprintf("%d %s had a property block that could not be read; its text is kept.", s.BadFrontMatter, plural(s.BadFrontMatter, "note", "notes")))
	}
	if s.NameClashes > 0 {
		parts = append(parts, fmt.Sprintf("%d notes share a name with another note; a link goes to the one nearest the note that links.", s.NameClashes))
	}
	if s.CaseClashes > 0 {
		parts = append(parts, fmt.Sprintf("%d %s differ only by capital letters.", s.CaseClashes, plural(s.CaseClashes, "pair of files", "pairs of files")))
	}
	return strings.Join(parts, " ")
}
