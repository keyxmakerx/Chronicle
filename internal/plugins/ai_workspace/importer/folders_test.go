// folders_test.go covers the `folder:` front-matter key: the parse rule, the
// review-time problems and the committer's file / take-out / refuse behavior.

package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type fakeFolderAccess struct {
	folders map[string]*entities.SidebarNode // lower-cased name, typeID 1 only
	ambig   map[string]bool
	sets    map[string]*string // entity id -> folder id written (nil = taken out)
}

func (f *fakeFolderAccess) FindFolder(_ context.Context, _ string, typeID int, name string) (*entities.SidebarNode, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	if f.ambig[n] {
		return nil, apperror.NewBadRequest("more than one folder is called \"" + name + "\" in this category")
	}
	if n, ok := f.folders[n]; ok && typeID == 1 {
		return n, nil
	}
	return nil, nil
}

func (f *fakeFolderAccess) SetEntityFolder(_ context.Context, _, entityID string, folderID *string) error {
	if f.sets == nil {
		f.sets = map[string]*string{}
	}
	f.sets[entityID] = folderID
	return nil
}

func newFolders() *fakeFolderAccess {
	return &fakeFolderAccess{
		folders: map[string]*entities.SidebarNode{"gods": {ID: "f-gods", Name: "Gods", EntityTypeID: 1}},
		ambig:   map[string]bool{"twin": true},
	}
}

func withFolder(name, folder, action string) ParsedPage {
	p := page(name, "lore", "# "+name+"\nBody.")
	p.FrontMatter.Folder = folder
	p.FrontMatter.Action = action
	return p
}

func TestParse_FolderAndParent(t *testing.T) {
	md := func(extra string) string {
		return "---\nname: Zeus\ntype: lore\n" + extra + "---\n# Zeus\nBody.\n"
	}
	tests := []struct {
		name    string
		extra   string
		wantErr string
	}{
		{"folder alone", "folder: Gods\n", ""},
		{"folder none", "folder: none\n", ""},
		{"both named", "parent: Olympus\nfolder: Gods\n", "can't be used together"},
		{"parent none with a folder", "parent: none\nfolder: Gods\n", ""},
		{"parent with folder none", "parent: Olympus\nfolder: none\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pages := Parse(md(tt.extra))
			if len(pages) != 1 {
				t.Fatalf("pages = %d", len(pages))
			}
			got := pages[0]
			if tt.wantErr != "" {
				if got.Status != StatusParseError || !strings.Contains(got.ParseError, tt.wantErr) {
					t.Fatalf("status %q, error %q", got.Status, got.ParseError)
				}
				return
			}
			if got.Status == StatusParseError {
				t.Fatalf("unexpected parse error: %s", got.ParseError)
			}
			for _, w := range got.Warnings {
				if strings.Contains(w, "folder") {
					t.Errorf("folder must be a known key, got warning %q", w)
				}
			}
		})
	}
}

func TestClassify_FolderProblems(t *testing.T) {
	lookup := &stubLookup{
		bySlug: map[string]*entities.Entity{"hera": {ID: "ent-hera", Name: "Hera", Slug: "hera", EntityTypeID: 1}},
		types:  []entities.EntityType{{ID: 1, Slug: "lore", Name: "Lore"}},
	}
	tests := []struct {
		name    string
		pages   []ParsedPage
		folders FolderAccess
		pending map[string]bool
		want    string // substring of the problem; "" = no problem
	}{
		{"existing folder", []ParsedPage{withFolder("Zeus", "gods", "")}, newFolders(), nil, ""},
		{"no folder key", []ParsedPage{withFolder("Zeus", "", "")}, newFolders(), nil, ""},
		{"none needs no lookup", []ParsedPage{withFolder("Zeus", "none", "")}, newFolders(), nil, ""},
		{"missing folder", []ParsedPage{withFolder("Zeus", "Titans", "")}, newFolders(), nil, "No folder called"},
		{"folder made in the paste", []ParsedPage{withFolder("Zeus", "Titans", "")}, newFolders(), map[string]bool{"lore|titans": true}, ""},
		{"same name in another category does not count", []ParsedPage{withFolder("Zeus", "Titans", "")}, newFolders(), map[string]bool{"places|titans": true}, "No folder called"},
		{"bare name does not count", []ParsedPage{withFolder("Zeus", "Titans", "")}, newFolders(), map[string]bool{"titans": true}, "No folder called"},
		{"ambiguous folder", []ParsedPage{withFolder("Zeus", "Twin", "")}, newFolders(), nil, "more than one folder"},
		{"folders not wired", []ParsedPage{withFolder("Zeus", "Gods", "")}, nil, nil, "can't be set"},
		{"update checks the page's own category", []ParsedPage{withFolder("Hera", "Gods", ActionUpdate)}, newFolders(), nil, ""},
		{"delete row ignores folder", []ParsedPage{withFolder("Hera", "Nowhere", ActionDelete)}, newFolders(), nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClassifier(lookup, "camp-1")
			if tt.folders != nil {
				c.WithFolders(tt.folders, tt.pending)
			}
			cls, err := c.ClassifyAll(context.Background(), tt.pages)
			if err != nil {
				t.Fatal(err)
			}
			got := cls[0]
			if tt.want == "" {
				if got.Status == StatusParentProblem {
					t.Fatalf("unexpected problem: %s", got.Reason)
				}
				return
			}
			if got.Status != StatusParentProblem || !strings.Contains(got.Reason, tt.want) {
				t.Fatalf("status %q reason %q, want problem containing %q", got.Status, got.Reason, tt.want)
			}
		})
	}
}

func TestCommit_Folder(t *testing.T) {
	types := []entities.EntityType{{ID: 1, Name: "Lore", Slug: "lore", Enabled: true}}
	existing := func() map[string]*entities.Entity {
		return map[string]*entities.Entity{
			"hera": {ID: "ent-hera", Name: "Hera", Slug: "hera", EntityTypeID: 1, CampaignID: "camp-1"},
		}
	}
	tests := []struct {
		name       string
		pages      []ParsedPage
		decisions  []RowDecision
		folders    FolderAccess
		wantStatus RowStatus
		wantReason string
		check      func(t *testing.T, f *fakeCreator, fa *fakeFolderAccess)
	}{
		{
			name:       "create files the page",
			pages:      []ParsedPage{withFolder("Zeus", "GODS", "")},
			decisions:  []RowDecision{decision(true, "Zeus", "lore", "private", "rename")},
			wantStatus: StatusCreated,
			check: func(t *testing.T, _ *fakeCreator, fa *fakeFolderAccess) {
				if got := fa.sets["ent-zeus"]; got == nil || *got != "f-gods" {
					t.Errorf("filed in %v, want f-gods", got)
				}
			},
		},
		{
			name:       "missing folder creates nothing",
			pages:      []ParsedPage{withFolder("Zeus", "Titans", "")},
			decisions:  []RowDecision{decision(true, "Zeus", "lore", "private", "rename")},
			wantStatus: StatusFailed,
			wantReason: `No folder called "Titans"`,
			check: func(t *testing.T, f *fakeCreator, fa *fakeFolderAccess) {
				if len(f.createCalls) != 0 || len(fa.sets) != 0 {
					t.Errorf("creates %d, folder writes %d; want none", len(f.createCalls), len(fa.sets))
				}
			},
		},
		{
			name:       "ambiguous folder creates nothing",
			pages:      []ParsedPage{withFolder("Zeus", "Twin", "")},
			decisions:  []RowDecision{decision(true, "Zeus", "lore", "private", "rename")},
			wantStatus: StatusFailed,
			wantReason: "more than one folder",
			check: func(t *testing.T, f *fakeCreator, _ *fakeFolderAccess) {
				if len(f.createCalls) != 0 {
					t.Error("page must not be created")
				}
			},
		},
		{
			name:       "no folder key writes nothing about folders",
			pages:      []ParsedPage{withFolder("Zeus", "", "")},
			decisions:  []RowDecision{decision(true, "Zeus", "lore", "private", "rename")},
			wantStatus: StatusCreated,
			check: func(t *testing.T, _ *fakeCreator, fa *fakeFolderAccess) {
				if len(fa.sets) != 0 {
					t.Errorf("folder writes = %v", fa.sets)
				}
			},
		},
		{
			name:       "update moves into a folder",
			pages:      []ParsedPage{withFolder("Hera", "Gods", ActionUpdate)},
			decisions:  []RowDecision{decisionWithAction(true, "Hera", "lore", "", "rename", ActionUpdate, false)},
			wantStatus: StatusUpdated,
			check: func(t *testing.T, _ *fakeCreator, fa *fakeFolderAccess) {
				if got := fa.sets["ent-hera"]; got == nil || *got != "f-gods" {
					t.Errorf("filed in %v", got)
				}
			},
		},
		{
			name:       "update with folder none takes the page out",
			pages:      []ParsedPage{withFolder("Hera", "none", ActionUpdate)},
			decisions:  []RowDecision{decisionWithAction(true, "Hera", "lore", "", "rename", ActionUpdate, false)},
			wantStatus: StatusUpdated,
			check: func(t *testing.T, _ *fakeCreator, fa *fakeFolderAccess) {
				got, wrote := fa.sets["ent-hera"]
				if !wrote || got != nil {
					t.Errorf("sets = %v, want an explicit take-out", fa.sets)
				}
			},
		},
		{
			name:       "update without the key keeps the folder",
			pages:      []ParsedPage{withFolder("Hera", "", ActionUpdate)},
			decisions:  []RowDecision{decisionWithAction(true, "Hera", "lore", "", "rename", ActionUpdate, false)},
			wantStatus: StatusUpdated,
			check: func(t *testing.T, _ *fakeCreator, fa *fakeFolderAccess) {
				if len(fa.sets) != 0 {
					t.Errorf("folder writes = %v", fa.sets)
				}
			},
		},
		{
			name:       "no folder service refuses the row",
			pages:      []ParsedPage{withFolder("Zeus", "Gods", "")},
			decisions:  []RowDecision{decision(true, "Zeus", "lore", "private", "rename")},
			folders:    nilFolders,
			wantStatus: StatusFailed,
			wantReason: "Folders can't be set",
			check:      func(t *testing.T, f *fakeCreator, _ *fakeFolderAccess) {},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeCreator{types: types, existing: existing()}
			fa := newFolders()
			c := NewCommitter(f)
			if tt.folders == nil {
				c.SetFolders(fa)
			}
			res, err := c.Commit(context.Background(), "camp-1", CommitInput{
				OwnerID: "u-1", Pages: tt.pages, Decisions: tt.decisions,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := res.Rows[0]; got.Status != tt.wantStatus || !strings.Contains(got.Reason, tt.wantReason) {
				t.Fatalf("status %q (%s), want %q containing %q", got.Status, got.Reason, tt.wantStatus, tt.wantReason)
			}
			tt.check(t, f, fa)
		})
	}
}

// nilFolders stands for "no folder service wired".
var nilFolders FolderAccess = nilFolderAccess{}

type nilFolderAccess struct{}

func (nilFolderAccess) FindFolder(context.Context, string, int, string) (*entities.SidebarNode, error) {
	return nil, nil
}
func (nilFolderAccess) SetEntityFolder(context.Context, string, string, *string) error { return nil }

// A parent given to an update takes the page out of its folder, since a page
// is under a page or in a folder.
func TestCommit_ParentUpdateLeavesFolder(t *testing.T) {
	f := &fakeCreator{
		types: []entities.EntityType{{ID: 1, Name: "Lore", Slug: "lore", Enabled: true}},
		existing: map[string]*entities.Entity{
			"hera":    {ID: "ent-hera", Name: "Hera", Slug: "hera", EntityTypeID: 1, CampaignID: "camp-1"},
			"olympus": {ID: "ent-olympus", Name: "Olympus", Slug: "olympus", EntityTypeID: 1},
		},
	}
	fa := newFolders()
	c := NewCommitter(f)
	c.SetFolders(fa)
	p := withFolder("Hera", "", ActionUpdate)
	p.FrontMatter.Parent = "Olympus"
	res, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1", Pages: []ParsedPage{p},
		Decisions: []RowDecision{decisionWithAction(true, "Hera", "lore", "", "rename", ActionUpdate, false)},
	})
	if err != nil || res.Rows[0].Status != StatusUpdated {
		t.Fatalf("err %v, row %+v", err, res.Rows[0])
	}
	if got, wrote := fa.sets["ent-hera"]; !wrote || got != nil {
		t.Errorf("sets = %v, want a take-out", fa.sets)
	}
}

// A folder named under the wrong category says where the folder is, since
// folders belong to one category and the owner can pick that one instead.
func TestClassify_FolderInAnotherCategory(t *testing.T) {
	lookup := &stubLookup{types: []entities.EntityType{
		{ID: 1, Slug: "lore", Name: "Lore"}, {ID: 2, Slug: "places", Name: "Places"}, {ID: 3, Slug: "races", Name: "Races"},
	}}
	inRaces := func(folder string) ParsedPage {
		p := page("The Khepri", "races", "# The Khepri\nBody.")
		p.FrontMatter.Folder = folder
		return p
	}
	tests := []struct {
		name    string
		folder  string
		pending map[string]bool
		want    string
		notWant string
	}{
		{"folder in another category", "Gods", nil, `No folder called "Gods" exists in the Races category or is made in this paste. There is one in Lore: under Change the details, pick that category, then answer Yes.`, ""},
		{"made in this paste elsewhere", "Titans", map[string]bool{"places|titans": true}, "There is one in Places:", ""},
		{"in two other categories", "Gods", map[string]bool{"places|gods": true}, "There is one in Lore or Places:", ""},
		{"nowhere", "Nowhere", nil, "No folder called", "There is one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClassifier(lookup, "camp-1").WithFolders(newFolders(), tt.pending)
			cls, err := c.ClassifyAll(context.Background(), []ParsedPage{inRaces(tt.folder)})
			if err != nil {
				t.Fatal(err)
			}
			got := cls[0]
			if got.Status != StatusParentProblem || !strings.Contains(got.Reason, tt.want) {
				t.Fatalf("status %q reason %q, want %q", got.Status, got.Reason, tt.want)
			}
			if tt.notWant != "" && strings.Contains(got.Reason, tt.notWant) {
				t.Errorf("reason %q should not say %q", got.Reason, tt.notWant)
			}
		})
	}
}
