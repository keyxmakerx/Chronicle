package systems

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// A hero's sheet stores the NAME of its ancestry, culture, career or kit,
// never a slug, and the name may be the printed one, an old one or one the
// campaign left out. So a sheet links to an entry by kind and name, and the
// address answers for the reader exactly as the entry's own address would,
// through the catalog's ResolveName: the entry the name leads to for them,
// or the left-out line.

// maxNamedLen bounds a name in an address; entry names are at most 100.
const maxNamedLen = 100

// maxToldAsked bounds one marks question; a sheet names a handful.
const maxToldAsked = 20

// namedCategory is the category a kind word names: the category's own slug,
// or a character field whose pick list is a rules category ("ancestry").
// "" when it names neither.
func namedCategory(m *SystemManifest, kind string) string {
	if findCategory(m, kind) != nil {
		return kind
	}
	if !ValidChoiceFieldKey(kind) {
		return ""
	}
	if cat := kindCategory(m, kind); findCategory(m, cat) != nil && kindField(m, cat) == kind {
		return cat
	}
	return ""
}

// namedTarget checks a kind and a name from an address: the category the
// kind names and the trimmed name, or ok false when either cannot lead to an
// entry this reader may see. A name is measured in characters, as entry
// names are capped, so a name in any script is allowed its full length.
func namedTarget(m *SystemManifest, kind, name string, r RulesReader) (category, trimmed string, ok bool) {
	category = namedCategory(m, kind)
	trimmed = strings.TrimSpace(name)
	if category == "" || hiddenCategory(m, category, r) || trimmed == "" || utf8.RuneCountInString(trimmed) > maxNamedLen {
		return "", "", false
	}
	return category, trimmed, true
}

func (p *RulesPages) resolveNamed(ctx context.Context, sys System, kind, name string, r RulesReader) (*CategoryDef, *Resolution, error) {
	m := sys.Info()
	category, name, ok := namedTarget(m, kind, name, r)
	if !ok {
		return nil, nil, apperror.NewNotFound("entry not found")
	}
	res, err := p.resolveName(ctx, sys, category, name, r)
	if err != nil {
		return nil, nil, err
	}
	if res.Kind == ResolvedNotFound || res.Entry == nil {
		return nil, nil, apperror.NewNotFound("entry not found")
	}
	return findCategory(m, category), res, nil
}

// NamedAddress is the own address of the entry a kind and a name lead to for
// the reader; a left-out name's address answers with its one line.
func (p *RulesPages) NamedAddress(ctx context.Context, sys System, kind, name string, r RulesReader) (string, error) {
	cat, res, err := p.resolveNamed(ctx, sys, kind, name, r)
	if err != nil {
		return "", err
	}
	return EntryURL(r.CampaignID, sys.Info().ID, cat.Slug, res.Entry.Slug, res.Entry.Printed), nil
}

// PeekNamed is the peek of the entry a kind and a name lead to.
func (p *RulesPages) PeekNamed(ctx context.Context, sys System, kind, name, heroID string, r RulesReader) (*EntryView, error) {
	cat, res, err := p.resolveNamed(ctx, sys, kind, name, r)
	if err != nil {
		return nil, err
	}
	return p.peekFrom(ctx, sys, cat, res, heroID, r)
}

// NamedRef is one kind and name a page asks about.
type NamedRef struct {
	Kind string
	Name string
}

// Told returns the refs whose entry, for this reader, carries a change the
// players were told about lately. A name the reader cannot reach is simply
// not told, so the answer gives nothing away. The names of one kind are
// resolved against one reading, so a sheet's question costs the same few
// reads however many names it asks about.
func (p *RulesPages) Told(ctx context.Context, sys System, refs []NamedRef, r RulesReader) ([]NamedRef, error) {
	if len(refs) > maxToldAsked {
		refs = refs[:maxToldAsked]
	}
	type group struct {
		at    []int
		names []string
	}
	m := sys.Info()
	groups := map[string]*group{}
	var cats []string
	for i, ref := range refs {
		cat, name, ok := namedTarget(m, ref.Kind, ref.Name, r)
		if !ok {
			continue
		}
		g := groups[cat]
		if g == nil {
			g = &group{}
			groups[cat] = g
			cats = append(cats, cat)
		}
		g.at = append(g.at, i)
		g.names = append(g.names, name)
	}
	told := make([]bool, len(refs))
	for _, cat := range cats {
		g := groups[cat]
		res, err := p.resolveNames(ctx, sys, cat, g.names, r)
		if err != nil {
			return nil, err
		}
		for k, rr := range res {
			if rr != nil && rr.Entry != nil && rr.Kind != ResolvedNotFound && rr.Kind != ResolvedLeftOut && rr.Entry.ToldRecently {
				told[g.at[k]] = true
			}
		}
	}
	out := []NamedRef{}
	for i, ref := range refs {
		if told[i] {
			out = append(out, ref)
		}
	}
	return out, nil
}
