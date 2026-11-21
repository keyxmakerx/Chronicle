package ai_workspace

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
)

func pageBlock(name, body string) string {
	return "---\nname: " + name + "\n---\n\n" + body + "\n"
}

// Two adds in one review that would make the same page, and an add that a
// waiting request already asks for, each show the quiet line, so the owner
// sees the pair before approving both.
func TestImportReview_PendingTwins(t *testing.T) {
	const lead = "Much like"
	tests := []struct {
		name    string
		paste   string
		waiting []string
		want    []string
		absent  []string
	}{
		{
			name:   "two adds in one review",
			paste:  pageBlock("New Player Guide", "Welcome.") + "\n" + pageBlock("New Player Guide (Imported)", "Welcome."),
			want:   []string{"Much like another page in this request:", "New Player Guide (Imported)"},
			absent: []string{"another waiting request", "Update that page instead"},
		},
		{
			name:    "an add and a waiting request",
			paste:   pageBlock("New Player Guide (Imported)", "Welcome."),
			waiting: []string{pageBlock("New Player Guide", "Welcome.")},
			want:    []string{"Much like a page in another waiting request:", "New Player Guide"},
			absent:  []string{"another page in this request", "Update that page instead"},
		},
		{
			name:    "unrelated pages say nothing",
			paste:   pageBlock("Harbour rumours", "Gulls.") + "\n" + pageBlock("Coral Court", "A market."),
			waiting: []string{pageBlock("Session 3", "We play.")},
			absent:  []string{lead},
		},
		{
			name:    "a waiting update is not a twin",
			paste:   pageBlock("New Player Guide", "Welcome."),
			waiting: []string{"---\nname: New Player Guide\naction: update\n---\n\nWelcome.\n"},
			absent:  []string{lead},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, _, _ := requestRig(t)
			for _, w := range tt.waiting {
				if _, err := h.connector.Ask(context.Background(), &connector.Connection{ID: 1, CampaignID: "camp-1", CanAsk: true}, "page", w); err != nil {
					t.Fatal(err)
				}
			}
			html := post(t, h.ParseImport, "/x", url.Values{"markdown_paste": {tt.paste}}).Body.String()
			for _, w := range tt.want {
				if !strings.Contains(html, w) {
					t.Errorf("review is missing %q", w)
				}
			}
			for _, w := range tt.absent {
				if strings.Contains(html, w) {
					t.Errorf("review should not contain %q", w)
				}
			}
		})
	}
}

// A request's own review never matches itself, but does match a second
// request that asks for the same page.
func TestImportReview_PendingSkipsItself(t *testing.T) {
	h, _, _, _ := requestRig(t)
	ask := func(body string) string {
		q, err := h.connector.Ask(context.Background(), &connector.Connection{ID: 1, CampaignID: "camp-1", CanAsk: true}, "page", body)
		if err != nil {
			t.Fatal(err)
		}
		return strconv.FormatInt(q.ID, 10)
	}
	first := ask(pageBlock("New Player Guide", "Welcome."))
	if html := post(t, h.ParseImport, "/x", url.Values{"ai_request": {first}}).Body.String(); strings.Contains(html, "Much like") {
		t.Error("a request matched itself")
	}
	ask(pageBlock("New Player Guide (Imported)", "Welcome."))
	if html := post(t, h.ParseImport, "/x", url.Values{"ai_request": {first}}).Body.String(); !strings.Contains(html, "Much like a page in another waiting request:") {
		t.Error("the second request's page was not named")
	}
}

func TestFindSimilarPending(t *testing.T) {
	long := strings.Repeat("alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau upsilon phi ", 1)
	pages := []importer.ParsedPage{
		{Name: "New Player Guide", Body: long},
		{Name: "New Player Guide (Imported)", Body: long + " extra"},
		{Name: "Harbour rumours", Body: "Gulls."},
	}
	others := []importer.PendingPage{{Index: 0, Name: pages[0].Name, Body: pages[0].Body}, {Index: 1, Name: pages[1].Name, Body: pages[1].Body}, {Index: 2, Name: pages[2].Name, Body: pages[2].Body}}
	got := importer.FindSimilarPending(pages[1], 1, others)
	if got == nil || got.Name != "New Player Guide" || !got.Pending || got.Elsewhere || !got.HasBody || got.Differ != 1 {
		t.Fatalf("got %+v", got)
	}
	if got := importer.FindSimilarPending(pages[2], 2, others); got != nil {
		t.Fatalf("an unrelated page matched %+v", got)
	}
	other := []importer.PendingPage{{Index: -1, Name: "New Player Guide", Body: long}}
	if got := importer.FindSimilarPending(pages[1], 1, other); got == nil || !got.Elsewhere {
		t.Fatalf("a page of another request should be marked elsewhere: %+v", got)
	}
}

// A line the page's reader-safe text had to drop says a GM-only part is not
// shown, instead of reading as a broken sentence marked "Only here".
func TestSimLines_GMOnlyPart(t *testing.T) {
	m := importer.GMOnlyMark
	got := simLines([]string{"Open line.", m, "Half a " + m + " sentence."}, []string{"Open line."})
	if len(got) != 3 {
		t.Fatalf("got %d lines", len(got))
	}
	if got[0].Only || got[0].GM {
		t.Errorf("a shared line = %+v", got[0])
	}
	if !got[1].GM || got[1].Only || got[1].Text != "" {
		t.Errorf("a wholly hidden line = %+v", got[1])
	}
	if !got[2].GM || got[2].Only || got[2].Text != "Half a sentence." {
		t.Errorf("a partly hidden line = %+v", got[2])
	}
	for _, l := range got {
		if strings.Contains(l.Text, m) {
			t.Errorf("the mark reached the text: %q", l.Text)
		}
	}
	html := renderReview(t, func() ImportReviewData {
		d := representativeReviewData()
		d.Similar = []*importer.SimilarPage{{Name: "Guide", CanUpdate: true, Have: []string{"Open line.", m}, Sent: []string{"Open line.", "Secret plan."}}}
		return d
	}())
	if n := strings.Count(html, "A GM-only part isn't shown."); n != 1 {
		t.Errorf("the note shows %d times", n)
	}
	if strings.Contains(html, m) {
		t.Error("the mark reached the page")
	}
}
