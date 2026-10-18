package aiexport

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func TestRenderEntities_NamesTheFolder(t *testing.T) {
	gods, unknown := "gods", "gone"
	types := []entities.EntityType{{ID: 1, Name: "Lore", NamePlural: "Lore"}}
	tests := []struct {
		name string
		ent  entities.Entity
		opts Options
		want string // "" = no Folder line
	}{
		{"filed page", entities.Entity{ID: "z", Name: "Zeus", EntityTypeID: 1, ParentNodeID: &gods}, Options{FolderNames: map[string]string{"gods": "Gods"}}, "**Folder:** Gods"},
		{"unfiled page", entities.Entity{ID: "h", Name: "Hera", EntityTypeID: 1}, Options{FolderNames: map[string]string{"gods": "Gods"}}, ""},
		{"folder with no known name", entities.Entity{ID: "z", Name: "Zeus", EntityTypeID: 1, ParentNodeID: &unknown}, Options{FolderNames: map[string]string{"gods": "Gods"}}, ""},
		{"folders not supplied", entities.Entity{ID: "z", Name: "Zeus", EntityTypeID: 1, ParentNodeID: &gods}, Options{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := RenderEntities(context.Background(), []entities.Entity{tt.ent}, types, nil, nil, tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(out, "**Folder:**"); got != (tt.want != "") {
				t.Fatalf("Folder line present = %v, want %v in:\n%s", got, tt.want != "", out)
			}
			if tt.want != "" && !strings.Contains(out, tt.want) {
				t.Errorf("missing %q in:\n%s", tt.want, out)
			}
		})
	}
}
