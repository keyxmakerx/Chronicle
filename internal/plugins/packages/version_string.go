package packages

import (
	"regexp"
	"strings"
)

// maxVersionLen bounds a version string; real release tags are far shorter.
const maxVersionLen = 50

// versionStringPattern is the allowlist for version strings that become a
// directory name or file name on disk. It requires an alphanumeric first
// character and permits only characters that are safe in a single path
// segment.
var versionStringPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,49}$`)

// ValidVersionString reports whether v is safe to use as a package version
// in a filesystem path or URL. Every path built from a stored or requested
// version must pass this check, so callers do not each re-implement it.
func ValidVersionString(v string) bool {
	if len(v) > maxVersionLen || strings.Contains(v, "..") {
		return false
	}
	return versionStringPattern.MatchString(v)
}
