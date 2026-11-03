package records

import (
	"sort"
	"strings"
)

// lookupWhatFormat is the `what:` that hands back one kind's block format.
const lookupWhatFormat = "format"

// format answers `what: format`: the full doc of one kind, read from the
// registry so a kind added later is answerable without a change here.
func (run *lookupRun) format(kind string) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Format"}
	if run.l.Kinds == nil {
		return ans, badRequestf("block formats can't be looked up on this server")
	}
	kind = normKind(kind)
	if kind == "" {
		return ans, badRequestf("it needs `block:`, the kind to explain: one of %s", run.kindNames())
	}
	k, ok := run.l.Kinds.Get(kind)
	if !ok {
		ans.Chip = "Format · " + kind
		return ans, badRequestf("there is no `kind: %s`; the kinds are %s", kind, run.kindNames())
	}
	ans.Chip = "Format · " + k.Name()
	ans.Text = "## Format: kind: " + k.Name() + " (" + k.Label() + ")\n\n" + strings.TrimSpace(k.Doc()) + "\n"
	return ans, nil
}

func (run *lookupRun) kindNames() string {
	var names []string
	for _, k := range run.l.Kinds.Kinds() {
		names = append(names, k.Name())
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
