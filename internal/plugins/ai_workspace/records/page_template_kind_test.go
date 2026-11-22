package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fakeTemplates is a template every campaign shares (id 1), a Tavern template
// for Locations (id 2) written with a grey writing prompt, and a Rumours
// template for every category (id 3).
type fakeTemplates struct {
	owner   string
	types   []entities.EntityType
	list    []entities.ContentTemplate
	nextID  int
	created []entities.CreateContentTemplateInput
	updates []entities.UpdateContentTemplateInput
	deleted []int
}

func newTemplatesFake() *fakeTemplates {
	return &fakeTemplates{owner: "own", nextID: 10,
		types: []entities.EntityType{
			{ID: 1, Name: "Character", NamePlural: "Characters", Slug: "character"},
			{ID: 2, Name: "Location", NamePlural: "Locations", Slug: "location"},
		},
		list: []entities.ContentTemplate{
			{ID: 1, Name: "Session Recap", Description: "Shared.", Icon: "fa-scroll", IsGlobal: true,
				ContentJSON: `{"type":"doc"}`, ContentHTML: "<h2>Summary</h2><p>Words</p>"},
			{ID: 2, CampaignID: strp("c"), EntityTypeID: intp(2), Name: "Tavern", Description: "A place to drink.", Icon: "fa-beer-mug-empty",
				ContentJSON: `{"type":"doc","content":[{"type":"paragraph","attrs":{"prompt":"Who is here?"}}]}`,
				ContentHTML: "<h2>The room</h2><ul><li>Bar</li><li>Stairs</li></ul>"},
			{ID: 3, CampaignID: strp("c"), Name: "Rumours", Icon: "fa-file-lines", ContentJSON: `{"type":"doc"}`, ContentHTML: "<p>Heard <strong>in town</strong></p>"},
		},
	}
}

func (f *fakeTemplates) IsOwner(_ context.Context, _, u string) (bool, error) {
	return u == f.owner, nil
}
func (f *fakeTemplates) Categories(context.Context, string) ([]entities.EntityType, error) {
	return f.types, nil
}
func (f *fakeTemplates) List(context.Context, string) ([]entities.ContentTemplate, error) {
	return append([]entities.ContentTemplate(nil), f.list...), nil
}
func (f *fakeTemplates) Create(_ context.Context, _ string, in entities.CreateContentTemplateInput) (*entities.ContentTemplate, error) {
	f.created = append(f.created, in)
	f.nextID++
	t := entities.ContentTemplate{ID: f.nextID, CampaignID: strp("c"), Name: in.Name, Description: in.Description, Icon: in.Icon, ContentJSON: in.ContentJSON, ContentHTML: in.ContentHTML}
	if in.EntityTypeID > 0 {
		t.EntityTypeID = intp(in.EntityTypeID)
	}
	f.list = append(f.list, t)
	return &t, nil
}
func (f *fakeTemplates) Update(_ context.Context, _ string, id int, in entities.UpdateContentTemplateInput) error {
	f.updates = append(f.updates, in)
	for i := range f.list {
		if f.list[i].ID == id {
			t := &f.list[i]
			t.Name, t.Description = in.Name.Val(t.Name), in.Description.Val(t.Description)
			t.Icon, t.ContentJSON, t.ContentHTML = in.Icon.Val(t.Icon), in.ContentJSON.Val(t.ContentJSON), in.ContentHTML.Val(t.ContentHTML)
		}
	}
	return nil
}
func (f *fakeTemplates) Delete(_ context.Context, _ string, id int) error {
	f.deleted = append(f.deleted, id)
	for i := range f.list {
		if f.list[i].ID == id {
			f.list = append(f.list[:i], f.list[i+1:]...)
			break
		}
	}
	return nil
}

// entryText renders markdown as a stand-in editor document and its HTML.
type entryText struct{ mdText }

func (entryText) Entry(b string) (*string, *string, error) {
	if strings.TrimSpace(b) == "" {
		return nil, nil, nil
	}
	doc, html := `{"type":"doc","md":"`+b+`"}`, "<p>"+b+"</p>"
	return &doc, &html, nil
}

func tplRec(action, name string, fields map[string]any, body string) Record {
	f := map[string]any{"name": name}
	for k, v := range fields {
		f[k] = v
	}
	return Record{Kind: "page-template", Action: action, Name: name, Fields: f, Body: body, Text: entryText{}}
}

func TestPageTemplateKind_Plan(t *testing.T) {
	cases := []struct {
		name    string
		a       Actor
		rec     Record
		want    []string
		wantErr string
	}{
		{name: "create for a category", a: layoutOwner, rec: tplRec("create", "Inn", map[string]any{"category": "Locations"}, "## Rooms"),
			want: []string{`adds the page template "Inn" for Locations: "## Rooms"`}},
		{name: "create for every category", a: layoutOwner, rec: tplRec("create", "Clue", nil, "Find out why"), want: []string{`adds the page template "Clue": `}},
		{name: "same name in another category is a new template", a: layoutOwner, rec: tplRec("create", "Tavern", map[string]any{"category": "Characters"}, "Barkeep"),
			want: []string{`adds the page template "Tavern" for Characters`}},
		{name: "create twice", a: layoutOwner, rec: tplRec("create", "tavern", map[string]any{"category": "Locations"}, "x"), wantErr: "already exists; use action: update"},
		{name: "create without words", a: layoutOwner, rec: tplRec("create", "Inn", nil, "  "), wantErr: "write the template's words"},
		{name: "create with rename_to", a: layoutOwner, rec: tplRec("create", "Inn", map[string]any{"rename_to": "X"}, "x"), wantErr: "nothing to rename"},
		{name: "unknown category", a: layoutOwner, rec: tplRec("create", "Inn", map[string]any{"category": "Ships"}, "x"), wantErr: "no category called"},
		{name: "bad icon", a: layoutOwner, rec: tplRec("create", "Inn", map[string]any{"icon": "<b>"}, "x"), wantErr: "Font Awesome"},
		{name: "words too long", a: layoutOwner, rec: tplRec("create", "Inn", nil, strings.Repeat("x", maxPageTemplateText+1)), wantErr: "run over 20000"},
		{name: "a GM-only marker", a: layoutOwner, rec: tplRec("create", "Inn", nil, "{gm{the baron is the killer}gm}"), wantErr: "can't hold GM-only text"},
		{name: "rewrite with prompts", a: layoutOwner, rec: tplRec("update", "Tavern", nil, "## The room\n\n- Cellar"),
			want: []string{"has its text replaced", "its grey writing prompts are replaced"}},
		{name: "text sent back as read is no change", a: layoutOwner, rec: tplRec("update", "Tavern", nil, writtenMarkdown("<h2>The room</h2><ul><li>Bar</li><li>Stairs</li></ul>")), wantErr: "already like this"},
		{name: "rename and describe", a: layoutOwner, rec: tplRec("update", "Tavern", map[string]any{"rename_to": "Inn", "description": "Rooms"}, ""),
			want: []string{`is renamed "Inn"`, `gets the description "Rooms"`}},
		{name: "category named for a template that has another", a: layoutOwner, rec: tplRec("update", "Tavern", map[string]any{"category": "Characters", "description": "x"}, ""),
			wantErr: "category can't be changed once it is made"},
		{name: "category named and matching", a: layoutOwner, rec: tplRec("update", "Tavern", map[string]any{"category": "Locations", "description": "x"}, ""), want: []string{"gets the description"}},
		{name: "update that does not exist", a: layoutOwner, rec: tplRec("update", "Inn", nil, "x"), wantErr: "use action: create"},
		{name: "shared update", a: layoutOwner, rec: tplRec("update", "Session Recap", nil, "x"), wantErr: "shared by every campaign"},
		{name: "shared delete", a: layoutOwner, rec: tplRec("delete", "Session Recap", nil, ""), wantErr: "shared by every campaign"},
		{name: "delete", a: layoutOwner, rec: tplRec("delete", "Tavern", nil, ""), want: []string{`removes the page template "Tavern"`, "keep their text"}},
		{name: "delete that does not exist", a: layoutOwner, rec: tplRec("delete", "Inn", nil, ""), wantErr: "no page template called"},
		{name: "a co-DM is refused", a: layoutCoDM, rec: tplRec("delete", "Tavern", nil, ""), wantErr: "only the campaign's owner"},
		{name: "a player is refused", a: Actor{UserID: "pl", Role: 1}, rec: tplRec("create", "Inn", nil, "x"), wantErr: "only the campaign's owner"},
		{name: "no name", a: layoutOwner, rec: tplRec("create", "", nil, "x"), wantErr: "name the page template"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := PageTemplateKind{Svc: newTemplatesFake()}.Plan(context.Background(), "c", tc.a, tc.rec)
			if tc.wantErr != "" {
				if !strings.Contains(p.Error, tc.wantErr) {
					t.Fatalf("error = %q, want %q", p.Error, tc.wantErr)
				}
				return
			}
			if p.Error != "" {
				t.Fatal(p.Error)
			}
			all := p.Summary + " " + strings.Join(p.Warnings, " ")
			for _, w := range tc.want {
				if !strings.Contains(all, w) {
					t.Errorf("missing %q in %q", w, all)
				}
			}
		})
	}
}

// Two templates may share a name when they are for different categories; a
// record that does not say which is refused.
func TestPageTemplateKind_AmbiguousName(t *testing.T) {
	f := newTemplatesFake()
	f.list = append(f.list, entities.ContentTemplate{ID: 4, CampaignID: strp("c"), Name: "Tavern", ContentJSON: "{}", ContentHTML: "<p>x</p>"})
	k := PageTemplateKind{Svc: f}
	if p := k.Plan(context.Background(), "c", layoutOwner, tplRec("delete", "Tavern", nil, "")); !strings.Contains(p.Error, "add `category:`") {
		t.Errorf("plan = %+v", p)
	}
	if p := k.Plan(context.Background(), "c", layoutOwner, tplRec("delete", "Tavern", map[string]any{"category": "Locations"}, "")); p.Error != "" {
		t.Errorf("plan = %+v", p)
	}
}

func TestPageTemplateKind_ApplyCreate(t *testing.T) {
	f := newTemplatesFake()
	rec := tplRec("create", "Inn", map[string]any{"category": "Locations", "description": "Rooms", "icon": "fa-bed"}, "## Rooms")
	if err := (PageTemplateKind{Svc: f}).Apply(context.Background(), "c", layoutOwner, rec); err != nil {
		t.Fatal(err)
	}
	if len(f.created) != 1 {
		t.Fatalf("created %v", f.created)
	}
	in := f.created[0]
	if in.Name != "Inn" || in.EntityTypeID != 2 || in.Description != "Rooms" || in.Icon != "fa-bed" ||
		!strings.Contains(in.ContentJSON, "## Rooms") || in.ContentHTML != "<p>## Rooms</p>" {
		t.Errorf("input %+v", in)
	}
}

// An update sends only what changes: a rename never rewrites the text, and new
// text never touches the name or description.
func TestPageTemplateKind_ApplyUpdateSendsOnlyChanges(t *testing.T) {
	ctx := context.Background()
	f := newTemplatesFake()
	k := PageTemplateKind{Svc: f}
	if err := k.Apply(ctx, "c", layoutOwner, tplRec("update", "Tavern", map[string]any{"rename_to": "Inn"}, "")); err != nil {
		t.Fatal(err)
	}
	in := f.updates[0]
	if v, _ := in.Name.Get(); v != "Inn" || in.ContentJSON.Present() || in.ContentHTML.Present() || in.Description.Present() || in.Icon.Present() {
		t.Errorf("rename update = %+v", in)
	}
	if err := k.Apply(ctx, "c", layoutOwner, tplRec("update", "Inn", nil, "New words")); err != nil {
		t.Fatal(err)
	}
	in = f.updates[1]
	if h, _ := in.ContentHTML.Get(); h != "<p>New words</p>" || !in.ContentJSON.Present() || in.Name.Present() || in.Description.Present() {
		t.Errorf("text update = %+v", in)
	}
	// Words sent back as they were read leave the stored text, prompts and
	// all, alone.
	f2 := newTemplatesFake()
	same := tplRec("update", "Tavern", map[string]any{"description": "Rooms"}, writtenMarkdown(f2.list[1].ContentHTML))
	if err := (PageTemplateKind{Svc: f2}).Apply(ctx, "c", layoutOwner, same); err != nil {
		t.Fatal(err)
	}
	if in := f2.updates[0]; in.ContentJSON.Present() || in.ContentHTML.Present() {
		t.Errorf("unchanged words were rewritten: %+v", in)
	}
}

func TestPageTemplateKind_ApplySharedNeverChanges(t *testing.T) {
	f := newTemplatesFake()
	k := PageTemplateKind{Svc: f}
	for _, rec := range []Record{tplRec("update", "Session Recap", nil, "x"), tplRec("delete", "Session Recap", nil, "")} {
		if err := k.Apply(context.Background(), "c", layoutOwner, rec); err == nil {
			t.Errorf("%s of a shared template went ahead", rec.Action)
		}
	}
	if len(f.updates)+len(f.deleted)+len(f.created) != 0 {
		t.Errorf("the service was called: %+v", f)
	}
}

func TestPageTemplateKind_Undo(t *testing.T) {
	ctx := context.Background()
	t.Run("a made template is removed", func(t *testing.T) {
		f := newTemplatesFake()
		k := PageTemplateKind{Svc: f}
		rec := tplRec("create", "Inn", map[string]any{"category": "Locations"}, "Rooms")
		snap, err := k.Before(ctx, "c", layoutOwner, rec)
		if err != nil || snap == nil || snap.Target != "made:2:Inn" {
			t.Fatalf("before = %+v, %v", snap, err)
		}
		if err := k.Apply(ctx, "c", layoutOwner, rec); err != nil {
			t.Fatal(err)
		}
		if err := k.Restore(ctx, "c", layoutOwner, *snap); err != nil {
			t.Fatal(err)
		}
		if len(f.deleted) != 1 || len(f.list) != 3 {
			t.Errorf("list %+v, deleted %v", f.list, f.deleted)
		}
	})
	t.Run("an edited template is put back", func(t *testing.T) {
		f := newTemplatesFake()
		k := PageTemplateKind{Svc: f}
		rec := tplRec("update", "Tavern", map[string]any{"rename_to": "Inn"}, "New words")
		snap, err := k.Before(ctx, "c", layoutOwner, rec)
		if err != nil || snap == nil || snap.Target != "edit:2" {
			t.Fatalf("before = %+v, %v", snap, err)
		}
		fp1, _ := k.Fingerprint(ctx, "c", layoutOwner, *snap)
		if err := k.Apply(ctx, "c", layoutOwner, rec); err != nil {
			t.Fatal(err)
		}
		if fp2, _ := k.Fingerprint(ctx, "c", layoutOwner, *snap); fp1 == fp2 {
			t.Error("the fingerprint did not move")
		}
		if err := k.Restore(ctx, "c", layoutOwner, *snap); err != nil {
			t.Fatal(err)
		}
		if got := f.list[1]; got.Name != "Tavern" || !strings.Contains(got.ContentJSON, `"prompt"`) || got.ContentHTML != "<h2>The room</h2><ul><li>Bar</li><li>Stairs</li></ul>" {
			t.Errorf("restored %+v", got)
		}
	})
	t.Run("a removed template is made again", func(t *testing.T) {
		f := newTemplatesFake()
		k := PageTemplateKind{Svc: f}
		rec := tplRec("delete", "Tavern", nil, "")
		snap, err := k.Before(ctx, "c", layoutOwner, rec)
		if err != nil || snap == nil || snap.Target != "gone:2" {
			t.Fatalf("before = %+v, %v", snap, err)
		}
		if err := k.Apply(ctx, "c", layoutOwner, rec); err != nil {
			t.Fatal(err)
		}
		if err := k.Restore(ctx, "c", layoutOwner, *snap); err != nil {
			t.Fatal(err)
		}
		in := f.created[len(f.created)-1]
		if in.Name != "Tavern" || in.EntityTypeID != 2 || !strings.Contains(in.ContentJSON, `"prompt"`) || in.Icon != "fa-beer-mug-empty" {
			t.Errorf("recreated %+v", in)
		}
	})
	t.Run("a shared template has nothing to put back", func(t *testing.T) {
		snap, err := PageTemplateKind{Svc: newTemplatesFake()}.Before(ctx, "c", layoutOwner, tplRec("delete", "Session Recap", nil, ""))
		if snap != nil || err != nil {
			t.Errorf("before = %+v, %v", snap, err)
		}
	})
}

// The export writes the campaign's own templates with their words after the
// block, never the shared ones, and what it writes changes nothing when sent
// back, through the real markdown renderer.
func TestPageTemplateKind_ExportRoundTrips(t *testing.T) {
	ctx := context.Background()
	k := PageTemplateKind{Svc: newTemplatesFake()}
	out, err := k.Export(ctx, "c", layoutOwner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kind: page-template", `name: "Tavern"`, `category: "Location"`, `icon: "fa-beer-mug-empty"`, "## The room", "Heard **in town**"} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Session Recap") {
		t.Errorf("a shared template was exported:\n%s", out)
	}
	for _, p := range parseBlocksWithBody(out) {
		p.Action, p.Text = ActionUpdate, BodyText(nil)
		if pl := k.Plan(ctx, "c", layoutOwner, p); !strings.Contains(pl.Error, "already like this") {
			t.Errorf("unchanged %q: plan = %+v\n%s", p.Name, pl, out)
		}
	}
	for _, a := range []Actor{layoutCoDM, {UserID: "pl", Role: 1}} {
		if out, err := k.Export(ctx, "c", a); err != nil || out != "" {
			t.Errorf("%+v: %q, %v", a, out, err)
		}
	}
}

// parseBlocksWithBody reads blocks as records that keep the markdown after
// each block.
func parseBlocksWithBody(s string) []Record {
	var out []Record
	for _, p := range importer.Parse(s) {
		fields := map[string]any{}
		for k, v := range p.Fields {
			fields[strings.ToLower(k)] = v
		}
		out = append(out, Record{Kind: p.FrontMatter.Kind, Action: p.FrontMatter.Action, Name: p.Name, Fields: fields, Body: p.Body})
	}
	return out
}
