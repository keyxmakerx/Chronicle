package records

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// memBookRepo is an in-memory systems.BookEditRepository, so these tests run
// the real book service and its checks.
type memBookRepo struct {
	mu       sync.Mutex
	chapters []systems.HouseChapter
	pages    []systems.StoredBookPage
	next     int64
}

func (m *memBookRepo) Load(_ context.Context, _, _ string) (*systems.BookEdits, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &systems.BookEdits{
		Chapters: append([]systems.HouseChapter(nil), m.chapters...),
		Pages:    append([]systems.StoredBookPage(nil), m.pages...),
	}, nil
}
func (m *memBookRepo) SaveCopy(context.Context, systems.StoredBookPage) error { return nil }
func (m *memBookRepo) AddOwn(_ context.Context, p systems.StoredBookPage) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	p.ID, p.SortOrder = m.next, int(m.next)
	m.pages = append(m.pages, p)
	return p.ID, nil
}
func (m *memBookRepo) UpdateOwn(context.Context, string, string, string, int64, string, string) error {
	return nil
}
func (m *memBookRepo) DeleteCopy(context.Context, string, string, string, int) (bool, error) {
	return false, nil
}
func (m *memBookRepo) DeleteOwn(context.Context, string, string, string, int64) (bool, error) {
	return false, nil
}
func (m *memBookRepo) PromoteCopy(context.Context, string, string, string, int) (int64, error) {
	return 0, nil
}
func (m *memBookRepo) AddChapter(_ context.Context, ch systems.HouseChapter, first systems.StoredBookPage) error {
	return m.WriteHouseChapter(context.Background(), ch, true, []systems.StoredBookPage{first})
}
func (m *memBookRepo) UpdateChapter(_ context.Context, ch systems.HouseChapter) error {
	return m.WriteHouseChapter(context.Background(), ch, false, nil)
}
func (m *memBookRepo) DeleteChapter(_ context.Context, _, _, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keep []systems.HouseChapter
	for _, c := range m.chapters {
		if c.ChapterID != id {
			keep = append(keep, c)
		}
	}
	m.chapters = keep
	var pk []systems.StoredBookPage
	for _, p := range m.pages {
		if p.ChapterID != id {
			pk = append(pk, p)
		}
	}
	m.pages = pk
	return true, nil
}
func (m *memBookRepo) WriteHouseChapter(_ context.Context, ch systems.HouseChapter, create bool, pages []systems.StoredBookPage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if create {
		ch.SortOrder = len(m.chapters) + 1
		m.chapters = append(m.chapters, ch)
	} else {
		for i, c := range m.chapters {
			if c.ChapterID == ch.ChapterID {
				ch.SortOrder = c.SortOrder
				m.chapters[i] = ch
			}
		}
	}
	if pages == nil {
		return nil
	}
	var keep []systems.StoredBookPage
	for _, p := range m.pages {
		if p.ChapterID != ch.ChapterID {
			keep = append(keep, p)
		}
	}
	for i, p := range pages {
		m.next++
		p.ID, p.CampaignID, p.SystemID, p.ChapterID, p.PackageIndex, p.SortOrder = m.next, ch.CampaignID, ch.SystemID, ch.ChapterID, nil, i+1
		keep = append(keep, p)
	}
	m.pages = keep
	return nil
}

type fakeRulebook struct {
	pkg *systems.BookPackage
	svc systems.BookEditService
}

func (f fakeRulebook) CampaignBookPackage(_, _ string) (*systems.BookPackage, systems.BookEditService, error) {
	return f.pkg, f.svc, nil
}

func newHouseRules(t *testing.T) (HouseRuleKind, *memBookRepo) {
	t.Helper()
	repo := &memBookRepo{}
	pkg := &systems.BookPackage{SystemID: "ds", SystemName: "Draw Steel", Book: &systems.Book{
		Title: "Handbook", SystemName: "Draw Steel", Turn: "flip", Look: "paper",
		Theme: map[string]string{"paper": "#efe6d2"}, Terms: map[string]systems.BookTerm{},
	}}
	k := HouseRuleKind{
		Book:     fakeRulebook{pkg: pkg, svc: systems.NewBookEditService(repo)},
		SystemOf: func(context.Context, string) string { return "ds" },
	}
	return k, repo
}

// houseRecord builds a record the way the importer does: the front matter
// parsed as YAML, keys lower-cased.
func houseRecord(t *testing.T, action, frontMatter, body string) Record {
	t.Helper()
	fields := map[string]any{}
	if err := yaml.Unmarshal([]byte(frontMatter), &fields); err != nil {
		t.Fatal(err)
	}
	name, _ := fields["name"].(string)
	return Record{Kind: "house-rule", Action: action, Name: name, Fields: fields, Body: body}
}

const tableRulesFM = `name: Table rules
intro: How we play at this table.
pages:
  - title: Fumbles
    blocks:
      - text: A couple of rules we added.
      - type: callout
        title: Critical fumbles
        text: A natural 2 drops your weapon.
      - type: roll
        dice: 1d6
        label: Heroic surge
        bands:
          - max: 2
            label: Nothing
          - label: Surge
  - title: Camping
    blocks:
      - type: note
        text: Roll the night encounter secretly.
`

func TestHouseRule_CreateWithPages(t *testing.T) {
	ctx := context.Background()
	k, repo := newHouseRules(t)
	r := houseRecord(t, ActionCreate, tableRulesFM, "")

	p := k.Plan(ctx, "c1", owner, r)
	if p.Error != "" {
		t.Fatal(p.Error)
	}
	if want := "new house-rules chapter: 2 pages, with 1 highlighted box, 1 dice roll, 1 Director's note"; p.Summary != want {
		t.Errorf("summary = %q, want %q", p.Summary, want)
	}
	var book systems.Book
	if err := json.Unmarshal([]byte(p.Preview), &book); err != nil {
		t.Fatalf("preview is not a book: %v", err)
	}
	if book.Look != "paper" || book.Theme["paper"] != "#efe6d2" || len(book.Parts) != 1 || book.Parts[0].Chapters[0].Title != "Table rules" {
		t.Errorf("preview = %+v", book)
	}
	if len(repo.chapters) != 0 {
		t.Fatal("planning wrote something")
	}

	if err := k.Apply(ctx, "c1", owner, r); err != nil {
		t.Fatal(err)
	}
	if len(repo.chapters) != 1 || len(repo.pages) != 2 || repo.chapters[0].Intro != "How we play at this table." {
		t.Fatalf("stored = %+v / %d pages", repo.chapters, len(repo.pages))
	}
	if !strings.Contains(repo.pages[0].PageJSON, `"type":"callout"`) {
		t.Errorf("page 1 = %s", repo.pages[0].PageJSON)
	}
	if p := k.Plan(ctx, "c1", owner, r); !strings.Contains(p.Error, "already exists") {
		t.Errorf("second create: %+v", p)
	}
}

func TestHouseRule_UpdateReplacesPagesAndWarns(t *testing.T) {
	ctx := context.Background()
	k, repo := newHouseRules(t)
	if err := k.Apply(ctx, "c1", owner, houseRecord(t, ActionCreate, tableRulesFM, "")); err != nil {
		t.Fatal(err)
	}
	upd := houseRecord(t, ActionUpdate, "name: table rules\nrename_to: House table\npages:\n  - title: Fumbles\n    blocks:\n      - text: Changed.\n", "")
	p := k.Plan(ctx, "c1", owner, upd)
	if p.Error != "" {
		t.Fatal(p.Error)
	}
	if p.Summary != "replaces its 2 pages with 1 page" {
		t.Errorf("summary = %q", p.Summary)
	}
	joined := strings.Join(p.Warnings, " | ")
	if !strings.Contains(joined, `"Camping"`) || !strings.Contains(joined, "Directors only") {
		t.Errorf("warnings = %v", p.Warnings)
	}
	if err := k.Apply(ctx, "c1", owner, upd); err != nil {
		t.Fatal(err)
	}
	if len(repo.pages) != 1 || repo.chapters[0].Title != "House table" || repo.chapters[0].Intro != "How we play at this table." {
		t.Errorf("after update: %+v / %+v", repo.chapters, repo.pages)
	}
}

func TestHouseRule_WarnsWhenAHiddenPageOpens(t *testing.T) {
	ctx := context.Background()
	hiddenPage := "pages:\n  - title: Traps\n    director: true\n    blocks:\n      - text: The pit.\n"
	openPage := "pages:\n  - title: traps\n    blocks:\n      - text: The pit.\n"
	tests := []struct {
		name, create string
		want         bool
	}{
		{"page loses director", "name: Open rules\n" + hiddenPage, true},
		{"chapter stays hidden", "name: Open rules\ndirector: true\n" + hiddenPage, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k, _ := newHouseRules(t)
			if err := k.Apply(ctx, "c1", owner, houseRecord(t, ActionCreate, tc.create, "")); err != nil {
				t.Fatal(err)
			}
			p := k.Plan(ctx, "c1", owner, houseRecord(t, ActionUpdate, "name: Open rules\n"+openPage, ""))
			if p.Error != "" {
				t.Fatal(p.Error)
			}
			got := strings.Contains(strings.Join(p.Warnings, " | "), `Players will be able to read these pages, which only Directors can today: "traps"`)
			if got != tc.want {
				t.Errorf("warned = %v, want %v: %v", got, tc.want, p.Warnings)
			}
		})
	}
}

func TestHouseRule_Refusals(t *testing.T) {
	ctx := context.Background()
	k, _ := newHouseRules(t)
	tests := []struct {
		name   string
		actor  Actor
		action string
		fm     string
		want   string
	}{
		{"player", player, ActionCreate, tableRulesFM, "Directors"},
		{"no name", owner, ActionCreate, "intro: x\n", "needs a name"},
		{"update missing", owner, ActionUpdate, "name: Nope\n", "no house-rules chapter"},
		{"bad block", owner, ActionCreate, "name: X\npages:\n  - title: A\n    blocks:\n      - type: roll\n        dice: lots\n", "page 1, block 1"},
		{"unknown page key", owner, ActionCreate, "name: X\npages:\n  - title: A\n    colour: red\n    blocks:\n      - text: a\n", `no setting called "colour"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := k.Plan(ctx, "c1", tc.actor, houseRecord(t, tc.action, tc.fm, ""))
			if !strings.Contains(p.Error, tc.want) {
				t.Errorf("error = %q, want %q", p.Error, tc.want)
			}
		})
	}
}

func TestHouseRule_BodyIsStillTheIntro(t *testing.T) {
	ctx := context.Background()
	k, repo := newHouseRules(t)
	r := houseRecord(t, ActionCreate, "name: Critical fumbles\n", "A natural 1 drops the weapon.")
	p := k.Plan(ctx, "c1", owner, r)
	if p.Error != "" || p.Summary != "new house-rules chapter" || p.Preview != "" {
		t.Fatalf("plan = %+v", p)
	}
	if err := k.Apply(ctx, "c1", owner, r); err != nil {
		t.Fatal(err)
	}
	if len(repo.chapters) != 1 || repo.chapters[0].Intro != "A natural 1 drops the weapon." || len(repo.pages) != 1 {
		t.Errorf("stored = %+v, %d pages", repo.chapters, len(repo.pages))
	}
}

func TestHouseRule_Export(t *testing.T) {
	ctx := context.Background()
	k, _ := newHouseRules(t)
	if err := k.Apply(ctx, "c1", owner, houseRecord(t, ActionCreate, tableRulesFM, "")); err != nil {
		t.Fatal(err)
	}
	if err := k.Apply(ctx, "c1", owner, houseRecord(t, ActionCreate, "name: Secret rules\ndirector: true\npages:\n  - title: S\n    blocks:\n      - text: hidden\n", "")); err != nil {
		t.Fatal(err)
	}

	all, err := k.Export(ctx, "c1", owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"### Table rules", "```yaml", "type: callout", "Roll the night", "### Secret rules"} {
		if !strings.Contains(all, want) {
			t.Errorf("owner export lacks %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "left out") {
		t.Errorf("owner export says something was left out:\n%s", all)
	}

	players, err := k.Export(ctx, "c1", player)
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"Roll the night", "Secret rules", "hidden"} {
		if strings.Contains(players, gone) {
			t.Errorf("player export has %q:\n%s", gone, players)
		}
	}
	if !strings.Contains(players, "type: callout") || !strings.Contains(players, "2 chapters, pages or blocks for Directors only are left out") {
		t.Errorf("player export:\n%s", players)
	}
}

func TestHouseRule_Delete(t *testing.T) {
	ctx := context.Background()
	k, repo := newHouseRules(t)
	if err := k.Apply(ctx, "c1", owner, houseRecord(t, ActionCreate, tableRulesFM, "")); err != nil {
		t.Fatal(err)
	}
	del := houseRecord(t, ActionDelete, "name: Table rules\n", "")
	if p := k.Plan(ctx, "c1", owner, del); p.Summary != "removes the chapter and its 2 pages" {
		t.Errorf("plan = %+v", p)
	}
	if err := k.Apply(ctx, "c1", owner, del); err != nil || len(repo.chapters) != 0 || len(repo.pages) != 0 {
		t.Errorf("after delete: %v, %+v", err, repo.chapters)
	}
}
