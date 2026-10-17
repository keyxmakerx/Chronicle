// Package semver orders package version strings. It is a leaf package so the
// systems loader and the packages plugin, which must not import each other,
// pick the "newest" version by the same rule.
package semver

import (
	"strconv"
	"strings"
)

// Compare returns -1, 0 or 1 as a is lower than, equal to or higher than b.
// A leading "v" is ignored; core components compare by their leading digits
// (a component with none counts as 0) and a missing component counts as 0.
// A pre-release suffix ("-rc1") sorts below the same plain release, which
// is what makes a beta never outrank the release it precedes. Build
// metadata ("+abc") is ignored.
func Compare(a, b string) int {
	ac, ap := split(a)
	bc, bp := split(b)

	n := len(ac)
	if len(bc) > n {
		n = len(bc)
	}
	for i := 0; i < n; i++ {
		if c := cmpInt(part(ac, i), part(bc, i)); c != 0 {
			return c
		}
	}

	switch {
	case ap == "" && bp == "":
		return 0
	case ap == "":
		return 1
	case bp == "":
		return -1
	}
	return comparePre(ap, bp)
}

// Less reports whether a is lower than b.
func Less(a, b string) bool { return Compare(a, b) < 0 }

// split separates the dot-separated core from the pre-release suffix.
func split(v string) (core []string, pre string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v, pre = v[:i], v[i+1:]
	}
	return strings.Split(v, "."), pre
}

// part returns the leading-digit value of component i, or 0.
func part(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	p := parts[i]
	end := 0
	for end < len(p) && p[end] >= '0' && p[end] <= '9' {
		end++
	}
	n, _ := strconv.Atoi(p[:end])
	return n
}

// comparePre orders two pre-release suffixes identifier by identifier, numeric
// identifiers numerically so "rc.2" < "rc.10".
func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		ai, aerr := strconv.Atoi(as[i])
		bi, berr := strconv.Atoi(bs[i])
		switch {
		case aerr == nil && berr == nil:
			if c := cmpInt(ai, bi); c != 0 {
				return c
			}
		case aerr == nil:
			return -1 // numeric identifiers sort below alphanumeric ones
		case berr == nil:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	return cmpInt(len(as), len(bs))
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
