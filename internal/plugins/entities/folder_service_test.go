package entities

import (
	"context"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

type fakeNodes struct {
	SidebarNodeRepository
	nodes   map[string]*SidebarNode
	deleted []string
	parents map[string]*string

	resequenced []string
}

func (f *fakeNodes) ListByType(_ context.Context, campaignID string, typeID int) ([]SidebarNode, error) {
	var out []SidebarNode
	for _, n := range f.nodes {
		if n.CampaignID == campaignID && n.EntityTypeID == typeID {
			out = append(out, *n)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}
func (f *fakeNodes) ResequenceNodes(_ context.Context, _ string, ids []string) error {
	f.resequenced = append([]string(nil), ids...)
	return nil
}
func (f *fakeNodes) FindByID(_ context.Context, id string) (*SidebarNode, error) {
	if n, ok := f.nodes[id]; ok {
		c := *n
		return &c, nil
	}
	return nil, apperror.NewNotFound("sidebar node not found")
}
func (f *fakeNodes) Create(_ context.Context, n *SidebarNode) error { f.nodes[n.ID] = n; return nil }
func (f *fakeNodes) Update(_ context.Context, n *SidebarNode) error {
	f.nodes[n.ID].Name = n.Name
	return nil
}
func (f *fakeNodes) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeNodes) UpdateParent(_ context.Context, id, _ string, p *string) error {
	f.parents[id] = p
	return nil
}

type fakeFolderEntities struct {
	EntityRepository
	ents     map[string]*Entity
	parent   map[string]*string
	nodeOf   map[string]*string
	nodeSets int

	siblings    []string // what ListSiblingIDsOrdered answers
	resequenced []string
}

func (f *fakeFolderEntities) ListSiblingIDsOrdered(context.Context, string, int, *string, *string) ([]string, error) {
	return f.siblings, nil
}
func (f *fakeFolderEntities) ResequenceSiblings(_ context.Context, _ string, ids []string) error {
	f.resequenced = append([]string(nil), ids...)
	return nil
}

type fakeOrders struct {
	set map[string]string
}

func (f *fakeOrders) ListByCampaign(context.Context, string) (map[string]string, error) {
	return f.set, nil
}
func (f *fakeOrders) Set(_ context.Context, _, kind, id, mode string) error {
	if mode == OrderAZ {
		delete(f.set, childOrderKey(kind, id))
	} else {
		f.set[childOrderKey(kind, id)] = mode
	}
	return nil
}

func (f *fakeFolderEntities) FindByID(_ context.Context, id string) (*Entity, error) {
	if e, ok := f.ents[id]; ok {
		return e, nil
	}
	return nil, apperror.NewNotFound("entity not found")
}
func (f *fakeFolderEntities) UpdateParent(_ context.Context, id, _ string, p *string) error {
	f.parent[id] = p
	return nil
}
func (f *fakeFolderEntities) UpdateParentNode(_ context.Context, id, _ string, p *string) error {
	f.nodeOf[id] = p
	f.nodeSets++
	return nil
}

type fakeFolderTypes struct {
	EntityTypeRepository
	types map[int]*EntityType
}

func (f *fakeFolderTypes) FindByID(_ context.Context, id int) (*EntityType, error) {
	if t, ok := f.types[id]; ok {
		return t, nil
	}
	return nil, apperror.NewNotFound("entity type not found")
}
func (f *fakeFolderTypes) ListByCampaign(_ context.Context, campaignID string) ([]EntityType, error) {
	var out []EntityType
	for _, t := range f.types {
		if t.CampaignID == campaignID {
			out = append(out, *t)
		}
	}
	return out, nil
}

func folderFixture() (FolderService, *fakeNodes, *fakeFolderEntities) {
	lore := 1
	nodes := &fakeNodes{parents: map[string]*string{}, nodes: map[string]*SidebarNode{
		"gods":  {ID: "gods", CampaignID: "c1", EntityTypeID: 1, Name: "Gods"},
		"elder": {ID: "elder", CampaignID: "c1", EntityTypeID: 1, Name: "Elder", ParentID: ptr("gods")},
		"dup1":  {ID: "dup1", CampaignID: "c1", EntityTypeID: 1, Name: "Twin"},
		"dup2":  {ID: "dup2", CampaignID: "c1", EntityTypeID: 1, Name: "twin"},
		"place": {ID: "place", CampaignID: "c1", EntityTypeID: 2, Name: "Towns"},
		"other": {ID: "other", CampaignID: "c2", EntityTypeID: 9, Name: "Foreign"},
	}}
	ents := &fakeFolderEntities{parent: map[string]*string{}, nodeOf: map[string]*string{}, ents: map[string]*Entity{
		"zeus":   {ID: "zeus", CampaignID: "c1", EntityTypeID: 1},
		"minor":  {ID: "minor", CampaignID: "c1", EntityTypeID: 3},
		"town":   {ID: "town", CampaignID: "c1", EntityTypeID: 2},
		"abroad": {ID: "abroad", CampaignID: "c2", EntityTypeID: 9},
	}}
	types := &fakeFolderTypes{types: map[int]*EntityType{
		1: {ID: 1, CampaignID: "c1"}, 2: {ID: 2, CampaignID: "c1"},
		3: {ID: 3, CampaignID: "c1", ParentTypeID: &lore},
		9: {ID: 9, CampaignID: "c2"},
	}}
	return NewFolderService(nodes, ents, types, &fakeOrders{set: map[string]string{}}), nodes, ents
}

func TestFolderService_FindFolder(t *testing.T) {
	svc, _, _ := folderFixture()
	tests := []struct {
		name, query string
		typeID      int
		wantID      string
		wantErr     string
	}{
		{"exact", "Gods", 1, "gods", ""},
		{"ignores case and spaces", "  gODs ", 1, "gods", ""},
		{"none", "Nope", 1, "", ""},
		{"other category is not searched", "Towns", 1, "", ""},
		{"two share a name", "twin", 1, "", "more than one folder"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.FindFolder(context.Background(), "c1", tt.typeID, tt.query)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if (got == nil) != (tt.wantID == "") || (got != nil && got.ID != tt.wantID) {
				t.Fatalf("got %+v, want %q", got, tt.wantID)
			}
		})
	}
}

func TestFolderService_CreateRenameMoveDelete(t *testing.T) {
	ctx := context.Background()
	t.Run("create trims and scopes", func(t *testing.T) {
		svc, nodes, _ := folderFixture()
		n, err := svc.CreateFolder(ctx, "c1", CreateSidebarNodeInput{Name: "  Cosmology ", EntityTypeID: 1})
		if err != nil || n.Name != "Cosmology" || nodes.nodes[n.ID] == nil {
			t.Fatalf("n=%+v err=%v", n, err)
		}
	})
	createRefusals := []struct {
		name string
		in   CreateSidebarNodeInput
	}{
		{"blank name", CreateSidebarNodeInput{Name: "  ", EntityTypeID: 1}},
		{"category of another campaign", CreateSidebarNodeInput{Name: "X", EntityTypeID: 9}},
		{"unknown parent", CreateSidebarNodeInput{Name: "X", EntityTypeID: 1, ParentID: "nope"}},
		{"parent in another category", CreateSidebarNodeInput{Name: "X", EntityTypeID: 1, ParentID: "place"}},
		{"too long", CreateSidebarNodeInput{Name: strings.Repeat("x", 256), EntityTypeID: 1}},
	}
	for _, tt := range createRefusals {
		t.Run("create refuses "+tt.name, func(t *testing.T) {
			svc, _, _ := folderFixture()
			if _, err := svc.CreateFolder(ctx, "c1", tt.in); err == nil {
				t.Fatal("want a refusal")
			}
		})
	}
	t.Run("rename and delete refuse another campaign's folder", func(t *testing.T) {
		svc, nodes, _ := folderFixture()
		if err := svc.RenameFolder(ctx, "c1", "other", "Mine"); err == nil {
			t.Error("rename crossed campaigns")
		}
		if err := svc.DeleteFolder(ctx, "c1", "other"); err == nil || len(nodes.deleted) != 0 {
			t.Error("delete crossed campaigns")
		}
	})
	t.Run("rename and delete work", func(t *testing.T) {
		svc, nodes, _ := folderFixture()
		if err := svc.RenameFolder(ctx, "c1", "gods", "Pantheon"); err != nil || nodes.nodes["gods"].Name != "Pantheon" {
			t.Errorf("rename: %v", err)
		}
		if err := svc.DeleteFolder(ctx, "c1", "gods"); err != nil || len(nodes.deleted) != 1 {
			t.Errorf("delete: %v", err)
		}
	})
	moves := []struct {
		name    string
		id      string
		parent  *string
		wantErr string
	}{
		{"into another folder", "dup1", ptr("gods"), ""},
		{"to the top level", "elder", nil, ""},
		{"into itself", "gods", ptr("gods"), "itself"},
		{"into its own sub-folder", "gods", ptr("elder"), "itself"},
		{"into another category's folder", "dup1", ptr("place"), "not found"},
	}
	for _, tt := range moves {
		t.Run("move "+tt.name, func(t *testing.T) {
			svc, nodes, _ := folderFixture()
			err := svc.MoveFolder(ctx, "c1", tt.id, tt.parent)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got, ok := nodes.parents[tt.id]; !ok || (got == nil) != (tt.parent == nil) {
				t.Errorf("parent write = %v, %v", got, ok)
			}
		})
	}
}

func TestFolderService_SetEntityFolder(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		entity   string
		folder   *string
		wantErr  bool
		wantNode *string
		wantPar  bool // parent page cleared
	}{
		{"files a page", "zeus", ptr("gods"), false, ptr("gods"), true},
		{"a sub-category page uses the parent category's folder", "minor", ptr("gods"), false, ptr("gods"), true},
		{"takes a page out", "zeus", nil, false, nil, false},
		{"other category's folder is refused", "town", ptr("gods"), true, nil, false},
		{"other campaign's folder is refused", "zeus", ptr("other"), true, nil, false},
		{"other campaign's page is refused", "abroad", ptr("gods"), true, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, ents := folderFixture()
			err := svc.SetEntityFolder(ctx, "c1", tt.entity, tt.folder)
			if tt.wantErr {
				if err == nil || ents.nodeSets != 0 {
					t.Fatalf("err = %v, writes = %d", err, ents.nodeSets)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := ents.nodeOf[tt.entity]
			if (got == nil) != (tt.wantNode == nil) || (got != nil && *got != *tt.wantNode) {
				t.Errorf("folder = %v, want %v", got, tt.wantNode)
			}
			if _, cleared := ents.parent[tt.entity]; cleared != tt.wantPar {
				t.Errorf("parent page cleared = %v, want %v", cleared, tt.wantPar)
			}
		})
	}
}

// Stored data can hold a loop (A under B under A) that the moved folder is
// not part of; the walk must stop and refuse rather than spin.
func TestFolderService_MoveFolderRefusesExistingLoop(t *testing.T) {
	svc, nodes, _ := folderFixture()
	nodes.nodes["gods"].ParentID = ptr("elder") // gods <-> elder
	done := make(chan error, 1)
	go func() { done <- svc.MoveFolder(context.Background(), "c1", "dup1", ptr("gods")) }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "loop") {
			t.Fatalf("err = %v, want a loop refusal", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("MoveFolder never returned")
	}
	if _, ok := nodes.parents["dup1"]; ok {
		t.Error("the move was written")
	}
}

func TestFolderService_SetChildOrder(t *testing.T) {
	tests := []struct {
		name, kind, id, mode, wantErr string
	}{
		{"category", OrderContainerCategory, "1", OrderHand, ""},
		{"folder", OrderContainerFolder, "gods", OrderNew, ""},
		{"page", OrderContainerPage, "zeus", OrderHand, ""},
		{"back to A to Z", OrderContainerFolder, "gods", OrderAZ, ""},
		{"unknown order", OrderContainerFolder, "gods", "random", "order must be"},
		{"unknown kind", "map", "gods", OrderHand, "kind must be"},
		{"category of another campaign", OrderContainerCategory, "9", OrderHand, "not found"},
		{"category id that is not a number", OrderContainerCategory, "x", OrderHand, "not found"},
		{"folder of another campaign", OrderContainerFolder, "other", OrderHand, "not found"},
		{"page of another campaign", OrderContainerPage, "abroad", OrderHand, "not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _ := folderFixture()
			err := svc.SetChildOrder(context.Background(), "c1", tt.kind, tt.id, tt.mode)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, _ := svc.ChildOrders(context.Background(), "c1")
			if m := childOrderOf(got, tt.kind, tt.id); m != tt.mode {
				t.Errorf("stored order = %q, want %q", m, tt.mode)
			}
		})
	}
}

func TestFolderService_MoveNextTo(t *testing.T) {
	tests := []struct {
		name, kind, id, next string
		after                bool
		want                 []string
		wantErr              string
	}{
		{"page up past a page", OrderContainerPage, "c", "b", false, []string{"a", "c", "b", "d"}, ""},
		{"page down past a page", OrderContainerPage, "a", "b", true, []string{"b", "a", "c", "d"}, ""},
		{"page past one the mover cannot see keeps it in place", OrderContainerPage, "d", "b", true, []string{"a", "b", "d", "c"}, ""},
		{"next to itself", OrderContainerPage, "a", "a", false, nil, "itself"},
		{"page in another list", OrderContainerPage, "a", "town", false, nil, "not in the same list"},
		{"page of another campaign", OrderContainerPage, "a", "abroad", false, nil, "not found"},
		{"folder up past a folder", OrderContainerFolder, "dup1", "gods", false, []string{"dup1", "gods", "dup2"}, ""},
		{"folder in another folder", OrderContainerFolder, "dup1", "elder", false, nil, "not in the same list"},
		{"unknown kind", "category", "1", "2", false, nil, "kind must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, nodes, ents := folderFixture()
			for _, id := range []string{"a", "b", "c", "d"} {
				ents.ents[id] = &Entity{ID: id, CampaignID: "c1", EntityTypeID: 1}
			}
			ents.siblings = []string{"a", "b", "c", "d"}
			nodes.nodes["dup1"].SortOrder = 2
			nodes.nodes["dup2"].SortOrder = 1
			err := svc.MoveNextTo(context.Background(), "c1", tt.kind, tt.id, tt.next, tt.after)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := ents.resequenced
			if tt.kind == OrderContainerFolder {
				got = nodes.resequenced
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("order = %v, want %v", got, tt.want)
			}
		})
	}
}
