package media

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// zipOf builds an in-memory zip of the named entries.
func zipOf(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// Deterministic order is not needed: every check looks at names only.
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPageFileMimeForName(t *testing.T) {
	tests := []struct {
		name string
		want string
		ok   bool
	}{
		{"map.pdf", mimePDF, true},
		{"MAP.PDF", mimePDF, true},
		{"photo.JPEG", "image/jpeg", true},
		{"photo.jpg", "image/jpeg", true},
		{"a.png", "image/png", true},
		{"a.webp", "image/webp", true},
		{"a.gif", "image/gif", true},
		{"notes.txt", mimeText, true},
		{"notes.md", mimeMD, true},
		{"sheet.docx", mimeDocx, true},
		{"sheet.xlsx", mimeXlsx, true},
		{"sheet.odt", mimeOdt, true},
		{"song.mp3", "audio/mpeg", true},
		{"song.ogg", "audio/ogg", true},
		{"pack.zip", mimeZip, true},
		{`C:\Users\me\handout.pdf`, mimePDF, true}, // a browser may send a Windows path
		{"page.html", "", false},
		{"page.htm", "", false},
		{"logo.svg", "", false},
		{"run.exe", "", false},
		{"run.sh", "", false},
		{"run.js", "", false},
		{"app.jar", "", false},
		{"a.pdf.exe", "", false},
		{"noextension", "", false},
		{"", "", false},
		{"evil.html.png", "image/png", true}, // type comes from the last extension, then the content must agree
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PageFileMimeForName(tt.name)
			if got != tt.want || ok != tt.ok {
				t.Errorf("PageFileMimeForName(%q) = %q, %v; want %q, %v", tt.name, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestCleanPageFileName(t *testing.T) {
	long := strings.Repeat("é", 300) + ".pdf"
	tests := []struct {
		name, in, want string
	}{
		{"plain", "handout.pdf", "handout.pdf"},
		{"unix path", "/etc/passwd.txt", "passwd.txt"},
		{"windows path", `C:\x\y.txt`, "y.txt"},
		{"control chars", "a\r\nb\x00c.txt", "abc.txt"},
		{"spaces trimmed", "  a.txt  ", "a.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanPageFileName(tt.in); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
	got := CleanPageFileName(long)
	if len(got) > 255 || !strings.HasSuffix(got, ".pdf") {
		t.Errorf("long name not bounded with its extension kept: len %d, %q", len(got), got[len(got)-8:])
	}
}

func TestValidatePageFileContent(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}
	docx := zipOf(t, map[string]string{"[Content_Types].xml": "<x/>", "word/document.xml": "<w/>"})
	xlsx := zipOf(t, map[string]string{"[Content_Types].xml": "<x/>", "xl/workbook.xml": "<w/>"})
	odt := zipOf(t, map[string]string{"mimetype": mimeOdt, "content.xml": "<c/>"})
	plainZip := zipOf(t, map[string]string{"a.txt": "hi"})

	// An ELF header dressed as every allowed type, and the other program
	// headers, must all be refused.
	elf := append([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0}, make([]byte, 64)...)
	pe := make([]byte, 0x90)
	copy(pe, "MZ")
	pe[0x3c] = 0x80
	copy(pe[0x80:], "PE\x00\x00")
	dos := append([]byte("MZ"), []byte(strings.Repeat(" ", 40)+"This program cannot be run in DOS mode.")...)
	macho := []byte{0xcf, 0xfa, 0xed, 0xfe, 7, 0, 0, 1}
	class := []byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 52}
	shebang := []byte("#!/bin/sh\nrm -rf /\n")

	tests := []struct {
		name    string
		data    []byte
		mime    string
		wantErr bool
	}{
		{"pdf", []byte("%PDF-1.7\n..."), mimePDF, false},
		{"pdf with leading junk inside 1k", append([]byte("junk\n"), []byte("%PDF-1.4")...), mimePDF, false},
		{"not a pdf", []byte("<html>hello</html>"), mimePDF, true},
		{"text", []byte("hello é world"), mimeText, false},
		{"markdown", []byte("# Title\n\nbody"), mimeMD, false},
		{"text with NUL", []byte("hel\x00lo"), mimeText, true},
		{"text invalid utf-8", []byte{0xff, 0xfe, 0xfd, 'a'}, mimeText, true},
		{"png", png, "image/png", false},
		{"html as png", []byte("<html><script>alert(1)</script></html>"), "image/png", true},
		{"svg as png", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`), "image/png", true},
		{"png bytes as jpeg", png, "image/jpeg", true},
		{"docx", docx, mimeDocx, false},
		{"docx that is a plain zip", plainZip, mimeDocx, true},
		{"docx that is not a zip", []byte("PK nope"), mimeDocx, true},
		{"xlsx", xlsx, mimeXlsx, false},
		{"xlsx that is a docx", docx, mimeXlsx, true},
		{"odt", odt, mimeOdt, false},
		{"odt without its mimetype", plainZip, mimeOdt, true},
		{"zip", plainZip, mimeZip, false},
		{"zip that is text", []byte("not a zip at all"), mimeZip, true},
		{"mp3", []byte("ID3\x04\x00\x00\x00\x00\x00\x00"), "audio/mpeg", false},
		{"ogg", []byte("OggS\x00\x02\x00\x00"), "audio/ogg", false},
		{"ogg that is text", []byte("hello world"), "audio/ogg", true},
		{"empty", nil, mimeText, true},
		{"elf as text", elf, mimeText, true},
		{"elf as pdf", elf, mimePDF, true},
		{"elf as zip", elf, mimeZip, true},
		{"elf as png", elf, "image/png", true},
		{"pe by header", pe, mimeText, true},
		{"dos stub", dos, mimeText, true},
		{"mach-o", macho, mimeZip, true},
		{"java class", class, mimeZip, true},
		{"shebang script", shebang, mimeText, true},
		{"text that merely starts MZ", []byte("MZ stands for Mara Zane"), mimeText, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePageFileContent(tt.data, tt.mime)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePageFileContent(%s) error = %v, wantErr %v", tt.mime, err, tt.wantErr)
			}
		})
	}
}

func TestPageFileTypesAreStoredWithAnExtension(t *testing.T) {
	// A mime with no extension would store a file with none, and the general
	// upload path must not accept any page-file-only type.
	for mime := range PageFileMimeTypes {
		if MimeToExtension[mime] == "" {
			t.Errorf("%s has no stored extension", mime)
		}
	}
	for _, mime := range []string{mimePDF, mimeText, mimeMD, mimeDocx, mimeXlsx, mimeOdt, mimeZip} {
		if AllowedMimeTypes[mime] {
			t.Errorf("%s must be page-file only, not accepted by the general upload path", mime)
		}
	}
	for _, bad := range []string{"text/html", "image/svg+xml", "application/javascript", "application/x-msdownload"} {
		if PageFileMimeTypes[bad] {
			t.Errorf("%s must never be a page file type", bad)
		}
	}
}
