package campaigns

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// TestRemapMediaReferences: every reference to a restored file, wherever it
// sits, points at the new copy; ids with no restored file, and the manifest
// itself, are left as they were.
func TestRemapMediaReferences(t *testing.T) {
	const (
		portrait = "aaaaaaaa-0000-0000-0000-000000000001"
		inline   = "aaaaaaaa-0000-0000-0000-000000000002"
		mapBg    = "aaaaaaaa-0000-0000-0000-000000000003"
		token    = "aaaaaaaa-0000-0000-0000-000000000004"
		absent   = "aaaaaaaa-0000-0000-0000-000000000009"
	)
	str := func(s string) *string { return &s }
	data := &CampaignExport{
		Entities: []ExportEntity{{
			Name:           "Aria",
			ImagePath:      str("2026/03/" + portrait + ".png"),
			CoverImagePath: str(absent),
			Entry:          str(`{"type":"doc","content":[{"type":"image","attrs":{"src":"/media/` + inline + `"}}]}`),
			EntryHTML:      str(`<img src="/media/` + inline + `"><img src="/media/` + absent + `">`),
		}},
		Maps: []ExportMap{{
			Name: "Coast", ImageID: str(mapBg),
			Tokens: []ExportToken{{Name: "Aria", ImagePath: str(token)}},
		}},
		Media: []ExportMediaFile{{OriginalID: portrait}, {OriginalID: inline}},
	}
	ids := map[string]string{
		portrait: "bbbbbbbb-0000-0000-0000-000000000001",
		inline:   "bbbbbbbb-0000-0000-0000-000000000002",
		mapBg:    "bbbbbbbb-0000-0000-0000-000000000003",
		token:    "bbbbbbbb-0000-0000-0000-000000000004",
		"a":      "never-applied", // not a media id: must not rewrite every "a"
	}
	if err := remapMediaReferences(data, ids); err != nil {
		t.Fatalf("remap: %v", err)
	}

	e := data.Entities[0]
	if *e.ImagePath != "2026/03/"+ids[portrait]+".png" {
		t.Errorf("portrait = %q", *e.ImagePath)
	}
	if *e.CoverImagePath != absent {
		t.Errorf("cover with no restored file was rewritten to %q", *e.CoverImagePath)
	}
	if !strings.Contains(*e.Entry, "/media/"+ids[inline]) || strings.Contains(*e.Entry, inline) {
		t.Errorf("editor document not remapped: %s", *e.Entry)
	}
	if want := `<img src="/media/` + ids[inline] + `"><img src="/media/` + absent + `">`; *e.EntryHTML != want {
		t.Errorf("entry HTML = %s, want %s", *e.EntryHTML, want)
	}
	if *data.Maps[0].ImageID != ids[mapBg] || *data.Maps[0].Tokens[0].ImagePath != ids[token] {
		t.Errorf("map = %q, token = %q", *data.Maps[0].ImageID, *data.Maps[0].Tokens[0].ImagePath)
	}
	if e.Name != "Aria" || data.Maps[0].Name != "Coast" {
		t.Errorf("a non-id string was rewritten: %q, %q", e.Name, data.Maps[0].Name)
	}
	if data.Media[0].OriginalID != portrait || data.Media[1].OriginalID != inline {
		t.Errorf("manifest was rewritten: %+v", data.Media)
	}
}

// TestImportMediaBundle_IgnoresUnsafeEntries: only flat media/<name> entries
// are offered to the importer; anything that could name a path is ignored.
func TestImportMediaBundle_IgnoresUnsafeEntries(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"media/ok.png", "media/../evil.png", "media/sub/deep.png", "other/x.png", "media/"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		_, _ = w.Write([]byte("x"))
	}
	_ = zw.Close()
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	b, err := NewImportMediaBundle(r)
	if err != nil {
		t.Fatalf("NewImportMediaBundle: %v", err)
	}
	if b.Len() != 1 {
		t.Errorf("bundle holds %d entries, want only media/ok.png", b.Len())
	}
	if _, ok, _ := b.Read(ExportMediaFile{Filename: "ok.png"}); !ok {
		t.Error("media/ok.png not found by its Filename")
	}
	if _, ok, _ := b.Read(ExportMediaFile{Filename: "../evil.png"}); ok {
		t.Error("an unsafe entry was readable")
	}
	var none *ImportMediaBundle
	if _, ok, err := none.Read(ExportMediaFile{OriginalID: "x"}); ok || err != nil {
		t.Error("a nil bundle (JSON upload) must read as 'no file'")
	}
}

// TestMediaZipName: stored names carry date folders; the zip entry is the
// bare file name.
func TestMediaZipName(t *testing.T) {
	for in, want := range map[string]string{
		"2026/03/abc.png": "abc.png",
		"abc.png":         "abc.png",
		`2026\03\abc.png`: "abc.png",
	} {
		if got := MediaZipName(in); got != want {
			t.Errorf("MediaZipName(%q) = %q, want %q", in, got, want)
		}
	}
}

// zipOf builds an in-memory zip of the given entries.
func zipOf(t *testing.T, entries map[string][]byte) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		_, _ = w.Write(body)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	return r
}

// TestImportMediaBundle_Bounds pins what a crafted import ZIP can cost: no
// more media entries than an import may carry, no file inflated past the
// upload limit, and an id lookup that is one map read, not a scan of every
// entry per manifest row.
func TestImportMediaBundle_Bounds(t *testing.T) {
	many := make(map[string][]byte, maxImportMediaFiles+1)
	for i := 0; i <= maxImportMediaFiles; i++ {
		many[fmt.Sprintf("media/%d.png", i)] = nil
	}
	if _, err := NewImportMediaBundle(zipOf(t, many)); err == nil {
		t.Fatalf("a zip with %d media entries was accepted", maxImportMediaFiles+1)
	}

	const id = "33333333-3333-3333-3333-333333333333"
	b, err := NewImportMediaBundle(zipOf(t, map[string][]byte{
		"media/" + id + ".png": bytes.Repeat([]byte("a"), 4096),
	}))
	if err != nil {
		t.Fatalf("NewImportMediaBundle: %v", err)
	}
	if _, ok, err := b.ReadAtMost(ExportMediaFile{OriginalID: id}, 1024); !ok || err == nil {
		t.Fatalf("a file over the limit: ok=%v err=%v, want found and refused", ok, err)
	}
	data, ok, err := b.ReadAtMost(ExportMediaFile{OriginalID: id}, 8192)
	if !ok || err != nil || len(data) != 4096 {
		t.Fatalf("a file under the limit: ok=%v err=%v len=%d", ok, err, len(data))
	}
}

// TestValidate_RefusesTooManyMediaRows pins the manifest side of the cap.
func TestValidate_RefusesTooManyMediaRows(t *testing.T) {
	svc := NewExportImportService(importStubCampaignSvc{})
	data := &CampaignExport{Format: ExportFormat, Campaign: ExportCampaignMeta{Name: "x"}}
	data.Media = make([]ExportMediaFile, maxImportMediaFiles)
	if err := svc.Validate(data); err != nil {
		t.Fatalf("a manifest at the cap was refused: %v", err)
	}
	data.Media = append(data.Media, ExportMediaFile{})
	if err := svc.Validate(data); err == nil {
		t.Fatal("a manifest over the cap was accepted")
	}
}
