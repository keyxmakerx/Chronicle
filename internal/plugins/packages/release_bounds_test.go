package packages

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// zeroReader yields n zero bytes.
func zeroReader(n int64) io.Reader { return io.LimitReader(zeros{}, n) }

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

// TestExtractZip_Bounds pins that a release can't fill the disk: too many
// entries are refused up front, and an entry that inflates past the budget is
// stopped by the bytes actually written, even when its header claims it is
// tiny.
func TestExtractZip_Bounds(t *testing.T) {
	dir := t.TempDir()

	t.Run("too many entries", func(t *testing.T) {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for i := 0; i <= maxPackageEntries; i++ {
			if _, err := zw.Create("f/" + strconv.Itoa(i)); err != nil {
				t.Fatal(err)
			}
		}
		_ = zw.Close()
		zp := filepath.Join(dir, "many.zip")
		_ = os.WriteFile(zp, buf.Bytes(), 0o644)
		if err := extractZip(zp, t.TempDir()); err == nil {
			t.Fatal("a release with too many entries was extracted")
		}
	})

	t.Run("lying size header", func(t *testing.T) {
		size := int64(maxPackageExtractedBytes) + 1
		var comp bytes.Buffer
		fw, _ := flate.NewWriter(&comp, flate.BestSpeed)
		crc := crc32.NewIEEE()
		if _, err := io.Copy(io.MultiWriter(fw, crc), zeroReader(size)); err != nil {
			t.Fatal(err)
		}
		_ = fw.Close()
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, err := zw.CreateRaw(&zip.FileHeader{
			Name: "bomb.json", Method: zip.Deflate, CRC32: crc.Sum32(),
			CompressedSize64: uint64(comp.Len()), UncompressedSize64: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(comp.Bytes())
		_ = zw.Close()
		zp := filepath.Join(dir, "bomb.zip")
		_ = os.WriteFile(zp, buf.Bytes(), 0o644)
		dest := t.TempDir()
		if err := extractZip(zp, dest); err == nil {
			t.Fatal("an entry past the unpacked budget was extracted")
		}
		if fi, err := os.Stat(filepath.Join(dest, "bomb.json")); err == nil && fi.Size() > maxPackageExtractedBytes+1 {
			t.Fatalf("wrote %d bytes, past the budget", fi.Size())
		}
	})
}

// TestDownloadAsset_Bounded pins that a download stops at the cap and leaves
// nothing behind.
func TestDownloadAsset_Bounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.Copy(w, zeroReader(maxPackageDownloadBytes+1))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "big.zip")
	c := &GitHubClient{httpClient: srv.Client()}
	if _, err := c.DownloadAsset(context.Background(), srv.URL, dest); err == nil {
		t.Fatal("a download past the cap succeeded")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("an over-size download was left on disk")
	}
}
