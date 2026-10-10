package media

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

// The types a person may attach to a page. A page file is only ever downloaded
// (never rendered inline), but the type still decides what the server will
// store: HTML, SVG, scripts and executables are not on this list, so they are
// refused at the door rather than relying on the response headers alone.
//
// The type comes from the file's extension, and the content must then prove it:
// the declared Content-Type of a browser is never trusted.
const (
	mimePDF  = "application/pdf"
	mimeText = "text/plain"
	mimeMD   = "text/markdown"
	mimeDocx = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	mimeXlsx = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	mimeOdt  = "application/vnd.oasis.opendocument.text"
	mimeZip  = "application/zip"
)

// pageFileExtensions maps a lower-case extension to the one MIME type stored
// for it.
var pageFileExtensions = map[string]string{
	".pdf":  mimePDF,
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
	".txt":  mimeText,
	".md":   mimeMD,
	".docx": mimeDocx,
	".xlsx": mimeXlsx,
	".odt":  mimeOdt,
	".mp3":  "audio/mpeg",
	".ogg":  "audio/ogg",
	".zip":  mimeZip,
}

// PageFileMimeTypes is the set of MIME types a page file may be stored as.
var PageFileMimeTypes = func() map[string]bool {
	m := make(map[string]bool, len(pageFileExtensions))
	for _, mime := range pageFileExtensions {
		m[mime] = true
	}
	return m
}()

// PageFileAcceptAttr is the file picker's accept list, so the browser greys out
// what the server would refuse anyway.
const PageFileAcceptAttr = ".pdf,.png,.jpg,.jpeg,.webp,.gif,.txt,.md,.docx,.xlsx,.odt,.mp3,.ogg,.zip"

// PageFileMimeForName returns the stored MIME type for a file name, from its
// extension alone. ok is false for anything off the list, including a name with
// no extension.
func PageFileMimeForName(name string) (string, bool) {
	mime, ok := pageFileExtensions[strings.ToLower(path.Ext(cleanFileName(name)))]
	return mime, ok
}

// cleanFileName reduces what a browser sent to a plain display name: no folder
// (either slash style), no control characters, bounded length.
func cleanFileName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if len(name) > 255 {
		// Cut on a rune boundary so the name stays valid UTF-8; keep the
		// extension so the type still reads right.
		ext := path.Ext(name)
		if len(ext) > 16 {
			ext = ""
		}
		stem := name[:255-len(ext)]
		for !utf8.ValidString(stem) {
			stem = stem[:len(stem)-1]
		}
		name = stem + ext
	}
	return name
}

// CleanPageFileName is cleanFileName for the handler.
func CleanPageFileName(name string) string { return cleanFileName(name) }

// errPageFileType is the one message for every content mismatch, so a refusal
// never says which check a crafted file failed.
var errPageFileType = errors.New("file content does not match its type")

// errPageFileExecutable names the executable refusal, which is worth saying
// plainly.
var errPageFileExecutable = errors.New("program files cannot be attached")

// maxZipEntries bounds how many entries the structure check will walk, so a
// crafted archive cannot make an upload slow.
const maxZipEntries = 10000

// validatePageFileContent checks that data really is what mime says, and that
// it is not a program. It runs on the bytes the server will store.
func validatePageFileContent(data []byte, mime string) error {
	if len(data) == 0 {
		return errors.New("file is empty")
	}
	if looksExecutable(data) {
		return errPageFileExecutable
	}
	switch mime {
	case mimePDF:
		// Readers accept the header within the first kilobyte.
		head := data
		if len(head) > 1024 {
			head = head[:1024]
		}
		if !bytes.Contains(head, []byte("%PDF-")) {
			return errPageFileType
		}
	case mimeText, mimeMD:
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return errPageFileType
		}
	case mimeZip:
		if _, err := openZip(data); err != nil {
			return errPageFileType
		}
	case mimeDocx:
		return requireZipEntry(data, "word/")
	case mimeXlsx:
		return requireZipEntry(data, "xl/")
	case mimeOdt:
		return requireODT(data)
	default:
		// Pictures and audio keep the checks the rest of media uses.
		if !validateMagicBytes(data, mime) {
			return errPageFileType
		}
	}
	return nil
}

// looksExecutable spots the headers of programs and scripts a file name could
// hide: ELF, Windows PE/DOS, Mach-O (thin, fat and Java class share 0xCAFEBABE),
// and a shebang line.
func looksExecutable(data []byte) bool {
	switch {
	case bytes.HasPrefix(data, []byte{0x7f, 'E', 'L', 'F'}):
		return true
	case bytes.HasPrefix(data, []byte("MZ")) && looksLikeDOSExecutable(data):
		return true
	case bytes.HasPrefix(data, []byte("#!")):
		return true
	case bytes.HasPrefix(data, []byte{0xca, 0xfe, 0xba, 0xbe}),
		bytes.HasPrefix(data, []byte{0xfe, 0xed, 0xfa, 0xce}),
		bytes.HasPrefix(data, []byte{0xfe, 0xed, 0xfa, 0xcf}),
		bytes.HasPrefix(data, []byte{0xce, 0xfa, 0xed, 0xfe}),
		bytes.HasPrefix(data, []byte{0xcf, 0xfa, 0xed, 0xfe}):
		return true
	}
	return false
}

// looksLikeDOSExecutable tells a Windows or DOS program from a text file that
// merely starts with the letters "MZ": a real one carries the PE signature at
// the offset its header names, or the stub's usual warning text.
func looksLikeDOSExecutable(data []byte) bool {
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	if bytes.Contains(head, []byte("This program")) {
		return true
	}
	if len(data) >= 0x40 {
		off := int(data[0x3c]) | int(data[0x3d])<<8 | int(data[0x3e])<<16 | int(data[0x3f])<<24
		if off >= 0 && off+4 <= len(data) && bytes.Equal(data[off:off+4], []byte("PE\x00\x00")) {
			return true
		}
	}
	return false
}

func openZip(data []byte) (*zip.Reader, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(zr.File) > maxZipEntries {
		return nil, errPageFileType
	}
	return zr, nil
}

// requireZipEntry accepts an Office Open XML package: a zip with the content
// types part and at least one part under dir.
func requireZipEntry(data []byte, dir string) error {
	zr, err := openZip(data)
	if err != nil {
		return errPageFileType
	}
	var types, inDir bool
	for _, f := range zr.File {
		switch {
		case f.Name == "[Content_Types].xml":
			types = true
		case strings.HasPrefix(f.Name, dir):
			inDir = true
		}
	}
	if !types || !inDir {
		return errPageFileType
	}
	return nil
}

// requireODT accepts an OpenDocument text file: a zip whose "mimetype" entry
// names the type.
func requireODT(data []byte) error {
	zr, err := openZip(data)
	if err != nil {
		return errPageFileType
	}
	for _, f := range zr.File {
		if f.Name != "mimetype" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return errPageFileType
		}
		defer func() { _ = rc.Close() }()
		got, err := io.ReadAll(io.LimitReader(rc, 128))
		if err != nil || string(got) != mimeOdt {
			return errPageFileType
		}
		return nil
	}
	return errPageFileType
}
