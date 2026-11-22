package systems

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities/cardborders"
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
	// maxEntryPropText bounds one text detail (a benefit, a drawback); the
	// bytes cap below still bounds the entry as a whole.
	maxEntryPropText   = 2000
	maxEntryPropsBytes = 32768
	// A detail may also be a list of named items (traits, features), as a
	// package's own entries carry, so the creator can show and sell them.
	maxEntryLists = 6
	// A detail may be a plain list of short words (a quick build, a list of
	// skills), as package entries carry; its own cap keeps them few.
	maxEntryWordLists = 6
	maxEntryListItems = 40
	maxEntryItemName  = 100
	maxEntryItemText  = 2000
	maxEntryItemCost  = 20
	maxEntrySlug      = 64
)

var entryPropKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// EntryManifestResolver returns the game system manifest active for a
// campaign (its custom upload, else the chosen built-in), or nil for none.
// The entry's system id is that manifest's id, so entries made under one
// system never show under another.
type EntryManifestResolver func(ctx context.Context, campaignID string) *SystemManifest

// SystemEntryService owns the rules for a campaign's own pick-list entries.
// It never sees a request: callers say who is asking through EntryActor, and
// the service refuses every write unless that is a Director, or a player a
// Director gave the permission to write their own entry (EntryGrant), so a
// handler that forgets its own check still cannot let a player write.
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
	// UpdateTelling is Update for the entry editor: beside the saved entry
	// it returns the question to ask the Director when the save removed or
	// altered a trait a hero holds (system_entries_tell.go), else nil. role
	// is the actor's visibility role, for which heroes they may see.
	UpdateTelling(ctx context.Context, campaignID string, actor EntryActor, role int, id int64, in UpdateSystemEntryInput) (*SystemEntry, *TellQuestion, error)
	// Delete removes the entry only. Characters keep the name they stored.
	Delete(ctx context.Context, campaignID string, actor EntryActor, id int64) error
	// Choices is the ChoiceSource body: the entries this viewer may see
	// (ChoiceViewerIsDirector, ChoiceViewerUser), shaped for the picker.
	Choices(ctx context.Context, campaignID, fieldKey string) ([]Choice, error)
	// FindCharacter returns the campaign's character page with this name, for
	// callers that name the character an entry is kept to; nil when none.
	FindCharacter(ctx context.Context, campaignID, name string) (*EntryCharacter, error)

	// Grants returns the permissions to write an entry: all of them for a
	// Director, a player's own otherwise.
	Grants(ctx context.Context, campaignID string, actor EntryActor) ([]EntryGrant, error)
	// Grant lets a player write one entry of their own for a pick list, or
	// renews the permission. Directors only.
	Grant(ctx context.Context, campaignID string, actor EntryActor, in GrantInput) (*EntryGrant, error)
	// RevokeGrant takes a permission back; the player can then no longer
	// edit a draft they made with it. Directors only.
	RevokeGrant(ctx context.Context, campaignID string, actor EntryActor, id int64) error
	// CanMake reports whether the user holds an unspent permission for the
	// field, so a pick list can offer "make your own".
	CanMake(ctx context.Context, campaignID, fieldKey, userID string) (bool, error)

	// EntryReading is how the campaign reads the standard entries in its own
	// words: ties, left-out entries, old names and notes
	// (system_entries_reading.go).
	EntryReading
	// SetCardLook sets or clears the card border and colour of one ACCEPTED
	// entry, as a Director's change; nothing else on the entry changes.
	SetCardLook(ctx context.Context, campaignID string, actor EntryActor, id int64, border, colour string) (*SystemEntry, error)
	// EntryCatalog is the one per-reader list every consumer draws on
	// (entry_catalog.go).
	EntryCatalog
}

type systemEntryService struct {
	repo     SystemEntryRepository
	manifest EntryManifestResolver
	chars    EntryCharacters
	rules    EntryRuleForgetter
	looks    EntryLookForgetter
	standard StandardSystemResolver
	people   EntryPeople
}

// EntryRuleForgetter removes the hero-choice rules that name a campaign
// entry, so deleting the entry leaves no rule behind that nothing can show or
// undo. The rules service implements it.
type EntryRuleForgetter interface {
	ForgetEntry(ctx context.Context, campaignID, systemID, fieldKey string, entryID int64) (int, error)
}

// WithEntryRules makes deleting an entry delete the rules that name it.
func WithEntryRules(r EntryRuleForgetter) EntryServiceOption {
	return func(s *systemEntryService) { s.rules = r }
}

// EntryServiceOption adds an optional dependency to the service.
type EntryServiceOption func(*systemEntryService)

// WithEntryCharacters lets entries be kept to one character. Without it an
// entry cannot be made for a character, and one already stored is shown to
// Directors only.
func WithEntryCharacters(c EntryCharacters) EntryServiceOption {
	return func(s *systemEntryService) { s.chars = c }
}

// WithStandardSystem gives the service the game system's own entries, so
// ties, rewrites and the per-reader catalog can read them. Without it the
// catalog holds the campaign's entries only and every tie is refused.
func WithStandardSystem(r StandardSystemResolver) EntryServiceOption {
	return func(s *systemEntryService) { s.standard = r }
}

// NewSystemEntryService wires the service to its repository and to the
// resolver that says which system a campaign plays.
func NewSystemEntryService(repo SystemEntryRepository, manifest EntryManifestResolver, opts ...EntryServiceOption) SystemEntryService {
	s := &systemEntryService{repo: repo, manifest: manifest}
	for _, o := range opts {
		o(s)
	}
	return s
}

// RegisterSystemEntryChoices plugs the campaign's entries into the character
// pick lists through the catalog's merge, for the viewer the pick-list
// handler marked on the context; an unmarked context reads as a player with
// no account, so a hidden entry never reaches a player.
func RegisterSystemEntryChoices(svc SystemEntryService) {
	RegisterChoiceOverlay(svc.PickList)
	RegisterMakeOwnCheck(svc.CanMake)
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
	entries, err := s.repo.List(ctx, campaignID, m.ID, fieldKey, actor.IsDirector)
	if err != nil {
		return nil, err
	}
	return s.visible(ctx, campaignID, entries, actor)
}

// visible keeps the entries this actor may see and shapes them for them.
// Directors see everything, with the name of the character a 'character'
// entry is kept to. Anyone else sees 'everyone' entries and the 'character'
// entries of a character they play, with secret spans stripped.
func (s *systemEntryService) visible(ctx context.Context, campaignID string, entries []SystemEntry, actor EntryActor) ([]SystemEntry, error) {
	owners := map[string]*EntryCharacter{}
	character := func(id string) (*EntryCharacter, error) {
		if c, ok := owners[id]; ok {
			return c, nil
		}
		var c *EntryCharacter
		if s.chars != nil && id != "" {
			var err error
			if c, err = s.chars.Character(ctx, campaignID, id); err != nil {
				return nil, err
			}
		}
		owners[id] = c
		return c, nil
	}
	out := make([]SystemEntry, 0, len(entries))
	for _, e := range entries {
		e.Status = entryStatusOrAccepted(e.Status)
		e.StandsForName = snapshotName(e.StandsForSeen)
		if e.Status != EntryStatusAccepted && !actor.IsDirector {
			// A draft is its writer's and the Directors' only, whatever its
			// visibility will be once accepted.
			if actor.UserID == "" || e.CreatedBy != actor.UserID {
				continue
			}
			if e.Visibility == EntryVisibilityCharacter {
				if c, err := character(e.ForEntityID); err != nil {
					return nil, err
				} else if c != nil {
					e.ForName = c.Name
				}
			}
			e.Description = sanitize.StripSecretsHTML(e.Description)
			out = append(out, e)
			continue
		}
		if actor.IsDirector {
			if e.Visibility == EntryVisibilityCharacter {
				c, err := character(e.ForEntityID)
				if err != nil {
					return nil, err
				}
				if c != nil {
					e.ForName = c.Name
				}
			}
			out = append(out, e)
			continue
		}
		switch e.Visibility {
		case EntryVisibilityEveryone:
		case EntryVisibilityCharacter:
			c, err := character(e.ForEntityID)
			if err != nil {
				return nil, err
			}
			if !playedBy(c, actor.UserID) {
				continue
			}
			e.ForName = c.Name
		default:
			continue
		}
		e.Description = sanitize.StripSecretsHTML(e.Description)
		out = append(out, e)
	}
	return out, nil
}

// snapshotName is the standard entry's name a tie's snapshot recorded.
func snapshotName(seen string) string {
	if seen == "" {
		return ""
	}
	var snap StandardSnapshot
	if json.Unmarshal([]byte(seen), &snap) != nil {
		return ""
	}
	return snap.Name
}

// playedBy reports whether userID plays the character. An unclaimed or
// missing character is played by nobody, so its entry stays with Directors.
func playedBy(c *EntryCharacter, userID string) bool {
	return c != nil && userID != "" && c.OwnerUserID == userID
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
	seen, err := s.visible(ctx, campaignID, []SystemEntry{*e}, actor)
	if err != nil || len(seen) == 0 {
		return nil, err
	}
	return &seen[0], nil
}

// FindCharacter resolves a character page by name for an entry kept to it.
func (s *systemEntryService) FindCharacter(ctx context.Context, campaignID, name string) (*EntryCharacter, error) {
	name = strings.TrimSpace(name)
	if s.chars == nil || name == "" {
		return nil, nil
	}
	return s.chars.CharacterNamed(ctx, campaignID, name)
}

// checkForCharacter makes sure a 'character' entry names a character page
// of this campaign.
func (s *systemEntryService) checkForCharacter(ctx context.Context, campaignID string, e *SystemEntry) error {
	if e.Visibility != EntryVisibilityCharacter {
		return nil
	}
	if s.chars == nil {
		return apperror.NewValidation("Entries can't be kept to one character here.")
	}
	c, err := s.chars.Character(ctx, campaignID, e.ForEntityID)
	if err != nil {
		return err
	}
	if c == nil {
		return apperror.NewValidation("That character isn't a character page in this campaign.")
	}
	return nil
}

// prepareCreate validates a create and returns the entry ready to store
// (without its slug), so Create and CheckCreate can never disagree. For a
// player it also returns the permission the entry will spend.
func (s *systemEntryService) prepareCreate(ctx context.Context, campaignID string, actor EntryActor, in CreateSystemEntryInput) (*SystemEntry, *EntryGrant, error) {
	m, err := s.system(ctx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	if !ValidChoiceFieldKey(in.FieldKey) || !isCharacterStringField(m, in.FieldKey) {
		return nil, nil, apperror.NewValidation("That is not a text field of this game system's character sheet, so it has no entries to add.")
	}
	e := &SystemEntry{
		CampaignID: campaignID, SystemID: m.ID, FieldKey: in.FieldKey,
		Name: strings.TrimSpace(in.Name), Summary: strings.TrimSpace(in.Summary),
		Description: in.Description, Properties: in.Properties,
		Visibility: in.Visibility, ForEntityID: strings.TrimSpace(in.ForEntityID), CreatedBy: actor.UserID,
		Status: EntryStatusAccepted,
	}
	var grant *EntryGrant
	if !actor.IsDirector {
		if grant, err = s.unspentGrant(ctx, campaignID, m.ID, in.FieldKey, actor); err != nil {
			return nil, nil, err
		}
		if in.Visibility != "" {
			return nil, nil, apperror.NewForbidden("a Director decides who sees your entry when they accept it")
		}
		if err := s.playerKeep(ctx, campaignID, e, actor); err != nil {
			return nil, nil, err
		}
		e.Status = EntryStatusDraft
		if in.Status == EntryStatusSent {
			e.Status = EntryStatusSent
		}
	}
	if e.Visibility == "" {
		e.Visibility = EntryVisibilityEveryone
	}
	if e.Properties == nil {
		e.Properties = map[string]any{}
	}
	if grant != nil {
		holdBudget(e.Properties, grant.Budget)
	}
	if err := normalizeEntry(e); err != nil {
		return nil, nil, err
	}
	if err := s.checkForCharacter(ctx, campaignID, e); err != nil {
		return nil, nil, err
	}
	if dup, err := s.repo.FindByName(ctx, campaignID, m.ID, e.FieldKey, e.Name); err != nil {
		return nil, nil, err
	} else if dup != nil {
		return nil, nil, apperror.NewConflict("an entry called " + e.Name + " already exists")
	}
	if err := s.refuseOldName(ctx, campaignID, m.ID, e.FieldKey, e.Name, 0, actor); err != nil {
		return nil, nil, err
	}
	n, err := s.repo.Count(ctx, campaignID, m.ID, e.FieldKey)
	if err != nil {
		return nil, nil, err
	}
	if n >= maxEntriesPerField {
		return nil, nil, apperror.NewValidation(fmt.Sprintf("A field can hold at most %d campaign entries.", maxEntriesPerField))
	}
	return e, grant, nil
}

func (s *systemEntryService) CheckCreate(ctx context.Context, campaignID string, actor EntryActor, in CreateSystemEntryInput) error {
	_, _, err := s.prepareCreate(ctx, campaignID, actor, in)
	return err
}

func (s *systemEntryService) Create(ctx context.Context, campaignID string, actor EntryActor, in CreateSystemEntryInput) (*SystemEntry, error) {
	e, grant, err := s.prepareCreate(ctx, campaignID, actor, in)
	if err != nil {
		return nil, err
	}
	if e.Slug, err = s.uniqueSlug(ctx, campaignID, e.SystemID, e.FieldKey, e.Name); err != nil {
		return nil, err
	}
	// The permission is claimed before the insert, so two saves at once
	// cannot both spend it, and released if the insert fails.
	if grant != nil {
		ok, err := s.repo.ClaimGrant(ctx, campaignID, grant.ID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, apperror.NewForbidden("you have already made your own entry for this list")
		}
	}
	if err := s.repo.Create(ctx, e); err != nil {
		if grant != nil {
			_ = s.repo.SetGrantEntry(ctx, campaignID, grant.ID, 0)
		}
		return nil, err
	}
	if grant != nil {
		if err := s.repo.SetGrantEntry(ctx, campaignID, grant.ID, e.ID); err != nil {
			return nil, err
		}
	}
	return s.repo.Get(ctx, campaignID, e.ID)
}

// unspentGrant is the player's permission to write their own entry for the
// field, refused when they have none or already used it.
func (s *systemEntryService) unspentGrant(ctx context.Context, campaignID, systemID, fieldKey string, actor EntryActor) (*EntryGrant, error) {
	if actor.UserID == "" {
		return nil, apperror.NewForbidden("only the campaign's Directors can change its system entries")
	}
	g, err := s.repo.FindGrant(ctx, campaignID, systemID, fieldKey, actor.UserID)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, apperror.NewForbidden("only the campaign's Directors can change its system entries")
	}
	if g.Spent() {
		return nil, apperror.NewForbidden("you have already made your own entry for this list")
	}
	return g, nil
}

// playerKeep sets who a player's draft is kept to: the character it names,
// which must be one they play, else nobody but them and the Directors.
func (s *systemEntryService) playerKeep(ctx context.Context, campaignID string, e *SystemEntry, actor EntryActor) error {
	if e.ForEntityID == "" {
		e.Visibility = EntryVisibilityDirectors
		return nil
	}
	var c *EntryCharacter
	if s.chars != nil {
		var err error
		if c, err = s.chars.Character(ctx, campaignID, e.ForEntityID); err != nil {
			return err
		}
	}
	if !playedBy(c, actor.UserID) {
		return apperror.NewForbidden("you can only make an entry for a character you play")
	}
	e.Visibility = EntryVisibilityCharacter
	return nil
}

// holdBudget sets every points budget in a player's entry to the one the
// Director gave, so the player decides what their points buy but never how
// many there are. A budget of 0 leaves the entry as written.
func holdBudget(p map[string]any, budget int) {
	if budget <= 0 {
		return
	}
	for k, v := range p {
		if _, ok := wholeNumber(v); ok && strings.HasSuffix(k, "_points") {
			p[k] = budget
		}
	}
}

func (s *systemEntryService) Update(ctx context.Context, campaignID string, actor EntryActor, id int64, in UpdateSystemEntryInput) (*SystemEntry, error) {
	e, err := s.repo.Get(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	e.Status = entryStatusOrAccepted(e.Status)
	var grant *EntryGrant
	if !actor.IsDirector {
		if grant, err = s.writerGrant(ctx, campaignID, e, actor, in); err != nil {
			return nil, err
		}
	}
	if in.Name.IsNull() {
		return nil, apperror.NewValidation("A name cannot be cleared.")
	}
	if in.Visibility.IsNull() {
		return nil, apperror.NewValidation("Visibility cannot be cleared.")
	}
	// Whether the name being given up was ever an accepted entry's, so one
	// others may have built heroes on: a status change in the same save does
	// not change what the old name was.
	wasAccepted := e.Status == EntryStatusAccepted
	if err := applyStatus(e, in, actor.IsDirector); err != nil {
		return nil, err
	}
	renamed := false
	prevName := e.Name
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
	// A character that has since gone keeps its entry Directors-only on read;
	// re-checking an unchanged link would lock the Director out of editing it.
	keptFor := e.Visibility == EntryVisibilityCharacter
	prevFor := e.ForEntityID
	e.Visibility = in.Visibility.Val(e.Visibility)
	if v, ok := in.ForEntityID.Get(); ok {
		e.ForEntityID = strings.TrimSpace(v)
	} else if in.ForEntityID.IsNull() {
		e.ForEntityID = ""
	}
	if grant != nil && in.ForEntityID.Present() {
		if err := s.playerKeep(ctx, campaignID, e, actor); err != nil {
			return nil, err
		}
	}
	if e.Visibility != EntryVisibilityCharacter {
		e.ForEntityID = ""
	}
	// What the row holds now is the only source a carried value may match.
	stored := e.Properties
	if in.Properties.IsNull() {
		e.Properties = map[string]any{}
	} else if v, ok := in.Properties.Get(); ok {
		e.Properties = v
	}
	if grant != nil {
		holdBudget(e.Properties, grant.Budget)
	}
	if err := normalizeEntryCarrying(e, stored); err != nil {
		return nil, err
	}
	if !keptFor || e.ForEntityID != prevFor {
		if err := s.checkForCharacter(ctx, campaignID, e); err != nil {
			return nil, err
		}
	}
	if renamed {
		if dup, err := s.repo.FindByName(ctx, campaignID, e.SystemID, e.FieldKey, e.Name); err != nil {
			return nil, err
		} else if dup != nil && dup.ID != e.ID {
			return nil, apperror.NewConflict("an entry called " + e.Name + " already exists")
		}
		if err := s.refuseOldName(ctx, campaignID, e.SystemID, e.FieldKey, e.Name, e.ID, actor); err != nil {
			return nil, err
		}
	}
	if err := s.repo.Update(ctx, e); err != nil {
		return nil, err
	}
	if renamed {
		if err := s.recordRename(ctx, e, prevName, wasAccepted); err != nil {
			return nil, err
		}
	}
	return s.repo.Get(ctx, campaignID, id)
}

// writerGrant lets a player change their own entry while it is a draft, as
// long as their permission still stands, and returns it for its budget. Who
// sees the entry and the Director's note stay the Director's.
func (s *systemEntryService) writerGrant(ctx context.Context, campaignID string, e *SystemEntry, actor EntryActor, in UpdateSystemEntryInput) (*EntryGrant, error) {
	if actor.UserID == "" || e.CreatedBy != actor.UserID || e.Status == EntryStatusAccepted {
		return nil, apperror.NewForbidden("only the campaign's Directors can change its system entries")
	}
	g, err := s.repo.FindGrant(ctx, campaignID, e.SystemID, e.FieldKey, actor.UserID)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, apperror.NewForbidden("your Director has taken back the permission to make this entry")
	}
	if in.Visibility.Present() || in.ReviewNote.Present() {
		return nil, apperror.NewForbidden("a Director decides who sees your entry when they accept it")
	}
	return g, nil
}

// maxReviewNote matches the review_note column.
const maxReviewNote = 500

// applyStatus moves an entry along: a writer between draft and sent; a
// Director to accepted (which clears the note) or back to draft with a note
// saying what to change. A Director's own entry is always accepted.
func applyStatus(e *SystemEntry, in UpdateSystemEntryInput, director bool) error {
	if in.Status.IsNull() {
		return apperror.NewValidation("Status cannot be cleared.")
	}
	if v, ok := in.Status.Get(); ok {
		switch {
		case v == EntryStatusDraft || v == EntryStatusSent:
			if director && e.Status == EntryStatusAccepted {
				return apperror.NewValidation("An accepted entry cannot go back to being a draft.")
			}
		case v == EntryStatusAccepted && director:
		case director:
			return apperror.NewValidation("Status must be \"draft\", \"sent\" or \"accepted\".")
		default:
			return apperror.NewForbidden("only a Director can accept an entry")
		}
		e.Status = v
	}
	if v, ok := in.ReviewNote.Get(); ok {
		v = strings.TrimSpace(v)
		if utf8.RuneCountInString(v) > maxReviewNote {
			return apperror.NewValidation(fmt.Sprintf("A note can be at most %d characters.", maxReviewNote))
		}
		e.ReviewNote = v
	} else if in.ReviewNote.IsNull() {
		e.ReviewNote = ""
	}
	if e.Status == EntryStatusAccepted {
		e.ReviewNote = ""
	}
	return nil
}

func (s *systemEntryService) Delete(ctx context.Context, campaignID string, actor EntryActor, id int64) error {
	if err := requireDirector(actor); err != nil {
		return err
	}
	e, err := s.repo.Get(ctx, campaignID, id)
	if err != nil {
		return err
	}
	ok, err := s.repo.Delete(ctx, campaignID, id)
	if err != nil {
		return err
	}
	if !ok {
		return apperror.NewNotFound("entry not found")
	}
	// Rules are keyed by the entry's id, which never repeats, so one left
	// behind can never attach to another entry; it is removed anyway so the
	// Directors' list shows none. A failure here is logged, not returned: the
	// entry is already gone and what is left is inert.
	if s.rules != nil {
		if _, err := s.rules.ForgetEntry(ctx, campaignID, e.SystemID, e.FieldKey, id); err != nil {
			slog.Warn("deleting an entry's hero rules failed", slog.Int64("entry_id", id), slog.Any("error", err))
		}
	}
	// Subclass cards derived from this entry are keyed by its slug, which a
	// new entry of the same name would reuse, so their looks go with it.
	if s.looks != nil {
		if _, err := s.looks.ForgetDerived(ctx, campaignID, e.SystemID, e.Slug); err != nil {
			slog.Warn("deleting an entry's derived card looks failed", slog.Int64("entry_id", id), slog.Any("error", err))
		}
	}
	return nil
}

// SetCardLook is the owner's choice of border for a campaign's own entry,
// written into the entry's own properties so the entry stays the one truth.
// Only an accepted entry can be changed this way: a draft or sent one is its
// writer's until a Director accepts it. The entry goes through the normal
// normalising and validation, and is saved only when the two properties
// actually change.
func (s *systemEntryService) SetCardLook(ctx context.Context, campaignID string, actor EntryActor, id int64, border, colour string) (*SystemEntry, error) {
	if err := requireDirector(actor); err != nil {
		return nil, err
	}
	e, err := s.repo.Get(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	e.Status = entryStatusOrAccepted(e.Status)
	if e.Status != EntryStatusAccepted {
		return nil, apperror.NewValidation("Accept this entry first: a card's border is its writer's until a Director accepts the entry.")
	}
	stored := e.Properties
	props := make(map[string]any, len(stored)+2)
	for k, v := range stored {
		if k != cardborders.PropBorder && k != cardborders.PropColour {
			props[k] = v
		}
	}
	if border != "" {
		props[cardborders.PropBorder] = border
		if colour != "" {
			props[cardborders.PropColour] = colour
		}
	}
	if sameLookProps(stored, props) {
		return e, nil
	}
	e.Properties = props
	if err := normalizeEntryCarrying(e, stored); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, e); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, campaignID, id)
}

// sameLookProps reports whether two property maps hold the same border and
// colour, so an unchanged choice does not rewrite the entry.
func sameLookProps(a, b map[string]any) bool {
	for _, k := range []string{cardborders.PropBorder, cardborders.PropColour} {
		av, aok := a[k]
		bv, bok := b[k]
		if aok != bok || !reflect.DeepEqual(av, bv) {
			return false
		}
	}
	return true
}

func (s *systemEntryService) Grants(ctx context.Context, campaignID string, actor EntryActor) ([]EntryGrant, error) {
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil {
		return []EntryGrant{}, nil
	}
	user := ""
	if !actor.IsDirector {
		if actor.UserID == "" {
			return []EntryGrant{}, nil
		}
		user = actor.UserID
	}
	return s.repo.ListGrants(ctx, campaignID, m.ID, user)
}

// maxGrantBudget matches the most points the entry editor offers.
const maxGrantBudget = 20

func (s *systemEntryService) Grant(ctx context.Context, campaignID string, actor EntryActor, in GrantInput) (*EntryGrant, error) {
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
	in.UserID = strings.TrimSpace(in.UserID)
	if in.UserID == "" || len(in.UserID) > 36 {
		return nil, apperror.NewValidation("Say which player may make their own entry.")
	}
	if in.Budget < 0 || in.Budget > maxGrantBudget {
		return nil, apperror.NewValidation(fmt.Sprintf("A budget is 0 to %d points.", maxGrantBudget))
	}
	g := &EntryGrant{CampaignID: campaignID, SystemID: m.ID, FieldKey: in.FieldKey, UserID: in.UserID, Budget: in.Budget, GrantedBy: actor.UserID}
	if err := s.repo.PutGrant(ctx, g); err != nil {
		return nil, err
	}
	return s.repo.FindGrant(ctx, campaignID, m.ID, in.FieldKey, in.UserID)
}

func (s *systemEntryService) RevokeGrant(ctx context.Context, campaignID string, actor EntryActor, id int64) error {
	if err := requireDirector(actor); err != nil {
		return err
	}
	ok, err := s.repo.DeleteGrant(ctx, campaignID, id)
	if err != nil {
		return err
	}
	if !ok {
		return apperror.NewNotFound("permission not found")
	}
	return nil
}

func (s *systemEntryService) CanMake(ctx context.Context, campaignID, fieldKey, userID string) (bool, error) {
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil || userID == "" || !ValidChoiceFieldKey(fieldKey) {
		return false, nil
	}
	g, err := s.repo.FindGrant(ctx, campaignID, m.ID, fieldKey, userID)
	if err != nil || g == nil {
		return false, err
	}
	return !g.Spent(), nil
}

func (s *systemEntryService) Choices(ctx context.Context, campaignID, fieldKey string) ([]Choice, error) {
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil || !ValidChoiceFieldKey(fieldKey) {
		return nil, nil
	}
	// A context the pick-list handler didn't mark counts as a player's with
	// no account, so hidden entries and secret spans stay out by default.
	actor := EntryActor{UserID: ChoiceViewerUser(ctx), IsDirector: ChoiceViewerIsDirector(ctx)}
	entries, err := s.repo.List(ctx, campaignID, m.ID, fieldKey, actor.IsDirector)
	if err != nil {
		return nil, err
	}
	if entries, err = s.visible(ctx, campaignID, entries, actor); err != nil {
		return nil, err
	}
	out := make([]Choice, 0, len(entries))
	for _, e := range entries {
		out = append(out, entryChoice(e, actor))
	}
	return out, nil
}

// entryChoice shapes a visible campaign entry as a pick-list row.
func entryChoice(e SystemEntry, actor EntryActor) Choice {
	sum := e.Summary
	if sum == "" {
		sum = cleanSummary(e.Description)
	}
	c := Choice{
		Slug: e.Slug, Name: e.Name, Summary: sum, Source: ChoiceSourceCampaign,
		Description: e.Description, Properties: e.Properties, ForName: e.ForName, Origin: OriginCampaign,
		Key: strconv.FormatInt(e.ID, 10), Visibility: e.Visibility,
	}
	// The writer of a draft is told it is theirs and still a draft, and
	// gets its id to keep it to the hero they make with it.
	if e.Status != EntryStatusAccepted {
		c.Status = e.Status
	}
	if actor.UserID != "" && e.CreatedBy == actor.UserID && !actor.IsDirector {
		c.Mine, c.EntryID = true, e.ID
	}
	return c
}

// normalizeEntry validates and cleans the editable fields in place. The
// description is sanitized here, at the one write path, so no caller can
// store script or event-handler markup.
func normalizeEntry(e *SystemEntry) error { return normalizeEntryCarrying(e, nil) }

// normalizeEntryCarrying is normalizeEntry for a write onto stored
// properties: a value the validator does not understand passes only when it
// is the one already stored (see validateEntryProperties).
func normalizeEntryCarrying(e *SystemEntry, stored map[string]any) error {
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
		if e.ForEntityID != "" {
			return apperror.NewValidation("Only an entry kept to one character names a character.")
		}
	case EntryVisibilityCharacter:
		if e.ForEntityID == "" {
			return apperror.NewValidation("Say which character the entry is for.")
		}
	default:
		return apperror.NewValidation("Visibility must be \"everyone\", \"directors\" or \"character\".")
	}
	return validateEntryProperties(e.Properties, stored)
}

// wholeNumber reads a cost from JSON (float64) or YAML (int) input.
func wholeNumber(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
	}
	return 0, false
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
