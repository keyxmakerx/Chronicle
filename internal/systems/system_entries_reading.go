package systems

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// A campaign reads its game system in its own words through three acts, all
// Directors' and all whole-entry: a tie lets a campaign entry take a standard
// entry's place (made only by a press, never by a matching name), "left out"
// stops offering a standard entry, and old names keep a renamed entry
// reachable from what heroes and links stored. Who then reads what is the
// catalog's job (entry_catalog.go); this file holds the writes and their rules.

const (
	// maxLeftOutSentence and maxNoteLine match their columns.
	maxLeftOutSentence = 300
	maxNoteLine        = 300
	// maxNotesShown bounds an entry's "told the players" history on read.
	maxNotesShown = 50
)

// StandardSystemResolver returns the game system whose entries a campaign's
// entries stand beside (the one EntryManifestResolver names), or nil.
type StandardSystemResolver func(ctx context.Context, campaignID string) System

// EntryReading is the service's half that ties, leaves out, renames and
// notes. Writes are Directors' only and refused in the service.
type EntryReading interface {
	// Rewritable reports whether the standard entry copies into a campaign
	// entry without loss: it passes every check a campaign entry must pass.
	// The page asks before offering "Rewrite"; false, not an error, when not.
	Rewritable(ctx context.Context, campaignID, fieldKey, slug string) (bool, error)
	// Rewrite copies the standard entry whole into a new Directors-only
	// campaign entry tied to it, with the snapshot stored.
	Rewrite(ctx context.Context, campaignID string, actor EntryActor, in RewriteInput) (*SystemEntry, error)
	// Tie lets an existing campaign entry take a standard entry's place.
	Tie(ctx context.Context, campaignID string, actor EntryActor, id int64, standardSlug string) (*SystemEntry, error)
	// Untie makes a tied entry the campaign's own again; its text stays.
	Untie(ctx context.Context, campaignID string, actor EntryActor, id int64) (*SystemEntry, error)
	// MarkSeen refreshes the snapshot to the standard entry as it is today.
	MarkSeen(ctx context.Context, campaignID string, actor EntryActor, id int64) (*SystemEntry, error)
	// StandardChange says in one short line what the package changed in the
	// standard entry since the snapshot; "" when nothing did. Directors only.
	StandardChange(ctx context.Context, campaignID string, actor EntryActor, id int64) (string, error)

	// LeaveOut stops offering a standard entry, or changes its sentence.
	LeaveOut(ctx context.Context, campaignID string, actor EntryActor, in LeftOutInput) (*LeftOut, error)
	// BringBack offers a left-out standard entry again.
	BringBack(ctx context.Context, campaignID string, actor EntryActor, fieldKey, slug string) error
	// LeftOutList returns the left-out standard entries of a field ("" = all).
	// Every reader may know them: the names still answer everywhere.
	LeftOutList(ctx context.Context, campaignID, fieldKey string) ([]LeftOut, error)

	// OldNames returns the old names of a field ("" = all) whose entries the
	// actor may see.
	OldNames(ctx context.Context, campaignID, fieldKey string, actor EntryActor) ([]EntryOldName, error)
	// FreeOldName lets another entry take the name; it no longer leads here.
	FreeOldName(ctx context.Context, campaignID string, actor EntryActor, id int64) error

	// AddNote records one line the players were told about an entry.
	AddNote(ctx context.Context, campaignID string, actor EntryActor, entryID int64, line string) (*EntryNote, error)
	// Notes returns an entry's lines newest first, for a reader who may see it.
	Notes(ctx context.Context, campaignID string, actor EntryActor, entryID int64) ([]EntryNote, error)
}

// RewriteInput names the standard entry to copy. Name is optional and gives
// the copy another name, for when the campaign already has an entry called
// the standard entry's name and wants a fresh copy beside it.
type RewriteInput struct {
	FieldKey string `json:"fieldKey"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
}

// LeftOutInput leaves a standard entry out, with an optional sentence.
type LeftOutInput struct {
	FieldKey string `json:"fieldKey"`
	Slug     string `json:"slug"`
	Sentence string `json:"sentence"`
}

// kindCategory is the rules category a character field's pick list draws
// from: the data file the manifest names, else the plural of the key.
func kindCategory(m *SystemManifest, fieldKey string) string {
	base := declaredChoices(m, fieldKey)
	if base == "" || !choiceBasenamePattern.MatchString(base) {
		base = pluralBasename(fieldKey)
	}
	return base
}

// kindField is the character field whose pick list is the category, or ""
// for a category that is not a kind (monsters, abilities...).
func kindField(m *SystemManifest, category string) string {
	if m == nil {
		return ""
	}
	for _, p := range m.EntityPresets {
		if p.Category != "character" {
			continue
		}
		for _, f := range p.Fields {
			if f.Type == "string" && ValidChoiceFieldKey(f.Key) && kindCategory(m, f.Key) == category {
				return f.Key
			}
		}
	}
	return ""
}

// standardSystem is the system whose entries stand beside the campaign's,
// only when it is the same system the entries were made under.
func (s *systemEntryService) standardSystem(ctx context.Context, campaignID string, m *SystemManifest) System {
	if s.standard == nil || m == nil {
		return nil
	}
	sys := s.standard(ctx, campaignID)
	if sys == nil || sys.Info() == nil || sys.Info().ID != m.ID || sys.DataProvider() == nil {
		return nil
	}
	return sys
}

// standardItem finds one standard entry of a kind; nil when there is none.
func (s *systemEntryService) standardItem(ctx context.Context, campaignID string, m *SystemManifest, fieldKey, slug string) (*ReferenceItem, error) {
	sys := s.standardSystem(ctx, campaignID, m)
	if sys == nil || slug == "" {
		return nil, nil
	}
	return sys.DataProvider().Get(kindCategory(m, fieldKey), slug)
}

// kindSystem resolves the campaign's system and checks the field is a kind.
func (s *systemEntryService) kindSystem(ctx context.Context, campaignID, fieldKey string) (*SystemManifest, error) {
	m, err := s.system(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if !ValidChoiceFieldKey(fieldKey) || !isCharacterStringField(m, fieldKey) {
		return nil, apperror.NewValidation("That is not a text field of this game system's character sheet, so it has no entries.")
	}
	return m, nil
}

// snapshotOf records a standard entry's words and figures as JSON.
func snapshotOf(it *ReferenceItem) string {
	raw, err := json.Marshal(StandardSnapshot{Name: it.Name, Summary: it.Summary, Description: it.Description, Properties: it.Properties})
	if err != nil {
		return ""
	}
	return string(raw)
}

// copyProps deep-copies properties so validation and budgets never write
// into the system's shared, in-memory data.
func copyProps(p map[string]any) map[string]any {
	out := map[string]any{}
	if len(p) == 0 {
		return out
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

// unrewritableKinds are the fields whose standard entries are never copied.
// A class's entry is only its outline; its abilities live elsewhere in the
// package, so a copy would promise a rewrite it cannot deliver.
// TODO(#2391): rewrite a class once its abilities can travel with it.
var unrewritableKinds = map[string]bool{"class": true}

// rewriteCopy is the campaign entry a standard entry becomes, before checks.
// Properties are copied verbatim except the generated "_display" twins, and
// the summary is cut the way the picker already shows package summaries.
func rewriteCopy(campaignID string, m *SystemManifest, fieldKey string, it *ReferenceItem, name, by string) *SystemEntry {
	if name == "" {
		name = it.Name
	}
	props := copyProps(it.Properties)
	dropDerivedTwins(props)
	return &SystemEntry{
		CampaignID: campaignID, SystemID: m.ID, FieldKey: fieldKey,
		Name: strings.TrimSpace(name), Summary: cleanSummary(it.Summary), Description: it.Description,
		Properties: props, Visibility: EntryVisibilityDirectors,
		Status: EntryStatusAccepted, StandsFor: it.ID, StandsForSeen: snapshotOf(it), CreatedBy: by,
	}
}

// checkRewrite is every refusal a rewrite of the standard entry would meet
// apart from its name, so Rewritable and Rewrite never disagree.
func (s *systemEntryService) checkRewrite(ctx context.Context, campaignID string, m *SystemManifest, fieldKey string, it *ReferenceItem) error {
	if unrewritableKinds[fieldKey] {
		return apperror.NewValidation("A class cannot be rewritten here yet: its entry is only the outline of the class, so a copy would leave its abilities behind.")
	}
	if err := s.refuseTie(ctx, campaignID, m.ID, fieldKey, it.ID); err != nil {
		return err
	}
	if n, err := s.repo.Count(ctx, campaignID, m.ID, fieldKey); err != nil {
		return err
	} else if n >= maxEntriesPerField {
		return apperror.NewValidation(fmt.Sprintf("A field can hold at most %d campaign entries.", maxEntriesPerField))
	}
	// The package's own values are what a copy may carry, so they are the
	// stored side of the carried rule.
	cp := rewriteCopy(campaignID, m, fieldKey, it, "", "")
	if normalizeEntryCarrying(cp, copyProps(it.Properties)) != nil {
		return apperror.NewValidation("The printed " + it.Name + " cannot be copied into a campaign entry without losing part of it, so it cannot be rewritten here.")
	}
	return nil
}

func (s *systemEntryService) Rewritable(ctx context.Context, campaignID, fieldKey, slug string) (bool, error) {
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil || !ValidChoiceFieldKey(fieldKey) || !isCharacterStringField(m, fieldKey) {
		return false, nil
	}
	it, err := s.standardItem(ctx, campaignID, m, fieldKey, slug)
	if err != nil || it == nil {
		return false, err
	}
	if err := s.checkRewrite(ctx, campaignID, m, fieldKey, it); err != nil {
		// A refusal is an answer; a failure to look is an error.
		if ae, ok := err.(*apperror.AppError); ok && ae.Code < 500 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *systemEntryService) Rewrite(ctx context.Context, campaignID string, actor EntryActor, in RewriteInput) (*SystemEntry, error) {
	if err := requireDirector(actor); err != nil {
		return nil, err
	}
	m, err := s.kindSystem(ctx, campaignID, in.FieldKey)
	if err != nil {
		return nil, err
	}
	it, err := s.standardItem(ctx, campaignID, m, in.FieldKey, strings.TrimSpace(in.Slug))
	if err != nil {
		return nil, err
	}
	if it == nil {
		return nil, apperror.NewNotFound("that standard entry is not in this game system")
	}
	if err := s.checkRewrite(ctx, campaignID, m, in.FieldKey, it); err != nil {
		return nil, err
	}
	e := rewriteCopy(campaignID, m, in.FieldKey, it, in.Name, actor.UserID)
	if err := normalizeEntryCarrying(e, copyProps(it.Properties)); err != nil {
		return nil, err
	}
	if dup, err := s.repo.FindByName(ctx, campaignID, m.ID, e.FieldKey, e.Name); err != nil {
		return nil, err
	} else if dup != nil {
		return nil, apperror.NewConflict(fmt.Sprintf("This campaign already has an entry called %s: tie that one, or give the copy another name.", e.Name))
	}
	if err := s.refuseOldName(ctx, campaignID, m.ID, e.FieldKey, e.Name, 0); err != nil {
		return nil, err
	}
	if e.Slug, err = s.uniqueSlug(ctx, campaignID, m.ID, e.FieldKey, e.Name); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, e); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, campaignID, e.ID)
}

// refuseTie keeps one stand-in per standard entry and keeps a left-out entry
// from also being replaced, so a reader never meets both answers at once.
func (s *systemEntryService) refuseTie(ctx context.Context, campaignID, systemID, fieldKey, slug string) error {
	if other, err := s.repo.FindStandIn(ctx, campaignID, systemID, fieldKey, slug); err != nil {
		return err
	} else if other != nil {
		return apperror.NewConflict(other.Name + " already takes this entry's place. Untie it first.")
	}
	outs, err := s.repo.ListLeftOut(ctx, campaignID, systemID, fieldKey)
	if err != nil {
		return err
	}
	for _, l := range outs {
		if l.Slug == slug {
			return apperror.NewConflict("This entry is left out of the campaign. Bring it back first.")
		}
	}
	return nil
}

// tiedEntry loads a campaign entry for a Director's tie write.
func (s *systemEntryService) tiedEntry(ctx context.Context, campaignID string, actor EntryActor, id int64) (*SystemEntry, *SystemManifest, error) {
	if err := requireDirector(actor); err != nil {
		return nil, nil, err
	}
	m, err := s.system(ctx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	e, err := s.repo.Get(ctx, campaignID, id)
	if err != nil {
		return nil, nil, err
	}
	if e.SystemID != m.ID {
		return nil, nil, apperror.NewNotFound("entry not found")
	}
	return e, m, nil
}

func (s *systemEntryService) Tie(ctx context.Context, campaignID string, actor EntryActor, id int64, standardSlug string) (*SystemEntry, error) {
	e, m, err := s.tiedEntry(ctx, campaignID, actor, id)
	if err != nil {
		return nil, err
	}
	if entryStatusOrAccepted(e.Status) != EntryStatusAccepted {
		return nil, apperror.NewValidation("Accept this entry before it takes a standard entry's place.")
	}
	it, err := s.standardItem(ctx, campaignID, m, e.FieldKey, strings.TrimSpace(standardSlug))
	if err != nil {
		return nil, err
	}
	if it == nil {
		return nil, apperror.NewValidation("That is not a standard entry of the same kind in this game system.")
	}
	if e.StandsFor == it.ID {
		return e, nil
	}
	if e.StandsFor != "" {
		return nil, apperror.NewConflict(e.Name + " already takes another entry's place. Untie it first.")
	}
	if err := s.refuseTie(ctx, campaignID, m.ID, e.FieldKey, it.ID); err != nil {
		return nil, err
	}
	if err := s.repo.SetStandsFor(ctx, campaignID, e.ID, it.ID, snapshotOf(it)); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, campaignID, e.ID)
}

func (s *systemEntryService) Untie(ctx context.Context, campaignID string, actor EntryActor, id int64) (*SystemEntry, error) {
	e, _, err := s.tiedEntry(ctx, campaignID, actor, id)
	if err != nil {
		return nil, err
	}
	if e.StandsFor == "" {
		return e, nil
	}
	if err := s.repo.SetStandsFor(ctx, campaignID, e.ID, "", ""); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, campaignID, e.ID)
}

func (s *systemEntryService) MarkSeen(ctx context.Context, campaignID string, actor EntryActor, id int64) (*SystemEntry, error) {
	e, m, err := s.tiedEntry(ctx, campaignID, actor, id)
	if err != nil {
		return nil, err
	}
	if e.StandsFor == "" {
		return nil, apperror.NewValidation(e.Name + " does not take a standard entry's place.")
	}
	it, err := s.standardItem(ctx, campaignID, m, e.FieldKey, e.StandsFor)
	if err != nil {
		return nil, err
	}
	if it == nil {
		return nil, apperror.NewNotFound("the standard entry is no longer in this game system")
	}
	if err := s.repo.SetStandsForSeen(ctx, campaignID, e.ID, snapshotOf(it)); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, campaignID, e.ID)
}

func (s *systemEntryService) StandardChange(ctx context.Context, campaignID string, actor EntryActor, id int64) (string, error) {
	e, m, err := s.tiedEntry(ctx, campaignID, actor, id)
	if err != nil || e.StandsFor == "" {
		return "", err
	}
	it, err := s.standardItem(ctx, campaignID, m, e.FieldKey, e.StandsFor)
	if err != nil {
		return "", err
	}
	var was StandardSnapshot
	if e.StandsForSeen != "" {
		_ = json.Unmarshal([]byte(e.StandsForSeen), &was)
	}
	if it == nil {
		return "removed from the package", nil
	}
	now := StandardSnapshot{Name: it.Name, Summary: it.Summary, Description: it.Description, Properties: it.Properties}
	return DescribeStandardChange(was, now), nil
}

func (s *systemEntryService) LeaveOut(ctx context.Context, campaignID string, actor EntryActor, in LeftOutInput) (*LeftOut, error) {
	if err := requireDirector(actor); err != nil {
		return nil, err
	}
	m, err := s.kindSystem(ctx, campaignID, in.FieldKey)
	if err != nil {
		return nil, err
	}
	sentence := strings.TrimSpace(in.Sentence)
	if utf8.RuneCountInString(sentence) > maxLeftOutSentence {
		return nil, apperror.NewValidation(fmt.Sprintf("The sentence can be at most %d characters.", maxLeftOutSentence))
	}
	it, err := s.standardItem(ctx, campaignID, m, in.FieldKey, strings.TrimSpace(in.Slug))
	if err != nil {
		return nil, err
	}
	if it == nil {
		return nil, apperror.NewNotFound("that standard entry is not in this game system")
	}
	if other, err := s.repo.FindStandIn(ctx, campaignID, m.ID, in.FieldKey, it.ID); err != nil {
		return nil, err
	} else if other != nil {
		return nil, apperror.NewConflict(other.Name + " takes this entry's place. Untie it before leaving the entry out.")
	}
	l := &LeftOut{CampaignID: campaignID, SystemID: m.ID, FieldKey: in.FieldKey, Slug: it.ID, Sentence: sentence, CreatedBy: actor.UserID}
	if err := s.repo.PutLeftOut(ctx, l); err != nil {
		return nil, err
	}
	outs, err := s.repo.ListLeftOut(ctx, campaignID, m.ID, in.FieldKey)
	if err != nil {
		return nil, err
	}
	for i := range outs {
		if outs[i].Slug == it.ID {
			return &outs[i], nil
		}
	}
	return l, nil
}

func (s *systemEntryService) BringBack(ctx context.Context, campaignID string, actor EntryActor, fieldKey, slug string) error {
	if err := requireDirector(actor); err != nil {
		return err
	}
	m, err := s.kindSystem(ctx, campaignID, fieldKey)
	if err != nil {
		return err
	}
	ok, err := s.repo.DeleteLeftOut(ctx, campaignID, m.ID, fieldKey, strings.TrimSpace(slug))
	if err != nil {
		return err
	}
	if !ok {
		return apperror.NewNotFound("that entry is not left out")
	}
	return nil
}

func (s *systemEntryService) LeftOutList(ctx context.Context, campaignID, fieldKey string) ([]LeftOut, error) {
	m := s.manifestOrNil(ctx, campaignID)
	if m == nil || (fieldKey != "" && !ValidChoiceFieldKey(fieldKey)) {
		return []LeftOut{}, nil
	}
	return s.repo.ListLeftOut(ctx, campaignID, m.ID, fieldKey)
}

// StandardSystemFor resolves the system a campaign's entries were made under
// to its loaded package: the campaign's custom upload when that is the one,
// else the built-in system of that id.
func StandardSystemFor(custom *CampaignSystemManager, manifest EntryManifestResolver) StandardSystemResolver {
	return func(ctx context.Context, campaignID string) System {
		if manifest == nil {
			return nil
		}
		m := manifest(ctx, campaignID)
		if m == nil {
			return nil
		}
		if custom != nil {
			if sys := custom.GetSystem(campaignID); sys != nil && sys.Info() != nil && sys.Info().ID == m.ID {
				return sys
			}
		}
		return FindSystem(m.ID)
	}
}
