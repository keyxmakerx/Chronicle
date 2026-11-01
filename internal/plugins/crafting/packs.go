package crafting

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// A recipe pack is a set of crafting recipes saved with their materials'
// names, so another campaign can bring them in and match each material to
// one of its own. What crosses between campaigns is only what packBody
// holds: names, kinds, quantities, the steps, the project's numbers and
// words, the cost and the bench colours. A recipe's rules (which can name
// secret pages and hold the Director's notes), its looks and extras, who
// knows it, its draft prompt, any id and any page's content stay behind.
// A pack is a copy made when it is saved, so later edits or a page turned
// secret never reach it.
//
// Who may read a pack: a Director of the campaign it was saved in, checked
// again on every read, or any Director on the server once its campaign's
// Director shares it. Other campaigns' names are shown only to their own
// Directors; a shared pack says only that a Director on this server shared
// it.

// CampaignDirectory says which campaigns a user directs (Owner or
// co-Director). Implemented in internal/app over the campaigns plugin.
type CampaignDirectory interface {
	DirectedBy(ctx context.Context, userID string) ([]Named, error)
	Directs(ctx context.Context, campaignID, userID string) (bool, error)
}

// Pack limits.
const (
	PackFormat       = "chronicle-recipe-pack"
	packVersion      = 1
	maxPackRecipes   = 200
	maxPackMaterials = 600
	maxPackBytes     = 1 << 20
	maxPackList      = 200
)

// packBody is a pack as it is stored and as its file holds it.
type packBody struct {
	Format    string         `json:"format"`
	Version   int            `json:"version"`
	Name      string         `json:"name"`
	Materials []PackMaterial `json:"materials"`
	Recipes   []PackRecipe   `json:"recipes"`
}

// PackMaterial is an item a pack's recipes ask for or make, by name.
type PackMaterial struct {
	Name  string   `json:"name"`
	Kinds []string `json:"kinds"`
}

// PackLine is one material line: a pack material by name, or any item of a
// kind.
type PackLine struct {
	Material string `json:"material,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Qty      int    `json:"qty"`
}

// PackRecipe is one recipe in a pack. Makes names a pack material.
type PackRecipe struct {
	Name         string     `json:"name"`
	Template     string     `json:"template,omitempty"`
	Bench        string     `json:"bench,omitempty"`
	Lines        []PackLine `json:"lines"`
	Steps        []Step     `json:"steps"`
	Makes        string     `json:"makes,omitempty"`
	MakesQty     int        `json:"makesQty,omitempty"`
	Goal         int        `json:"goal"`
	Stat         string     `json:"stat,omitempty"`
	Prerequisite string     `json:"prerequisite,omitempty"`
	Source       string     `json:"source,omitempty"`
	Cost         Cost       `json:"cost"`
	Who          string     `json:"who,omitempty"`
	Colors       Colors     `json:"colors"`
}

// PackView is a pack as the Workshop lists it. From is "here" (this
// campaign), "campaign" (another the viewer directs) or "server" (shared by
// a Director on this server).
type PackView struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	From     string    `json:"from"`
	Campaign string    `json:"campaign,omitempty"`
	File     bool      `json:"file,omitempty"`
	Shared   bool      `json:"shared"`
	Count    int       `json:"count"`
	Updated  time.Time `json:"updated"`
}

// PackDetail is one pack opened to bring in: its recipes and materials, and
// for each material the campaign's item of the same name, if any.
type PackDetail struct {
	PackView
	Recipes   []PackRecipe   `json:"recipes"`
	Materials []PackMaterial `json:"materials"`
	// Matches maps a material's name to the id of this campaign's item of
	// the same name.
	Matches map[string]string `json:"matches"`
}

// SavePackInput saves recipes as a pack.
type SavePackInput struct {
	Name    string   `json:"name"`
	Recipes []string `json:"recipes"`
	Shared  bool     `json:"shared"`
}

// PackChange renames a pack or shares it.
type PackChange struct {
	Name   string `json:"name"`
	Shared bool   `json:"shared"`
}

// ImportInput brings a pack in. Matches maps a material's name to an item
// id of this campaign, or "" to make it new; a material left out matches
// an item of the same name, else is made new. Recipes names the ones to
// bring in; none means all.
type ImportInput struct {
	Matches map[string]string `json:"matches"`
	Recipes []string          `json:"recipes"`
}

// ImportResult says what bringing a pack in did.
type ImportResult struct {
	Made    []Named       `json:"made"`
	Items   []Named       `json:"items"`
	Skipped []PackSkipped `json:"skipped"`
}

// PackSkipped is a recipe left out, with why.
type PackSkipped struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

func (s *service) packsReady() error {
	if s.projects.Packs == nil || s.projects.Campaigns == nil {
		return apperror.NewNotFound("recipe packs are not set up on this server")
	}
	return nil
}

// ListPacks lists the packs a Director may bring in: this campaign's, those
// of the other campaigns they direct, and shared ones.
func (s *service) ListPacks(ctx context.Context, campaignID string, v Viewer) ([]PackView, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if err := s.packsReady(); err != nil {
		return nil, err
	}
	mine, err := s.projects.Campaigns.DirectedBy(ctx, v.UserID)
	if err != nil {
		return nil, errInternal(err)
	}
	names := map[string]string{}
	ids := []string{campaignID}
	for _, c := range mine {
		if c.ID != campaignID {
			names[c.ID] = c.Name
			ids = append(ids, c.ID)
		}
	}
	rows, err := s.projects.Packs.ListPacks(ctx, ids, true, maxPackList)
	if err != nil {
		return nil, errInternal(err)
	}
	out := make([]PackView, 0, len(rows))
	for _, p := range rows {
		out = append(out, packView(p, campaignID, names))
	}
	order := map[string]int{"here": 0, "campaign": 1, "server": 2}
	sort.SliceStable(out, func(i, j int) bool { return order[out[i].From] < order[out[j].From] })
	return out, nil
}

func packView(p PackRow, campaignID string, directed map[string]string) PackView {
	v := PackView{ID: p.ID, Name: p.Name, File: p.Source == "file", Shared: p.Shared, Count: p.RecipeCount, Updated: p.UpdatedAt}
	switch {
	case p.CampaignID == campaignID:
		v.From = "here"
	case directed[p.CampaignID] != "":
		v.From, v.Campaign = "campaign", directed[p.CampaignID]
	default:
		v.From = "server"
	}
	return v
}

// readablePack loads a pack the viewer may read, checking again that they
// direct its campaign or that it is shared. Anything else is a 404, so a
// pack's existence is never confirmed to someone who can't read it.
func (s *service) readablePack(ctx context.Context, campaignID, id string, v Viewer) (PackRow, map[string]string, error) {
	p, ok, err := s.projects.Packs.GetPack(ctx, id)
	if err != nil {
		return p, nil, errInternal(err)
	}
	if !ok {
		return p, nil, errNotFound("pack not found")
	}
	names := map[string]string{}
	if p.CampaignID == campaignID {
		return p, names, nil
	}
	directs, err := s.projects.Campaigns.Directs(ctx, p.CampaignID, v.UserID)
	if err != nil {
		return p, nil, errInternal(err)
	}
	if directs {
		mine, err := s.projects.Campaigns.DirectedBy(ctx, v.UserID)
		if err != nil {
			return p, nil, errInternal(err)
		}
		for _, c := range mine {
			names[c.ID] = c.Name
		}
		return p, names, nil
	}
	if p.Shared {
		return p, names, nil
	}
	return p, nil, errNotFound("pack not found")
}

// PackDetail opens a pack to bring in.
func (s *service) PackDetail(ctx context.Context, campaignID, id string, v Viewer) (*PackDetail, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if err := s.packsReady(); err != nil {
		return nil, err
	}
	p, names, err := s.readablePack(ctx, campaignID, id, v)
	if err != nil {
		return nil, err
	}
	body, err := decodePack([]byte(p.Body))
	if err != nil {
		return nil, err
	}
	out := &PackDetail{PackView: packView(p, campaignID, names), Recipes: body.Recipes, Materials: body.Materials, Matches: map[string]string{}}
	items, _, err := s.dir.ListItems(ctx, campaignID, v.VisibilityRole, v.UserID)
	if err != nil {
		return nil, errInternal(err)
	}
	byName := map[string]string{}
	for _, it := range items {
		byName[strings.ToLower(it.Name)] = it.ID
	}
	for _, m := range body.Materials {
		if id := byName[strings.ToLower(m.Name)]; id != "" {
			out.Matches[m.Name] = id
		}
	}
	return out, nil
}

// SavePack saves some of this campaign's recipes as a pack.
func (s *service) SavePack(ctx context.Context, campaignID string, v Viewer, in SavePackInput) (*PackView, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if err := s.packsReady(); err != nil {
		return nil, err
	}
	name, err := cleanName(in.Name, maxNameLen, "the pack's name")
	if err != nil {
		return nil, err
	}
	if len(in.Recipes) == 0 {
		return nil, errInvalid("pick at least one recipe for the pack")
	}
	if len(in.Recipes) > maxPackRecipes {
		return nil, errInvalid("a pack holds at most 200 recipes")
	}
	all, err := s.repo.ListRecipes(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	items, _, err := s.dir.ListItems(ctx, campaignID, v.VisibilityRole, v.UserID)
	if err != nil {
		return nil, errInternal(err)
	}
	tags, err := s.repo.ListTags(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	body := buildPack(name, all, in.Recipes, items, tags)
	if len(body.Recipes) == 0 {
		return nil, errInvalid("none of those recipes is in this campaign")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, errInternal(err)
	}
	if len(raw) > maxPackBytes {
		return nil, errInvalid("that pack is too big; save fewer recipes in it")
	}
	row := PackRow{ID: s.newID(), CampaignID: campaignID, CreatedBy: v.UserID, Name: name, Source: "saved", Shared: in.Shared, RecipeCount: len(body.Recipes), Body: string(raw)}
	if err := s.projects.Packs.CreatePack(ctx, row); err != nil {
		return nil, errInternal(err)
	}
	pv := packView(row, campaignID, nil)
	pv.Updated = s.now()
	return &pv, nil
}

// buildPack copies the chosen recipes into a pack, naming each item. An
// item that is gone is left out of a recipe's lines; a recipe whose result
// is gone keeps no "makes".
func buildPack(name string, all []Recipe, ids []string, items []Named, tags map[string][]string) packBody {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	itemName := map[string]string{}
	for _, it := range items {
		itemName[it.ID] = it.Name
	}
	body := packBody{Format: PackFormat, Version: packVersion, Name: name, Materials: []PackMaterial{}, Recipes: []PackRecipe{}}
	mats := map[string]bool{}
	addMat := func(id string) string {
		n := itemName[id]
		if n == "" {
			return ""
		}
		if !mats[strings.ToLower(n)] {
			mats[strings.ToLower(n)] = true
			kinds := append([]string{}, knownTags(tags[id])...)
			body.Materials = append(body.Materials, PackMaterial{Name: n, Kinds: kinds})
		}
		return n
	}
	for _, r := range all {
		if !want[r.ID] {
			continue
		}
		pr := PackRecipe{Name: r.Name, Template: r.Template, Bench: r.Bench, Lines: []PackLine{}, Steps: []Step{}, Goal: r.Goal, Stat: r.Stat,
			Prerequisite: r.Prerequisite, Source: r.Source, Cost: r.Cost, Who: r.Who, Colors: r.Colors}
		// Steps point at lines by index, so a dropped line moves them.
		index := map[int]int{}
		for i, m := range r.Materials {
			if m.Tag != "" {
				index[i] = len(pr.Lines)
				pr.Lines = append(pr.Lines, PackLine{Kind: m.Tag, Qty: m.Qty})
				continue
			}
			if n := addMat(m.ItemID); n != "" {
				index[i] = len(pr.Lines)
				pr.Lines = append(pr.Lines, PackLine{Material: n, Qty: m.Qty})
			}
		}
		for _, st := range r.Steps {
			if st.Material != nil {
				if j, ok := index[*st.Material]; ok {
					n := j
					st.Material = &n
				} else {
					st.Material = nil
				}
			}
			pr.Steps = append(pr.Steps, st)
		}
		if r.Result.ItemID != "" {
			if n := addMat(r.Result.ItemID); n != "" {
				pr.Makes, pr.MakesQty = n, r.Result.Qty
			}
		}
		body.Recipes = append(body.Recipes, pr)
	}
	sort.SliceStable(body.Materials, func(i, j int) bool {
		return strings.ToLower(body.Materials[i].Name) < strings.ToLower(body.Materials[j].Name)
	})
	return body
}

// UploadPack keeps a pack file in this campaign, after checking every part
// of it: a file is anyone's writing.
func (s *service) UploadPack(ctx context.Context, campaignID string, v Viewer, raw []byte) (*PackView, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if err := s.packsReady(); err != nil {
		return nil, err
	}
	if len(raw) > maxPackBytes {
		return nil, errInvalid("that file is too big for a recipe pack")
	}
	body, err := decodePack(raw)
	if err != nil {
		return nil, err
	}
	clean, err := json.Marshal(body)
	if err != nil {
		return nil, errInternal(err)
	}
	row := PackRow{ID: s.newID(), CampaignID: campaignID, CreatedBy: v.UserID, Name: body.Name, Source: "file", RecipeCount: len(body.Recipes), Body: string(clean)}
	if err := s.projects.Packs.CreatePack(ctx, row); err != nil {
		return nil, errInternal(err)
	}
	pv := packView(row, campaignID, nil)
	pv.Updated = s.now()
	return &pv, nil
}

// decodePack reads and checks a pack: the format, names, kinds, lines that
// point at the pack's own materials, and every limit. Steps and numbers
// are checked again when a recipe is brought in, by the same rules as any
// recipe save.
func decodePack(raw []byte) (packBody, error) {
	var b packBody
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if err := dec.Decode(&b); err != nil {
		return b, errInvalid("that isn't a recipe pack file")
	}
	if b.Format != PackFormat {
		return b, errInvalid("that isn't a recipe pack file")
	}
	if b.Version != packVersion {
		return b, errInvalid("that recipe pack was made by a newer Chronicle")
	}
	var err error
	if b.Name, err = cleanName(b.Name, maxNameLen, "the pack's name"); err != nil {
		return b, err
	}
	if len(b.Recipes) == 0 {
		return b, errInvalid("the pack has no recipes")
	}
	if len(b.Recipes) > maxPackRecipes || len(b.Materials) > maxPackMaterials {
		return b, errInvalid("the pack holds more recipes or materials than Chronicle brings in at once")
	}
	known := map[string]bool{}
	for i := range b.Materials {
		m := &b.Materials[i]
		if m.Name, err = cleanName(m.Name, maxNameLen, "a material's name"); err != nil {
			return b, err
		}
		if m.Kinds, err = cleanKinds(m.Kinds); err != nil {
			return b, err
		}
		known[strings.ToLower(m.Name)] = true
	}
	seen := map[string]bool{}
	for i := range b.Recipes {
		r := &b.Recipes[i]
		if r.Name, err = cleanName(r.Name, maxNameLen, "a recipe's name"); err != nil {
			return b, err
		}
		if seen[strings.ToLower(r.Name)] {
			return b, errInvalid("the pack has two recipes called " + quote(r.Name))
		}
		seen[strings.ToLower(r.Name)] = true
		if len(r.Lines) > maxMaterials || len(r.Steps) > maxSteps {
			return b, errInvalid(quote(r.Name) + " has more materials or steps than a recipe holds")
		}
		for _, l := range r.Lines {
			switch {
			case l.Material != "" && l.Kind != "":
				return b, errInvalid(quote(r.Name) + ": a line is a material or a kind, not both")
			case l.Material != "":
				if !known[strings.ToLower(l.Material)] {
					return b, errInvalid(quote(r.Name) + " asks for " + quote(l.Material) + ", which the pack doesn't list")
				}
			case l.Kind != "":
				if !tagKnown(l.Kind) {
					return b, errInvalid(quote(r.Name) + ": there is no crafting kind " + quote(l.Kind))
				}
			default:
				return b, errInvalid(quote(r.Name) + " has an empty material line")
			}
		}
		if r.Makes != "" && !known[strings.ToLower(r.Makes)] {
			return b, errInvalid(quote(r.Name) + " makes " + quote(r.Makes) + ", which the pack doesn't list")
		}
		if r.Steps == nil {
			r.Steps = []Step{}
		}
	}
	return b, nil
}

// UpdatePack renames or shares one of this campaign's packs.
func (s *service) UpdatePack(ctx context.Context, campaignID, id string, v Viewer, in PackChange) (*PackView, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if err := s.packsReady(); err != nil {
		return nil, err
	}
	name, err := cleanName(in.Name, maxNameLen, "the pack's name")
	if err != nil {
		return nil, err
	}
	ok, err := s.projects.Packs.UpdatePack(ctx, campaignID, id, name, in.Shared)
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errNotFound("pack not found")
	}
	p, _, err := s.projects.Packs.GetPack(ctx, id)
	if err != nil {
		return nil, errInternal(err)
	}
	pv := packView(p, campaignID, nil)
	return &pv, nil
}

// DeletePack removes one of this campaign's packs. Recipes already brought
// in from it elsewhere are copies and stay.
func (s *service) DeletePack(ctx context.Context, campaignID, id string, v Viewer) error {
	if err := directorOnly(v); err != nil {
		return err
	}
	if err := s.packsReady(); err != nil {
		return err
	}
	if err := s.projects.Packs.DeletePack(ctx, campaignID, id); err != nil {
		return errInternal(err)
	}
	return nil
}

// PackFile is a pack as a file to download, with a file name.
func (s *service) PackFile(ctx context.Context, campaignID, id string, v Viewer) ([]byte, string, error) {
	if err := directorOnly(v); err != nil {
		return nil, "", err
	}
	if err := s.packsReady(); err != nil {
		return nil, "", err
	}
	p, _, err := s.readablePack(ctx, campaignID, id, v)
	if err != nil {
		return nil, "", err
	}
	body, err := decodePack([]byte(p.Body))
	if err != nil {
		return nil, "", err
	}
	raw, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return nil, "", errInternal(err)
	}
	return raw, fileName(p.Name) + ".recipes.json", nil
}

// fileName keeps letters, digits and dashes, so the name is safe in a
// Content-Disposition header.
func fileName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
				b.WriteByte('-')
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "recipe-pack"
	}
	if len(out) > 60 {
		out = strings.Trim(out[:60], "-")
	}
	return out
}

// ImportPack brings a pack's recipes in as drafts. Each material is matched
// to one of this campaign's items or made new as a Director-only item with
// the pack's kinds. A recipe whose name is taken, or that the recipe checks
// refuse, is skipped with the reason; the others still arrive.
func (s *service) ImportPack(ctx context.Context, campaignID, id string, v Viewer, in ImportInput) (*ImportResult, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if err := s.packsReady(); err != nil {
		return nil, err
	}
	p, _, err := s.readablePack(ctx, campaignID, id, v)
	if err != nil {
		return nil, err
	}
	body, err := decodePack([]byte(p.Body))
	if err != nil {
		return nil, err
	}
	pick := map[string]bool{}
	for _, n := range in.Recipes {
		pick[strings.ToLower(strings.TrimSpace(n))] = true
	}
	var chosen []PackRecipe
	for _, r := range body.Recipes {
		if len(pick) == 0 || pick[strings.ToLower(r.Name)] {
			chosen = append(chosen, r)
		}
	}
	if len(chosen) == 0 {
		return nil, errInvalid("pick at least one recipe to bring in")
	}

	items, _, err := s.dir.ListItems(ctx, campaignID, v.VisibilityRole, v.UserID)
	if err != nil {
		return nil, errInternal(err)
	}
	byName := map[string]string{}
	for _, it := range items {
		byName[strings.ToLower(it.Name)] = it.ID
	}
	matches := map[string]string{}
	for k, val := range in.Matches {
		matches[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(val)
	}
	kindsOf := map[string][]string{}
	for _, m := range body.Materials {
		kindsOf[strings.ToLower(m.Name)] = m.Kinds
	}

	// Resolve only the materials the chosen recipes use.
	resolved := map[string]string{}
	out := &ImportResult{Made: []Named{}, Items: []Named{}, Skipped: []PackSkipped{}}
	resolve := func(name string) (string, error) {
		key := strings.ToLower(name)
		if id, ok := resolved[key]; ok {
			return id, nil
		}
		want, given := matches[key]
		if !given {
			want = byName[key]
		}
		if want != "" {
			e, err := s.dir.Entity(ctx, campaignID, want)
			if err != nil {
				return "", errInternal(err)
			}
			if e == nil || !e.IsItem {
				return "", errInvalid(quote(name) + " is matched to something that isn't an Armory item here")
			}
			resolved[key] = want
			return want, nil
		}
		made, err := s.CreateMaterial(ctx, campaignID, v, MaterialInput{Name: name, Kinds: kindsOf[key]})
		if err != nil {
			return "", err
		}
		resolved[key] = made.ID
		out.Items = append(out.Items, Named{ID: made.ID, Name: made.Name})
		return made.ID, nil
	}

	all, err := s.repo.ListRecipes(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	taken := map[string]bool{}
	for _, r := range all {
		taken[strings.ToLower(r.Name)] = true
	}
	for _, r := range chosen {
		if taken[strings.ToLower(r.Name)] {
			out.Skipped = append(out.Skipped, PackSkipped{Name: r.Name, Reason: "a recipe of that name is already here"})
			continue
		}
		ri := RecipeInput{Name: r.Name, Template: r.Template, Bench: r.Bench, Steps: r.Steps, Goal: r.Goal, Stat: r.Stat,
			Prerequisite: r.Prerequisite, Source: r.Source, Cost: r.Cost, Who: r.Who, Colors: r.Colors, Draft: true}
		failed := ""
		for _, l := range r.Lines {
			if l.Kind != "" {
				ri.Materials = append(ri.Materials, Material{Tag: l.Kind, Qty: l.Qty})
				continue
			}
			mid, err := resolve(l.Material)
			if err != nil {
				failed = err.Error()
				break
			}
			ri.Materials = append(ri.Materials, Material{ItemID: mid, Qty: l.Qty})
		}
		if failed == "" && r.Makes != "" {
			mid, err := resolve(r.Makes)
			if err != nil {
				failed = err.Error()
			} else {
				ri.Result = Result{ItemID: mid, Qty: r.MakesQty}
			}
		}
		if failed != "" {
			out.Skipped = append(out.Skipped, PackSkipped{Name: r.Name, Reason: failed})
			continue
		}
		made, err := s.CreateRecipe(ctx, campaignID, v, ri)
		if err != nil {
			out.Skipped = append(out.Skipped, PackSkipped{Name: r.Name, Reason: plainReason(err)})
			continue
		}
		taken[strings.ToLower(made.Name)] = true
		out.Made = append(out.Made, Named{ID: made.ID, Name: made.Name})
	}
	return out, nil
}

// plainReason is an error's words for the Director, never an internal one.
func plainReason(err error) string {
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code < 500 {
		return ae.Message
	}
	return "it couldn't be saved"
}
