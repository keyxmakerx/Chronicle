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
}

// ChoiceList is the full answer for one field: the choices plus the system
// they came from, so the UI can say "<System> doesn't list these yet" when
// the list is empty.
type ChoiceList struct {
	FieldKey   string   `json:"fieldKey"`
	SystemID   string   `json:"systemId"`
	SystemName string   `json:"systemName"`
	Choices    []Choice `json:"choices"`
}

// ChoiceSource contributes extra choices for a field. It lets a later feature
// add campaign-made entries (Source "campaign") without this package knowing
// about them. A source returns nil for fields it has nothing for.
type ChoiceSource func(ctx context.Context, campaignID, fieldKey string) ([]Choice, error)

var (
	choiceSourcesMu sync.RWMutex
	choiceSources   []ChoiceSource
)

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
}

type characterChoiceService struct {
	addons    addonChecker
	custom    *CampaignSystemManager
	manifests func() []*SystemManifest
	dirOf     func(id string) string
}

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
	choiceSourcesMu.RUnlock()
	for _, src := range sources {
		extra, err := src(ctx, campaignID, fieldKey)
		if err != nil {
			slog.Warn("choice source failed", slog.String("field", fieldKey), slog.Any("error", err))
			continue
		}
		res.Choices = append(res.Choices, extra...)
	}
	return res, nil
}
