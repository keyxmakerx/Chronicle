package entities

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// identity_edit.go is the one narrow door that lets a Player-role member edit
// a page the fields, metadata, entry and image routes otherwise reserve for
// Scribes: their own claimed character's identity, the picks the hero builder
// offers them, and the hero's description and picture. Everything outside
// this file still needs Scribe, so widening the door means editing this file,
// nowhere else.

// maxOwnerIdentityValue bounds an identity value a player writes: a name such
// as "Dragon Knight", never a document.
const maxOwnerIdentityValue = 200

// maxOwnerStoryValue bounds a story a player writes about their hero.
const maxOwnerStoryValue = 20000

// A list the player keeps on their hero (skills, languages, perks) holds at
// most maxOwnerListItems names of at most maxOwnerListItem characters each.
const (
	maxOwnerListItems = 40
	maxOwnerListItem  = 64
)

// ownerIdentityFieldKeys are the short free-text keys a claimed owner may
// change. They describe who the character is (ancestry, upbringing,
// calling), never rules-bearing numbers or GM-facing data, so a player
// cannot rewrite their own stats. Game systems name the same idea
// differently, hence the synonyms (race/species/heritage, career/kit).
var ownerIdentityFieldKeys = map[string]bool{
	"ancestry":   true,
	"culture":    true,
	"career":     true,
	"kit":        true,
	"race":       true,
	"species":    true,
	"heritage":   true,
	"background": true,
	"pronouns":   true,
	"gender":     true,
}

// ownerStoryFieldKeys are the long texts that are the player's own words
// about their hero.
var ownerStoryFieldKeys = map[string]bool{
	"backstory":  true,
	"appearance": true,
}

// ownerListFieldKeys are the lists of names the player chooses for their
// hero from what their picks grant: a JSON array of short strings, the form
// the system's sheet reads. Names only, never numbers.
var ownerListFieldKeys = map[string]bool{
	"skills_json":    true,
	"languages_json": true,
	"perks_json":     true,
}

// ownerListLabels are how the bell names each list. They sit beside the keys
// because the type's own labels read "Skills Data".
var ownerListLabels = map[string]string{
	"skills_json":    "Skills",
	"languages_json": "Languages",
	"perks_json":     "Perks",
}

// OwnerIdentityFieldKeys returns the short free-text keys, sorted, for UI and
// docs.
func OwnerIdentityFieldKeys() []string {
	keys := make([]string, 0, len(ownerIdentityFieldKeys))
	for k := range ownerIdentityFieldKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// IsClaimedOwnerPlayer reports whether the viewer is a campaign member below
// Scribe who owns this entity. The claim addon's on/off state is irrelevant:
// an existing claim keeps its meaning if the addon is later switched off.
func IsClaimedOwnerPlayer(role campaigns.Role, entity *Entity, userID string) bool {
	return role >= campaigns.RolePlayer && role < campaigns.RoleScribe && entity.IsOwnedBy(userID)
}

// CanEditIdentity reports whether the viewer may edit the identity fields and
// rename the page: any Scribe or above, or the claimed owner.
func CanEditIdentity(role campaigns.Role, entity *Entity, userID string) bool {
	return role >= campaigns.RoleScribe || IsClaimedOwnerPlayer(role, entity, userID)
}

// builderStep is one hero builder step as a player gets it: the entries on
// the list by lower-case name, and what the campaign's rules made of it.
type builderStep struct {
	names map[string]HeroChoice
	// limited: a rule applies to this player here, so even an identity key
	// (which takes free text elsewhere) must hold a name from the list.
	limited bool
	// required: the list is a short list; the step can't be emptied.
	required bool
	// parent is the step this one's entries live inside; a value must then
	// belong to the parent entry the hero holds (or the patch sets).
	parent string
	// parentsOf lists, by lower-case entry name, every parent that lists an
	// entry of that name: two classes can share a subclass name.
	parentsOf map[string]map[string]bool
}

// builderPicks is what the hero builder lets a player choose: for each step's
// field key, its entries and rules.
type builderPicks map[string]builderStep

// builderPicksFrom turns the campaign's creator plan, as a player sees it,
// into builderPicks. A nil plan gives none, so the door stays at identity.
// What the rules took off a list is carried flagged Held, so a hero that
// already holds it stays editable but it is never a valid new pick.
func builderPicksFrom(plan *HeroPlan) builderPicks {
	out := builderPicks{}
	if plan == nil {
		return out
	}
	for _, st := range plan.Steps {
		names := make(map[string]HeroChoice, len(st.Choices)+len(st.Held))
		var parentsOf map[string]map[string]bool
		for _, ch := range st.Choices {
			k := strings.ToLower(strings.TrimSpace(ch.Name))
			if _, on := names[k]; !on {
				names[k] = ch
			}
			if ch.Parent != "" {
				if parentsOf == nil {
					parentsOf = map[string]map[string]bool{}
				}
				if parentsOf[k] == nil {
					parentsOf[k] = map[string]bool{}
				}
				parentsOf[k][strings.ToLower(strings.TrimSpace(ch.Parent))] = true
			}
		}
		for _, ch := range st.Held {
			k := strings.ToLower(strings.TrimSpace(ch.Name))
			if _, on := names[k]; !on {
				ch.Held = true
				names[k] = ch
			}
		}
		bs := builderStep{names: names, parent: st.Parent, parentsOf: parentsOf}
		if st.Rules != nil {
			bs.limited, bs.required = st.Rules.Limited, st.Rules.Required
		}
		out[st.FieldKey] = bs
	}
	return out
}

// needsBuilderPicks reports whether a patch reaches past the long texts and
// lists judged without the creator plan, so the handler loads the plan only
// when it can matter. An identity key needs it too: a rule on that step turns
// its free text into a pick from the list.
func needsBuilderPicks(patch map[string]any) bool {
	for k := range patch {
		if !ownerStoryFieldKeys[k] && !ownerListFieldKeys[k] {
			return true
		}
	}
	return false
}

// authorizeFieldsWrite decides whether a fields PUT may proceed. Scribes pass
// untouched. A claimed owner may send only fields_patch; fields_data replaces
// the whole map, so it is never allowed to a Player, and one disallowed key
// rejects the whole request rather than being silently dropped (the first,
// by key, is named). What each key may hold is ownerFieldVerdicts.
func authorizeFieldsWrite(role campaigns.Role, entity *Entity, userID, campaignID string, hasFieldsData bool, patch map[string]any, typeFields []FieldDefinition, picks builderPicks) error {
	return authorizeFieldsWriteAs(role, entity, userID, campaignID, hasFieldsData, patch, typeFields, picks, false)
}

// authorizeFieldsWriteAs is authorizeFieldsWrite with a say over ask-first
// entries: the web door lets one through (the handler records it as a
// request instead of storing it), a player's AI is refused.
func authorizeFieldsWriteAs(role campaigns.Role, entity *Entity, userID, campaignID string, hasFieldsData bool, patch map[string]any, typeFields []FieldDefinition, picks builderPicks, refuseAsk bool) error {
	if role >= campaigns.RoleScribe {
		return nil
	}
	if entity.CampaignID != campaignID || !IsClaimedOwnerPlayer(role, entity, userID) {
		return apperror.NewForbidden("insufficient permissions")
	}
	if hasFieldsData || len(patch) == 0 {
		return apperror.NewForbidden("you can only change your character's identity fields")
	}
	held := ownerFieldVerdicts(entity, patch, typeFields, picks, refuseAsk)
	keys := make([]string, 0, len(held))
	for k := range held {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		return held[keys[0]]
	}
	return nil
}

// ownerFieldVerdicts judges each key of a claimed owner's patch on its own
// and returns the refusal for every key that may not be written; an empty
// map means the whole patch may. A key must be a field of the character's
// category (when typeFields is known) and one of: a short identity text
// (ownerIdentityFieldKeys), the player's story (ownerStoryFieldKeys), a list
// of names (ownerListFieldKeys), a hero builder step in picks, which takes
// only a name from that step's list, so a player can choose their class but
// never type a stat into it, or what a step's entry lets the hero buy, which
// must be on its list and fit its points. A key the type marks GM-only is
// refused whatever it is, since the owner may not see it.
func ownerFieldVerdicts(entity *Entity, patch map[string]any, typeFields []FieldDefinition, picks builderPicks, refuseAsk bool) map[string]error {
	gmOnly := map[string]bool{}
	onType := map[string]bool{}
	for i := range typeFields {
		onType[typeFields[i].Key] = true
		if typeFields[i].GMOnly {
			gmOnly[typeFields[i].Key] = true
		}
	}
	known := func(k string) bool { return typeFields == nil || onType[k] }
	held := map[string]error{}
	for k, v := range patch {
		listed, isStep := picks[k]
		step, isOptions := strings.CutSuffix(k, heroOptionsSuffix)
		if isOptions {
			_, isOptions = picks[step]
		}
		switch {
		case gmOnly[k] || (!ownerIdentityFieldKeys[k] && !ownerStoryFieldKeys[k] && !ownerListFieldKeys[k] && !isStep && !isOptions):
			// Naming the key lets a player's AI drop it and resend the rest.
			held[k] = apperror.NewForbidden(fmt.Sprintf("%q is your GM's to change, not yours", k))
		case isOptions && !known(step):
			held[k] = apperror.NewForbidden(fmt.Sprintf("this character has no %q field", step))
		case !isOptions && !known(k):
			held[k] = apperror.NewForbidden(fmt.Sprintf("this character has no %q field", k))
		case isOptions:
			if err := checkStepOptions(entity, patch, step, v, picks[step], refuseAsk); err != nil {
				held[k] = err
			}
		default:
			if err := checkOwnerValue(entity, k, v, listed, isStep, refuseAsk); err != nil {
				held[k] = err
			} else if err := checkParentFit(entity, patch, k, v, listed); err != nil {
				held[k] = err
			}
		}
	}
	return held
}

// checkOwnerValue checks the value a claimed owner writes to key k.
//
// A step whose list the rules limit (or require) takes only a name from that
// list, identity keys included, and a required step can't be emptied. The
// refusal for an off-list name is the same whether or not an entry the player
// can't read has that name, so it can't be used to find hidden entries. A hero
// keeps what it already holds: re-sending its current value is always fine.
func checkOwnerValue(entity *Entity, k string, v any, step builderStep, isStep, refuseAsk bool) error {
	if v == nil {
		if isStep && step.required {
			return apperror.NewBadRequest(fmt.Sprintf("%q can't be left empty; choose from the list", k))
		}
		return nil
	}
	val, ok := v.(string)
	if !ok {
		return apperror.NewBadRequest(fmt.Sprintf("%q takes text", k))
	}
	switch {
	case ownerStoryFieldKeys[k]:
		if utf8.RuneCountInString(val) > maxOwnerStoryValue {
			return apperror.NewBadRequest(fmt.Sprintf("%q is too long; keep it under %d characters", k, maxOwnerStoryValue))
		}
	case ownerListFieldKeys[k]:
		_, err := ownerNameList(k, val)
		return err
	default:
		if utf8.RuneCountInString(val) > maxOwnerIdentityValue {
			return apperror.NewBadRequest(fmt.Sprintf("%q is too long", k))
		}
		if !isStep {
			return nil
		}
		trimmed := strings.TrimSpace(val)
		if trimmed == "" && step.required {
			return apperror.NewBadRequest(fmt.Sprintf("%q can't be left empty; choose from the list", k))
		}
		if ownerIdentityFieldKeys[k] && !step.limited {
			return nil
		}
		if trimmed == "" && ownerIdentityFieldKeys[k] {
			return nil
		}
		unchanged := holdsValue(entity, k, trimmed)
		ch, on := step.names[strings.ToLower(trimmed)]
		if (!on || ch.Held) && (!unchanged || !step.limited) {
			return apperror.NewBadRequest(fmt.Sprintf("%q is not on the %s list", val, k))
		}
		if on && !ch.Held && ch.State == HeroChoiceStateAskFirst && refuseAsk && !unchanged {
			return apperror.NewForbidden(askYourDirector(ch.Name))
		}
	}
	return nil
}

// checkParentFit refuses a value on a dependent step (a subclass) that does
// not belong to the hero's current parent value (a class), which is the
// patch's own when the same patch sets it. A hero keeps what it already
// holds while its parent is not being changed. A step with no parent, a
// value off the list (other checks judge that) and an empty value pass.
// Offering nothing for a count-0 step is the builder's job, not a refusal here.
func checkParentFit(entity *Entity, patch map[string]any, k string, v any, step builderStep) error {
	val, _ := v.(string)
	val = strings.TrimSpace(val)
	if step.parent == "" || val == "" {
		return nil
	}
	key := strings.ToLower(val)
	ch, on := step.names[key]
	if !on || ch.Parent == "" {
		return nil
	}
	parentVal, parentInPatch := "", false
	if pv, has := patch[step.parent]; has {
		parentInPatch = true
		parentVal, _ = pv.(string)
	} else if entity != nil {
		parentVal, _ = entity.FieldsData[step.parent].(string)
	}
	if pv := strings.ToLower(strings.TrimSpace(parentVal)); strings.EqualFold(pv, ch.Parent) || step.parentsOf[key][pv] {
		return nil
	}
	if holdsValue(entity, k, val) && !parentInPatch {
		return nil
	}
	return apperror.NewBadRequest(fmt.Sprintf("%q belongs to %s; choose that %s first", val, ch.Parent, step.parent))
}

// askYourDirector is the refusal for writing an ask-first entry where the
// pick can't be recorded as a request, such as a player's AI.
func askYourDirector(name string) string {
	return fmt.Sprintf("%q needs your Director's yes first; ask your Director for it", name)
}

// holdsValue reports whether the hero already holds this value in the field,
// so re-sending it changes nothing.
func holdsValue(entity *Entity, key, value string) bool {
	if entity == nil || strings.TrimSpace(value) == "" {
		return false
	}
	cur, _ := entity.FieldsData[key].(string)
	return strings.EqualFold(strings.TrimSpace(cur), strings.TrimSpace(value))
}

// askPick is a pick of an ask-first entry taken out of a write.
type askPick struct {
	fieldKey string
	name     string
}

// takeAskFirst removes the picks of ask-first entries from a claimed owner's
// patch, with what they would have bought, and returns them to be recorded as
// requests. The hero's fields never hold a pick that waits for a Director:
// the sheet and Foundry read the field as real. A pick the hero already holds
// is left alone, since an entry it holds counts as allowed.
func takeAskFirst(entity *Entity, patch map[string]any, picks builderPicks) []askPick {
	var asks []askPick
	for k, v := range patch {
		st, isStep := picks[k]
		val, _ := v.(string)
		if !isStep || strings.TrimSpace(val) == "" {
			continue
		}
		ch, on := st.names[strings.ToLower(strings.TrimSpace(val))]
		if !on || ch.Held || ch.State != HeroChoiceStateAskFirst || holdsValue(entity, k, val) {
			continue
		}
		asks = append(asks, askPick{fieldKey: k, name: ch.Name})
		delete(patch, k)
		delete(patch, k+heroOptionsSuffix)
	}
	sort.Slice(asks, func(i, j int) bool { return asks[i].fieldKey < asks[j].fieldKey })
	return asks
}

// ownerNameList parses a list a player writes: "" or a JSON array of
// distinct short names.
func ownerNameList(k, val string) ([]string, error) {
	if strings.TrimSpace(val) == "" {
		return nil, nil
	}
	var names []string
	if err := json.Unmarshal([]byte(val), &names); err != nil {
		return nil, apperror.NewBadRequest(fmt.Sprintf("%q takes a list of names", k))
	}
	if len(names) > maxOwnerListItems {
		return nil, apperror.NewBadRequest(fmt.Sprintf("%q holds at most %d names", k, maxOwnerListItems))
	}
	seen := map[string]bool{}
	for _, n := range names {
		t := strings.ToLower(strings.TrimSpace(n))
		if t == "" || utf8.RuneCountInString(n) > maxOwnerListItem || seen[t] {
			return nil, apperror.NewBadRequest(fmt.Sprintf("%q takes distinct names of up to %d characters", k, maxOwnerListItem))
		}
		seen[t] = true
	}
	return names, nil
}

// checkStepOptions checks what a hero buys from a step's entry (an
// ancestry's traits): names on that entry's list, each once, within its
// points. The entry is the one the same patch picks, else the one the hero
// has.
func checkStepOptions(entity *Entity, patch map[string]any, step string, v any, st builderStep, refuseAsk bool) error {
	k := step + heroOptionsSuffix
	if v == nil {
		return nil
	}
	val, ok := v.(string)
	if !ok {
		return apperror.NewBadRequest(fmt.Sprintf("%q takes a list of names", k))
	}
	names, err := ownerNameList(k, val)
	if err != nil || len(names) == 0 {
		return err
	}
	entry, has := patch[step]
	if !has && entity != nil {
		entry = entity.FieldsData[step]
	}
	name, _ := entry.(string)
	choice, on := st.names[strings.ToLower(strings.TrimSpace(name))]
	// An entry the rules took off the list only counts while the hero already
	// holds it, so a grandfathered hero can still change what it bought.
	if on && choice.Held && has && !holdsValue(entity, step, name) {
		on = false
	}
	if !on {
		return apperror.NewBadRequest(fmt.Sprintf("choose the %s before what it lets you buy", step))
	}
	if refuseAsk && has && choice.State == HeroChoiceStateAskFirst && !holdsValue(entity, step, name) {
		return apperror.NewForbidden(askYourDirector(choice.Name))
	}
	costs, budget, ok := heroBuyList(choice.Properties)
	if !ok {
		return apperror.NewBadRequest(choice.Name + " has nothing to choose from")
	}
	spent := 0.0
	for _, n := range names {
		cost, known := costs[strings.ToLower(strings.TrimSpace(n))]
		if !known {
			return apperror.NewBadRequest(fmt.Sprintf("%q can't be chosen for %s", n, choice.Name))
		}
		spent += cost
	}
	if spent > budget {
		return apperror.NewBadRequest(fmt.Sprintf("those choices cost %g points; %s has %g", spent, choice.Name, budget))
	}
	return nil
}

// ClearStaleStepOptions adds a clear of what a hero bought from a step's
// entry when the patch changes that step and says nothing of the options:
// traits bought from the old ancestry don't belong to the new one. It works
// on any writer's patch, and needs no plan.
func ClearStaleStepOptions(entity *Entity, patch map[string]any) {
	if entity == nil {
		return
	}
	for k, v := range patch {
		if strings.HasSuffix(k, heroOptionsSuffix) {
			continue
		}
		opts := k + heroOptionsSuffix
		if _, said := patch[opts]; said {
			continue
		}
		had, _ := entity.FieldsData[opts].(string)
		if strings.TrimSpace(had) == "" || strings.TrimSpace(had) == "[]" {
			continue
		}
		old, _ := entity.FieldsData[k].(string)
		now, _ := v.(string)
		if !strings.EqualFold(strings.TrimSpace(old), strings.TrimSpace(now)) {
			patch[opts] = nil
		}
	}
}

// CheckClaimedOwnerPatch is authorizeFieldsWrite for a sheet change that
// does not come through the fields route: one a player's AI asked for and
// the player applied. It always judges the writer as a Player, whatever
// their role, and takes the hero builder plan that player is shown, so the
// AI can change exactly what the player could change by hand and no more.
func CheckClaimedOwnerPatch(entity *Entity, userID, campaignID string, patch map[string]any, typeFields []FieldDefinition, plan *HeroPlan) error {
	return authorizeFieldsWriteAs(campaigns.RolePlayer, entity, userID, campaignID, false, patch, typeFields, builderPicksFrom(plan), true)
}

// ClaimedOwnerPatchHeld is CheckClaimedOwnerPatch key by key: it returns
// why each key may not be written, so a player's AI change can keep the
// rest. err is set when the writer may not change the character at all.
func ClaimedOwnerPatchHeld(entity *Entity, userID, campaignID string, patch map[string]any, typeFields []FieldDefinition, plan *HeroPlan) (held map[string]string, err error) {
	if entity.CampaignID != campaignID || !IsClaimedOwnerPlayer(campaigns.RolePlayer, entity, userID) {
		return nil, apperror.NewForbidden("insufficient permissions")
	}
	held = map[string]string{}
	for k, e := range ownerFieldVerdicts(entity, patch, typeFields, builderPicksFrom(plan), true) {
		held[k] = apperror.SafeMessage(e)
	}
	return held, nil
}

// authorizeMetadataWrite decides whether a metadata PUT may proceed. A claimed
// owner may send the name and nothing else; descriptor, parent and anything
// unknown are refused so the narrow path cannot become a general edit.
func authorizeMetadataWrite(role campaigns.Role, entity *Entity, userID, campaignID string, rawBody []byte) error {
	if role >= campaigns.RoleScribe {
		return nil
	}
	if entity.CampaignID != campaignID || !IsClaimedOwnerPlayer(role, entity, userID) {
		return apperror.NewForbidden("insufficient permissions")
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &keys); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	name, ok := keys["name"]
	if !ok || string(name) == "null" || len(keys) != 1 {
		return apperror.NewForbidden("you can only rename your character")
	}
	return nil
}

// CanEditOwnPage reports whether the viewer may write the page's description
// and change its picture: any Scribe or above, or the player who claimed it.
// The player shapes their own hero; the Director keeps the numbers, the
// GM-only parts and who can see the page.
func CanEditOwnPage(role campaigns.Role, entity *Entity, userID string) bool {
	return CanEditIdentity(role, entity, userID)
}

// authorizeOwnPageWrite decides whether a description or picture write may
// proceed: Scribes pass; below that only the claimed owner, on a page of
// this campaign. An edit grant on someone else's page is not enough, so the
// route opening to players does not widen what those grants reach.
func authorizeOwnPageWrite(role campaigns.Role, entity *Entity, userID, campaignID string) error {
	if role >= campaigns.RoleScribe {
		return nil
	}
	if entity.CampaignID != campaignID || !IsClaimedOwnerPlayer(role, entity, userID) {
		return apperror.NewForbidden("insufficient permissions")
	}
	return nil
}

// requireOwnPageOrPageWrite guards the description and picture routes, which
// open to players. Below Scribe it passes the request on, and the handler's
// authorizeOwnPageWrite admits only the claimed owner; a Scribe or above
// meets requirePageWrite, as on every other page write.
func (h *Handler) requireOwnPageOrPageWrite(next echo.HandlerFunc) echo.HandlerFunc {
	guarded := h.requirePageWrite(next)
	return func(c *echo.Context) error {
		if cc := campaigns.GetCampaignContext(c); cc != nil && cc.MemberRole < campaigns.RoleScribe {
			return next(c)
		}
		return guarded(c)
	}
}

// ownerEntryWrite shapes a claimed owner's description save. The player's
// editor loads the text with the Director's parts taken out (secrets, GM-only
// pictures and diagrams, table rollers), so saving it over a text that has
// them would erase them: such a text stays the Director's to change. What the
// player sends is stripped of those parts too, since they could never read
// back text marked for the Director.
func ownerEntryWrite(stored *Entity, entryJSON, entryHTML string) (string, string, error) {
	if entryHasDirectorParts(derefStr(stored.Entry), derefStr(stored.EntryHTML)) {
		return "", "", apperror.NewForbidden("this description has parts only the game master can see, so only they can change it")
	}
	return sanitize.StripSecretsJSON(entryJSON), sanitize.StripSecretsHTML(entryHTML), nil
}

// entryHasDirectorParts reports whether a stored text holds anything a player
// is not shown. The HTML strip only removes, so any change means a part was
// there; the JSON strip re-encodes, so it is compared with a plain re-encode.
func entryHasDirectorParts(entryJSON, entryHTML string) bool {
	if entryHTML != "" && sanitize.StripSecretsHTML(entryHTML) != entryHTML {
		return true
	}
	if entryJSON == "" {
		return false
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(entryJSON), &doc); err != nil {
		return false
	}
	plain, err := json.Marshal(doc)
	if err != nil {
		return false
	}
	return sanitize.StripSecretsJSON(entryJSON) != string(plain)
}

// OwnerFieldKind is how a claimed owner may write a field of their hero, as
// the door judges it, for briefs and editors that offer only what it takes.
type OwnerFieldKind string

// The kinds a claimed owner's field can be.
const (
	OwnerFieldText   OwnerFieldKind = "text"   // short free text
	OwnerFieldChoice OwnerFieldKind = "choice" // a name from a builder step's list
	OwnerFieldStory  OwnerFieldKind = "story"  // the player's long text
	OwnerFieldList   OwnerFieldKind = "list"   // a JSON list of short names
)

// OwnerFieldKindOf says how a claimed owner may write key, or "" when they
// may not. typeFields are the hero's category's fields with any the page
// added; a GM-only field or one the category lacks is never theirs. A step
// whose key is also an identity key takes its list's names or the player's
// own (ownChoice).
func OwnerFieldKindOf(key string, typeFields []FieldDefinition, plan *HeroPlan) (kind OwnerFieldKind, ownChoice bool) {
	on := false
	for i := range typeFields {
		if typeFields[i].Key == key {
			if typeFields[i].GMOnly {
				return "", false
			}
			on = true
		}
	}
	if !on {
		return "", false
	}
	if plan != nil {
		for _, st := range plan.Steps {
			if st.FieldKey != key {
				continue
			}
			limited := st.Rules != nil && st.Rules.Limited
			if len(st.Choices) > 0 || limited {
				// A limited step is a pick from the list even where the
				// system's own text would be free.
				return OwnerFieldChoice, ownerIdentityFieldKeys[key] && !limited
			}
		}
	}
	switch {
	case ownerIdentityFieldKeys[key]:
		return OwnerFieldText, false
	case ownerStoryFieldKeys[key]:
		return OwnerFieldStory, false
	case ownerListFieldKeys[key]:
		return OwnerFieldList, false
	}
	return "", false
}

// HeroBuyList is what an entry lets a hero buy: the names on its list (in
// their own spelling), each one's cost and the points to spend. ok is false
// when the entry has nothing to buy.
func HeroBuyList(c HeroChoice) (names []string, costs map[string]float64, budget float64, ok bool) {
	lower, budget, ok := heroBuyList(c.Properties)
	if !ok {
		return nil, nil, 0, false
	}
	costs = map[string]float64{}
	for _, k := range sortedKeys(c.Properties) {
		items, isList := c.Properties[k].([]any)
		if !isList {
			continue
		}
		for _, it := range items {
			m, _ := it.(map[string]any)
			name, _ := m["name"].(string)
			if cost, on := lower[strings.ToLower(strings.TrimSpace(name))]; on && name != "" {
				if _, dup := costs[name]; !dup {
					names = append(names, strings.TrimSpace(name))
					costs[strings.TrimSpace(name)] = cost
				}
			}
		}
		if len(names) > 0 {
			break
		}
	}
	return names, costs, budget, true
}

// HeroOptionsKey is the field that keeps what a hero bought from a step's
// entry ("ancestry" → "ancestry_choices_json").
func HeroOptionsKey(step string) string { return step + heroOptionsSuffix }
