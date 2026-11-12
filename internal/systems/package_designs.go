package systems

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

// Limits on the designs a package ships. They bound what a third-party package
// can put in front of every Owner of a campaign on its system.
const (
	MaxPackageDesigns = 100
	// MaxPackageDesignBytes matches the designs plugin's own data cap, so a
	// package design can always be copied into a library.
	MaxPackageDesignBytes = 16 * 1024
	maxDesignName         = 60
	maxDesignNote         = 140
	maxDesignMaker        = 100
)

// DesignDef is one entry of a manifest's "designs" list: a saved design
// (a room theme, a magic circle...) the package ships for its campaigns to
// start from. The data itself lives in File, so the manifest stays readable.
type DesignDef struct {
	// Kind names a design kind the server knows ("room-theme", ...). The
	// designs kind registry is the authority, consulted when the file is read.
	Kind string `json:"kind"`
	// Name is what the library shows. Up to 60 characters.
	Name string `json:"name"`
	// Note is an optional line under the name. Up to 140 characters.
	Note string `json:"note,omitempty"`
	// Maker credits the person who made it. Optional, up to 100 characters.
	Maker string `json:"maker,omitempty"`
	// File is a JSON file inside the package holding the design's data: a
	// relative path with no "..", not absolute, ending in .json.
	File string `json:"file"`
}

// cleanDesignFile returns the package-relative form of a design file path, or
// why it is not acceptable. It is checked again against the real directory
// when read, because a symlink inside a package can still point out of it.
func cleanDesignFile(p string) (string, error) {
	switch {
	case p == "":
		return "", fmt.Errorf("file is required")
	case strings.ContainsAny(p, "\x00\\"):
		return "", fmt.Errorf("file %q must use forward slashes only", p)
	case strings.HasPrefix(p, "/") || filepath.IsAbs(p):
		return "", fmt.Errorf("file %q must be inside the package, not an absolute path", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", fmt.Errorf("file %q must not climb out with a .. segment", p)
		}
	}
	c := filepath.ToSlash(filepath.Clean(p))
	if c == "." || !strings.HasSuffix(strings.ToLower(c), ".json") {
		return "", fmt.Errorf("file %q must be a .json file", p)
	}
	return c, nil
}

// sanitizeDesigns keeps the entries that pass the manifest-level checks and
// records a warning for each one dropped. A bad design must not refuse the
// whole package, which is mostly something else; like play blocks, it is
// skipped and reported. The data file is checked later, when read, because
// that needs the designs kind registry.
func (m *SystemManifest) sanitizeDesigns() {
	if len(m.Designs) == 0 {
		return
	}
	kept := make([]DesignDef, 0, len(m.Designs))
	for i, d := range m.Designs {
		if len(kept) >= MaxPackageDesigns {
			m.DesignWarnings = append(m.DesignWarnings, fmt.Sprintf("Designs: only the first %d are used; %d more ignored", MaxPackageDesigns, len(m.Designs)-i))
			break
		}
		if reason := checkDesignDef(&d); reason != "" {
			m.DesignWarnings = append(m.DesignWarnings, fmt.Sprintf("Design %d (%q) skipped: %s", i+1, d.Name, reason))
			continue
		}
		kept = append(kept, d)
	}
	m.Designs = kept
}

// checkDesignDef normalises d in place and returns why it is unusable, or "".
func checkDesignDef(d *DesignDef) string {
	d.Kind, d.Name, d.Note, d.Maker = strings.TrimSpace(d.Kind), strings.TrimSpace(d.Name), strings.TrimSpace(d.Note), strings.TrimSpace(d.Maker)
	switch {
	case d.Kind == "":
		return "kind is required"
	case d.Name == "":
		return "name is required"
	case utf8.RuneCountInString(d.Name) > maxDesignName:
		return fmt.Sprintf("name is longer than %d characters", maxDesignName)
	case utf8.RuneCountInString(d.Note) > maxDesignNote:
		return fmt.Sprintf("note is longer than %d characters", maxDesignNote)
	case utf8.RuneCountInString(d.Maker) > maxDesignMaker:
		return fmt.Sprintf("maker is longer than %d characters", maxDesignMaker)
	}
	f, err := cleanDesignFile(d.File)
	if err != nil {
		return err.Error()
	}
	d.File = f
	return ""
}

// PackageDesign is a design read from a package and validated.
type PackageDesign struct {
	DesignDef
	Data json.RawMessage
}

// DesignKindChecker is the designs kind registry as the loader needs it. App
// wiring adapts it, so this package never imports the designs plugin.
type DesignKindChecker interface {
	// ValidateDesign returns the canonical data, or an error for an unknown
	// kind or data the kind refuses.
	ValidateDesign(ctx context.Context, kind string, raw json.RawMessage) (json.RawMessage, error)
}

// LoadPackageDesigns reads and validates a manifest's designs from the
// package directory dir. A design that fails is left out and named in the
// second result, so the diagnostics page can say why; the rest still load.
func LoadPackageDesigns(ctx context.Context, m *SystemManifest, dir string, kinds DesignKindChecker) ([]PackageDesign, []string) {
	if m == nil || len(m.Designs) == 0 {
		return nil, nil
	}
	var out []PackageDesign
	var skipped []string
	skip := func(d DesignDef, why string) {
		skipped = append(skipped, fmt.Sprintf("Design %q (%s) skipped: %s", d.Name, d.File, why))
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil || kinds == nil {
		for _, d := range m.Designs {
			skip(d, "the package's designs could not be read yet")
		}
		return nil, skipped
	}
	for _, d := range m.Designs {
		raw, why := readInsidePackage(root, d.File)
		if why != "" {
			skip(d, why)
			continue
		}
		data, err := kinds.ValidateDesign(ctx, d.Kind, raw)
		if err != nil {
			skip(d, err.Error())
			continue
		}
		if len(data) > MaxPackageDesignBytes {
			skip(d, "the design is too large after checking")
			continue
		}
		out = append(out, PackageDesign{DesignDef: d, Data: data})
	}
	return out, skipped
}

// readInsidePackage reads a file under root, refusing one that resolves
// outside it (a symlink), that is not a regular file, or that is over the cap.
func readInsidePackage(root, rel string) (json.RawMessage, string) {
	full, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, "the file is not in the package"
	}
	if full != root && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		return nil, "the file is outside the package"
	}
	// Checked before opening: opening a FIFO would block the request.
	if st, err := os.Stat(full); err != nil || !st.Mode().IsRegular() {
		return nil, "the file is not a regular file"
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, "the file could not be read"
	}
	defer f.Close()
	// One byte past the cap tells "exactly at the cap" from "over it".
	raw, err := io.ReadAll(io.LimitReader(f, MaxPackageDesignBytes+1))
	if err != nil {
		return nil, "the file could not be read"
	}
	if len(raw) > MaxPackageDesignBytes {
		return nil, fmt.Sprintf("the file is larger than %d KB", MaxPackageDesignBytes/1024)
	}
	if !json.Valid(raw) {
		return nil, "the file is not valid JSON"
	}
	return raw, ""
}

var (
	designKindsMu sync.RWMutex
	designKinds   DesignKindChecker
)

// SetDesignKinds gives the loader the kind registry. App wiring calls it once
// the designs plugin is up; without it a package's designs are reported as
// not yet readable rather than trusted.
func SetDesignKinds(k DesignKindChecker) {
	designKindsMu.Lock()
	designKinds = k
	designKindsMu.Unlock()
}

// PackageDesignsFor returns the validated designs of a loaded system and the
// reasons any were skipped. A system with none, or one that is not loaded,
// yields nothing.
func PackageDesignsFor(ctx context.Context, systemID string) ([]PackageDesign, []string) {
	m := Find(systemID)
	if m == nil || len(m.Designs) == 0 {
		return nil, nil
	}
	designKindsMu.RLock()
	k := designKinds
	designKindsMu.RUnlock()
	return LoadPackageDesigns(ctx, m, Dir(systemID), k)
}
