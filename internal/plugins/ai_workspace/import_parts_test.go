package ai_workspace

import (
	"context"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/prompt"
)

// memParts is an in-memory ImportMemory.
type memParts struct {
	rows   map[string]StoredPart
	blocks map[string]time.Time
}

func newMemParts() *memParts { return &memParts{rows: map[string]StoredPart{}} }

func (m *memParts) key(k string, n int) string { return k + "#" + string(rune('0'+n)) }

func (m *memParts) Parts(_ context.Context, _, key string) ([]StoredPart, error) {
	var out []StoredPart
	for _, r := range m.rows {
		if r.AnswerKey == key {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memParts) Tray(_ context.Context, _ string) ([]StoredPart, error) {
	var out []StoredPart
	for _, r := range m.rows {
		if r.Markdown != "" {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memParts) PutInTray(_ context.Context, _ string, p StoredPart) error {
	if old, ok := m.rows[m.key(p.AnswerKey, p.Number)]; ok {
		p.Applied = old.Applied
	}
	m.rows[m.key(p.AnswerKey, p.Number)] = p
	return nil
}

func (m *memParts) MarkApplied(_ context.Context, _ string, p StoredPart) error {
	p.Applied, p.Markdown = true, ""
	m.rows[m.key(p.AnswerKey, p.Number)] = p
	return nil
}

func (m *memParts) ClearTray(_ context.Context, _, key string) error {
	for k, r := range m.rows {
		if r.AnswerKey != key {
			continue
		}
		if !r.Applied {
			delete(m.rows, k)
			continue
		}
		r.Markdown = ""
		m.rows[k] = r
	}
	return nil
}

func (m *memParts) Seen(_ context.Context, _ string, hashes []string) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	for _, h := range hashes {
		if at, ok := m.blocks[h]; ok {
			out[h] = at
		}
	}
	return out, nil
}

func (m *memParts) Remember(_ context.Context, _ string, hashes []string) error {
	if m.blocks == nil {
		m.blocks = map[string]time.Time{}
	}
	for _, h := range hashes {
		m.blocks[h] = time.Date(2026, 10, 19, 23, 0, 0, 0, time.UTC)
	}
	return nil
}

// Part 3's first table links a page part 2 makes; its second relies on
// nothing.
const partThree = `---
kind: part
name: Lastwell fixes
part: 3
of: 3
needs: [2]
---
---
kind: table
name: Contract rolls
---
- See @[The Lastwell Contract]
---
kind: table
name: Full moons
---
- Red
- White`

const partOne = `---
kind: part
name: Lastwell fixes
part: 1
of: 3
---
---
kind: table
name: Opening rites
---
- Salt
- Ash`

const partTwo = `---
kind: part
name: Lastwell fixes
part: 2
of: 3
needs: [1]
---
---
kind: table
name: Contract clauses
---
- Blood
- Silver`

func partsHandler(store *memParts) (*Handler, *memTables) {
	m := &memTables{}
	h := recordsHandler(m)
	h.SetImportMemory(store)
	return h, m
}

func parsePaste(t *testing.T, h *Handler, paste string) string {
	t.Helper()
	body, ct := multipartBody(t, "markdown_paste", paste)
	req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/ai-workspace/import/parse", body)
	req.Header.Set(echo.HeaderContentType, ct)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	ownerCtx(c)
	if err := h.ParseImport(c); err != nil {
		t.Fatal(err)
	}
	return html.UnescapeString(rec.Body.String())
}

func commitPaste(t *testing.T, h *Handler, form url.Values) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/ai-workspace/import/commit", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	ownerCtx(c)
	if err := h.CommitImport(c); err != nil {
		t.Fatal(err)
	}
	return html.UnescapeString(rec.Body.String())
}

func hasTable(m *memTables, name string) bool {
	for _, tb := range m.doc.Tables {
		if tb.Name == name {
			return true
		}
	}
	return false
}

// Parts of an answer wait in the tray until the whole answer is in, then
// open as one review in order; applying it empties the tray.
func TestImportParts_TrayThenOneReview(t *testing.T) {
	store := newMemParts()
	h, m := partsHandler(store)

	// Part 3 of 3 first: it goes to the tray, nothing is reviewed.
	notice := parsePaste(t, h, partThree)
	for _, want := range []string{
		`Part 3 of "Lastwell fixes" went to the tray: 1 of 3 in.`,
		"Paste parts 1 and 2 next.",
		`id="ai-import-tray"`, `hx-swap-oob="true"`, "Lastwell fixes", "1 of 3 in. Still to paste: parts 1 and 2.",
	} {
		if !strings.Contains(notice, want) {
			t.Errorf("tray notice missing %q", want)
		}
	}
	if strings.Contains(notice, "markdown_source") {
		t.Errorf("a part that went to the tray must not open a review")
	}
	if got := store.rows["lastwell-fixes#3"]; !strings.Contains(got.Markdown, "kind: part") || !strings.Contains(got.Markdown, "Full moons") {
		t.Fatalf("part 3 not kept whole in the tray: %+v", got)
	}

	// Part 1 arrives: still waiting for part 2.
	notice = parsePaste(t, h, partOne)
	if !strings.Contains(notice, "2 of 3 in.") || !strings.Contains(notice, "Paste part 2 next.") {
		t.Errorf("after part 1 the tray should hold 2 of 3")
	}

	// Part 2, the last: one review of all three, in order.
	review := parsePaste(t, h, partTwo)
	src := review[strings.Index(review, `name="markdown_source" value="`):]
	one, two, three := strings.Index(src, "part: 1"), strings.Index(src, "part: 2"), strings.Index(src, "part: 3")
	if one < 0 || two < 0 || three < 0 || one > two || two > three {
		t.Fatalf("the review should hold parts 1, 2 and 3 in order: %d %d %d", one, two, three)
	}
	for _, want := range []string{`"Lastwell fixes", part 1 of 3.`, `"Lastwell fixes", part 3 of 3.`, "Opening rites", "Contract clauses", "Full moons"} {
		if !strings.Contains(review, want) {
			t.Errorf("combined review missing %q", want)
		}
	}

	// Apply it: the tables go in and the tray empties.
	form := url.Values{"markdown_source": {html.UnescapeString(sourceValue(review))}}
	for i := 0; i < 4; i++ {
		form.Set("rec_"+string(rune('0'+i))+"_include", "on")
	}
	commitPaste(t, h, form)
	for _, name := range []string{"Opening rites", "Contract clauses", "Full moons"} {
		if !hasTable(m, name) {
			t.Errorf("table %q was not applied", name)
		}
	}
	for k, r := range store.rows {
		if r.Markdown != "" || !r.Applied {
			t.Errorf("part %s should be applied and out of the tray: %+v", k, r)
		}
	}
}

// sourceValue is the review form's markdown_source, as rendered.
func sourceValue(review string) string {
	const at = `name="markdown_source" value="`
	s := review[strings.Index(review, at)+len(at):]
	return s[:strings.Index(s, `"`)]
}

// "Review what's here" opens the tray's parts before the answer is complete,
// warning about the part that is missing; "Empty" takes them out.
func TestImportParts_ReviewTrayEarlyAndEmpty(t *testing.T) {
	store := newMemParts()
	h, _ := partsHandler(store)
	parsePaste(t, h, partThree)

	body, ct := multipartBody(t, "tray_review", "lastwell-fixes")
	req := httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/ai-workspace/import/parse", body)
	req.Header.Set(echo.HeaderContentType, ct)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	ownerCtx(c)
	if err := h.ParseImport(c); err != nil {
		t.Fatal(err)
	}
	review := html.UnescapeString(rec.Body.String())
	if !strings.Contains(review, `Part 3 of "Lastwell fixes" needs part 2, which isn't applied or in this review`) ||
		!strings.Contains(review, "Full moons") {
		t.Errorf("early review should warn about part 2 and show part 3's rows")
	}

	form := url.Values{"answer": {"lastwell-fixes"}}
	req = httptest.NewRequest(http.MethodPost, "/campaigns/camp-1/ai-workspace/import/tray/clear", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	ownerCtx(c)
	if err := h.ClearImportTray(c); err != nil {
		t.Fatal(err)
	}
	if len(store.rows) != 0 || strings.Contains(rec.Body.String(), "In the tray") {
		t.Errorf("the tray should be empty: %+v", store.rows)
	}
}

// A part with no count and nothing it needs missing is reviewed at once, and
// several answers pasted together are reviewed as pasted.
func TestImportParts_ReadyPartsSkipTheTray(t *testing.T) {
	store := newMemParts()
	h, _ := partsHandler(store)
	loose := strings.Replace(partOne, "of: 3\n", "", 1)
	if review := parsePaste(t, h, loose); !strings.Contains(review, "markdown_source") {
		t.Errorf("a lone part 1 with no count should open its review")
	}
	other := strings.Replace(partTwo, "Lastwell fixes", "Other answer", 1)
	if review := parsePaste(t, h, partThree+"\n"+other); !strings.Contains(review, "markdown_source") || len(store.rows) != 0 {
		t.Errorf("two answers in one paste should be reviewed as pasted, not trayed")
	}
}

// Without the import memory the labels are read but nothing waits.
func TestImportParts_NoStore(t *testing.T) {
	h := recordsHandler(&memTables{})
	review := parsePaste(t, h, partThree)
	if !strings.Contains(review, "markdown_source") || !strings.Contains(review, `"Lastwell fixes", part 3 of 3.`) {
		t.Errorf("with no memory the part is reviewed as pasted")
	}
}

// The label the prompt shows the AI is one the importer reads.
func TestImportParts_PromptExampleParses(t *testing.T) {
	got, err := prompt.NewService(nil, nil, nil).Build(context.Background(), "Ashfall", "owner-1", "camp-1",
		prompt.Input{IncludeFrontMatterExample: true, ContentMode: "none"})
	if err != nil {
		t.Fatal(err)
	}
	sec := got[strings.Index(got, "## If your answer comes in parts"):]
	start := strings.Index(sec, "```\n") + 4
	example := sec[start : start+strings.Index(sec[start:], "```")]
	lab := importer.TakeParts(importer.Parse(example))
	if len(lab.Parts) != 1 || lab.Parts[0].Number != 2 || len(lab.Parts[0].Needs) != 1 || lab.Parts[0].Needs[0] != 1 {
		t.Fatalf("prompt example read as %+v (blocks %+v)", lab.Parts, lab.Blocks)
	}
}

// The same paste applied twice: the second review says so, and a new block
// in it still starts on Yes. (A repeated table is refused as existing, so it
// sits in the problems box with when it was applied.)
// A combined review answered No throughout applied nothing, so the waiting
// parts stay in the tray and pasting the last part again reopens the review.
func TestImportParts_NothingAppliedKeepsTheTray(t *testing.T) {
	store := newMemParts()
	h, _ := partsHandler(store)
	parsePaste(t, h, partThree)
	parsePaste(t, h, partOne)
	review := parsePaste(t, h, partTwo)
	commitPaste(t, h, url.Values{"markdown_source": {html.UnescapeString(sourceValue(review))}})
	for _, n := range []int{1, 3} {
		if got := store.rows[store.key("lastwell-fixes", n)]; got.Applied || got.Markdown == "" {
			t.Errorf("part %d: applied %v, in tray %v; nothing was applied", n, got.Applied, got.Markdown != "")
		}
	}
	if got, ok := store.rows[store.key("lastwell-fixes", 2)]; ok && got.Applied {
		t.Errorf("part 2 counted as applied")
	}
	if review := parsePaste(t, h, partTwo); !strings.Contains(review, "Full moons") {
		t.Errorf("pasting part 2 again should reopen the whole review")
	}
}

func TestImportParts_RepeatPaste(t *testing.T) {
	store := newMemParts()
	h, _ := partsHandler(store)
	// An unlabelled paste, so it goes straight to review.
	clauses := "---\nkind: table\nname: Contract clauses\n---\n- Blood\n- Silver"
	commitPaste(t, h, url.Values{"markdown_source": {clauses}, "rec_0_include": {"on"}})
	today := "19 October 2026" // the date memParts.Remember records

	review := parsePaste(t, h, clauses)
	for _, want := range []string{
		"You already applied this paste, on " + today + ".",
		"Already applied on " + today + ". Contract clauses:", "1 already applied",
	} {
		if !strings.Contains(review, want) {
			t.Errorf("repeat review missing %q", want)
		}
	}

	review = parsePaste(t, h, clauses+"\n---\nkind: table\nname: New\n---\n- One")
	for _, want := range []string{"1 block in this paste was already applied.", `name="rec_1_include" value="on" checked`} {
		if !strings.Contains(review, want) {
			t.Errorf("mixed review missing %q", want)
		}
	}
}

// A repeated page that is still offered starts on No, and "Yes to all" leaves
// it alone (data-air-hold).
func TestImportParts_RepeatRowStartsNo(t *testing.T) {
	pages := importer.Parse("---\nname: Moon\ntype: character\n---\nBody.")
	d := ImportReviewData{
		CampaignID: "camp-1", Pages: pages,
		Classes:       []importer.Classification{{Status: importer.StatusNew}},
		SummaryCounts: ReviewSummary{Total: 1, Selectable: 1, Repeats: 1},
		Repeats:       map[int]string{0: "19 October 2026"},
	}
	var b strings.Builder
	if err := ImportReview(d).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	got := html.UnescapeString(b.String())
	for _, want := range []string{`name="page_0_include" value="off" checked`, "data-air-hold", "Already applied on 19 October 2026.", `aiImportReview(0)`} {
		if !strings.Contains(got, want) {
			t.Errorf("review missing %q", want)
		}
	}
}
