package addons

// Extension settings / onboarding framework.
//
// Each addon may register a SetupProvider keyed by its slug. When an owner opens
// an extension's settings page, the generic handler asks that provider to (a)
// run health/QOL checks against the campaign, (b) describe the onboarding
// questions to ask, and (c) apply the owner's answers. The provider lives
// OUTSIDE this package (in the app layer) so it can import entities/systems
// without creating an import cycle — exactly like PresetApplier. The contract
// types below are all addons-local so the interface never leaks an entities type.
//
// Per-campaign setup state (completed / dismissed / prior answers) is persisted
// in the existing campaign_addons.config_json under the "setup" key — no new
// table or migration. Reads merge-in via UpdateCampaignConfig so other config
// keys are preserved.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// setupConfigKey is the key under campaign_addons.config_json that holds the
// per-campaign SetupState for an addon.
const setupConfigKey = "setup"

// SetupSeverity classifies a setup check result for rendering (icon + colour)
// and for deciding whether the Features card should nudge the owner.
type SetupSeverity string

const (
	SeverityInfo       SetupSeverity = "info"       // neutral fact, no action needed
	SeveritySuggestion SetupSeverity = "suggestion" // optional QOL improvement
	SeverityWarning    SetupSeverity = "warning"    // something the owner probably wants to fix
	SeverityError      SetupSeverity = "error"      // a real problem the owner should resolve
)

// SetupCheck is one diagnostic line shown on the extension settings page.
type SetupCheck struct {
	ID       string        // stable id, e.g. "pc.duplicate"
	Severity SetupSeverity // drives icon/colour and the needs-setup nudge
	Title    string        // short headline
	Detail   string        // one-sentence explanation
	// ActionLabel optionally names the action the questions below let the owner
	// take to resolve this check (rendered as a hint). May be empty.
	ActionLabel string
}

// QuestionKind enumerates the input control the template renders for a question.
type QuestionKind string

const (
	QuestionChoice QuestionKind = "choice" // radio group over Options
	QuestionText   QuestionKind = "text"   // free-text input
	QuestionBool   QuestionKind = "bool"   // checkbox
)

// SetupOption is one selectable answer for a QuestionChoice.
type SetupOption struct {
	Value string
	Label string
	Hint  string // optional helper text shown next to the option
}

// SetupQuestion is one onboarding question rendered as a form control.
type SetupQuestion struct {
	ID      string       // form field name, e.g. "naming"
	Kind    QuestionKind // control type
	Title   string       // label
	Help    string       // optional helper text under the control
	Options []SetupOption // for QuestionChoice
	Default string        // default Value (choice/text) or "true"/"false" (bool)
	// ShowIfCheckID, when set, marks this question as only relevant if a check
	// with that ID was emitted. The template still renders it (progressive
	// disclosure is best-effort), but providers use it to keep Questions in sync
	// with RunChecks. May be empty (always relevant).
	ShowIfCheckID string
}

// SetupResult is what Apply returns for the success/refresh render.
type SetupResult struct {
	Messages  []string // human-readable summary lines ("Merged 3 characters into Heroes")
	Completed bool     // whether setup is now considered complete (clears the nudge)
}

// SetupState is the persisted per-campaign setup state for an addon, stored
// under campaign_addons.config_json["setup"].
type SetupState struct {
	Completed bool              `json:"completed"`
	Dismissed bool              `json:"dismissed"`
	Answers   map[string]string `json:"answers,omitempty"`
}

// SetupProvider supplies the checks, questions, and apply logic for one addon's
// settings page. Implemented in the app layer (see internal/app/setup_pc.go) so
// addons never imports entities/systems. One provider per addon slug.
type SetupProvider interface {
	// Slug is the addon slug this provider configures.
	Slug() string
	// RunChecks inspects the campaign and returns health/QOL findings.
	RunChecks(ctx context.Context, campaignID string) ([]SetupCheck, error)
	// Questions returns the onboarding questions to ask the owner.
	Questions(ctx context.Context, campaignID string) ([]SetupQuestion, error)
	// Apply applies the owner's answers. Must be idempotent — the owner may
	// re-open the page and Apply again. answers maps question ID -> value.
	Apply(ctx context.Context, campaignID string, answers map[string]string) (SetupResult, error)
}

// --- Registry + state methods on addonService (declared on AddonService) ---

// RegisterSetupProvider registers a provider for its slug. Called once at
// startup from the app wiring layer (mirrors SetPresetApplier).
func (s *addonService) RegisterSetupProvider(p SetupProvider) {
	if s.setupProviders == nil {
		s.setupProviders = make(map[string]SetupProvider)
	}
	s.setupProviders[p.Slug()] = p
}

// SetupProviderFor returns the registered provider for an addon slug, if any.
func (s *addonService) SetupProviderFor(slug string) (SetupProvider, bool) {
	p, ok := s.setupProviders[slug]
	return p, ok
}

// NeedsSetup reports whether the Features card should nudge the owner to open
// this addon's settings page: a provider exists, the addon is enabled, setup is
// neither completed nor dismissed, and at least one check is actionable
// (warning/error/suggestion). Pure read — safe to call per-card.
func (s *addonService) NeedsSetup(ctx context.Context, campaignID, addonSlug string) (bool, error) {
	provider, ok := s.setupProviders[addonSlug]
	if !ok {
		return false, nil
	}
	ca, err := s.campaignAddonBySlug(ctx, campaignID, addonSlug)
	if err != nil {
		return false, err
	}
	if ca == nil || !ca.Enabled {
		return false, nil
	}
	state := parseSetupState(ca.ConfigJSON)
	if state.Completed || state.Dismissed {
		return false, nil
	}
	checks, err := provider.RunChecks(ctx, campaignID)
	if err != nil {
		return false, err
	}
	for _, ck := range checks {
		switch ck.Severity {
		case SeverityWarning, SeverityError, SeveritySuggestion:
			return true, nil
		}
	}
	return false, nil
}

// GetSetupState returns the persisted setup state for an addon in a campaign.
// Returns a zero SetupState when the addon has never been configured.
func (s *addonService) GetSetupState(ctx context.Context, campaignID, addonSlug string) (SetupState, error) {
	ca, err := s.campaignAddonBySlug(ctx, campaignID, addonSlug)
	if err != nil {
		return SetupState{}, err
	}
	if ca == nil {
		return SetupState{}, nil
	}
	return parseSetupState(ca.ConfigJSON), nil
}

// SaveSetupState persists the setup state for an addon, merging it into the
// existing config_json so other config keys are preserved. No-ops silently if
// the addon has no campaign_addons row yet (i.e. not enabled) — setup only runs
// on enabled addons, which always have a row.
func (s *addonService) SaveSetupState(ctx context.Context, campaignID, addonSlug string, state SetupState) error {
	ca, err := s.campaignAddonBySlug(ctx, campaignID, addonSlug)
	if err != nil {
		return err
	}
	if ca == nil {
		return apperror.NewNotFound(fmt.Sprintf("addon %q is not enabled for this campaign", addonSlug))
	}
	config := ca.ConfigJSON
	if config == nil {
		config = map[string]any{}
	}
	config[setupConfigKey] = state
	return s.UpdateCampaignConfig(ctx, campaignID, ca.AddonID, config)
}

// campaignAddonBySlug returns the per-campaign addon row for a slug, or nil if
// the addon is not registered for the campaign. Reuses ListForCampaign (which
// already returns ConfigJSON, Enabled, and AddonID) — no new repo SQL.
func (s *addonService) campaignAddonBySlug(ctx context.Context, campaignID, slug string) (*CampaignAddon, error) {
	list, err := s.repo.ListForCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing campaign addons: %w", err)
	}
	for i := range list {
		if list[i].AddonSlug == slug {
			return &list[i], nil
		}
	}
	return nil, nil
}

// parseSetupState extracts the SetupState stored under config_json["setup"].
// Tolerant: any missing/!malformed value yields a zero SetupState.
func parseSetupState(config map[string]any) SetupState {
	if config == nil {
		return SetupState{}
	}
	raw, ok := config[setupConfigKey]
	if !ok {
		return SetupState{}
	}
	// config_json round-trips as map[string]any, so re-marshal the sub-value
	// and decode it into the typed struct.
	b, err := json.Marshal(raw)
	if err != nil {
		return SetupState{}
	}
	var st SetupState
	if err := json.Unmarshal(b, &st); err != nil {
		return SetupState{}
	}
	return st
}
