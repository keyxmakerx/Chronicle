// era_test.go covers `era:` / `known_as:` blocks: how they parse, how the
// review classifies them and what the committer writes, with stub
// interfaces for the calendar side.

package importer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func TestParse_EraBlocks(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantStatus  ParseStatus
		wantEra     string
		wantKnownAs string
		wantErr     string
		wantWarn    string
	}{
		{
			name:       "era with known_as",
			in:         "---\nname: Stonehold\nera: Age of Ash\nknown_as: Ashen Keep\n---\n\nBurnt.",
			wantStatus: StatusNew, wantEra: "Age of Ash", wantKnownAs: "Ashen Keep",
		},
		{
			name:       "era alone, empty body is fine",
			in:         "---\nname: Stonehold\nera: Age of Ash\n---\n",
			wantStatus: StatusNew, wantEra: "Age of Ash",
		},
		{
			name:    "known_as without era",
			in:      "---\nname: Stonehold\nknown_as: Ashen Keep\n---\n\nBody.",
			wantErr: "known_as: only works together with era:", wantStatus: StatusParseError,
		},
		{
			name:    "known_as with blank era",
			in:      "---\nname: Stonehold\nera: \"  \"\nknown_as: Ashen Keep\n---\n\nBody.",
			wantErr: "known_as: only works together with era:", wantStatus: StatusParseError,
		},
		{
			name:       "page keys are ignored with a warning",
			in:         "---\nname: Stonehold\nera: Age of Ash\ntype: location\nvisibility: public\nparent: Realm\ntags: [a]\n---\n\nBody.",
			wantStatus: StatusNew, wantEra: "Age of Ash", wantWarn: "ignored: type, visibility, parent, tags",
		},
		{
			name:       "a bad visibility does not block an era block",
			in:         "---\nname: Stonehold\nera: Age of Ash\nvisibility: everyone\n---\n\nBody.",
			wantStatus: StatusNew, wantEra: "Age of Ash", wantWarn: "visibility",
		},
		{
			name:       "no unknown-key warning for the new keys",
			in:         "---\nname: Stonehold\nera: Age of Ash\nknown_as: X\n---\n\nBody.",
			wantStatus: StatusNew, wantEra: "Age of Ash", wantKnownAs: "X",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pages := Parse(tt.in)
			if len(pages) != 1 {
				t.Fatalf("got %d pages", len(pages))
			}
			p := pages[0]
			if p.Status != tt.wantStatus {
				t.Fatalf("status = %q (%s), want %q", p.Status, p.ParseError, tt.wantStatus)
			}
			if tt.wantErr != "" && !strings.Contains(p.ParseError, tt.wantErr) {
				t.Errorf("ParseError = %q, want it to contain %q", p.ParseError, tt.wantErr)
			}
			if p.FrontMatter.EraName() != tt.wantEra {
				t.Errorf("era = %q, want %q", p.FrontMatter.EraName(), tt.wantEra)
			}
			if tt.wantStatus == StatusNew && p.FrontMatter.KnownAs != tt.wantKnownAs {
				t.Errorf("known_as = %q, want %q", p.FrontMatter.KnownAs, tt.wantKnownAs)
			}
			joined := strings.Join(p.Warnings, "|")
			if tt.wantWarn != "" && !strings.Contains(joined, tt.wantWarn) {
				t.Errorf("warnings %q lack %q", joined, tt.wantWarn)
			}
			if tt.wantWarn == "" && tt.wantStatus == StatusNew && joined != "" {
				t.Errorf("unexpected warnings: %q", joined)
			}
		})
	}
}

// fakeEras is the calendar side of an import: a fixed era list that records
// what the committer asked it to write.
type fakeEras struct {
	eras     []EraRef
	listErr  error
	writeErr error
	writes   []eraWriteCall
	deletes  []eraDeleteCall
}

type eraWriteCall struct {
	EntityID string
	EraID    int
	UserID   string
	In       EraVersionWrite
}

type eraDeleteCall struct {
	EntityID string
	EraID    int
}

func (f *fakeEras) Eras(_ context.Context, _ string, _ Viewer) ([]EraRef, error) {
	return f.eras, f.listErr
}

func (f *fakeEras) WriteVersion(_ context.Context, _, entityID string, eraID int, userID string, in EraVersionWrite) error {
	f.writes = append(f.writes, eraWriteCall{entityID, eraID, userID, in})
	return f.writeErr
}

func (f *fakeEras) DeleteVersion(_ context.Context, _, entityID string, eraID int) error {
	f.deletes = append(f.deletes, eraDeleteCall{entityID, eraID})
	return nil
}

func eraPage(name, era, knownAs, body string) ParsedPage {
	p := page(name, "", body)
	p.FrontMatter.Era, p.FrontMatter.KnownAs = era, knownAs
	return p
}

var testEras = []EraRef{{ID: 7, Name: "Age of Ash"}, {ID: 8, Name: "Golden Age"}}

func TestClassify_EraRows(t *testing.T) {
	lookup := &stubLookup{
		bySlug: map[string]*entities.Entity{"stonehold": {ID: "ent-stonehold", Name: "Stonehold", Slug: "stonehold"}},
		types:  []entities.EntityType{{ID: 1, Slug: "location", Name: "Location"}},
	}
	del := page("Stonehold", "", "")
	del.FrontMatter.Action = ActionDelete
	tests := []struct {
		name       string
		pages      []ParsedPage
		eras       EraLookup
		row        int
		wantStatus ParseStatus
		wantReason string
	}{
		{"existing page", []ParsedPage{eraPage("Stonehold", "Age of Ash", "", "x")}, &fakeEras{eras: testEras}, 0, StatusNew, ""},
		{"era matched ignoring case", []ParsedPage{eraPage("Stonehold", "age of ASH", "", "x")}, &fakeEras{eras: testEras}, 0, StatusNew, ""},
		{
			"page created earlier in the paste",
			[]ParsedPage{page("Newtown", "location", "x"), eraPage("Newtown", "Golden Age", "", "x")},
			&fakeEras{eras: testEras}, 1, StatusNew, "",
		},
		{
			"missing page",
			[]ParsedPage{eraPage("Nowhere", "Age of Ash", "", "x")},
			&fakeEras{eras: testEras}, 0, StatusActionMismatch, "Create the page first, in a block without era:",
		},
		{
			"page only appears later in the paste",
			[]ParsedPage{eraPage("Newtown", "Golden Age", "", "x"), page("Newtown", "location", "x")},
			&fakeEras{eras: testEras}, 0, StatusActionMismatch, "Create the page first",
		},
		{
			"an earlier era block does not create the page",
			[]ParsedPage{eraPage("Newtown", "Golden Age", "", "x"), eraPage("Newtown", "Age of Ash", "", "x")},
			&fakeEras{eras: testEras}, 1, StatusActionMismatch, "Create the page first",
		},
		{
			"page deleted earlier in the paste",
			[]ParsedPage{del, eraPage("Stonehold", "Age of Ash", "", "x")},
			&fakeEras{eras: testEras}, 1, StatusActionMismatch, "being deleted",
		},
		{
			"unknown era names the era and lists the owner's eras",
			[]ParsedPage{eraPage("Stonehold", "Age of Glass", "", "x")},
			&fakeEras{eras: testEras}, 0, StatusActionMismatch, `"Age of Glass". Its eras are: Age of Ash, Golden Age`,
		},
		{
			"calendar without eras",
			[]ParsedPage{eraPage("Stonehold", "Age of Ash", "", "x")},
			&fakeEras{}, 0, StatusActionMismatch, "has no eras",
		},
		{
			"era read fails",
			[]ParsedPage{eraPage("Stonehold", "Age of Ash", "", "x")},
			&fakeEras{listErr: errors.New("db down")}, 0, StatusActionMismatch, "Could not read the main calendar's eras",
		},
		{
			"no era lookup wired",
			[]ParsedPage{eraPage("Stonehold", "Age of Ash", "", "x")},
			nil, 0, StatusActionMismatch, "cannot write pages by era",
		},
		{
			"parent on an era row is not checked",
			[]ParsedPage{func() ParsedPage {
				p := eraPage("Stonehold", "Age of Ash", "", "x")
				p.FrontMatter.Parent = "Nowhere"
				return p
			}()},
			&fakeEras{eras: testEras}, 0, StatusNew, "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClassifier(lookup, "camp-1")
			if tt.eras != nil {
				c.WithEras(tt.eras, Viewer{Role: 3, UserID: "u-1"})
			}
			cls, err := c.ClassifyAll(context.Background(), tt.pages)
			if err != nil {
				t.Fatal(err)
			}
			got := cls[tt.row]
			if got.Status != tt.wantStatus {
				t.Fatalf("status = %q (%s), want %q", got.Status, got.Reason, tt.wantStatus)
			}
			if !strings.Contains(got.Reason, tt.wantReason) {
				t.Errorf("reason = %q, want it to contain %q", got.Reason, tt.wantReason)
			}
		})
	}
}

func TestCommit_EraRows(t *testing.T) {
	stonehold := &entities.Entity{ID: "ent-stonehold", Name: "Stonehold", Slug: "stonehold"}
	tests := []struct {
		name       string
		pages      []ParsedPage
		decisions  []RowDecision
		eras       *fakeEras
		noWriter   bool
		wantStatus RowStatus
		wantReason string
		check      func(t *testing.T, f *fakeCreator, e *fakeEras)
	}{
		{
			name:       "writes name and text",
			pages:      []ParsedPage{eraPage("Stonehold", "Age of Ash", "Ashen Keep", "Burnt <script>x()</script>.")},
			decisions:  []RowDecision{{Include: true, Name: "Stonehold", Action: ActionCreate}},
			eras:       &fakeEras{eras: testEras},
			wantStatus: StatusUpdated, wantReason: "Wrote the Age of Ash version",
			check: func(t *testing.T, f *fakeCreator, e *fakeEras) {
				if len(e.writes) != 1 {
					t.Fatalf("writes = %d, want 1", len(e.writes))
				}
				w := e.writes[0]
				if w.EntityID != "ent-stonehold" || w.EraID != 7 || w.UserID != "u-1" || w.In.KnownAs != "Ashen Keep" {
					t.Errorf("write = %+v", w)
				}
				if !strings.Contains(w.In.EntryHTML, "Burnt") || strings.Contains(strings.ToLower(w.In.EntryHTML), "<script") {
					t.Errorf("EntryHTML not converted and sanitized: %q", w.In.EntryHTML)
				}
				if w.In.EntryJSON == "" {
					t.Error("EntryJSON empty")
				}
				if len(f.createCalls)+len(f.updateCalls)+len(f.updateEntries)+len(f.deleteCalls) != 0 {
					t.Errorf("usual page was touched: %+v", f)
				}
			},
		},
		{
			name:       "empty body leaves the text alone",
			pages:      []ParsedPage{eraPage("Stonehold", "golden age", "The Golden Keep", "")},
			decisions:  []RowDecision{{Include: true, Name: "Stonehold", Action: ActionUpdate}},
			eras:       &fakeEras{eras: testEras},
			wantStatus: StatusUpdated,
			check: func(t *testing.T, _ *fakeCreator, e *fakeEras) {
				w := e.writes[0]
				if w.EraID != 8 || w.In.KnownAs != "The Golden Keep" || w.In.EntryJSON != "" || w.In.EntryHTML != "" {
					t.Errorf("write = %+v", w)
				}
			},
		},
		{
			name:       "delete removes only the version",
			pages:      []ParsedPage{eraPage("Stonehold", "Age of Ash", "", "")},
			decisions:  []RowDecision{{Include: true, Name: "Stonehold", Action: ActionDelete, DeleteConfirmed: true}},
			eras:       &fakeEras{eras: testEras},
			wantStatus: StatusUpdated, wantReason: "Removed the Age of Ash version",
			check: func(t *testing.T, f *fakeCreator, e *fakeEras) {
				if len(e.deletes) != 1 || e.deletes[0] != (eraDeleteCall{"ent-stonehold", 7}) {
					t.Errorf("deletes = %+v", e.deletes)
				}
				if len(f.deleteCalls) != 0 {
					t.Errorf("the page itself was deleted: %v", f.deleteCalls)
				}
			},
		},
		{
			name:       "delete needs its confirmation",
			pages:      []ParsedPage{eraPage("Stonehold", "Age of Ash", "", "")},
			decisions:  []RowDecision{{Include: true, Name: "Stonehold", Action: ActionDelete}},
			eras:       &fakeEras{eras: testEras},
			wantStatus: StatusFailed, wantReason: "not confirmed",
			check: func(t *testing.T, _ *fakeCreator, e *fakeEras) {
				if len(e.deletes) != 0 {
					t.Errorf("deleted without confirmation: %+v", e.deletes)
				}
			},
		},
		{
			name:       "unknown era",
			pages:      []ParsedPage{eraPage("Stonehold", "Age of Glass", "", "x")},
			decisions:  []RowDecision{{Include: true, Name: "Stonehold", Action: ActionCreate}},
			eras:       &fakeEras{eras: testEras},
			wantStatus: StatusFailed, wantReason: `"Age of Glass". Its eras are: Age of Ash, Golden Age`,
			check: func(t *testing.T, _ *fakeCreator, e *fakeEras) {
				if len(e.writes) != 0 {
					t.Errorf("wrote for an unknown era: %+v", e.writes)
				}
			},
		},
		{
			name:       "unwired writer",
			pages:      []ParsedPage{eraPage("Stonehold", "Age of Ash", "", "x")},
			decisions:  []RowDecision{{Include: true, Name: "Stonehold", Action: ActionCreate}},
			noWriter:   true,
			wantStatus: StatusFailed, wantReason: "cannot write pages by era",
		},
		{
			name:       "page missing",
			pages:      []ParsedPage{eraPage("Nowhere", "Age of Ash", "", "x")},
			decisions:  []RowDecision{{Include: true, Name: "Nowhere", Action: ActionCreate}},
			eras:       &fakeEras{eras: testEras},
			wantStatus: StatusFailed, wantReason: "Create the page first",
		},
		{
			name:       "writer failure shows a plain reason",
			pages:      []ParsedPage{eraPage("Stonehold", "Age of Ash", "", "x")},
			decisions:  []RowDecision{{Include: true, Name: "Stonehold", Action: ActionCreate}},
			eras:       &fakeEras{eras: testEras, writeErr: errors.New("sql: connection refused")},
			wantStatus: StatusFailed, wantReason: "Could not write the Age of Ash version",
		},
		{
			name:       "excluded row is skipped",
			pages:      []ParsedPage{eraPage("Stonehold", "Age of Ash", "", "x")},
			decisions:  []RowDecision{{Include: false, Name: "Stonehold"}},
			eras:       &fakeEras{eras: testEras},
			wantStatus: StatusSkipped,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeCreator{existing: map[string]*entities.Entity{"stonehold": stonehold}}
			c := NewCommitter(f)
			if !tt.noWriter {
				c.SetEraWriter(tt.eras)
			}
			res, err := c.Commit(context.Background(), "camp-1", CommitInput{
				OwnerID: "u-1", Pages: tt.pages, Decisions: tt.decisions,
			})
			if err != nil {
				t.Fatal(err)
			}
			row := res.Rows[0]
			if row.Status != tt.wantStatus {
				t.Fatalf("status = %q (%s), want %q", row.Status, row.Reason, tt.wantStatus)
			}
			if !strings.Contains(row.Reason, tt.wantReason) {
				t.Errorf("reason = %q, want it to contain %q", row.Reason, tt.wantReason)
			}
			if tt.wantStatus == StatusUpdated && res.Updated != 1 {
				t.Errorf("Updated = %d, want 1", res.Updated)
			}
			if tt.check != nil {
				tt.check(t, f, tt.eras)
			}
		})
	}
}

// A version block for a page created earlier in the same paste writes to that
// new page, even when the page was saved under a renamed slug, and the era
// block never turns into a body write on the usual page.
func TestCommit_EraRowFollowsPageCreatedInPaste(t *testing.T) {
	f := &fakeCreator{
		types:    []entities.EntityType{{ID: 1, Name: "Location", Slug: "location", Enabled: true}},
		existing: map[string]*entities.Entity{"newtown": {ID: "ent-old", Name: "Newtown", Slug: "newtown"}},
	}
	eras := &fakeEras{eras: testEras}
	c := NewCommitter(f)
	c.SetEraWriter(eras)
	res, err := c.Commit(context.Background(), "camp-1", CommitInput{
		OwnerID: "u-1",
		Pages: []ParsedPage{
			page("Newtown", "location", "Usual text."),
			eraPage("Newtown", "Age of Ash", "", "Ash text."),
		},
		Decisions: []RowDecision{
			decision(true, "Newtown", "location", "private", "rename"),
			{Include: true, Name: "Newtown", Action: ActionCreate},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows[0].Status != StatusRenamed || res.Rows[1].Status != StatusUpdated {
		t.Fatalf("rows = %+v", res.Rows)
	}
	if len(eras.writes) != 1 || eras.writes[0].EntityID != "ent-newtown-(imported)" {
		t.Errorf("version went to %+v, want the renamed new page", eras.writes)
	}
	if len(f.updateEntries) != 1 || !strings.Contains(f.updateEntries[0].EntryHTML, "Usual text.") {
		t.Errorf("usual page body writes = %+v", f.updateEntries)
	}
}
