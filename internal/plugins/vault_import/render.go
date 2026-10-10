package vault_import

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// md renders Markdown to HTML. Raw HTML in a note is NOT passed through
// (html.WithUnsafe is off), and the result still goes through the shared
// sanitizer, so a hostile note cannot place script, handlers or styles on a
// page. Hard wraps match Obsidian, where a single line break is a line break.
var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(html.WithHardWraps()),
)

// markdownToHTML converts Markdown and sanitises the result, the same shared
// policy typed pages are stored through.
func markdownToHTML(src string) (string, error) {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return "", fmt.Errorf("could not read the note's Markdown: %w", err)
	}
	return sanitize.HTML(buf.String()), nil
}

// pageLinkHref is the address of a page inside its campaign: the href the
// editor writes for a [[page link]].
func pageLinkHref(campaignID, pageID string) string {
	return "/campaigns/" + campaignID + "/entities/" + pageID
}

// uuidRe is the shape of every page and media id.
const uuidPat = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`

var (
	imgTagRe   = regexp.MustCompile(`<img\b[^>]*?\bsrc="/media/(` + uuidPat + `)"[^>]*?>`)
	paraRe     = regexp.MustCompile(`(?s)<p>(.*?)</p>`)
	altAttrRe  = regexp.MustCompile(`\balt="([^"]*)"`)
	sizeAttrRe = regexp.MustCompile(`\btitle="w(\d{1,3})"`)
	emptyPara  = regexp.MustCompile(`^(\s|<br\s*/?>)*$`)
)

// finishHTML turns the renderer's plain links and images into the markup the
// editor itself writes, so the page opens in the editor with working page
// links and pictures:
//   - an <a> to this campaign's page becomes the [[page link]] anchor
//     (data-mention-id, the address, the hover-card address);
//   - a picture on its own becomes the editor's <figure class="ce-img">.
//
// Both are built only from ids this import created or got back from the media
// service, and the result is sanitised again before it is stored.
func finishHTML(htmlIn, campaignID string) string {
	linkRe := regexp.MustCompile(`<a\b[^>]*?\bhref="(/campaigns/` + regexp.QuoteMeta(campaignID) + `/entities/(` + uuidPat + `))"[^>]*?>`)
	out := linkRe.ReplaceAllString(htmlIn, `<a data-mention-id="$2" href="$1" data-entity-preview="$1/preview">`)
	out = paraRe.ReplaceAllStringFunc(out, splitPictures)
	return sanitize.HTML(out)
}

// splitPictures lifts pictures out of a paragraph into figures: the editor's
// picture is a block, so a paragraph holding both words and pictures becomes
// words, picture, words.
func splitPictures(p string) string {
	inner := p[len("<p>") : len(p)-len("</p>")]
	locs := imgTagRe.FindAllStringSubmatchIndex(inner, -1)
	if len(locs) == 0 {
		return p
	}
	var b strings.Builder
	last := 0
	flush := func(text string) {
		if !emptyPara.MatchString(text) {
			b.WriteString("<p>" + strings.TrimSpace(text) + "</p>")
		}
	}
	for _, m := range locs {
		flush(inner[last:m[0]])
		tag := inner[m[0]:m[1]]
		id := inner[m[2]:m[3]]
		alt := ""
		if a := altAttrRe.FindStringSubmatch(tag); a != nil {
			alt = a[1]
		}
		width := 100
		if w := sizeAttrRe.FindStringSubmatch(tag); w != nil {
			if n, err := strconv.Atoi(w[1]); err == nil {
				width = n
			}
		}
		fmt.Fprintf(&b, `<figure class="ce-img ce-img--w%d ce-img--center"><img src="/media/%s" alt="%s"></figure>`, width, id, alt)
		last = m[1]
	}
	flush(inner[last:])
	return b.String()
}

// widthFromHint turns Obsidian's pixel size ("300" or "300x200") into the
// editor's percentage steps (10 to 100, in fives), measured against a page
// column about 800 px wide. No hint means the full column.
func widthFromHint(hint string) int {
	if hint == "" {
		return 100
	}
	w, _, _ := strings.Cut(hint, "x")
	px, err := strconv.Atoi(w)
	if err != nil || px <= 0 {
		return 100
	}
	pct := (px*100/800 + 2) / 5 * 5
	if pct < 10 {
		pct = 10
	}
	if pct > 100 {
		pct = 100
	}
	return pct
}
