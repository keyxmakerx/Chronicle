package foundry_modules

import (
	"strconv"
	"strings"
)

// semverLess reports whether version a is strictly older than version b.
// Used by the "notify older campaigns" repository query and by the
// service's LatestAvailable fallback when only deprecated versions
// remain.
//
// Implementation handles the dialect Foundry modules actually ship:
//
//   - Optional leading "v" (Foundry release tags vary)
//   - 3-segment dotted decimals: "0.1.5", "1.10.0"
//   - Optional pre-release after "-": "0.2.0-beta.1" sorts < "0.2.0"
//
// Non-numeric segments compare lexicographically (so "alpha" < "beta").
// Missing segments are treated as 0 ("1.0" == "1.0.0"). This is
// intentionally permissive — Chronicle isn't enforcing strict semver,
// just ordering versions the way operators expect.
func semverLess(a, b string) bool {
	pa, pra := splitVersion(a)
	pb, prb := splitVersion(b)

	for i := 0; i < len(pa) || i < len(pb); i++ {
		var ai, bi int
		if i < len(pa) {
			ai, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			bi, _ = strconv.Atoi(pb[i])
		}
		if ai != bi {
			return ai < bi
		}
	}

	// Numeric parts equal — compare pre-release. Per semver, a version
	// with a pre-release sorts BEFORE the same version without one.
	switch {
	case pra == "" && prb == "":
		return false
	case pra == "":
		return false // a is the release; release > pre-release
	case prb == "":
		return true // b is the release; pre-release < release
	default:
		return pra < prb
	}
}

// splitVersion drops a leading "v", splits on "-" to separate the
// core dotted-decimal from the pre-release tag, and returns the
// dotted segments + the (possibly empty) pre-release string.
func splitVersion(s string) ([]string, string) {
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	core, pre, _ := strings.Cut(s, "-")
	return strings.Split(core, "."), pre
}
