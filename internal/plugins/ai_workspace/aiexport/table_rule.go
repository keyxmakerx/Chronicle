package aiexport

import (
	"strconv"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
)

// tableRule writes a page's table as a Markdown table. Without it the
// converter keeps only the cells' text, so a table reaches the AI as a run of
// words with no rows or columns. The first row is the header, since Markdown
// needs one and page tables put their headings there. Each cell is converted
// on its own and kept on one line, because a Markdown cell cannot hold a line
// break that AI Import would read back (raw <br> is off there).
func tableRule() md.Rule {
	return md.Rule{
		Filter: []string{"table"},
		Replacement: func(_ string, sel *goquery.Selection, _ *md.Options) *string {
			var rows [][]string
			width := 0
			sel.Find("tr").Each(func(_ int, tr *goquery.Selection) {
				// A table inside a cell is that cell's content, not rows of this one.
				if !tr.Closest("table").IsSelection(sel) {
					return
				}
				var cells []string
				tr.ChildrenFiltered("th, td").Each(func(_ int, cell *goquery.Selection) {
					cells = append(cells, tableCell(cell))
					span, _ := strconv.Atoi(cell.AttrOr("colspan", "1"))
					for i := 1; i < span && i < 50; i++ {
						cells = append(cells, "")
					}
				})
				if len(cells) == 0 {
					return
				}
				width = max(width, len(cells))
				rows = append(rows, cells)
			})
			if len(rows) == 0 {
				return md.String("")
			}
			var b strings.Builder
			b.WriteString("\n\n")
			if c := strings.Join(strings.Fields(sel.ChildrenFiltered("caption").Text()), " "); c != "" {
				b.WriteString(c + "\n\n")
			}
			for i, cells := range rows {
				for len(cells) < width {
					cells = append(cells, "")
				}
				b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
				if i == 0 {
					b.WriteString("|" + strings.Repeat(" --- |", width) + "\n")
				}
			}
			b.WriteString("\n")
			return md.String(b.String())
		},
	}
}

// tableCell is one cell's content as Markdown on a single line, with every
// pipe escaped so it cannot end the cell early.
func tableCell(cell *goquery.Selection) string {
	h, err := cell.Html()
	if err != nil {
		return ""
	}
	got, err := getConverter().ConvertString(h)
	if err != nil {
		return strings.Join(strings.Fields(cell.Text()), " ")
	}
	got = strings.Join(strings.Fields(got), " ")
	return strings.ReplaceAll(strings.ReplaceAll(got, `\|`, "|"), "|", `\|`)
}
