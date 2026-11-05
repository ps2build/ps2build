package update

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, entries map[string]struct {
	mode os.FileMode
	body string
}) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for name, e := range entries {
		hdr := &zip.FileHeader{Name: name, Method: zip.Store}
		hdr.SetMode(e.mode)
		fw, err := w.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write([]byte(e.body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return path
}

func TestExtractZipSymlink(t *testing.T) {
	type entry = struct {
		mode os.FileMode
		body string
	}
	zipPath := writeZip(t, map[string]entry{
		"tc/bin/clang-24": {0o755, "binary"},
		"tc/bin/clang":    {os.ModeSymlink | 0o777, "clang-24"},
	})
	dest := t.TempDir()
	if err := extractZip(zipPath, dest); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got, err := os.Readlink(filepath.Join(dest, "tc", "bin", "clang"))
	if err != nil || got != "clang-24" {
		t.Fatalf("Readlink = %q, %v; want clang-24", got, err)
	}
}

func TestExtractZipSymlinkEscape(t *testing.T) {
	type entry = struct {
		mode os.FileMode
		body string
	}
	zipPath := writeZip(t, map[string]entry{
		"tc/bin/evil": {os.ModeSymlink | 0o777, "../../../outside"},
	})
	if err := extractZip(zipPath, t.TempDir()); err == nil {
		t.Fatal("expected an error for a symlink leaving the destination")
	}
}
