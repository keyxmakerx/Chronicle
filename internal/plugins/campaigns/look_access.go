package campaigns

import (
	"fmt"
	"slices"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// The "who can change" settings: for each part of the look, the owner says who
// may pick their own version of it. A pick only ever changes what that person
// sees, so the setting widens a personal choice, never the campaign's look.
const (
	LookAccessOwner    = "owner"    // Only the owner (who edits the campaign's look itself).
	LookAccessCoDM     = "codm"     // The owner and the DM team.
	LookAccessEveryone = "everyone" // Every member.
)

// The parts of the look a member can pick for themselves.
const (
	LookPartSheet    = "sheet"    // The character sheet style.
	LookPartPage     = "page"     // The pages' style kit.
	LookPartHover    = "hover"    // The hover card look.
	LookPartDmScreen = "dmscreen" // The DM Screen's style kit.
)

// LookPart describes one part of the look a member may own a pick of.
type LookPart struct {
	Key     string
	Label   string
	Choices []string // The values a pick may take; also the campaign look's own list.
	// Settings are the access settings the owner may give this part.
	Settings []string
	Default  string // Access setting used while the owner has not set one.
}

// LookParts lists the parts in the order the pages show them. The DM Screen
// never allows everyone: it shows hidden material, so it stays with the DM
// team and so does its look. Pages and hover cards start with the owner so a
// campaign that never opens them looks the same to everyone.
var LookParts = []LookPart{
	{Key: LookPartSheet, Label: "Character sheet", Choices: AppearanceSheetStyles,
		Settings: []string{LookAccessOwner, LookAccessCoDM, LookAccessEveryone}, Default: LookAccessEveryone},
	{Key: LookPartPage, Label: "Pages", Choices: AppearanceStyleKits,
		Settings: []string{LookAccessOwner, LookAccessCoDM, LookAccessEveryone}, Default: LookAccessOwner},
	{Key: LookPartHover, Label: "Hover cards", Choices: AppearanceHoverCards,
		Settings: []string{LookAccessOwner, LookAccessCoDM, LookAccessEveryone}, Default: LookAccessOwner},
	{Key: LookPartDmScreen, Label: "DM Screen", Choices: AppearanceStyleKits,
		Settings: []string{LookAccessOwner, LookAccessCoDM}, Default: LookAccessCoDM},
}

// LookPartByKey returns the part with the given key.
func LookPartByKey(key string) (LookPart, bool) {
	for _, p := range LookParts {
		if p.Key == key {
			return p, true
		}
	}
	return LookPart{}, false
}

// stored is the access setting saved for p on the campaign's appearance.
func (p LookPart) stored(ap *Appearance) string {
	if ap == nil {
		return ""
	}
	switch p.Key {
	case LookPartSheet:
		return ap.AccessSheet
	case LookPartPage:
		return ap.AccessPage
	case LookPartHover:
		return ap.AccessHover
	case LookPartDmScreen:
		return ap.AccessDmScreen
	}
	return ""
}

// Access is the setting in force for p. An empty or unknown stored value, or
// one the part does not allow (a hand-edited "everyone" on the DM Screen),
// reads as the part's default, so a bad value can never widen access.
func (p LookPart) Access(ap *Appearance) string {
	if v := p.stored(ap); slices.Contains(p.Settings, v) {
		return v
	}
	return p.Default
}

// isCoDM is the DM team: scribes and the owner, plus any member the owner
// gave DM access. It matches who may open the DM Screen.
func isCoDM(cc *CampaignContext) bool {
	return cc.MemberRole >= RoleScribe || cc.IsDmGranted
}

// LookAccessAllows reports whether a viewer falls inside an access setting.
// The owner is inside every setting. A non-member, or an unknown setting,
// is never inside.
func LookAccessAllows(setting string, cc *CampaignContext) bool {
	if cc == nil || !cc.IsMember {
		return false
	}
	if cc.MemberRole >= RoleOwner {
		return true
	}
	switch setting {
	case LookAccessEveryone:
		return cc.MemberRole >= RolePlayer
	case LookAccessCoDM:
		return isCoDM(cc)
	}
	return false
}

// MayPick reports whether the viewer can keep a personal pick of p. The owner
// never does: their choice is the campaign's look itself.
func (p LookPart) MayPick(ap *Appearance, cc *CampaignContext) bool {
	if cc == nil || cc.MemberRole >= RoleOwner {
		return false
	}
	return LookAccessAllows(p.Access(ap), cc)
}

// PickableParts lists the parts the viewer may pick for themselves right now.
func PickableParts(ap *Appearance, cc *CampaignContext) []LookPart {
	var out []LookPart
	for _, p := range LookParts {
		if p.MayPick(ap, cc) {
			out = append(out, p)
		}
	}
	return out
}

// AppliedLookPicks filters a viewer's stored picks down to the ones that
// still count: the part must still be pickable by them and the value still
// valid. A pick that no longer counts is skipped, never deleted, so widening
// the setting again brings it back.
func AppliedLookPicks(ap *Appearance, cc *CampaignContext, stored map[string]string) map[string]string {
	if len(stored) == 0 {
		return nil
	}
	var out map[string]string
	for _, p := range PickableParts(ap, cc) {
		v, ok := stored[p.Key]
		if !ok || !slices.Contains(p.Choices, v) {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[p.Key] = v
	}
	return out
}

// LookAccessInput is the "who can change" part of a Save from the Customize
// page. Empty means the part's default.
type LookAccessInput struct {
	Sheet    string `json:"sheet"`
	Page     string `json:"page"`
	Hover    string `json:"hover"`
	DmScreen string `json:"dmscreen"`
}

// accessChoice validates one part's setting for a save and returns what to
// store: the default is stored as empty, like the other defaults.
func accessChoice(p LookPart, v string) (string, error) {
	if v == "" || v == p.Default {
		return "", nil
	}
	if !slices.Contains(p.Settings, v) {
		return "", apperror.NewBadRequest(fmt.Sprintf("%s cannot be changed by %q", p.Label, v))
	}
	return v, nil
}

// applyLookAccess validates in and writes it onto a. A nil in keeps what the
// campaign had, so an editor or a preset that never sends the setting cannot
// quietly reset it.
func applyLookAccess(a *Appearance, in *LookAccessInput, prev *Appearance) error {
	if in == nil {
		if prev != nil {
			a.AccessSheet, a.AccessPage = prev.AccessSheet, prev.AccessPage
			a.AccessHover, a.AccessDmScreen = prev.AccessHover, prev.AccessDmScreen
		}
		return nil
	}
	var err error
	sheet, _ := LookPartByKey(LookPartSheet)
	if a.AccessSheet, err = accessChoice(sheet, in.Sheet); err != nil {
		return err
	}
	page, _ := LookPartByKey(LookPartPage)
	if a.AccessPage, err = accessChoice(page, in.Page); err != nil {
		return err
	}
	hover, _ := LookPartByKey(LookPartHover)
	if a.AccessHover, err = accessChoice(hover, in.Hover); err != nil {
		return err
	}
	dm, _ := LookPartByKey(LookPartDmScreen)
	if a.AccessDmScreen, err = accessChoice(dm, in.DmScreen); err != nil {
		return err
	}
	return nil
}

// ValidateLookPick checks a member's pick before it is stored: the part must
// exist, the person must be allowed to pick it now, and the value must be one
// of the part's choices. An empty value is a reset and always passes.
func ValidateLookPick(ap *Appearance, cc *CampaignContext, partKey, value string) (LookPart, error) {
	p, ok := LookPartByKey(partKey)
	if !ok {
		return LookPart{}, apperror.NewNotFound("unknown part of the look")
	}
	if cc == nil || !cc.IsMember {
		return LookPart{}, apperror.NewForbidden("only members of this campaign can pick a look")
	}
	if cc.MemberRole >= RoleOwner {
		return LookPart{}, apperror.NewBadRequest("the owner changes the campaign's look in Customize")
	}
	if !p.MayPick(ap, cc) {
		return LookPart{}, apperror.NewForbidden(fmt.Sprintf("the owner has not let you change the %s", p.Label))
	}
	if value != "" && !slices.Contains(p.Choices, value) {
		return LookPart{}, apperror.NewBadRequest(fmt.Sprintf("invalid %s choice", p.Label))
	}
	return p, nil
}

// lookChoiceLabels name each choice the way the Customize page does, so a
// member sees the same words the owner picked from.
var lookChoiceLabels = map[string]string{
	"modern": "Modern", "parchment": "Parchment", "ledger": "Ledger", "journal": "Field journal",
	"vellum": "Vellum", "night": "Night", "deck": "Card deck", "pencil": "Pencil",
	"starship": "Starship", "neon": "Neon terminal", "runes": "Rune slate", "brass": "Brass gauges",
	"material": "Material", "sharp": "Sharp", "ornate": "Ornate", "flowing": "Flowing",
	"lumen": "Lumen", "sketchbook": "Sketchbook",
	"paper": "Paper", "plain": "Plain", "compact": "Compact",
}

// LookChoiceLabel is the display name of one choice.
func LookChoiceLabel(id string) string {
	if l, ok := lookChoiceLabels[id]; ok {
		return l
	}
	return id
}

// campaignChoice is the choice the campaign's own look has for p, which is
// what a member sees until they pick (or after they reset).
func (p LookPart) campaignChoice(ap *Appearance) string {
	v := ""
	if ap != nil {
		switch p.Key {
		case LookPartSheet:
			v = ap.SheetStyle
		case LookPartPage:
			v = ap.StyleKit
		case LookPartHover:
			v = ap.HoverCard
		case LookPartDmScreen:
			v = ap.StyleKit
		}
	}
	if slices.Contains(p.Choices, v) {
		return v
	}
	return p.Choices[0]
}

// yourLookPart is one part as the "Your look" page draws it.
type yourLookPart struct {
	Part     LookPart
	URL      string // PUT address for this part.
	Current  string // The member's applied pick, or "" when they follow the campaign.
	Campaign string // The campaign's own choice for the part.
}
