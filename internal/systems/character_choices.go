// character_choices.go answers "which names can this character field be
// picked from?" for the active game system, so the attributes editor can
// offer a list (Ancestry, Kit, Race, Class) instead of a bare text box.
//
// The stored value stays the chosen entry's NAME string in fields_data: the
// Foundry module writes item names into the same fields, so a richer stored
// shape would break its pulls. This package therefore only SUGGESTS names;
// it never validates a stored value against the list, and free text is
// always allowed.
package systems

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities/cardborders"
)

// ChoiceSourcePackage marks entries that come from a system's own data file.
// Later features register further sources (for example campaign-made
// entries, "campaign").
const ChoiceSourcePackage = "package"

const (
	// maxChoices bounds one list so a hostile or accidental huge data file
	// cannot turn the picker into a multi-megabyte response.
	maxChoices = 1000
	// maxChoiceSummary keeps a row to roughly one line of prose.
	maxChoiceSummary = 200
)

// Choice is one pickable entry.
type Choice struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Source  string `json:"source"`
	// Description and Properties are the entry's authored text and data,
	// untouched (reference markers intact), for features that show more than
	// the one-line Summary, such as a character creator. The picker ignores them.
	Description string         `json:"description,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	// ForName is set on a campaign entry kept to one character, so a picker
	// can say only that character's player and Directors see it.
	ForName string `json:"forName,omitempty"`
	// Status is "draft" or "sent" on a player's entry a Director has not
	// accepted yet; empty otherwise.
	Status string `json:"status,omitempty"`
	// Mine and EntryID are set for the player who wrote the entry.
	Mine    bool  `json:"mine,omitempty"`
	EntryID int64 `json:"entryId,omitempty"`
	// Key is what a hero-choice rule names the entry by: a package entry's
	// slug, a campaign entry's id (never its slug, which a deleted entry
	// hands to the next one of that name).
	Key string `json:"key,omitempty"`
	// State is ChoiceStateAskFirst on an entry the viewer may pick only with
	// a Director's yes; empty for an entry they may simply pick.
	State string `json:"state,omitempty"`
	// Visibility is a campaign entry's EntryVisibility*. Kept off the wire:
	// the Directors' rules screen needs it to say who can read the entry.
	Visibility string `json:"-"`
	// Origin says whose words these are: "standard" (the package's),
	// "campaign" (the campaign's own) or "rewritten" (a campaign entry in a
	// standard entry's place, named by StandsFor). Empty when no campaign
	// overlay ran.
	Origin    string `json:"origin,omitempty"`
	StandsFor string `json:"standsFor,omitempty"`
	// ToldRecently marks a campaign entry the players were told changed
	// within the last two weeks (ToldWindow).
	ToldRecently bool `json:"toldRecently,omitempty"`
	// Parent is the name of the parent step's entry this one belongs to, set
	// on entries of a step that declares creator.from (a subclass lists the
	// class it comes with). Empty on every other entry.
	Parent string `json:"parent,omitempty"`
	// SystemLook is the card look the game system itself gives a package entry
	// the campaign's owner has laid their own look over (LookChanged); nil when
	// the package names no valid border. A Director's plan carries it so the
	// owner can compare and reset; it is never served to players.
	SystemLook *cardborders.Look `json:"-"`
	// LookChanged marks a package entry whose card_border / card_colour are the
	// owner's choice, not the package's.
	LookChanged bool `json:"-"`
}

// ChoiceList is the full answer for one field: the choices plus the system
// they came from, so the UI can say "<System> doesn't list these yet" when
// the list is empty.
type ChoiceList struct {
	FieldKey   string   `json:"fieldKey"`
	SystemID   string   `json:"systemId"`
	SystemName string   `json:"systemName"`
	Choices    []Choice `json:"choices"`
	// CanMake says the viewer may write their own entry for this field, a
	// permission a Director gave them (see EntryGrant).
	CanMake bool `json:"canMake,omitempty"`
	// Rules says what the campaign's hero-choice rules did to this list for
	// the viewer; nil when none applies.
	Rules *FieldRules `json:"rules,omitempty"`
	// Excluded holds the entries the rules took off the list, with their data,
	// so a hero that already holds one stays editable. Never served.
	Excluded []Choice `json:"-"`
}

// ChoiceStateAskFirst marks a Choice the viewer must ask a Director for.
const ChoiceStateAskFirst = RuleAskFirst

// ChoiceSource contributes extra choices for a field. It lets a later feature
// add campaign-made entries (Source "campaign") without this package knowing
// about them. A source returns nil for fields it has nothing for.
type ChoiceSource func(ctx context.Context, campaignID, fieldKey string) ([]Choice, error)

var (
	choiceSourcesMu sync.RWMutex
	choiceSources   []ChoiceSource
)

// choiceViewerKey marks a context whose viewer is one of the campaign's
// Directors, so a ChoiceSource may include entries kept from players.
type choiceViewerKey struct{}

// WithChoiceViewer records whether the pick list is being read by a Director.
func WithChoiceViewer(ctx context.Context, isDirector bool) context.Context {
	return context.WithValue(ctx, choiceViewerKey{}, isDirector)
}

// choiceViewerUserKey carries the account reading the pick list, so a source
// can include entries kept to a character that person plays.
type choiceViewerUserKey struct{}

// WithChoiceViewerUser records which account is reading the pick list.
func WithChoiceViewerUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, choiceViewerUserKey{}, userID)
}

// ChoiceViewerUser reports what WithChoiceViewerUser recorded, "" when none.
func ChoiceViewerUser(ctx context.Context) string {
	v, _ := ctx.Value(choiceViewerUserKey{}).(string)
	return v
}

// ChoiceViewerIsDirector reports what WithChoiceViewer recorded; a context
// without it is treated as a player's, so sources fail closed.
func ChoiceViewerIsDirector(ctx context.Context) bool {
	v, _ := ctx.Value(choiceViewerKey{}).(bool)
	return v
}

// ChoiceOverlay merges the campaign's entries into the package's choices
// for the viewer the context marks, deciding which standard rows a stand-in
// replaces and which are left out. It replaces the plain append of
// ChoiceSources, so a tied entry never shows twice.
type ChoiceOverlay func(ctx context.Context, campaignID, systemID, fieldKey string, pkg []Choice) ([]Choice, error)

var choiceOverlay ChoiceOverlay

// RegisterChoiceOverlay sets the overlay. Call at start-up; the last call wins.
func RegisterChoiceOverlay(fn ChoiceOverlay) {
	choiceSourcesMu.Lock()
	choiceOverlay = fn
	choiceSourcesMu.Unlock()
}

// MakeOwnCheck reports whether a player may write their own entry for a
// field. Like ChoiceSource it lets campaign entries plug in from outside.
type MakeOwnCheck func(ctx context.Context, campaignID, fieldKey, userID string) (bool, error)

var makeOwnChecks []MakeOwnCheck

// RegisterMakeOwnCheck adds a check. Call at start-up.
func RegisterMakeOwnCheck(fn MakeOwnCheck) {
	if fn == nil {
		return
	}
	choiceSourcesMu.Lock()
	makeOwnChecks = append(makeOwnChecks, fn)
	choiceSourcesMu.Unlock()
}

// canMakeOwn asks the registered checks for the viewer in ctx. A Director
// writes entries on their own page, so the offer is for players only.
func canMakeOwn(ctx context.Context, campaignID, fieldKey string) bool {
	user := ChoiceViewerUser(ctx)
	if user == "" || ChoiceViewerIsDirector(ctx) {
		return false
	}
	choiceSourcesMu.RLock()
	checks := append([]MakeOwnCheck(nil), makeOwnChecks...)
	choiceSourcesMu.RUnlock()
	for _, fn := range checks {
		ok, err := fn(ctx, campaignID, fieldKey, user)
		if err != nil {
			slog.Warn("make-own check failed", slog.String("field", fieldKey), slog.Any("error", err))
			continue
		}
		if ok {
			return true
		}
	}
	return false
}

// RegisterChoiceSource adds a source whose entries are appended after the
// system package's own. Call at start-up; a failing source is logged and
// skipped rather than blanking the package's list.
func RegisterChoiceSource(src ChoiceSource) {
	if src == nil {
		return
	}
	choiceSourcesMu.Lock()
	choiceSources = append(choiceSources, src)
	choiceSourcesMu.Unlock()
}

// choiceFieldKeyPattern is a plain identifier. The key ends up in a file
// name, so nothing that could carry a path separator or traversal may pass.
var choiceFieldKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

// choiceBasenamePattern is the same shape SystemDataAPI allows for a data
// file's base name (without .json).
var choiceBasenamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// ValidChoiceFieldKey reports whether key is a plain identifier.
func ValidChoiceFieldKey(key string) bool { return choiceFieldKeyPattern.MatchString(key) }

// pluralBasename turns a field key into the conventional data file base name:
// ancestry -> ancestries, class -> classes, kit -> kits. A vowel before the
// y keeps a plain s (key -> keys).
func pluralBasename(key string) string {
	k := strings.ToLower(key)
	switch {
	case strings.HasSuffix(k, "y") && len(k) > 1 && !strings.ContainsRune("aeiou", rune(k[len(k)-2])):
		return k[:len(k)-1] + "ies"
	case strings.HasSuffix(k, "s"), strings.HasSuffix(k, "x"), strings.HasSuffix(k, "z"),
		strings.HasSuffix(k, "ch"), strings.HasSuffix(k, "sh"):
		return k + "es"
	default:
		return k + "s"
	}
}

var (
	// {@category term|display} reference markers and HTML tags in prose.
	refMarkupPattern = regexp.MustCompile(`\{@[a-zA-Z]+\s+([^}|]*)(?:\|([^}]*))?\}`)
	htmlTagPattern   = regexp.MustCompile(`<[^>]*>`)
	spacePattern     = regexp.MustCompile(`\s+`)
)

// cleanSummary reduces authored text to plain prose for a one-line preview:
// reference markers keep their display text, HTML tags go, whitespace
// collapses, and the result is cut at maxChoiceSummary runes.
func cleanSummary(s string) string {
	s = refMarkupPattern.ReplaceAllStringFunc(s, func(m string) string {
		sub := refMarkupPattern.FindStringSubmatch(m)
		if sub[2] != "" {
			return sub[2]
		}
		return sub[1]
	})
	s = htmlTagPattern.ReplaceAllString(s, "")
	s = strings.TrimSpace(spacePattern.ReplaceAllString(s, " "))
	if utf8.RuneCountInString(s) > maxChoiceSummary {
		r := []rune(s)
		s = strings.TrimSpace(string(r[:maxChoiceSummary-1])) + "…"
	}
	return s
}

// scalarString renders a JSON scalar as text; non-scalars give "".
func scalarString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return fmt.Sprintf("%v", t)
	}
	return ""
}

// loadChoicesFile reads one data file as a list of choices. Packages use
// "slug" while some use "id", and summary text may live in "summary" or
// "description", so all are tolerated. ok is false when the file is absent or
// unreadable, so callers can try the next candidate system.
func loadChoicesFile(path string) (out []Choice, ok bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxDataFileSize {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		slog.Warn("choices data file is not a JSON array of objects",
			slog.String("file", filepath.Base(path)), slog.Any("error", err))
		return nil, false
	}
	seen := map[string]bool{}
	for _, it := range items {
		name := scalarString(it["name"])
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		slug := scalarString(it["slug"])
		if slug == "" {
			slug = scalarString(it["id"])
		}
		summary := scalarString(it["summary"])
		if summary == "" {
			summary = scalarString(it["description"])
		}
		desc, _ := it["description"].(string)
		props, _ := it["properties"].(map[string]any)
		out = append(out, Choice{
			Key:         slug,
			Slug:        slug,
			Name:        name,
			Summary:     cleanSummary(summary),
			Source:      ChoiceSourcePackage,
			Description: desc,
			Properties:  props,
		})
		if len(out) >= maxChoices {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, true
}

// choiceSystem is one system a campaign can draw lists from.
type choiceSystem struct {
	manifest *SystemManifest
	dir      string
}

// CharacterChoiceService resolves a field key to the campaign's pick list.
type CharacterChoiceService interface {
	// CharacterChoices returns the pick list for a character field, empty
	// when the active system has none.
	CharacterChoices(ctx context.Context, campaignID, fieldKey string) ([]Choice, error)
	// CharacterChoiceList is CharacterChoices plus the system's name.
	CharacterChoiceList(ctx context.Context, campaignID, fieldKey string) (*ChoiceList, error)
	// CreatorPlan lists every character field with entries, for the hero creator.
	CreatorPlan(ctx context.Context, campaignID string) (*CreatorPlan, error)
	// RulesTerms reads the rules words (the Rules book glossary) of the system
	// the creator builds from, for the builder's hover meanings.
	RulesTerms(ctx context.Context, campaignID string) (*RulesTerms, error)
	// SetChoiceLimiter plugs in the hero-choice rules. Every list is then
	// narrowed for the viewer after its sources are merged.
	SetChoiceLimiter(l ChoiceLimiter)
	// SetChoiceLooks plugs in the owner's card borders for package entries.
	// Every list then carries them, whoever reads it.
	SetChoiceLooks(r CardLookReader)
}

type characterChoiceService struct {
	addons    addonChecker
	custom    *CampaignSystemManager
	manifests func() []*SystemManifest
	dirOf     func(id string) string
	limiter   ChoiceLimiter
	looks     CardLookReader
}

func (s *characterChoiceService) SetChoiceLimiter(l ChoiceLimiter) { s.limiter = l }

func (s *characterChoiceService) SetChoiceLooks(r CardLookReader) { s.looks = r }

// NewCharacterChoiceService wires the service to the live system registry,
// the campaign's addon switches and its custom uploaded system (either may
// be nil).
func NewCharacterChoiceService(addons addonChecker, custom *CampaignSystemManager) CharacterChoiceService {
	return &characterChoiceService{addons: addons, custom: custom, manifests: Registry, dirOf: Dir}
}

// candidates lists the systems active for the campaign: its custom upload
// first (the owner chose it deliberately), then enabled built-in systems.
func (s *characterChoiceService) candidates(ctx context.Context, campaignID string) []choiceSystem {
	var out []choiceSystem
	if s.custom != nil {
		if m := s.custom.GetManifest(campaignID); m != nil {
			if d := s.custom.Dir(campaignID); d != "" {
				out = append(out, choiceSystem{manifest: m, dir: d})
			}
		}
	}
	if s.addons == nil {
		return out
	}
	for _, m := range s.manifests() {
		if m == nil {
			continue
		}
		on, err := s.addons.IsEnabledForCampaign(ctx, campaignID, m.ID)
		if err != nil || !on {
			continue
		}
		if d := s.dirOf(m.ID); d != "" {
			out = append(out, choiceSystem{manifest: m, dir: d})
		}
	}
	return out
}

// declaredChoices is the explicit data file a manifest field names, if any.
func declaredChoices(m *SystemManifest, fieldKey string) string {
	for _, p := range m.EntityPresets {
		for _, f := range p.Fields {
			if f.Key == fieldKey && f.Choices != "" {
				return strings.TrimSuffix(f.Choices, ".json")
			}
		}
	}
	return ""
}

// declaresField reports whether any preset of the manifest has the field.
func declaresField(m *SystemManifest, fieldKey string) bool {
	for _, p := range m.EntityPresets {
		for _, f := range p.Fields {
			if f.Key == fieldKey {
				return true
			}
		}
	}
	return false
}

func (s *characterChoiceService) CharacterChoices(ctx context.Context, campaignID, fieldKey string) ([]Choice, error) {
	l, err := s.CharacterChoiceList(ctx, campaignID, fieldKey)
	if err != nil {
		return nil, err
	}
	return l.Choices, nil
}

func (s *characterChoiceService) CharacterChoiceList(ctx context.Context, campaignID, fieldKey string) (*ChoiceList, error) {
	if !ValidChoiceFieldKey(fieldKey) {
		return nil, apperror.NewBadRequest("invalid field key")
	}
	res := &ChoiceList{FieldKey: fieldKey, Choices: []Choice{}}

	cands := s.candidates(ctx, campaignID)
	if from := s.creatorFromOf(cands, fieldKey); from != nil {
		return s.dependentChoiceList(ctx, campaignID, fieldKey, from)
	}
	// A system whose own preset declares the field is the best owner of the
	// answer and of the name shown in "<System> doesn't list these yet".
	sort.SliceStable(cands, func(i, j int) bool {
		return declaresField(cands[i].manifest, fieldKey) && !declaresField(cands[j].manifest, fieldKey)
	})
	if len(cands) > 0 {
		res.SystemID, res.SystemName = cands[0].manifest.ID, cands[0].manifest.Name
	}

	for _, c := range cands {
		base := declaredChoices(c.manifest, fieldKey)
		if base == "" || !choiceBasenamePattern.MatchString(base) {
			base = pluralBasename(fieldKey)
		}
		// A Director-only category's file is the Directors' to read, the same
		// rule /data/:file applies, so any other reader's list (a player's,
		// a Scribe's, a visitor's) never draws on it. Any field key reaches
		// here, not only a hero's steps.
		if !ChoiceViewerIsDirector(ctx) && c.manifest.DirectorOnly(base) {
			continue
		}
		dataDir := filepath.Clean(filepath.Join(c.dir, "data"))
		p := filepath.Clean(filepath.Join(dataDir, base+".json"))
		// The base name is validated, but clamp anyway like SystemDataAPI.
		if !strings.HasPrefix(p, dataDir+string(os.PathSeparator)) {
			continue
		}
		if list, ok := loadChoicesFile(p); ok && len(list) > 0 {
			res.Choices = list
			res.SystemID, res.SystemName = c.manifest.ID, c.manifest.Name
			break
		}
	}

	choiceSourcesMu.RLock()
	sources := append([]ChoiceSource(nil), choiceSources...)
	overlay := choiceOverlay
	choiceSourcesMu.RUnlock()
	if overlay != nil {
		merged, err := overlay(ctx, campaignID, res.SystemID, fieldKey, res.Choices)
		if err != nil {
			// No list rather than the package's own: that one still holds
			// entries the Director left out or replaced, and a hero made from
			// it would carry them. The picker and creator say to try again.
			return nil, err
		}
		res.Choices = merged
	}
	if choiceVisitor(ctx) {
		// Sources are not part of what a signed-out reader's catalog serves.
		sources = nil
		res.Choices = visitorChoices(res.Choices)
	}
	for _, src := range sources {
		extra, err := src(ctx, campaignID, fieldKey)
		if err != nil {
			slog.Warn("choice source failed", slog.String("field", fieldKey), slog.Any("error", err))
			continue
		}
		res.Choices = append(res.Choices, extra...)
	}
	// The owner's card borders go on before the rules narrow the list, so the
	// limiter's held-back entries carry them too, and on every reader's list
	// (a Director's, a visitor's), which is why they are not the limiter's job.
	s.applyLooks(ctx, campaignID, res)
	// The rules narrow the merged list last, so package and campaign entries
	// are limited by the same code and every reader of the list agrees.
	if s.limiter != nil {
		if err := s.limiter.Limit(ctx, campaignID, res); err != nil {
			return nil, err
		}
	}
	res.CanMake = !choiceVisitor(ctx) && canMakeOwn(ctx, campaignID, fieldKey)
	return res, nil
}

// creatorFromOf returns the creator.from declaration of a character preset
// field, nil when no active system declares one. Only the character preset
// counts: other presets' fields never become hero steps.
func (s *characterChoiceService) creatorFromOf(cands []choiceSystem, fieldKey string) *CreatorFrom {
	for _, c := range cands {
		p := characterPresetOf(c.manifest)
		if p == nil {
			continue
		}
		for _, f := range p.Fields {
			if f.Key == fieldKey && f.Creator != nil && f.Creator.From != nil {
				return f.Creator.From
			}
		}
	}
	return nil
}

// dependentChoiceList builds the list of a step whose entries live inside
// the entries of another step (subclasses inside classes). It reads the
// parent through CharacterChoiceList, so whatever the viewer may not pick at
// the parent (Director limits, visitor reach) never yields children either.
// The hero-choice rules and campaign entries do not apply to the derived
// entries: they have no list of their own to be named by.
func (s *characterChoiceService) dependentChoiceList(ctx context.Context, campaignID, fieldKey string, from *CreatorFrom) (*ChoiceList, error) {
	parent, err := s.CharacterChoiceList(ctx, campaignID, from.Step)
	if err != nil {
		return nil, err
	}
	res := &ChoiceList{FieldKey: fieldKey, SystemID: parent.SystemID, SystemName: parent.SystemName, Choices: []Choice{}}
	// Derived entries bypass the overlay and the limiter, but an owner's card
	// border still applies to them.
	defer s.applyLooks(ctx, campaignID, res)
	for _, pe := range parent.Choices {
		items, _ := pe.Properties[from.Property].([]any)
		seen := map[string]bool{}
		for _, it := range items {
			c, ok := derivedChoice(pe, it)
			if !ok || seen[strings.ToLower(c.Name)] {
				continue
			}
			seen[strings.ToLower(c.Name)] = true
			res.Choices = append(res.Choices, c)
			if len(res.Choices) >= maxChoices {
				return res, nil
			}
		}
	}
	return res, nil
}

// derivedChoice turns one item of a parent entry's list (a name, or an
// object with a name) into an entry. An object's other keys become the
// entry's properties, so a creator leaf can show them like any entry's.
func derivedChoice(parent Choice, item any) (Choice, bool) {
	var name, summary, desc string
	var props map[string]any
	switch t := item.(type) {
	case string:
		name = strings.TrimSpace(t)
	case map[string]any:
		name = scalarString(t["name"])
		summary = scalarString(t["summary"])
		desc, _ = t["description"].(string)
		if summary == "" {
			summary = scalarString(t["description"])
		}
		for k, v := range t {
			if k == "name" || k == "summary" || k == "description" {
				continue
			}
			if props == nil {
				props = map[string]any{}
			}
			props[k] = v
		}
	}
	if name == "" {
		return Choice{}, false
	}
	key := parent.Slug + "--" + slugifyName(name)
	return Choice{
		Key: key, Slug: key, Name: name, Summary: cleanSummary(summary),
		Source: ChoiceSourcePackage, Description: desc, Properties: props, Parent: parent.Name,
	}, true
}

var nonSlugRun = regexp.MustCompile(`[^a-z0-9]+`)

// slugifyName makes a lower-case, hyphen-joined key from an item's name.
func slugifyName(name string) string {
	return strings.Trim(nonSlugRun.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

// applyLooks lays the owner's card borders over the package entries of a list.
// A package entry's properties map is shared and cached across campaigns, so a
// changed entry gets a copy; the package's own look is recorded first so a
// Director can see what a reset would give back. A campaign's own entries are
// untouched: their look is their own properties. A failed read leaves the
// package looks in place, since a look is not a rule and must not take the
// pick list down.
func (s *characterChoiceService) applyLooks(ctx context.Context, campaignID string, l *ChoiceList) {
	if s.looks == nil || l == nil || l.SystemID == "" || len(l.Choices) == 0 {
		return
	}
	rows, err := s.looks.CardLooks(ctx, campaignID, l.SystemID, l.FieldKey)
	if err != nil {
		slog.Warn("card looks could not be read; serving the package looks",
			slog.String("field", l.FieldKey), slog.Any("error", err))
		return
	}
	for i := range l.Choices {
		c := &l.Choices[i]
		row, ok := rows[c.Key]
		if !ok || c.Source != ChoiceSourcePackage {
			continue
		}
		// A border that has since left the library is ignored: the package look stands.
		if row.Border != "" {
			if _, known := cardborders.Get(row.Border); !known {
				continue
			}
		}
		if own, has := cardborders.LookFrom(c.Properties); has {
			c.SystemLook = &own
		}
		c.LookChanged = true
		props := make(map[string]any, len(c.Properties)+2)
		for k, v := range c.Properties {
			if k != cardborders.PropBorder && k != cardborders.PropColour {
				props[k] = v
			}
		}
		if row.Border != "" {
			props[cardborders.PropBorder] = row.Border
			if cardborders.ValidColour(row.Colour) {
				props[cardborders.PropColour] = strings.ToLower(row.Colour)
			}
		}
		c.Properties = props
	}
}
