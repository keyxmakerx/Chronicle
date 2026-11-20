package aiexport

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// markTaskItems puts a stand-in at the start of each checklist item's words,
// which convertToMarkdown turns into "[ ] " or "[x] ". Without it the
// converter writes a checklist as a plain bullet list, and an AI that reads
// the page and writes it back would turn the boxes into bullets. A stand-in,
// not the real text, because the converter escapes the brackets.
func markTaskItems(in string) string {
	if !strings.Contains(in, `data-type="taskItem"`) {
		return in
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(in))
	if err != nil {
		return in
	}
	doc.Find(`li[data-type="taskItem"]`).Each(func(_ int, li *goquery.Selection) {
		mark := taskOpenMark
		if li.AttrOr("data-checked", "") == "true" {
			mark = taskDoneMark
		}
		if p := li.ChildrenFiltered("p").First(); p.Length() > 0 {
			p.PrependHtml(mark)
		} else {
			li.PrependHtml(mark)
		}
	})
	out, err := doc.Find("body").Html()
	if err != nil {
		return in
	}
	return out
}
