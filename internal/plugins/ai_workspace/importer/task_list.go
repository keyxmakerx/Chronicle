package importer

// task_list.go turns GFM task lists ("- [ ] item") into the page editor's
// checklist markup. goldmark spells them as <li><input type="checkbox"> text,
// and the sanitizer drops the input, so without this step a written
// checklist would arrive as a plain bullet list with the box lost. The
// editor's own form carries the state in data-checked, which the sanitizer
// keeps and htmlconv reads back into the editor's JSON.

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// taskListMarkup rewrites each list whose every item opens with a checkbox.
// A list that mixes task and plain items is left alone: the editor's
// checklist holds only checkbox items, so splitting it would reorder the
// author's list. Output with no checkbox is returned byte for byte.
func taskListMarkup(in string) string {
	if !strings.Contains(in, `type="checkbox"`) {
		return in
	}
	body := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
	nodes, err := html.ParseFragment(strings.NewReader(in), body)
	if err != nil {
		return in
	}
	for _, n := range nodes {
		rewriteTaskLists(n)
	}
	var buf bytes.Buffer
	for _, n := range nodes {
		if err := html.Render(&buf, n); err != nil {
			return in
		}
	}
	return buf.String()
}

// rewriteTaskLists walks depth first so a nested checklist is rewritten
// before the item that holds it is read.
func rewriteTaskLists(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		rewriteTaskLists(c)
	}
	if n.Type != html.ElementNode || n.DataAtom != atom.Ul {
		return
	}
	var items []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.DataAtom != atom.Li {
			continue
		}
		if checkboxOf(c) == nil {
			return
		}
		items = append(items, c)
	}
	if len(items) == 0 {
		return
	}
	setAttr(n, "data-type", "taskList")
	for _, li := range items {
		box := checkboxOf(li)
		checked := "false"
		for _, a := range box.Attr {
			if a.Key == "checked" {
				checked = "true"
			}
		}
		box.Parent.RemoveChild(box)
		setAttr(li, "data-type", "taskItem")
		setAttr(li, "data-checked", checked)
		trimLeadingSpace(li)
		wrapInlineRun(li)
	}
}

// checkboxOf finds the checkbox that opens a list item: its first child, or
// the first child of its first paragraph when the list is loose.
func checkboxOf(li *html.Node) *html.Node {
	first := firstContent(li)
	if first != nil && first.DataAtom == atom.P {
		first = firstContent(first)
	}
	if first == nil || first.Type != html.ElementNode || first.DataAtom != atom.Input {
		return nil
	}
	for _, a := range first.Attr {
		if a.Key == "type" && a.Val == "checkbox" {
			return first
		}
	}
	return nil
}

// firstContent is the first child that is not blank text.
func firstContent(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) == "" {
			continue
		}
		return c
	}
	return nil
}

// trimLeadingSpace drops the space goldmark leaves between the box and the
// words, in the item or in its first paragraph.
func trimLeadingSpace(li *html.Node) {
	holder := li
	if p := firstContent(li); p != nil && p.Type == html.ElementNode && p.DataAtom == atom.P {
		holder = p
	}
	if t := holder.FirstChild; t != nil && t.Type == html.TextNode {
		t.Data = strings.TrimLeft(t.Data, " \t\n")
	}
}

// wrapInlineRun puts a tight item's words in a paragraph, as the editor's
// item holds them; a nested list stays beside the paragraph, not inside it.
func wrapInlineRun(li *html.Node) {
	if p := firstContent(li); p != nil && p.Type == html.ElementNode && p.DataAtom == atom.P {
		return
	}
	p := &html.Node{Type: html.ElementNode, DataAtom: atom.P, Data: "p"}
	for c := li.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode && (c.DataAtom == atom.Ul || c.DataAtom == atom.Ol) {
			break
		}
		li.RemoveChild(c)
		p.AppendChild(c)
		c = next
	}
	// The newline goldmark leaves before a nested list is not part of the words.
	if t := p.LastChild; t != nil && t.Type == html.TextNode {
		t.Data = strings.TrimRight(t.Data, " \t\n")
	}
	li.InsertBefore(p, li.FirstChild)
}

func setAttr(n *html.Node, key, val string) {
	for i, a := range n.Attr {
		if a.Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}
