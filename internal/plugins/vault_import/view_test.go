package vault_import

import "testing"

func TestFolderLine(t *testing.T) {
	tests := []struct {
		name string
		s    Stats
		want string
	}{
		{"none", Stats{}, "The notes sit at the top level of the Imported folder."},
		{"one folder", Stats{FolderPages: 1}, "1 folder becomes a page that holds the notes inside it."},
		{"many folders", Stats{FolderPages: 3}, "3 folders become pages that hold the notes inside them."},
		{"one folder note", Stats{FolderNotes: 1}, "1 folder uses the note of the same name as the folder's page."},
		{"many folder notes", Stats{FolderNotes: 2}, "2 folders use the note of the same name as the folder's page."},
		{"both", Stats{FolderPages: 1, FolderNotes: 2}, "1 folder becomes a page that holds the notes inside it, and 2 use the note of the same name."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := folderLine(tc.s); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
