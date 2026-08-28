package calendar

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

// presets.go — the shipped calendar presets, kept alive through the importer.
//
// CALV5 SALVAGE. The pre-deletion tree had builder_presets.go (417 lines)
// carrying these payloads plus the wizard's gallery UI. The UI half depended on
// the old builder's draft model, which V5 replaces, so only the half that is
// still true is recovered: the payloads, and the ruling that made them cheap.
//
// THE RULING, WHICH IS WORTH KEEPING VERBATIM: a preset IS an export. Presets
// are applied through the EXACT SAME DetectAndParse path a user's uploaded file
// takes — no preset table, no preset migration, no second parser, no second
// apply path. The gallery and the importer are one code path with two front
// doors.
//
// The proof is structural rather than asserted: there is no way to read a
// preset's months here without going through the shipped parser, so a malformed
// preset fails exactly where a malformed upload fails, with the same message.
//
// They are embedded (not left as loose files) deliberately: an unreferenced
// data file in the tree is precisely the kind of corpse the 2026-08-21
// demolition existed to clear.

//go:embed presets/*.json
var presetFS embed.FS

// PresetNames returns the available preset ids, sorted for determinism.
//
// Determinism matters more than it looks: the pre-deletion tree carried a
// dedicated regression test (import_order_determinism_test.go) because map
// iteration had produced a different month order between runs, which turned
// into a different calendar depending on when you clicked.
func PresetNames() ([]string, error) {
	entries, err := fs.Glob(presetFS, "presets/*.json")
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e[len("presets/"):len(e)-len(".json")])
	}
	sort.Strings(out)
	return out, nil
}

// LoadPreset parses one shipped preset through the ordinary import path.
//
// The return type is the importer's own ImportResult on purpose — a caller
// cannot tell a preset from an uploaded file, which is the whole point of the
// ruling above.
func LoadPreset(name string) (*ImportResult, error) {
	data, err := presetFS.ReadFile("presets/" + name + ".json")
	if err != nil {
		return nil, fmt.Errorf("unknown preset %q: %w", name, err)
	}
	return DetectAndParse(data)
}
