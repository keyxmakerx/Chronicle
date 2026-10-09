package systems

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// Limits keep one campaign's entries from growing without bound and keep a
// row to something a picker and a Foundry sync can carry.
const (
	maxEntryName        = 100
	maxEntrySummary     = maxChoiceSummary
	maxEntryDescription = 20000
	maxEntriesPerField  = 500
	maxEntryProps       = 20
	maxEntryPropKey     = 40
	maxEntryPropString  = 200
	maxEntryPropsBytes  = 4096
	maxEntrySlug        = 64
)

var entryPropKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// EntryManifestResolver returns the game system manifest active for a
// campaign (its custom upload, else the chosen built-in), or nil for none.
// The entry's system id is that manifest's id, so entries made under one
// system never show under another.
type EntryManifestResolver func(ctx context.Context, campaignID string) *SystemManifest

// SystemEntryService owns the rules for a campaign's own pick-list entries.
// It never sees a request: callers say who is asking through EntryActor, and
// the service refuses every write unless that is a Director, so a handler
// that forgets its own check still cannot let a player write.
type SystemEntryService interface {
	// List returns the campaign's entries (all fields when fieldKey is "").
	// Non-Directors never receive 'directors' entries.
	List(ctx context.Context, campaignID, fieldKey string, actor EntryActor) ([]SystemEntry, error)
	// FindByName looks an entry up by field and name; nil when absent or not
	// visible to the actor.
	FindByName(ctx context.Context, campaignID, fieldKey, name string, actor EntryActor) (*SystemEntry, error)
	Create(ctx context.Context, campaignID string, actor EntryActor, in CreateSystemEntryInput) (*SystemEntry, error)
	// CheckCreate runs every validation Create would, without writing, so a
	// review screen can refuse a bad entry before anything is committed.
	CheckCreate(ctx context.Context, campaignID string, actor EntryActor, in CreateSystemEntryInput) error
	Update(ctx context.Context, campaignID string, actor EntryActor, id int64, in UpdateSystemEntryInput) (*SystemEntry, error)
	// Delete removes the entry only. Characters keep the name they stored.
	Delete(ctx context.Context, campaignID string, actor EntryActor, id int64) error
	// Choices is the ChoiceSource body: the entries every member may see,
	// shaped for the picker.
	Choices(ctx context.Context, campaignID, fieldKey string) ([]Choice, error)
}

type systemEntryService struct {
	repo     SystemEntryRepository
	manifest EntryManifestResolver
}

// NewSystemEntryService wires the service to its repository and to the
// resolver that says which system a campaign plays.
func NewSystemEntryService(repo SystemEntryRepository, manifest EntryManifestResolver) SystemEntryService {
	return &systemEntryService{repo: repo, manifest: manifest}
}

// RegisterSystemEntryChoices plugs the campaign's entries into the character
// pick lists. Only 'everyone' entries are offered: the hook's context does
// not say who is asking, and a hidden entry must never reach a player.
func RegisterSystemEntryChoices(svc SystemEntryService) {
	RegisterChoiceSource(svc.Choices)
}

func requireDirector(a EntryActor) error {
	if !a.IsDirector {
		return apperror.NewForbidden("only the campaign's Directors can change its system entries")
	}
	return nil
}

func (s *systemEntryService) system(ctx context.Context, campaignID string) (*SystemManifest, error) {
	var m *SystemManifest
	if s.manifest != nil {
		m = s.manifest(ctx, campaignID)
	}
	if m == nil {
		return nil, apperror.NewValidation("This campaign has no game system, so there is nothing to add entries to.")
	}
	if len(m.ID) > 64 {
		return nil, apperror.NewValidation("This game system's id is too long to hold campaign entries.")
	}
	return m, nil
}

// isCharacterStringField reports whether the key is a string field of a
// character preset, the only fields that have pick lists.
func isCharacterStringField(m *SystemManifest, key string) bool {
	for _, p := range m.EntityPresets {
		if p.Category != "character" {
			continue
		}
		for _, f := range p.Fields {
			if f.Key == key && f.Type == "string" {
				return true
			}
		}
	}
	return false
}

func (s *systemEntryService) List(ctx context.Context, campaignID, fieldKey string, actor EntryActor) ([]SystemEntry, error) {
	if fieldKey != "" && !ValidChoiceFieldKey(fieldKey) {
		return nil, apperror.NewValidation("invalid field key")
	}
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil {
		return []SystemEntry{}, nil
	}
	return s.repo.List(ctx, campaignID, m.ID, fieldKey, actor.IsDirector)
}

func (s *systemEntryService) manifestOrNil(ctx context.Context, campaignID string) *SystemManifest {
	if s.manifest == nil {
		return nil
	}
	return s.manifest(ctx, campaignID)
}

func (s *systemEntryService) FindByName(ctx context.Context, campaignID, fieldKey, name string, actor EntryActor) (*SystemEntry, error) {
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil {
		return nil, nil
	}
	e, err := s.repo.FindByName(ctx, campaignID, m.ID, fieldKey, strings.TrimSpace(name))
	if err != nil || e == nil {
		return nil, err
	}
	if e.Visibility == EntryVisibilityDirectors && !actor.IsDirector {
		return nil, nil
	}
	return e, nil
}

func (s *systemEntryService) CheckCreate(ctx context.Context, campaignID string, actor EntryActor, in CreateSystemEntryInput) error {
	_, err := s.prepareCreate(ctx, campaignID, actor, in)
	return err
}

// prepareCreate validates a create and returns the entry ready to store
// (without its slug), so Create and CheckCreate can never disagree.
func (s *systemEntryService) prepareCreate(ctx context.Context, campaignID string, actor EntryActor, in CreateSystemEntryInput) (*SystemEntry, error) {
	if err := requireDirector(actor); err != nil {
		return nil, err
	}
	m, err := s.system(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if !ValidChoiceFieldKey(in.FieldKey) || !isCharacterStringField(m, in.FieldKey) {
		return nil, apperror.NewValidation("That is not a text field of this game system's character sheet, so it has no entries to add.")
	}
	e := &SystemEntry{
		CampaignID: campaignID, SystemID: m.ID, FieldKey: in.FieldKey,
		Name: strings.TrimSpace(in.Name), Summary: strings.TrimSpace(in.Summary),
		Description: in.Description, Properties: in.Properties,
		Visibility: in.Visibility, CreatedBy: actor.UserID,
	}
	if e.Visibility == "" {
		e.Visibility = EntryVisibilityEveryone
	}
	if e.Properties == nil {
		e.Properties = map[string]any{}
	}
	if err := normalizeEntry(e); err != nil {
		return nil, err
	}
	if dup, err := s.repo.FindByName(ctx, campaignID, m.ID, e.FieldKey, e.Name); err != nil {
		return nil, err
	} else if dup != nil {
		return nil, apperror.NewConflict("an entry called " + e.Name + " already exists")
	}
	n, err := s.repo.Count(ctx, campaignID, m.ID, e.FieldKey)
	if err != nil {
		return nil, err
	}
	if n >= maxEntriesPerField {
		return nil, apperror.NewValidation(fmt.Sprintf("A field can hold at most %d campaign entries.", maxEntriesPerField))
	}
	return e, nil
}

func (s *systemEntryService) Create(ctx context.Context, campaignID string, actor EntryActor, in CreateSystemEntryInput) (*SystemEntry, error) {
	e, err := s.prepareCreate(ctx, campaignID, actor, in)
	if err != nil {
		return nil, err
	}
	if e.Slug, err = s.uniqueSlug(ctx, campaignID, e.SystemID, e.FieldKey, e.Name); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, e); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, campaignID, e.ID)
}

func (s *systemEntryService) Update(ctx context.Context, campaignID string, actor EntryActor, id int64, in UpdateSystemEntryInput) (*SystemEntry, error) {
	if err := requireDirector(actor); err != nil {
		return nil, err
	}
	e, err := s.repo.Get(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if in.Name.IsNull() {
		return nil, apperror.NewValidation("A name cannot be cleared.")
	}
	if in.Visibility.IsNull() {
		return nil, apperror.NewValidation("Visibility cannot be cleared.")
	}
	renamed := false
	if v, ok := in.Name.Get(); ok {
		v = strings.TrimSpace(v)
		renamed = !strings.EqualFold(v, e.Name)
		e.Name = v
	}
	e.Summary = in.Summary.Val(e.Summary)
	if in.Summary.IsNull() {
		e.Summary = ""
	}
	e.Description = in.Description.Val(e.Description)
	if in.Description.IsNull() {
		e.Description = ""
	}
	e.Visibility = in.Visibility.Val(e.Visibility)
	if in.Properties.IsNull() {
		e.Properties = map[string]any{}
	} else if v, ok := in.Properties.Get(); ok {
		e.Properties = v
	}
	if err := normalizeEntry(e); err != nil {
		return nil, err
	}
	if renamed {
		if dup, err := s.repo.FindByName(ctx, campaignID, e.SystemID, e.FieldKey, e.Name); err != nil {
			return nil, err
		} else if dup != nil && dup.ID != e.ID {
			return nil, apperror.NewConflict("an entry called " + e.Name + " already exists")
		}
	}
	if err := s.repo.Update(ctx, e); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, campaignID, id)
}

func (s *systemEntryService) Delete(ctx context.Context, campaignID string, actor EntryActor, id int64) error {
	if err := requireDirector(actor); err != nil {
		return err
	}
	ok, err := s.repo.Delete(ctx, campaignID, id)
	if err != nil {
		return err
	}
	if !ok {
		return apperror.NewNotFound("entry not found")
	}
	return nil
}

func (s *systemEntryService) Choices(ctx context.Context, campaignID, fieldKey string) ([]Choice, error) {
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil || !ValidChoiceFieldKey(fieldKey) {
		return nil, nil
	}
	entries, err := s.repo.List(ctx, campaignID, m.ID, fieldKey, false)
	if err != nil {
		return nil, err
	}
	out := make([]Choice, 0, len(entries))
	for _, e := range entries {
		sum := e.Summary
		if sum == "" {
			sum = cleanSummary(e.Description)
		}
		out = append(out, Choice{
			Slug: e.Slug, Name: e.Name, Summary: sum, Source: ChoiceSourceCampaign,
			Description: e.Description, Properties: e.Properties,
		})
	}
	return out, nil
}

// normalizeEntry validates and cleans the editable fields in place. The
// description is sanitized here, at the one write path, so no caller can
// store script or event-handler markup.
func normalizeEntry(e *SystemEntry) error {
	if e.Name == "" {
		return apperror.NewValidation("A name is required.")
	}
	if utf8.RuneCountInString(e.Name) > maxEntryName {
		return apperror.NewValidation(fmt.Sprintf("A name can be at most %d characters.", maxEntryName))
	}
	e.Summary = strings.TrimSpace(e.Summary)
	if utf8.RuneCountInString(e.Summary) > maxEntrySummary {
		return apperror.NewValidation(fmt.Sprintf("A summary can be at most %d characters.", maxEntrySummary))
	}
	if utf8.RuneCountInString(e.Description) > maxEntryDescription {
		return apperror.NewValidation(fmt.Sprintf("A description can be at most %d characters.", maxEntryDescription))
	}
	e.Description = sanitize.HTML(e.Description)
	switch e.Visibility {
	case EntryVisibilityEveryone, EntryVisibilityDirectors:
	default:
		return apperror.NewValidation("Visibility must be \"everyone\" or \"directors\".")
	}
	return validateEntryProperties(e.Properties)
}

// ValidateEntryProperties is validateEntryProperties for callers outside the
// package (AI import's review step).
func ValidateEntryProperties(p map[string]any) error { return validateEntryProperties(p) }

// validateEntryProperties accepts a flat object of strings, numbers and
// booleans, so a consumer never has to walk nested data and the Go-syntax
// garbage the reference browser prints for non-scalars cannot appear.
func validateEntryProperties(p map[string]any) error {
	if len(p) > maxEntryProps {
		return apperror.NewValidation(fmt.Sprintf("At most %d details are allowed.", maxEntryProps))
	}
	for k, v := range p {
		if !entryPropKeyPattern.MatchString(k) || len(k) > maxEntryPropKey {
			return apperror.NewValidation("Detail names must start with a letter and use only letters, digits and underscores.")
		}
		switch t := v.(type) {
		case string:
			if utf8.RuneCountInString(t) > maxEntryPropString {
				return apperror.NewValidation(fmt.Sprintf("The detail %q is longer than %d characters.", k, maxEntryPropString))
			}
		case bool, float64, int, int64:
		default:
			return apperror.NewValidation(fmt.Sprintf("The detail %q must be text, a number or yes/no.", k))
		}
	}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > maxEntryPropsBytes {
		return apperror.NewValidation("The details are too large.")
	}
	return nil
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

// slugFromName lowercases a name to a-z0-9 words joined by "-". A name with
// no ASCII letters or digits falls back to "entry" so the slug is never empty.
func slugFromName(name string) string {
	s := strings.Trim(slugUnsafe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if s == "" {
		s = "entry"
	}
	if len(s) > maxEntrySlug-5 {
		s = strings.Trim(s[:maxEntrySlug-5], "-")
	}
	return s
}

func (s *systemEntryService) uniqueSlug(ctx context.Context, campaignID, systemID, fieldKey, name string) (string, error) {
	base := slugFromName(name)
	slug := base
	for i := 2; i < 1000; i++ {
		taken, err := s.repo.SlugExists(ctx, campaignID, systemID, fieldKey, slug)
		if err != nil {
			return "", err
		}
		if !taken {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	return "", apperror.NewConflict("could not find a free slug for that name")
}
