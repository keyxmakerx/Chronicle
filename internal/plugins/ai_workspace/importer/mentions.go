// mentions.go turns `@[Page Name]` and `@[Page Name|shown text]` in an AI
// body into the same anchor the editor writes for an @mention, so an
// imported page gets the hover card, click-to-preview and backlinks that a
// typed mention gets. A name that resolves to no page the operator can see
// is left as its plain words: a link that goes nowhere is worse than none.

package importer

import (
	"context"
	"html"
	"regexp"
	"sort"
	"strings"

	htmltok "golang.org/x/net/html"

	"github.com/keyxmakerx/chronicle/internal/pagelink"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// PageAccess is the slice of the entities service link resolution needs.
// Access is checked per page with the viewer's role so a name never turns
// into a link to a page that viewer cannot open.
type PageAccess interface {
	GetBySlug(ctx context.Context, campaignID, slug string) (*entities.Entity, error)
	CheckEntityAccess(ctx context.Context, entityID string, role int, userID string) (*entities.EffectivePermission, error)
}

// Viewer is who the import runs as, for the access check.
type Viewer struct {
	Role   int
	UserID string
}

// PageLink is the page an @[Name] resolved to.
type PageLink struct{ ID, Name string }

// PageLinks resolves link names to pages: first the pages this same paste
// has saved (Add), then the campaign. It holds its request's context because
// the render hook it plugs into takes only a name.
type PageLinks struct {
	ctx        context.Context
	access     PageAccess
	campaignID string
	viewer     Viewer
	saved      map[string]PageLink // slug → page saved by this paste
	cache      map[string]*PageLink
	missed     map[string]string // slug → name as written, for names that did not resolve
}

// NewPageLinks builds a resolver for one request.
func NewPageLinks(ctx context.Context, access PageAccess, campaignID string, v Viewer) *PageLinks {
	return &PageLinks{
		ctx: ctx, access: access, campaignID: campaignID, viewer: v,
		saved: map[string]PageLink{}, cache: map[string]*PageLink{}, missed: map[string]string{},
	}
}

// Add registers a page this paste saved, under each name it answers to.
func (p *PageLinks) Add(id, name string, asNames ...string) {
	l := PageLink{ID: id, Name: name}
	for _, n := range append([]string{name}, asNames...) {
		if s := entities.Slugify(n); s != "" {
			p.saved[s] = l
		}
	}
}

// Resolve finds the page for a written name, matched by slug like every
// other name in the importer. A miss is remembered for Missed.
func (p *PageLinks) Resolve(name string) (PageLink, bool) {
	slug := entities.Slugify(name)
	if slug == "" {
		return PageLink{}, false
	}
	if l, ok := p.saved[slug]; ok {
		return l, true
	}
	if c, ok := p.cache[slug]; ok {
		if c == nil {
			p.missed[slug] = name
			return PageLink{}, false
		}
		return *c, true
	}
	var found *PageLink
	if e, _ := p.access.GetBySlug(p.ctx, p.campaignID, slug); e != nil && e.CampaignID == p.campaignID {
		if perm, err := p.access.CheckEntityAccess(p.ctx, e.ID, p.viewer.Role, p.viewer.UserID); err == nil && perm != nil && perm.CanView {
			found = &PageLink{ID: e.ID, Name: e.Name}
		}
	}
	p.cache[slug] = found
	if found == nil {
		p.missed[slug] = name
		return PageLink{}, false
	}
	return *found, true
}

// Missed returns the slug and written name of every link that did not resolve.
func (p *PageLinks) Missed() map[string]string { return p.missed }

// RenderOption adjusts MarkdownToHTML.
type RenderOption func(*renderOpts)

type renderOpts struct{ links *PageLinks }

// WithPageLinks makes MarkdownToHTML turn @[Name] into page links. Without
// it the text stays as written.
func WithPageLinks(p *PageLinks) RenderOption {
	return func(o *renderOpts) { o.links = p }
}

// linkRe matches @[Name] and @[Name|text]. Neither part may hold a bracket,
// so a stray "@[" cannot swallow a paragraph.
var linkRe = regexp.MustCompile(`@\[([^\[\]|]+)(?:\|([^\[\]]*))?\]`)

// linkMentions rewrites @[...] in the text nodes of rendered HTML. It runs on
// the HTML, not the markdown, so code samples and existing links keep their
// text, and before sanitize, which then vets the anchors like any other.
func linkMentions(htmlIn string, p *PageLinks) string {
	if !strings.Contains(htmlIn, "@[") {
		return htmlIn
	}
	var out strings.Builder
	z := htmltok.NewTokenizer(strings.NewReader(htmlIn))
	skip := 0 // depth inside <code>, <pre> or <a>
	for {
		tt := z.Next()
		if tt == htmltok.ErrorToken {
			return out.String()
		}
		raw := z.Raw()
		switch tt {
		case htmltok.StartTagToken, htmltok.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "code", "pre", "a":
				if tt == htmltok.StartTagToken {
					skip++
				} else if skip > 0 {
					skip--
				}
			}
			out.Write(raw)
		case htmltok.TextToken:
			if skip > 0 {
				out.Write(raw)
				continue
			}
			out.WriteString(linkText(html.UnescapeString(string(raw)), p))
		default:
			out.Write(raw)
		}
	}
}

// linkText rewrites one text node (already unescaped) and escapes the result.
func linkText(text string, p *PageLinks) string {
	var b strings.Builder
	last := 0
	for _, m := range linkRe.FindAllStringSubmatchIndex(text, -1) {
		b.WriteString(html.EscapeString(text[last:m[0]]))
		last = m[1]
		name := strings.TrimSpace(text[m[2]:m[3]])
		shown := ""
		if m[4] >= 0 {
			shown = strings.TrimSpace(text[m[4]:m[5]])
		}
		target, ok := p.Resolve(name)
		switch {
		case ok && shown != "":
			b.WriteString(pagelink.Anchor(p.campaignID, target.ID, shown))
		case ok:
			// The editor shows a mention as "@" and the page's own name.
			b.WriteString(pagelink.Anchor(p.campaignID, target.ID, "@"+target.Name))
		case shown != "":
			b.WriteString(html.EscapeString(shown))
		default:
			b.WriteString(html.EscapeString(name))
		}
	}
	b.WriteString(html.EscapeString(text[last:]))
	return b.String()
}

// LinkNames lists the page names a markdown body links with @[Name], outside
// code, without repeats, in a stable order.
func LinkNames(markdown string) []string {
	p := NewPageLinks(context.Background(), noPages{}, "-", Viewer{})
	if _, err := MarkdownToHTML(markdown, WithPageLinks(p)); err != nil {
		return nil
	}
	names := make([]string, 0, len(p.missed))
	for _, n := range p.missed {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// PasteSlugs is the set of page names (as slugs) a paste creates or updates,
// so a link to a page later in the same paste is not reported as unknown.
func PasteSlugs(pages []ParsedPage) map[string]bool {
	out := map[string]bool{}
	for _, pg := range pages {
		if pg.Status == StatusParseError || pg.FrontMatter.Action == ActionDelete {
			continue
		}
		if s := entities.Slugify(pg.Name); s != "" {
			out[s] = true
		}
	}
	return out
}

// LinkWarnings names each @[Name] in a body that matches no page the viewer
// can see and no page in the same paste. A warning, never a block: at commit
// the words stay as plain text.
func LinkWarnings(p *PageLinks, markdown string, inPaste map[string]bool) []string {
	var out []string
	for _, n := range LinkNames(markdown) {
		if inPaste[entities.Slugify(n)] {
			continue
		}
		if _, ok := p.Resolve(n); !ok {
			out = append(out, "The page link @["+n+"] matches no page in this campaign or paste; it will be saved as plain text.")
		}
	}
	return out
}

// noPages resolves nothing, for reading link names out of a body.
type noPages struct{}

func (noPages) GetBySlug(context.Context, string, string) (*entities.Entity, error) { return nil, nil }
func (noPages) CheckEntityAccess(context.Context, string, int, string) (*entities.EffectivePermission, error) {
	return nil, nil
}
