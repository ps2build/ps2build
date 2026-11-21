package update

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// release builds a layer zip holding the given files (paths relative to the sdk root) and serves it.
type release struct {
	srv   *httptest.Server
	files map[string]string
	code  int
	raw   []byte // served instead of a zip when set
}

func serve(t *testing.T, files map[string]string) *release {
	t.Helper()
	r := &release{files: files, code: http.StatusOK}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if r.code != http.StatusOK {
			http.Error(w, "nope", r.code)
			return
		}
		if r.raw != nil {
			w.Write(r.raw)
			return
		}
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for name, body := range r.files {
			f, err := zw.Create(name)
			if err != nil {
				t.Error(err)
				return
			}
			f.Write([]byte(body))
		}
		zw.Close()
		w.Write(buf.Bytes())
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func apply(t *testing.T, root, layer string, files map[string]string) error {
	t.Helper()
	return applyLayer(root, layer, serve(t, files).srv.URL)
}

func mustApply(t *testing.T, root, layer string, files map[string]string) {
	t.Helper()
	if err := apply(t, root, layer, files); err != nil {
		t.Fatalf("applyLayer(%s): %v", layer, err)
	}
}

func exists(root, rel string) bool {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func put(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyLayer_RemovesAFileTheNewReleaseDropped(t *testing.T) {
	root := t.TempDir()
	mustApply(t, root, "sdk-core", map[string]string{"packages/core/a/a.h": "a1", "packages/core/a/old.h": "old"})
	mustApply(t, root, "sdk-core", map[string]string{"packages/core/a/a.h": "a2"})

	if exists(root, "packages/core/a/old.h") {
		t.Error("packages/core/a/old.h survived an update whose release dropped it")
	}
	if got := read(t, root, "packages/core/a/a.h"); got != "a2" {
		t.Errorf("a.h = %q, want the new release's content", got)
	}
}

func TestApplyLayer_NeverTouchesAFileItDidNotInstall(t *testing.T) {
	root := t.TempDir()
	mustApply(t, root, "sdk-world", map[string]string{"packages/world/lib/x.h": "x", "packages/world/lib/y.h": "y"})
	put(t, root, "packages/world/mine/mine.h", "user package")
	put(t, root, "packages/world/lib/notes.txt", "user file beside installed ones")

	mustApply(t, root, "sdk-world", map[string]string{"packages/world/lib/x.h": "x"})

	if exists(root, "packages/world/lib/y.h") {
		t.Error("the dropped y.h should be gone")
	}
	if got := read(t, root, "packages/world/mine/mine.h"); got != "user package" {
		t.Errorf("a user's package under packages/world changed: %q", got)
	}
	if got := read(t, root, "packages/world/lib/notes.txt"); got != "user file beside installed ones" {
		t.Errorf("a user's file next to installed ones changed: %q", got)
	}
}

func TestApplyLayer_KeepsAnEditedFile(t *testing.T) {
	root := t.TempDir()
	mustApply(t, root, "toolchain", map[string]string{"toolchain/etc/conf": "shipped", "toolchain/bin/tool": "t"})
	put(t, root, "toolchain/etc/conf", "edited by the user")

	mustApply(t, root, "toolchain", map[string]string{"toolchain/bin/tool": "t2"})

	if got := read(t, root, "toolchain/etc/conf"); got != "edited by the user" {
		t.Errorf("an edited file was deleted or reset: %q", got)
	}
	rec, err := loadOwned(root, "toolchain")
	if err != nil || rec == nil {
		t.Fatal(err)
	}
	for _, f := range rec.Files {
		if f.Path == "toolchain/etc/conf" {
			t.Error("the new record lists a file this release does not ship")
		}
	}
}

func TestApplyLayer_KeepsAFileAnotherComponentNowOwns(t *testing.T) {
	root := t.TempDir()
	mustApply(t, root, "sdk-core", map[string]string{"packages/core/shared.h": "same", "packages/core/core.h": "c"})
	mustApply(t, root, "sdk-world", map[string]string{"packages/core/shared.h": "same", "packages/world/w.h": "w"})

	mustApply(t, root, "sdk-core", map[string]string{"packages/core/core.h": "c"})

	if got := read(t, root, "packages/core/shared.h"); got != "same" {
		t.Errorf("a file that sdk-world installs was deleted by sdk-core's update: %q", got)
	}
}

func TestApplyLayer_FirstUpdateDeletesNothing(t *testing.T) {
	root := t.TempDir()
	put(t, root, "packages/core/leftover.h", "from an install made before records existed")
	put(t, root, "packages/core/a.h", "old a")

	mustApply(t, root, "sdk-core", map[string]string{"packages/core/a.h": "new a"})

	if got := read(t, root, "packages/core/leftover.h"); got == "" {
		t.Error("a file was removed by an update that had no previous record")
	}
	rec, err := loadOwned(root, "sdk-core")
	if err != nil || rec == nil || len(rec.Files) != 1 || rec.Files[0].Path != "packages/core/a.h" {
		t.Errorf("record = %+v, %v, want exactly the extracted file", rec, err)
	}
}

func TestApplyLayer_FailedDownloadLeavesInstallAndRecordAlone(t *testing.T) {
	root := t.TempDir()
	mustApply(t, root, "sdk-core", map[string]string{"packages/core/a.h": "a", "packages/core/b.h": "b"})
	before, err := os.ReadFile(ownedRecordPath(root, "sdk-core"))
	if err != nil {
		t.Fatal(err)
	}

	broken := serve(t, nil)
	broken.code = http.StatusInternalServerError
	if err := applyLayer(root, "sdk-core", broken.srv.URL); err == nil {
		t.Fatal("applyLayer succeeded against a failing download")
	}
	corrupt := serve(t, nil)
	corrupt.raw = []byte("this is not a zip")
	if err := applyLayer(root, "sdk-core", corrupt.srv.URL); err == nil {
		t.Fatal("applyLayer succeeded on a corrupt archive")
	}

	for _, rel := range []string{"packages/core/a.h", "packages/core/b.h"} {
		if !exists(root, rel) {
			t.Errorf("%s was removed by a failed update", rel)
		}
	}
	after, err := os.ReadFile(ownedRecordPath(root, "sdk-core"))
	if err != nil || string(after) != string(before) {
		t.Errorf("the record changed after a failed update (err %v):\n%s\n---\n%s", err, before, after)
	}
}

func TestApplyLayer_PrunesDirectoriesTheRemovalEmptied(t *testing.T) {
	root := t.TempDir()
	mustApply(t, root, "toolchain", map[string]string{
		"toolchain/ee/lib/deep/er/gone.a": "x", "toolchain/ee/lib/keep.a": "k", "extra/only.txt": "o",
	})
	put(t, root, "toolchain/ee/lib/deep/other.a", "user")

	mustApply(t, root, "toolchain", map[string]string{"toolchain/ee/lib/keep.a": "k"})

	if exists(root, "toolchain/ee/lib/deep/er") {
		t.Error("the emptied directory toolchain/ee/lib/deep/er was left behind")
	}
	if !exists(root, "toolchain/ee/lib/deep/other.a") {
		t.Error("a directory that still holds a user's file was removed")
	}
	if exists(root, "extra/only.txt") {
		t.Error("extra/only.txt should have been removed")
	}
	if !exists(root, "extra") {
		t.Error("a top-level directory was removed; only directories below the top level may be")
	}
	if !exists(root, ".") {
		t.Error("the sdk root is gone")
	}
}

func TestApplyLayer_RecordFormat(t *testing.T) {
	root := t.TempDir()
	mustApply(t, root, "sdk-core", map[string]string{"packages/core/b/two.h": "2", "packages/core/a.h": "1"})

	data, err := os.ReadFile(filepath.Join(root, ".ps2build", "owned", "sdk-core.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rec struct {
		Files []struct{ Path, SHA256 string }
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	// sha256 of "1" and "2"
	want := []struct{ path, sum string }{
		{"packages/core/a.h", "6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b"},
		{"packages/core/b/two.h", "d4735e3a265e16eee03f59718b9b5d03019c07d8b6c51f90da3a666eec13ab35"},
	}
	if len(rec.Files) != len(want) {
		t.Fatalf("record lists %d files, want %d: %s", len(rec.Files), len(want), data)
	}
	for i, w := range want {
		if rec.Files[i].Path != w.path || rec.Files[i].SHA256 != w.sum {
			t.Errorf("file %d = %+v, want %s %s", i, rec.Files[i], w.path, w.sum)
		}
	}
	leftovers, _ := filepath.Glob(filepath.Join(root, ".ps2build", "owned", ".*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left in the record directory: %v", leftovers)
	}
}

func TestApplyLayer_UnreadableOrHostileRecordsRemoveNothing(t *testing.T) {
	root := t.TempDir()
	mustApply(t, root, "sdk-core", map[string]string{"packages/core/a.h": "a", "packages/core/old.h": "old"})
	if err := os.WriteFile(ownedRecordPath(root, "sdk-core"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustApply(t, root, "sdk-core", map[string]string{"packages/core/a.h": "a"})
	if !exists(root, "packages/core/old.h") {
		t.Error("an unreadable record led to a deletion")
	}

	// a record naming paths outside the sdk, or the bookkeeping directory, never deletes them
	outside := filepath.Join(filepath.Dir(root), "outside-"+filepath.Base(root))
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(outside) })
	sum, _ := fileSHA256(outside)
	hostile := ownership{Layer: "sdk-core", Files: []ownedFile{
		{Path: "../" + filepath.Base(outside), SHA256: sum},
		{Path: ".ps2build/owned/sdk-core.json", SHA256: "x"},
		{Path: "/etc/hostname", SHA256: "x"},
	}}
	data, _ := json.Marshal(hostile)
	if err := os.WriteFile(ownedRecordPath(root, "sdk-core"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	mustApply(t, root, "sdk-core", map[string]string{"packages/core/a.h": "a"})
	if _, err := os.Stat(outside); err != nil {
		t.Error("a path outside the sdk root was deleted")
	}
	if !exists(root, ".ps2build/owned/sdk-core.json") {
		t.Error("the record itself was deleted")
	}
}

func TestApplySDKTier_RecordsEachLayerUnderItsOwnName(t *testing.T) {
	root := t.TempDir()
	gcc := serve(t, map[string]string{"packages/core/g.h": "g"})
	llvm := serve(t, map[string]string{"packages-llvm/core/l.h": "l"})
	rel := releaseInfo{TagName: "v1", Assets: []releaseAsset{
		{Name: "sdk-core-v1-gcc.zip", BrowserDownloadURL: gcc.srv.URL},
		{Name: "sdk-core-v1-llvm.zip", BrowserDownloadURL: llvm.srv.URL},
	}}
	c := component{Name: "sdk-core", AssetPrefix: "sdk-core-", AssetSuffix: ".zip", Tier: "core"}
	if err := applySDKTier(root, c, rel); err != nil {
		t.Fatal(err)
	}
	for layer, path := range map[string]string{"sdk-core": "packages/core/g.h", "sdk-core-llvm": "packages-llvm/core/l.h"} {
		rec, err := loadOwned(root, layer)
		if err != nil || rec == nil || len(rec.Files) != 1 || rec.Files[0].Path != path {
			t.Errorf("record %s = %+v, %v, want %s", layer, rec, err, path)
		}
	}

	// dropping a file from only the llvm half removes it from there and leaves the gcc half alone
	llvm.files = map[string]string{"packages-llvm/core/new.h": "n"}
	if err := applySDKTier(root, c, rel); err != nil {
		t.Fatal(err)
	}
	if exists(root, "packages-llvm/core/l.h") || !exists(root, "packages-llvm/core/new.h") || !exists(root, "packages/core/g.h") {
		t.Error("the llvm layer's update did not stay within the llvm layer")
	}
	if strings.Contains(read(t, root, ".ps2build/owned/sdk-core.json"), "packages-llvm") {
		t.Error("the gcc record lists llvm files")
	}
}
