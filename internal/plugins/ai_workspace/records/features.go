package records

import (
	"sort"
	"strings"
)

// OffFeatures maps a kind (as Get normalizes it) to the name of the campaign
// feature it writes into, for the features switched off in Game & features.
// A switched-off feature's pages redirect to the dashboard, so what the
// import made there would be out of the owner's sight: the prompt leaves
// those kinds out and the review refuses them.
type OffFeatures map[string]string

// For is the switched-off feature a record would write into, or "". The
// generator is judged by what it makes: names go into a rolling table,
// everything else onto the calendar.
func (o OffFeatures) For(rec Record) string {
	if len(o) == 0 {
		return ""
	}
	k := normKind(rec.Kind)
	if k == "generator" {
		if strings.EqualFold(strings.TrimSpace(rec.Str("generator")), "names") {
			return ""
		}
		return o[KindCalendar]
	}
	return o[k]
}

// OffFeatureReason is the review row's wording for a record For refuses.
func OffFeatureReason(feature string) string {
	return feature + " is switched off in this campaign, so you couldn't open what this makes. Turn it on in Game & features, then paste again."
}

// Without is the registry less the kinds o switches off, for the prompt.
// The generator stays, since its names still work; Note tells the AI which
// of its runs don't.
func (r *Registry) Without(o OffFeatures) *Registry {
	if r == nil || len(o) == 0 {
		return r
	}
	var kinds []Kind
	for _, k := range r.kinds {
		if o[normKind(k.Name())] == "" {
			kinds = append(kinds, k)
		}
	}
	return NewRegistry(kinds...)
}

// Note is the prompt's line naming what is switched off and which kinds
// that rules out; "" when nothing is.
func (o OffFeatures) Note() string {
	if len(o) == 0 {
		return ""
	}
	byFeature := map[string][]string{}
	for k, f := range o {
		byFeature[f] = append(byFeature[f], "`"+k+"`")
	}
	features := make([]string, 0, len(byFeature))
	for f := range byFeature {
		features = append(features, f)
	}
	sort.Strings(features)
	parts := make([]string, len(features))
	for i, f := range features {
		ks := byFeature[f]
		sort.Strings(ks)
		if f == o[KindCalendar] {
			ks = append(ks, "`generator` runs other than names")
		}
		parts[i] = f + " (" + strings.Join(ks, ", ") + ")"
	}
	return "Switched off in this campaign, so don't write these blocks: " + strings.Join(parts, "; ") +
		". If the request needs one, tell me to turn it on in Game & features first."
}
