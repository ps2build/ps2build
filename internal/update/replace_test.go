package update

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyLayer_MergesTierWithoutWipingSiblingTier(t *testing.T) {
	ps2dev := t.TempDir()

	// Pre-existing content from an earlier, separate component apply
	// (sdk-core) - must survive applying sdk-world's own zip.
	corePath := filepath.Join(ps2dev, "packages", "core", "loadcore", "include", "loadcore.h")
	if err := os.MkdirAll(filepath.Dir(corePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corePath, []byte("// real loadcore header"), 0o644); err != nil {
		t.Fatal(err)
	}

	zipData := buildZip(t, map[string]string{
		"packages/world/gskit/include/gsKit.h": "// real gskit header",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipData)
	}))
	defer server.Close()

	if err := applyLayer(ps2dev, "sdk-world", server.URL); err != nil {
		t.Fatalf("applyLayer failed: %v", err)
	}

	// sdk-core's own content must survive untouched.
	if data, err := os.ReadFile(corePath); err != nil {
		t.Fatalf("packages/core was wiped by applying sdk-world's zip: %v", err)
	} else if string(data) != "// real loadcore header" {
		t.Fatalf("loadcore.h content changed unexpectedly: %q", data)
	}

	// sdk-world's own content must have landed at packages/world, not
	// a bare top-level "world" sibling of packages/.
	worldPath := filepath.Join(ps2dev, "packages", "world", "gskit", "include", "gsKit.h")
	if _, err := os.Stat(worldPath); err != nil {
		t.Fatalf("expected %s to exist after applyLayer: %v", worldPath, err)
	}
	if _, err := os.Stat(filepath.Join(ps2dev, "world")); err == nil {
		t.Fatal("a bare top-level world/ directory was created - the exact bug this test guards against")
	}

	c := component{Name: "sdk-world", Tier: "world", Test: filepath.Join("packages", "world")}
	if !componentInstalled(ps2dev, c) {
		t.Fatal("componentInstalled reports sdk-world missing after a real applyLayer - update would redownload every single run")
	}
}

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestRemoveAsideDeletesOldAndPIDSuffixedLeftovers(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "ps2build.exe")
	for _, name := range []string{"ps2build.exe.old", "ps2build.exe.old.1234", "ps2build.exe.old.99"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	removeAside(exe)
	left, _ := filepath.Glob(filepath.Join(dir, "ps2build.exe.old*"))
	if len(left) != 0 {
		t.Fatalf("leftovers remain: %v", left)
	}
}

func TestApplyLayer_OggZipLandsBesideSDKWorld(t *testing.T) {
	ps2dev := t.TempDir()

	gskit := filepath.Join(ps2dev, "packages", "world", "gskit", "include", "gsKit.h")
	if err := os.MkdirAll(filepath.Dir(gskit), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gskit, []byte("// gskit"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The layout ps2build/ogg's release zips use: packages/world/ogg at the zip root.
	zipData := buildZip(t, map[string]string{
		"packages/world/ogg/package.yaml": "name: ogg\n",
		"packages/world/ogg/lib/libogg.a": "!<arch>\n",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipData)
	}))
	defer server.Close()

	if err := applyLayer(ps2dev, "ogg", server.URL); err != nil {
		t.Fatalf("applyLayer failed: %v", err)
	}
	if _, err := os.Stat(gskit); err != nil {
		t.Fatalf("sdk-world content was wiped by applying ogg's zip: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ps2dev, "packages", "world", "ogg", "lib", "libogg.a")); err != nil {
		t.Fatalf("ogg did not land at packages/world/ogg: %v", err)
	}

	c, _ := findComponent("ogg")
	if !componentInstalled(ps2dev, c) {
		t.Fatal("componentInstalled reports ogg missing after applyLayer - a bare update would never keep it current")
	}
}
