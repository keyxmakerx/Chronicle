package crafting

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// EntityInfo is what the service needs to know about a page another plugin
// owns: its name, and whether it is an Armory item or a character.
type EntityInfo struct {
	ID          string
	Name        string
	IsItem      bool
	IsCharacter bool
}

// Directory reads the campaign's pages. Implemented in internal/app over the
// entities and Armory plugins, so this plugin never reads their tables.
type Directory interface {
	// Entity returns one of the campaign's pages, nil when there is no such
	// page in this campaign.
	Entity(ctx context.Context, campaignID, id string) (*EntityInfo, error)
	// ListItems lists the Armory items the role and user may see; the bool
	// is true when the list was cut short.
	ListItems(ctx context.Context, campaignID string, role int, userID string) ([]Named, bool, error)
	// ListCharacters lists the characters the role and user may see.
	ListCharacters(ctx context.Context, campaignID string, role int, userID string) ([]Named, bool, error)
	// OwnedCharacterIDs lists the characters the user has claimed.
	OwnedCharacterIDs(ctx context.Context, campaignID, userID string) (map[string]bool, error)
	// Party lists the claimed characters the role and user may see.
	Party(ctx context.Context, campaignID string, role int, userID string) ([]Named, error)
}

// RulesSource gives the crafting rules of the campaign's enabled system,
// with the system's name ("" and the defaults when none is enabled).
type RulesSource interface {
	CraftingRules(ctx context.Context, campaignID string) (systems.CraftingRules, string)
}

// Service is crafting's business logic. Every method takes the viewer and
// decides what they may see or change, so a route cannot widen access.
type Service interface {
	Library(ctx context.Context, campaignID string, v Viewer) (*Library, error)
	Catalog(ctx context.Context, campaignID string, v Viewer) (*Catalog, error)
	SaveSettings(ctx context.Context, campaignID string, v Viewer, s Settings) (*Settings, error)

	ListRecipes(ctx context.Context, campaignID string, v Viewer) ([]Recipe, error)
	GetRecipe(ctx context.Context, campaignID, id string, v Viewer) (*Recipe, error)
	CreateRecipe(ctx context.Context, campaignID string, v Viewer, in RecipeInput) (*Recipe, error)
	UpdateRecipe(ctx context.Context, campaignID, id string, v Viewer, in RecipeInput) (*Recipe, error)
	SaveRules(ctx context.Context, campaignID, id string, v Viewer, in RulesInput) (*Recipe, error)
	Approve(ctx context.Context, campaignID, id string, v Viewer) (*Recipe, error)
	DeleteRecipe(ctx context.Context, campaignID, id string, v Viewer) error
	Book(ctx context.Context, campaignID string, v Viewer) ([]BookPage, error)

	SetTags(ctx context.Context, campaignID, itemID string, v Viewer, tags []string) ([]string, error)

	ListBenches(ctx context.Context, campaignID string, v Viewer) ([]Bench, error)
	GetBench(ctx context.Context, campaignID, id string, v Viewer) (*Bench, error)
	CreateBench(ctx context.Context, campaignID string, v Viewer, in BenchInput) (*Bench, error)
	UpdateBench(ctx context.Context, campaignID, id string, v Viewer, in BenchInput) (*Bench, error)
	SaveArrangement(ctx context.Context, campaignID, id, skin string, v Viewer, in ArrangementInput) (*Bench, error)
	SaveUpgrades(ctx context.Context, campaignID, id string, v Viewer, in UpgradesInput) (*Bench, error)
	DeleteBench(ctx context.Context, campaignID, id string, v Viewer) error

	// CanCraftFor says whether the viewer may see a character's Crafting
	// block: a Director, or the character's player.
	CanCraftFor(ctx context.Context, campaignID, characterID string, v Viewer) (bool, error)
	Character(ctx context.Context, campaignID, characterID string, v Viewer) (*CharacterCrafting, error)
	StartProject(ctx context.Context, campaignID, characterID string, v Viewer, in StartInput) (*ProjectView, error)
	RollProject(ctx context.Context, campaignID, id string, v Viewer, in RollInput) (*RollResult, error)
	Experiment(ctx context.Context, campaignID, characterID string, v Viewer, in TryInput) (*TryResult, error)
	BreakDown(ctx context.Context, campaignID, characterID string, v Viewer, in BreakInput) (*BreakResult, error)
	Help(ctx context.Context, campaignID, projectID string, v Viewer, in HelpInput) (*RollResult, error)
	CancelProject(ctx context.Context, campaignID, id string, v Viewer) error

	Projects(ctx context.Context, campaignID string, v Viewer) (*ProjectsPage, error)
	LogRespite(ctx context.Context, campaignID string, v Viewer) (*RespiteResult, error)
	RespitePressed(ctx context.Context, campaignID string, v Viewer) error
	GiveRoll(ctx context.Context, campaignID, id string, v Viewer) (*ProjectView, error)
	SetPoints(ctx context.Context, campaignID, id string, v Viewer, in SetPointsInput) (*ProjectView, error)

	Spoils(ctx context.Context, campaignID string, v Viewer) (*SpoilsPage, error)
	SaveDrops(ctx context.Context, campaignID, pageID string, v Viewer, in DropInput) (*DropView, error)
	DeleteDrops(ctx context.Context, campaignID, pageID string, v Viewer) error
	PutSpoils(ctx context.Context, campaignID string, v Viewer, in PutInput) (*PutResult, error)
	ClearTray(ctx context.Context, campaignID string, v Viewer) error
	ClearSpoil(ctx context.Context, campaignID, id string, v Viewer) error
	ClearSpoilLine(ctx context.Context, campaignID string, id int64, v Viewer) error
	TakeSpoil(ctx context.Context, campaignID string, lineID int64, v Viewer, in TakeInput) (*TakeResult, error)
	// FightEnded puts what a fight's defeated creatures drop in the tray.
	FightEnded(ctx context.Context, campaignID string, v Viewer, f Fight) (*FightSpoils, error)
	// DropNotes says which of a fight's pages have a drop list.
	DropNotes(ctx context.Context, campaignID string, v Viewer, pageIDs []string) (map[string]string, error)

	// SaveAIRecipe and CheckDrops are the AI connection's: a recipe written
	// by an AI (a new one arrives as a draft), and a drop list's checks run
	// at review without saving.
	SaveAIRecipe(ctx context.Context, campaignID string, v Viewer, in AIRecipe, check bool) (*Recipe, error)
	CheckDrops(ctx context.Context, campaignID, pageID string, v Viewer, in DropInput) error
	// SetKindPictures changes what kinds wear and returns every kind as it
	// now stands; SetItemPicture gives one item its own picture, or nil to
	// follow its kind.
	SetKindPictures(ctx context.Context, campaignID string, v Viewer, in KindPicturesInput) ([]Tag, error)
	SetItemPicture(ctx context.Context, campaignID, itemID string, v Viewer, in *MaterialPicture) (*MaterialPicture, error)
}

type service struct {
	repo     Repository
	dir      Directory
	rules    RulesSource
	projects ProjectDeps
	newID    func() string
}

// NewService builds the service. rules may be nil, which means every
// campaign uses the default crafting rules. projects may be empty, and the
// project routes then answer that projects are not set up.
func NewService(repo Repository, dir Directory, rules RulesSource, projects ProjectDeps) Service {
	return &service{repo: repo, dir: dir, rules: rules, projects: projects, newID: uuid.NewString}
}

func (s *service) systemRules(ctx context.Context, campaignID string) (systems.CraftingRules, string) {
	if s.rules == nil {
		return systems.DefaultCraftingRules(), ""
	}
	return s.rules.CraftingRules(ctx, campaignID)
}

func directorOnly(v Viewer) error {
	if !v.Director {
		return errDirectorsOnly()
	}
	return nil
}

// ---------- Library, catalog, settings ----------

func (s *service) Library(ctx context.Context, campaignID string, v Viewer) (*Library, error) {
	set, err := s.repo.GetSettings(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	book, err := s.pictureBook(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	rules, name := s.systemRules(ctx, campaignID)
	return &Library{Tags: book.tags(), TagGroups: TagGroups, Pictures: Pictures, Tools: Tools, Steps: StepKinds, BenchKinds: BenchKinds, Templates: Templates, Skins: Skins,
		Difficulties: Difficulties, Moments: Moments, Rules: rules, System: name, Settings: set, Director: v.Director}, nil
}

func (s *service) Catalog(ctx context.Context, campaignID string, v Viewer) (*Catalog, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	items, cutItems, err := s.dir.ListItems(ctx, campaignID, v.VisibilityRole, v.UserID)
	if err != nil {
		return nil, errInternal(err)
	}
	chars, cutChars, err := s.dir.ListCharacters(ctx, campaignID, v.VisibilityRole, v.UserID)
	if err != nil {
		return nil, errInternal(err)
	}
	tags, err := s.repo.ListTags(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	book, err := s.pictureBook(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := &Catalog{Items: make([]MaterialItem, 0, len(items)), Characters: chars, Truncated: cutItems || cutChars}
	if out.Characters == nil {
		out.Characters = []Named{}
	}
	for _, it := range items {
		out.Items = append(out.Items, MaterialItem{ID: it.ID, Name: it.Name, Tags: knownTags(tags[it.ID]), Picture: book.own(it.ID)})
	}
	out.Tables, out.Skills = []CatalogTable{}, []string{}
	if s.projects.Tables != nil {
		all, err := s.projects.Tables.All(ctx, campaignID)
		if err != nil {
			return nil, errInternal(err)
		}
		if len(all) > maxCatalogTables {
			all, out.Truncated = all[:maxCatalogTables], true
		}
		out.Tables = append(out.Tables, all...)
	}
	// Each sheet is its own read, so only the first few characters' skills
	// are offered; a skill nobody's sheet lists yet can't be picked anyway.
	seen := map[string]bool{}
	for i, ch := range out.Characters {
		if i >= maxSkillSheets {
			out.Truncated = true
			break
		}
		if s.projects.Sheets == nil {
			break
		}
		list, _, err := s.projects.Sheets.Skills(ctx, campaignID, ch.ID)
		if err != nil {
			continue
		}
		for _, k := range list {
			if k = strings.TrimSpace(k); k != "" && !seen[strings.ToLower(k)] {
				seen[strings.ToLower(k)] = true
				out.Skills = append(out.Skills, k)
			}
		}
	}
	sort.Strings(out.Skills)
	return out, nil
}

// The catalog reads each sheet on its own, so it reads only so many, and
// sends only so many tables.
const (
	maxSkillSheets   = 40
	maxCatalogTables = 60
)

func (s *service) SaveSettings(ctx context.Context, campaignID string, v Viewer, in Settings) (*Settings, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	set, err := cleanSettings(in)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveSettings(ctx, campaignID, set); err != nil {
		return nil, errInternal(err)
	}
	return &set, nil
}

// ---------- Recipes ----------

func (s *service) ListRecipes(ctx context.Context, campaignID string, v Viewer) ([]Recipe, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	out, err := s.repo.ListRecipes(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	return out, nil
}

func (s *service) GetRecipe(ctx context.Context, campaignID, id string, v Viewer) (*Recipe, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	return s.load(ctx, campaignID, id)
}

func (s *service) load(ctx context.Context, campaignID, id string) (*Recipe, error) {
	r, ok, err := s.repo.GetRecipe(ctx, campaignID, id)
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errNotFound("recipe not found")
	}
	return &r, nil
}

// facts looks up the pages a recipe names that the stored recipe does not
// already hold, so a save checks only what is new.
func (s *service) facts(ctx context.Context, campaignID string, in RecipeInput, stored *Recipe) (recipeFacts, error) {
	f := recipeFacts{Items: map[string]bool{}, Characters: map[string]bool{}, KeptItems: map[string]bool{}, KeptCharacters: map[string]bool{}}
	f.Rules, _ = s.systemRules(ctx, campaignID)
	if stored != nil {
		for _, m := range stored.Materials {
			if m.ItemID != "" {
				f.KeptItems[m.ItemID] = true
			}
		}
		if stored.Result.ItemID != "" {
			f.KeptItems[stored.Result.ItemID] = true
		}
		for _, id := range stored.Known {
			f.KeptCharacters[id] = true
		}
	}
	check := func(id string) error {
		if id == "" || f.KeptItems[id] || f.KeptCharacters[id] || f.Items[id] || f.Characters[id] {
			return nil
		}
		e, err := s.dir.Entity(ctx, campaignID, id)
		if err != nil {
			return errInternal(err)
		}
		if e != nil {
			f.Items[id] = e.IsItem
			f.Characters[id] = e.IsCharacter
		}
		return nil
	}
	for _, m := range in.Materials {
		if err := check(m.ItemID); err != nil {
			return f, err
		}
	}
	if err := check(in.Result.ItemID); err != nil {
		return f, err
	}
	if len(in.Known) > maxKnown {
		return f, nil
	}
	for _, id := range in.Known {
		if err := check(id); err != nil {
			return f, err
		}
	}
	return f, nil
}

func (s *service) CreateRecipe(ctx context.Context, campaignID string, v Viewer, in RecipeInput) (*Recipe, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	f, err := s.facts(ctx, campaignID, in, nil)
	if err != nil {
		return nil, err
	}
	r, err := cleanRecipe(in, f)
	if err != nil {
		return nil, err
	}
	r.ID = s.newID()
	r.Rules, r.Looks = []Rule{}, map[string]*Look{}
	if err := s.repo.CreateRecipe(ctx, campaignID, v.UserID, r); err != nil {
		if errors.Is(err, errDuplicate) {
			return nil, errNameTaken("recipe")
		}
		return nil, errInternal(err)
	}
	return s.load(ctx, campaignID, r.ID)
}

func (s *service) UpdateRecipe(ctx context.Context, campaignID, id string, v Viewer, in RecipeInput) (*Recipe, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	stored, err := s.load(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if in.Version != stored.Version {
		return nil, errConflict()
	}
	f, err := s.facts(ctx, campaignID, in, stored)
	if err != nil {
		return nil, err
	}
	r, err := cleanRecipe(in, f)
	if err != nil {
		return nil, err
	}
	r.ID = id
	ok, err := s.repo.UpdateRecipe(ctx, campaignID, v.UserID, r, in.Version)
	if err != nil {
		if errors.Is(err, errDuplicate) {
			return nil, errNameTaken("recipe")
		}
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errConflict()
	}
	return s.load(ctx, campaignID, id)
}

func (s *service) SaveRules(ctx context.Context, campaignID, id string, v Viewer, in RulesInput) (*Recipe, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	stored, err := s.load(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if in.Version != stored.Version {
		return nil, errConflict()
	}
	if raw := strings.TrimSpace(string(in.Rules)); raw != "" && raw != "null" && raw[0] != '[' {
		return nil, errInvalid("rules are a list")
	}
	rules, looks := CleanRules(in.Rules), CleanLooks(in.Looks)
	extras := stored.Extras
	if raw := strings.TrimSpace(string(in.Extras)); raw != "" && raw != "null" {
		extras = CleanExtras(in.Extras, len(stored.Materials))
	}
	ok, err := s.repo.UpdateRules(ctx, campaignID, id, rules, looks, extras, in.Version)
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errConflict()
	}
	return s.load(ctx, campaignID, id)
}

func (s *service) Approve(ctx context.Context, campaignID, id string, v Viewer) (*Recipe, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if _, err := s.load(ctx, campaignID, id); err != nil {
		return nil, err
	}
	if err := s.repo.SetDraft(ctx, campaignID, id, false); err != nil {
		return nil, errInternal(err)
	}
	return s.load(ctx, campaignID, id)
}

func (s *service) DeleteRecipe(ctx context.Context, campaignID, id string, v Viewer) error {
	if err := directorOnly(v); err != nil {
		return err
	}
	if _, err := s.load(ctx, campaignID, id); err != nil {
		return err
	}
	if err := s.repo.DeleteRecipe(ctx, campaignID, id); err != nil {
		return errInternal(err)
	}
	return nil
}

// ---------- The recipe book ----------

// Book is the recipes the viewer can read: for a player, every approved
// recipe everyone knows plus those one of their characters has learned; for
// a Director, every approved recipe. Item names come from the items the
// viewer may see, so the book never names an item they cannot.
func (s *service) Book(ctx context.Context, campaignID string, v Viewer) ([]BookPage, error) {
	all, err := s.repo.ListRecipes(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	var owned map[string]bool
	knows := map[string][]string{} // recipe id -> the viewer's characters who know it
	if !v.Director {
		if owned, err = s.dir.OwnedCharacterIDs(ctx, campaignID, v.UserID); err != nil {
			return nil, errInternal(err)
		}
		ids := make([]string, 0, len(owned))
		for id := range owned {
			ids = append(ids, id)
		}
		byChar, err := s.repo.KnownBy(ctx, campaignID, ids)
		if err != nil {
			return nil, errInternal(err)
		}
		for cid, rids := range byChar {
			e, err := s.dir.Entity(ctx, campaignID, cid)
			if err != nil {
				return nil, errInternal(err)
			}
			if e == nil {
				continue
			}
			for _, rid := range rids {
				knows[rid] = append(knows[rid], e.Name)
			}
		}
	}
	items, _, err := s.dir.ListItems(ctx, campaignID, v.VisibilityRole, v.UserID)
	if err != nil {
		return nil, errInternal(err)
	}
	names := make(map[string]string, len(items))
	for _, it := range items {
		names[it.ID] = it.Name
	}
	rules, _ := s.systemRules(ctx, campaignID)
	out := []BookPage{}
	for i := range all {
		r := &all[i]
		if r.Draft {
			continue
		}
		if !v.Director && r.Who == "learned" && len(knows[r.ID]) == 0 {
			continue
		}
		p := bookPage(r, names, rules)
		if r.Who == "learned" {
			p.KnownBy = knows[r.ID]
		}
		out = append(out, p)
	}
	return out, nil
}

// ---------- Material tags ----------

func (s *service) SetTags(ctx context.Context, campaignID, itemID string, v Viewer, tags []string) ([]string, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	clean, err := cleanTags(tags)
	if err != nil {
		return nil, err
	}
	e, err := s.dir.Entity(ctx, campaignID, itemID)
	if err != nil {
		return nil, errInternal(err)
	}
	if e == nil || !e.IsItem {
		return nil, errNotFound("item not found")
	}
	if err := s.repo.SetTags(ctx, campaignID, itemID, clean); err != nil {
		return nil, errInternal(err)
	}
	return clean, nil
}

// ---------- Benches ----------

func (s *service) ListBenches(ctx context.Context, campaignID string, v Viewer) ([]Bench, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	out, err := s.repo.ListBenches(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	return out, nil
}

func (s *service) GetBench(ctx context.Context, campaignID, id string, v Viewer) (*Bench, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	return s.loadBench(ctx, campaignID, id)
}

func (s *service) loadBench(ctx context.Context, campaignID, id string) (*Bench, error) {
	b, ok, err := s.repo.GetBench(ctx, campaignID, id)
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errNotFound("bench not found")
	}
	return &b, nil
}

func cleanBench(in BenchInput) (Bench, error) {
	name, err := cleanName(in.Name, maxBenchNameLen, "the bench's name")
	if err != nil {
		return Bench{}, err
	}
	kind := in.Kind
	if kind == "" {
		kind = BenchKinds[0].Key
	}
	if benchKind(kind) == nil {
		return Bench{}, errInvalid(fmt.Sprintf("there is no bench kind %q", in.Kind))
	}
	return Bench{Name: name, Kind: kind}, nil
}

func (s *service) CreateBench(ctx context.Context, campaignID string, v Viewer, in BenchInput) (*Bench, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	b, err := cleanBench(in)
	if err != nil {
		return nil, err
	}
	b.ID = s.newID()
	if err := s.repo.CreateBench(ctx, campaignID, b); err != nil {
		if errors.Is(err, errDuplicate) {
			return nil, errNameTaken("bench")
		}
		return nil, errInternal(err)
	}
	return s.loadBench(ctx, campaignID, b.ID)
}

func (s *service) UpdateBench(ctx context.Context, campaignID, id string, v Viewer, in BenchInput) (*Bench, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if _, err := s.loadBench(ctx, campaignID, id); err != nil {
		return nil, err
	}
	b, err := cleanBench(in)
	if err != nil {
		return nil, err
	}
	b.ID = id
	ok, err := s.repo.UpdateBench(ctx, campaignID, b, in.Version)
	if err != nil {
		if errors.Is(err, errDuplicate) {
			return nil, errNameTaken("bench")
		}
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errConflict()
	}
	return s.loadBench(ctx, campaignID, id)
}

func (s *service) SaveArrangement(ctx context.Context, campaignID, id, skin string, v Viewer, in ArrangementInput) (*Bench, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if !choiceKnown(Skins, skin) {
		return nil, errNotFound("look not found")
	}
	if _, err := s.loadBench(ctx, campaignID, id); err != nil {
		return nil, err
	}
	state, err := cleanArrangement(in.State)
	if err != nil {
		return nil, err
	}
	ok, err := s.repo.SaveArrangement(ctx, campaignID, id, skin, state, in.Version)
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errConflict()
	}
	return s.loadBench(ctx, campaignID, id)
}

func (s *service) SaveUpgrades(ctx context.Context, campaignID, id string, v Viewer, in UpgradesInput) (*Bench, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if _, err := s.loadBench(ctx, campaignID, id); err != nil {
		return nil, err
	}
	ok, err := s.repo.SaveUpgrades(ctx, campaignID, id, CleanUpgrades(in.Upgrades), in.Version)
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errConflict()
	}
	return s.loadBench(ctx, campaignID, id)
}

func (s *service) DeleteBench(ctx context.Context, campaignID, id string, v Viewer) error {
	if err := directorOnly(v); err != nil {
		return err
	}
	if _, err := s.loadBench(ctx, campaignID, id); err != nil {
		return err
	}
	if err := s.repo.DeleteBench(ctx, campaignID, id); err != nil {
		return errInternal(err)
	}
	return nil
}
