package designs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// OriginPackage labels a library row copied from a game system package. Like
// the other origins it is a label only: the copy never links back.
const OriginPackage = "package"

// MaxSuggestNoteLen caps the optional note to a package maintainer.
const MaxSuggestNoteLen = 240

// SuggestionFormat names the export file's shape; the version lets a later
// shape coexist with files maintainers already hold.
const SuggestionFormat = "chronicle-design-suggestion:v1"

// PackageItem is one design a game system package ships, already validated by
// the package loader. App wiring supplies it, so this package never imports
// the systems package.
type PackageItem struct {
	Kind, Name, Note, Maker, File string
	Data                          json.RawMessage
}

// PackageSource reads the designs a game system package ships.
type PackageSource interface {
	// Package returns the system's display name and its designs. A system with
	// none, or an unknown one, returns no items.
	Package(ctx context.Context, systemID string) (name string, items []PackageItem)
}

// PackageDesign is a package design as the library shows it. The id is
// derived from the kind and file, so it stays the same across restarts and
// never collides with a library row's id.
type PackageDesign struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Name      string          `json:"name"`
	Note      string          `json:"note,omitempty"`
	MakerName string          `json:"makerName,omitempty"`
	Data      json.RawMessage `json:"data"`
}

// PackageGroup is the read-only "From <system>" group: the designs of the
// campaign's own game system package, and nothing from any other.
type PackageGroup struct {
	SystemID   string `json:"systemId"`
	SystemName string `json:"systemName"`
	// You is the name a "Credit me" suggestion would carry, so the page can
	// show it before anything is exported.
	You     string          `json:"you"`
	Designs []PackageDesign `json:"designs"`
}

// SuggestInput is what the Owner chooses in the Suggest fold.
type SuggestInput struct {
	// Note is for the maintainer, up to MaxSuggestNoteLen.
	Note string `json:"note"`
	// Credit puts the Owner's display name on the design as its maker.
	Credit bool `json:"credit"`
}

// Suggestion is the export: a file a maintainer can paste into a package. The
// Designs entries are exactly manifest `designs` entries, and Data holds the
// content of each entry's File, keyed by that path.
type Suggestion struct {
	Format   string                     `json:"format"`
	System   string                     `json:"system"`
	Designs  []SuggestionEntry          `json:"designs"`
	Data     map[string]json.RawMessage `json:"data"`
	Message  string                     `json:"messageForMaintainer,omitempty"`
	Filename string                     `json:"-"`
}

// SuggestionEntry is one manifest `designs` entry.
type SuggestionEntry struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Note  string `json:"note,omitempty"`
	Maker string `json:"maker,omitempty"`
	File  string `json:"file"`
}

func packageID(kind, file string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + file))
	return "pkg-" + hex.EncodeToString(sum[:])[:16]
}

// packageItems is the campaign's own system's package designs. No source, no
// system or a custom system (which has no maintainer to ship anything) is an
// empty group, never an error.
func (s *service) packageItems(ctx context.Context, c *CampaignInfo) (string, []PackageItem) {
	if s.pkgs == nil || c.SystemID == "" || isCustomSystem(c.SystemID) {
		return "", nil
	}
	return s.pkgs.Package(ctx, c.SystemID)
}

func isCustomSystem(id string) bool { return strings.HasPrefix(id, "custom:") }

// Package lists the read-only package group, optionally for one kind.
func (s *service) Package(ctx context.Context, campaignID, userID, kind string) (*PackageGroup, error) {
	c, you, err := s.requireOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	name, items := s.packageItems(ctx, c)
	g := &PackageGroup{SystemID: c.SystemID, SystemName: name, You: clip(cleanText(you), MaxMakerNameLen), Designs: []PackageDesign{}}
	for _, it := range items {
		if kind != "" && it.Kind != kind {
			continue
		}
		g.Designs = append(g.Designs, PackageDesign{ID: packageID(it.Kind, it.File), Kind: it.Kind, Name: it.Name, Note: it.Note, MakerName: it.Maker, Data: it.Data})
	}
	return g, nil
}

// CopyPackage makes an independent library row from a package design, with
// origin package. The package is never edited, and the row is re-validated
// through the kind like any take.
func (s *service) CopyPackage(ctx context.Context, campaignID, userID, id string) (*Design, error) {
	c, _, err := s.requireOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	sysName, items := s.packageItems(ctx, c)
	for _, it := range items {
		if packageID(it.Kind, it.File) != id {
			continue
		}
		data, err := s.validData(it.Kind, it.Data)
		if err != nil {
			return nil, err
		}
		maker := it.Maker
		if maker == "" {
			maker = sysName
		}
		return s.store(ctx, c, newDesign{kind: it.Kind, name: it.Name, note: it.Note, data: data, makerName: maker, origin: OriginPackage, ref: sysName})
	}
	return nil, apperror.NewNotFound(msgNotFound)
}

// Suggest builds the export file for one of the Owner's designs. Nothing is
// sent anywhere: the Owner downloads it and hands it to the maintainer.
func (s *service) Suggest(ctx context.Context, campaignID, userID, id string, in SuggestInput) (*Suggestion, error) {
	c, maker, err := s.requireOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	if c.SystemID == "" || isCustomSystem(c.SystemID) {
		return nil, apperror.NewBadRequest("only a game system package can be sent designs")
	}
	d, err := s.own(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	// Only the Owner's own work goes to a maintainer: a copy carries someone
	// else's choices, and crediting it to the exporter would misattribute it.
	if d.Origin != OriginMade {
		return nil, apperror.NewBadRequest("only a design you made yourself can be suggested; this one is a copy")
	}
	if d.SystemID != "" && d.SystemID != c.SystemID {
		return nil, apperror.NewBadRequest("this design was made for another game system")
	}
	msg, err := checkLabel("note", in.Note, MaxSuggestNoteLen, false)
	if err != nil {
		return nil, err
	}
	// The data is checked again so a design made before a part was retired is
	// never exported as a suggestion.
	data, err := s.validData(d.Kind, d.Data)
	if err != nil {
		return nil, err
	}
	name := d.Name
	if utf8.RuneCountInString(name) > MaxNameLen {
		name = string([]rune(name)[:MaxNameLen])
	}
	slug := fileSlug(name)
	file := "designs/" + slug + ".json"
	e := SuggestionEntry{Kind: d.Kind, Name: name, Note: d.Note, File: file}
	if in.Credit {
		e.Maker = clip(cleanText(maker), MaxMakerNameLen)
	}
	return &Suggestion{
		Format: SuggestionFormat, System: c.SystemID,
		Designs: []SuggestionEntry{e}, Data: map[string]json.RawMessage{file: data},
		Message: msg, Filename: fileSlug(c.SystemID) + "-design-" + slug + ".json",
	}, nil
}

// fileSlug is a name as lowercase letters, digits and single hyphens, so it is
// safe as a file name and as a package path; "design" if nothing is left.
func fileSlug(s string) string {
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 40 {
		out = strings.Trim(out[:40], "-")
	}
	if out == "" {
		return "design"
	}
	return out
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
