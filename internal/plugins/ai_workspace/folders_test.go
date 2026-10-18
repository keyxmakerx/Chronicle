package ai_workspace

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
)

func folderBlock(action, name, rename string) importer.ParsedPage {
	f := map[string]any{"name": name, "category": "Lore"}
	if rename != "" {
		f["rename_to"] = rename
	}
	return importer.ParsedPage{
		Name:        name,
		FrontMatter: importer.FrontMatter{Kind: "folder", Action: action},
		Fields:      f,
	}
}

// Folder blocks that make or rename a folder run before pages; a removal runs
// with the other records, after them.
func TestEarlyFolders(t *testing.T) {
	other := importer.ParsedPage{FrontMatter: importer.FrontMatter{Kind: "event", Action: "create"}}
	tests := []struct {
		name string
		p    importer.ParsedPage
		want bool
	}{
		{"create", folderBlock("create", "Gods", ""), true},
		{"update", folderBlock("update", "Gods", "Pantheon"), true},
		{"delete", folderBlock("delete", "Gods", ""), false},
		{"another kind", other, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEarlyFolder(tt.p); got != tt.want {
				t.Errorf("isEarlyFolder = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPendingFolderNames(t *testing.T) {
	got := pendingFolderNames([]importer.ParsedPage{
		folderBlock("create", " Gods ", ""),
		folderBlock("update", "Cosmology", "The Heavens"),
		folderBlock("delete", "Old", ""),
	})
	for _, want := range []string{"gods", "cosmology", "the heavens"} {
		if !got[want] {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	if got["old"] {
		t.Error("a removed folder must not count as made")
	}
}
