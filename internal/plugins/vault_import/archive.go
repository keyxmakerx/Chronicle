package vault_import

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Limits bound what one uploaded archive may ask of the server. Every number is
// checked against what the zip declares AND against what is actually read, since
// a hostile archive controls its own headers.
type Limits struct {
	MaxUploadBytes int64 // the .zip file itself
	MaxEntries     int   // files and folders listed in the zip
	MaxTotalBytes  int64 // all files, uncompressed
	MaxFileBytes   int64 // one picture or file, uncompressed
	MaxNoteBytes   int64 // one Markdown note, uncompressed
	MaxRatio       int64 // uncompressed / compressed, for entries over ratioFloor
	MaxDepth       int   // folders deep
	MaxPathBytes   int
}

// DefaultLimits suit a large vault (a few thousand notes and their pictures)
// while keeping one request's worst case small enough to hold in a campaign's
// storage quota and the server's memory.
var DefaultLimits = Limits{
	MaxUploadBytes: 200 << 20,
	MaxEntries:     10000,
	MaxTotalBytes:  600 << 20,
	MaxFileBytes:   25 << 20,
	MaxNoteBytes:   2 << 20,
	MaxRatio:       200,
	MaxDepth:       24,
	MaxPathBytes:   1024,
}

// ratioFloor is the size under which a high compression ratio proves nothing:
// a few kilobytes of repeated text legitimately shrinks 1000:1.
const ratioFloor = 1 << 20

// ArchiveError is a refusal whose text is safe and useful to show the owner.
// Anything else the archive code returns is an internal problem.
type ArchiveError struct{ Msg string }

func (e *ArchiveError) Error() string { return e.Msg }

func refuse(format string, a ...any) error {
	return &ArchiveError{Msg: fmt.Sprintf(format, a...)}
}

// IsArchiveError reports whether err is a refusal to show the owner as is.
func IsArchiveError(err error) bool {
	var ae *ArchiveError
	return errors.As(err, &ae)
}

// EntryKind says what an admitted archive entry is for.
type EntryKind int

const (
	// KindNote is a Markdown note that becomes a page.
	KindNote EntryKind = iota
	// KindPicture is a picture that can be shown inside page text.
	KindPicture
	// KindFile is any other file; it can only be attached to a page.
	KindFile
)

// Entry is one admitted file. Path is clean, relative to the vault root and
// always uses "/".
type Entry struct {
	Path string
	Kind EntryKind
	Size int64
	// raw is the entry's name inside the zip, which is how Read finds it in
	// whichever Archive value is open: an Entry outlives the Archive that made it.
	raw string
}

// SkippedEntry is a file the archive held that was left out, with the reason.
type SkippedEntry struct {
	Path   string
	Reason string
}

// Archive is a validated zip. Nothing in it has been extracted: entries are
// read one at a time, in memory, with a hard size ceiling.
type Archive struct {
	// Name is the vault's name: the zip's single top folder, else the file name.
	Name    string
	Entries []Entry
	// Ignored counts hidden and system files (".obsidian", ".git", ".DS_Store",
	// "__MACOSX") that are never imported and never worth listing.
	Ignored int
	Skipped []SkippedEntry
	// CaseClashes lists pairs of files whose names differ only by capitals or
	// by how an accent is encoded. Both are imported; link resolution is
	// deterministic about which a link means.
	CaseClashes [][2]string

	rc     *zip.ReadCloser
	byRaw  map[string]*zip.File
	limits Limits
}

// pictureExts are the picture types the media service accepts; SVG is left out
// on purpose (script risk) and so is anything the service would refuse.
var pictureExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}

// IsPictureName reports whether a file name is a picture that can sit inside
// page text.
func IsPictureName(name string) bool {
	return pictureExts[strings.ToLower(path.Ext(name))]
}

// OpenArchive validates the zip at zipPath against lim. It refuses the whole
// archive for anything that looks hostile (unsafe paths, a repeated name, too
// many or too large files, a compression ratio no real notes have, a
// password), and skips, with a reason, what is merely unsupported (shortcuts,
// files over the per-file ceiling, unusual compression).
func OpenArchive(zipPath, fallbackName string, lim Limits) (*Archive, error) {
	rc, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, refuse("That doesn't look like a .zip file, or it is damaged. Zip the folder again and retry.")
	}
	a, err := admit(rc, fallbackName, lim)
	if err != nil {
		_ = rc.Close()
		return nil, err
	}
	return a, nil
}

// Close releases the zip file.
func (a *Archive) Close() error { return a.rc.Close() }

// CleanEntryName validates a name exactly as stored in the zip and returns it
// without a trailing slash. Backslashes, absolute paths, drive letters, "..",
// "." and empty segments, control characters and invalid UTF-8 are all refused:
// the importer never writes these names to disk, but a name that cannot be
// trusted also cannot be trusted to mean what a link says.
func CleanEntryName(raw string, lim Limits) (string, error) {
	if raw == "" {
		return "", errors.New("empty name")
	}
	if !utf8.ValidString(raw) {
		return "", errors.New("name is not valid UTF-8")
	}
	if strings.Contains(raw, `\`) {
		return "", errors.New("backslash in path")
	}
	if strings.HasPrefix(raw, "/") {
		return "", errors.New("absolute path")
	}
	if len(raw) >= 2 && raw[1] == ':' {
		return "", errors.New("drive letter in path")
	}
	if len(raw) > lim.MaxPathBytes {
		return "", errors.New("path too long")
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return "", errors.New("control character in path")
		}
	}
	name := strings.TrimSuffix(raw, "/")
	segs := strings.Split(name, "/")
	if len(segs) > lim.MaxDepth {
		return "", errors.New("folders nested too deeply")
	}
	for _, s := range segs {
		switch {
		case s == "" || s == "." || s == "..":
			return "", errors.New("relative segment in path")
		case len(s) > 255:
			return "", errors.New("name too long")
		}
	}
	return name, nil
}

// foldKey is the form names are compared in: accents composed, capitals folded.
func foldKey(s string) string { return strings.ToLower(norm.NFC.String(s)) }

func isHiddenPath(p string) bool {
	for _, s := range strings.Split(p, "/") {
		if strings.HasPrefix(s, ".") || s == "__MACOSX" || strings.EqualFold(s, "Thumbs.db") {
			return true
		}
	}
	return false
}

func admit(rc *zip.ReadCloser, fallbackName string, lim Limits) (*Archive, error) {
	if len(rc.File) > lim.MaxEntries {
		return nil, refuse("This zip lists %d files and folders; the limit is %d. Import a smaller part of the vault.", len(rc.File), lim.MaxEntries)
	}
	a := &Archive{rc: rc, limits: lim, byRaw: map[string]*zip.File{}}

	seenExact := make(map[string]bool, len(rc.File))
	seenFold := make(map[string]string, len(rc.File))
	var total uint64
	var files []Entry

	for _, f := range rc.File {
		name, err := CleanEntryName(f.Name, lim)
		if err != nil {
			return nil, refuse("This zip holds a file with an unsafe name (%s), so nothing was read. Re-create the zip from the vault folder.", err.Error())
		}
		if f.Flags&0x1 != 0 {
			return nil, refuse("This zip is password protected. Make an ordinary zip and retry.")
		}
		isDir := strings.HasSuffix(f.Name, "/") || f.Mode().IsDir()
		if seenExact[name] {
			return nil, refuse("This zip lists the same file name twice, so it is unclear which copy counts. Re-create the zip.")
		}
		seenExact[name] = true
		if isDir {
			continue
		}
		if mode := f.Mode(); mode&os.ModeSymlink != 0 || !mode.IsRegular() {
			a.Skipped = append(a.Skipped, SkippedEntry{Path: name, Reason: "a shortcut, not a file"})
			continue
		}
		if f.Method != zip.Store && f.Method != zip.Deflate {
			a.Skipped = append(a.Skipped, SkippedEntry{Path: name, Reason: "uses a compression this import cannot read"})
			continue
		}
		total += f.UncompressedSize64
		if total > uint64(lim.MaxTotalBytes) {
			return nil, refuse("This zip unpacks to more than %d MB, which is over the limit. Import a smaller part of the vault.", lim.MaxTotalBytes>>20)
		}
		if f.UncompressedSize64 > ratioFloor {
			if f.CompressedSize64 == 0 || int64(f.UncompressedSize64/f.CompressedSize64) > lim.MaxRatio {
				return nil, refuse("This zip is compressed far more than real notes and pictures are, so it was refused for safety.")
			}
		}
		if k := foldKey(name); seenFold[k] != "" && seenFold[k] != name {
			a.CaseClashes = append(a.CaseClashes, [2]string{seenFold[k], name})
		} else {
			seenFold[k] = name
		}
		if isHiddenPath(name) {
			a.Ignored++
			continue
		}
		files = append(files, Entry{Path: name, raw: name, Size: int64(f.UncompressedSize64)})
		a.byRaw[name] = f
	}

	root := commonRoot(files)
	a.Name = strings.TrimSpace(strings.TrimSuffix(fallbackName, path.Ext(fallbackName)))
	if root != "" {
		a.Name = root
	}
	if a.Name == "" {
		a.Name = "Vault"
	}

	// Skips were recorded before the root was known; show them vault-relative
	// like every other path.
	if root != "" {
		for i := range a.Skipped {
			a.Skipped[i].Path = strings.TrimPrefix(a.Skipped[i].Path, root+"/")
		}
	}

	for _, e := range files {
		if root != "" {
			e.Path = strings.TrimPrefix(e.Path, root+"/")
		}
		ext := strings.ToLower(path.Ext(e.Path))
		switch {
		case ext == ".md" || ext == ".markdown":
			e.Kind = KindNote
			if e.Size > lim.MaxNoteBytes {
				a.Skipped = append(a.Skipped, SkippedEntry{Path: e.Path, Reason: fmt.Sprintf("a note over %d MB", lim.MaxNoteBytes>>20)})
				continue
			}
		case pictureExts[ext]:
			e.Kind = KindPicture
		default:
			e.Kind = KindFile
		}
		if e.Kind != KindNote && e.Size > lim.MaxFileBytes {
			a.Skipped = append(a.Skipped, SkippedEntry{Path: e.Path, Reason: fmt.Sprintf("larger than %d MB", lim.MaxFileBytes>>20)})
			continue
		}
		a.Entries = append(a.Entries, e)
	}
	sort.Slice(a.Entries, func(i, j int) bool { return a.Entries[i].Path < a.Entries[j].Path })
	return a, nil
}

// commonRoot is the single top folder every file sits under, or "" when files
// sit at the top or under several folders. Zipping a vault folder from a file
// manager nests everything under the vault's name; that name is not a page.
func commonRoot(files []Entry) string {
	root := ""
	for _, e := range files {
		i := strings.IndexByte(e.Path, '/')
		if i < 0 {
			return ""
		}
		top := e.Path[:i]
		if root == "" {
			root = top
		} else if top != root {
			return ""
		}
	}
	return root
}

// Read returns an entry's bytes, never more than max. The zip's declared size
// is not trusted: the reader is cut off at max+1 and a longer entry is an error.
func (a *Archive) Read(e Entry, max int64) ([]byte, error) {
	f, ok := a.byRaw[e.raw]
	if !ok {
		return nil, fmt.Errorf("%s is not in this archive", e.Path)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", e.Path, err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, max+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", e.Path, err)
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%s is larger than its limit", e.Path)
	}
	return data, nil
}
