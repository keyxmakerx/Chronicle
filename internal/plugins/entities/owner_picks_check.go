package entities

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// PlayerPicksHeld judges the hero-choice part of a sheet write made on a
// claimed player's behalf outside the web door (Foundry pushes a whole mapped
// field set under the GM's key). It returns why each pick may not be stored;
// an empty map means every key may go through.
//
// Only the keys the web door judges as picks are looked at: a builder step,
// what a step's entry lets the hero buy, and the identity texts (which a rule
// turns into a pick from the list). Every other key is left to the caller's
// own rules, because Foundry legitimately writes mapped numbers such as
// stamina that the web door would refuse a player. A value sent back
// unchanged is never held: a client resends the whole stored set, and a hero
// keeps what it already holds. Ask-first picks are held too, since Foundry
// has no way to record a request and the sheet must never store a pick that
// is still waiting for a Director.
func PlayerPicksHeld(entity *Entity, patch map[string]any, plan *HeroPlan) map[string]string {
	picks := builderPicksFrom(plan)
	judged := make(map[string]any, len(patch))
	for k, v := range patch {
		if !isPickKey(k, picks) || sentBackUnchanged(entity, k, v) {
			continue
		}
		judged[k] = v
	}
	held := map[string]string{}
	if len(judged) == 0 {
		return held
	}
	for k, err := range ownerFieldVerdicts(entity, judged, nil, picks, true) {
		held[k] = apperror.SafeMessage(err)
	}
	return held
}

// PlayerGMOnlyHeld returns why each key the category marks GM-only may not be
// stored from a sheet write made on a claimed player's behalf: the player
// cannot see that field, so they may not set it either, whichever door the
// write came through. A value sent back unchanged is never held.
func PlayerGMOnlyHeld(entity *Entity, patch map[string]any, typeFields []FieldDefinition) map[string]string {
	held := map[string]string{}
	for i := range typeFields {
		k := typeFields[i].Key
		v, sent := patch[k]
		if !typeFields[i].GMOnly || !sent || sentBackUnchanged(entity, k, v) {
			continue
		}
		held[k] = fmt.Sprintf("%q is your GM's to change, not yours", k)
	}
	return held
}

// PatchChangesNothing reports whether every key in patch is what the hero
// already stores, so a caller can skip building the hero plan: Foundry
// resends the whole mapped set, and most pushes change nothing a plan judges.
func PatchChangesNothing(entity *Entity, patch map[string]any) bool {
	for k, v := range patch {
		if !sentBackUnchanged(entity, k, v) {
			return false
		}
	}
	return true
}

// isPickKey reports whether the web door judges k as a pick.
func isPickKey(k string, picks builderPicks) bool {
	if _, ok := picks[k]; ok || ownerIdentityFieldKeys[k] {
		return true
	}
	if step, ok := strings.CutSuffix(k, heroOptionsSuffix); ok {
		_, isStep := picks[step]
		return isStep
	}
	return false
}

// sentBackUnchanged reports whether v is what the hero already stores at k.
// A null over an empty or absent value changes nothing either.
func sentBackUnchanged(entity *Entity, k string, v any) bool {
	if entity == nil {
		return false
	}
	cur, has := entity.FieldsData[k]
	if v == nil {
		s, isText := cur.(string)
		return !has || cur == nil || (isText && strings.TrimSpace(s) == "")
	}
	if s, ok := v.(string); ok {
		old, isText := cur.(string)
		return isText && strings.EqualFold(strings.TrimSpace(old), strings.TrimSpace(s))
	}
	return reflect.DeepEqual(cur, v)
}
