package connector

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

type stubFolders struct{}

func (stubFolders) Categories(context.Context, string) ([]records.FolderCategory, error) {
	return []records.FolderCategory{{ID: 1, Name: "Lore"}, {ID: 2, Name: "Location"}}, nil
}
func (stubFolders) List(context.Context, string) ([]records.FolderRef, error) {
	return []records.FolderRef{{ID: "g", Name: "Gods", CategoryID: 1}, {ID: "e", Name: "Elder Gods", CategoryID: 1, ParentID: "g"}}, nil
}
func (stubFolders) Create(context.Context, string, int, string, string) error { panic("read only") }
func (stubFolders) Rename(context.Context, string, string, string) error      { panic("read only") }
func (stubFolders) Move(context.Context, string, string, *string) error       { panic("read only") }
func (stubFolders) Delete(context.Context, string, string) error              { panic("read only") }

// list_folders is the `folders` lookup as a tool: on every key, since the
// lookup itself keeps a player to the categories the sidebar shows them,
// taking an optional category, and answered through the lookup path.
func TestTools_ListFolders(t *testing.T) {
	l := &records.Lookups{FolderTree: stubFolders{}}
	for _, c := range []*Connection{{}, {ForPlayer: true}} {
		if findTool(c, l, "list_folders") == nil {
			t.Fatalf("list_folders is not listed (player %v)", c.ForPlayer)
		}
	}
	tl := findTool(&Connection{}, l, "list_folders")
	if tl.What != "folders" || !tl.takes("category") || !tl.takes("piece") {
		t.Fatalf("tool = %+v", tl)
	}
	args, err := tl.readArgs(map[string]any{"category": "Lore"})
	if err != nil {
		t.Fatal(err)
	}
	text, _, ok := tl.answer(context.Background(), l, Session{Conn: &Connection{CampaignID: "c"}, CampaignName: "Camp", Actor: aiplug.Actor{UserID: "u", Role: 3}}, args)
	if !ok || !strings.Contains(text, "- Gods (no pages)\n  - Elder Gods (no pages)") || strings.Contains(text, "Location") {
		t.Errorf("answer = %q, %v", text, ok)
	}
}
