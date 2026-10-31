package sanitize

import (
	"context"
	"encoding/json"
	"strings"

	"golang.org/x/net/html"
)

// HiddenMentionText stands in for a mention of a page the reader can't open.
// The stored link text is the page's name, so it must not reach that reader.
const HiddenMentionText = "a hidden page"

// MentionIDs returns the page ids mentioned in stored editor content, HTML or
// ProseMirror JSON, each once, in order of first appearance.
func MentionIDs(text string) []string {
	var ids []string
	seen := map[string]bool{}
	HideMentions(text, func(id string) bool {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
		return true
	})
	return ids
}

// HideMentions replaces every page mention in stored editor content whose id
// keep rejects with HiddenMentionText as plain text: the link, its address
// and its stored name all go. JSON content (a ProseMirror document) and HTML
// are both accepted. It returns the rewritten text and how many mentions it
// replaced; content with nothing replaced comes back byte for byte.
func HideMentions(text string, keep func(id string) bool) (string, int) {
	if !strings.Contains(text, "data-mention-id") {
		return text, 0
	}
	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		return hideMentionsJSON(text, keep)
	}
	return hideMentionsHTML(text, keep, HiddenMentionText)
}

// DropMentionText removes every page mention's link and stored name from
// HTML, leaving nothing in its place. Text indexed once for every reader
// uses it: whether a mentioned page is hidden depends on who reads, and can
// change after the index is written.
func DropMentionText(htmlIn string) string {
	if !strings.Contains(htmlIn, "data-mention-id") {
		return htmlIn
	}
	out, _ := hideMentionsHTML(htmlIn, func(string) bool { return false }, "")
	return out
}

// hideMentionsHTML rewrites <a data-mention-id> elements. Everything inside a
// hidden mention's link is dropped, including nested formatting, up to the
// link's own closing tag; an unclosed link is dropped to the end of the input,
// failing closed like stripSecretSpans.
func hideMentionsHTML(src string, keep func(id string) bool, standIn string) (string, int) {
	z := html.NewTokenizer(strings.NewReader(src))
	var out strings.Builder
	out.Grow(len(src))
	depth, count := 0, 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if count == 0 {
				return src, 0
			}
			return out.String(), count
		}
		raw := z.Raw()
		if depth > 0 {
			switch tt {
			case html.StartTagToken:
				if name, _ := z.TagName(); string(name) == "a" {
					depth++
				}
			case html.EndTagToken:
				if name, _ := z.TagName(); string(name) == "a" {
					depth--
				}
			}
			continue
		}
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			name, hasAttr := z.TagName()
			if string(name) == "a" && hasAttr {
				if id, ok := mentionAttr(z); ok && !keep(id) {
					count++
					out.WriteString(standIn)
					if tt == html.StartTagToken {
						depth = 1
					}
					continue
				}
			}
		}
		out.Write(raw)
	}
}

// mentionAttr returns the tokenizer's current tag's data-mention-id.
func mentionAttr(z *html.Tokenizer) (string, bool) {
	for {
		key, val, more := z.TagAttr()
		if string(key) == "data-mention-id" {
			return string(val), true
		}
		if !more {
			return "", false
		}
	}
}

// hideMentionsJSON rewrites text nodes carrying a link mark with a
// data-mention-id. A mention split over several text nodes (part of it bold,
// say) becomes one stand-in, not one per piece. Other marks on the first piece
// are kept, so the stand-in reads in the same style as the line.
func hideMentionsJSON(src string, keep func(id string) bool) (string, int) {
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(src), &doc); err != nil {
		return src, 0
	}
	count := hideMentionNodes(doc, keep)
	if count == 0 {
		return src, 0
	}
	out, err := json.Marshal(doc)
	if err != nil {
		// The document parsed, so this can't happen; failing closed still
		// means the reader gets nothing rather than the names.
		return "", count
	}
	return string(out), count
}

func hideMentionNodes(node map[string]interface{}, keep func(id string) bool) int {
	content, ok := node["content"].([]interface{})
	if !ok {
		return 0
	}
	count := 0
	filtered := make([]interface{}, 0, len(content))
	lastHidden := ""
	for _, child := range content {
		childMap, ok := child.(map[string]interface{})
		if !ok {
			filtered = append(filtered, child)
			lastHidden = ""
			continue
		}
		if id, at := mentionMark(childMap); at >= 0 && !keep(id) {
			if id == lastHidden {
				continue // the rest of a mention already replaced
			}
			marks := childMap["marks"].([]interface{})
			rest := append(append([]interface{}{}, marks[:at]...), marks[at+1:]...)
			if len(rest) == 0 {
				delete(childMap, "marks")
			} else {
				childMap["marks"] = rest
			}
			childMap["text"] = HiddenMentionText
			filtered = append(filtered, childMap)
			lastHidden = id
			count++
			continue
		}
		lastHidden = ""
		count += hideMentionNodes(childMap, keep)
		filtered = append(filtered, childMap)
	}
	node["content"] = filtered
	return count
}

// mentionMark returns the mentioned page id and the index of the link mark
// carrying it, or -1 when the node is not a page mention.
func mentionMark(node map[string]interface{}) (string, int) {
	if node["type"] != "text" {
		return "", -1
	}
	marks, _ := node["marks"].([]interface{})
	for i, m := range marks {
		mark, ok := m.(map[string]interface{})
		if !ok || mark["type"] != "link" {
			continue
		}
		attrs, _ := mark["attrs"].(map[string]interface{})
		if id, ok := attrs["data-mention-id"].(string); ok && id != "" {
			return id, i
		}
	}
	return "", -1
}

// MentionHider hides, in place, each mention in texts of a page the viewer
// can't open, and reports how many it replaced. The entities service
// implements it; plugins that serve stored editor text take it as a seam.
type MentionHider interface {
	HideUnviewableMentions(ctx context.Context, campaignID string, role int, userID string, texts ...*string) (int, error)
}

// HideMentionsFor runs hider over texts, skipping the check when no text
// holds a mention. With no hider wired, every mention is hidden: a missing
// seam must fail closed, never serve stored names.
func HideMentionsFor(ctx context.Context, hider MentionHider, campaignID string, role int, userID string, texts ...*string) (int, error) {
	found := false
	for _, t := range texts {
		if t != nil && strings.Contains(*t, "data-mention-id") {
			found = true
			break
		}
	}
	if !found {
		return 0, nil
	}
	if hider != nil {
		return hider.HideUnviewableMentions(ctx, campaignID, role, userID, texts...)
	}
	total := 0
	for _, t := range texts {
		if t != nil {
			var n int
			*t, n = HideMentions(*t, func(string) bool { return false })
			total += n
		}
	}
	return total, nil
}
