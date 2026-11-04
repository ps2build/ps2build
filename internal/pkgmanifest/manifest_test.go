package pkgmanifest

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, dir, yaml string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `
name: wavekit
origin: user
artifacts:
  - kind: library
    target: [ee]
    include_dirs: [include]
    lib: lib/libwavekit.a
    cxx: true
`)

	m, err := Load(filepath.Join(dir, "package.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if m.Name != "wavekit" {
		t.Errorf("Name = %q, want %q", m.Name, "wavekit")
	}
	if len(m.Artifacts) != 1 {
		t.Fatalf("len(Artifacts) = %d, want 1", len(m.Artifacts))
	}

	a, ok := m.LibraryArtifact("ee")
	if !ok {
		t.Fatal("LibraryArtifact(\"ee\") ok = false, want true")
	}
	if !a.CXX {
		t.Error("Artifact.CXX = false, want true")
	}

	if _, ok := m.LibraryArtifact("iop"); ok {
		t.Error("LibraryArtifact(\"iop\") ok = true, want false - artifact only declares target: [ee]")
	}
}

func TestLoad_MissingName(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `
artifacts:
  - kind: library
    target: [ee]
`)
	if _, err := Load(filepath.Join(dir, "package.yaml")); err == nil {
		t.Error("Load() error = nil, want an error for a manifest missing name:")
	}
}

func TestAbsIncludeDirsAndAbsLib(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `
name: wavekit
artifacts:
  - kind: library
    target: [ee]
    include_dirs: [include]
    lib: lib/libwavekit.a
`)

	m, err := Load(filepath.Join(dir, "package.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := m.LibraryArtifact("ee")

	wantInc := filepath.ToSlash(filepath.Join(dir, "include"))
	gotInc := m.AbsIncludeDirs(a)
	if len(gotInc) != 1 || gotInc[0] != wantInc {
		t.Errorf("AbsIncludeDirs() = %v, want [%q]", gotInc, wantInc)
	}

	wantLib := filepath.ToSlash(filepath.Join(dir, "lib/libwavekit.a"))
	if got := m.AbsLib(a); got != wantLib {
		t.Errorf("AbsLib() = %q, want %q", got, wantLib)
	}
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, filepath.Join(root, "world", "wavekit"), `
name: wavekit
artifacts:
  - kind: library
    target: [ee]
    include_dirs: [include]
    lib: lib/libwavekit.a
`)
	writeManifest(t, filepath.Join(root, "core", "draw"), `
name: draw
artifacts:
  - kind: library
    target: [ee]
    lib: lib/libdraw.a
`)

	t.Run("finds a package in world/", func(t *testing.T) {
		m, ok, err := Find(root, "wavekit")
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if m.Name != "wavekit" {
			t.Errorf("Name = %q, want %q", m.Name, "wavekit")
		}
	})

	t.Run("finds a package in core/", func(t *testing.T) {
		m, ok, err := Find(root, "draw")
		if err != nil {
			t.Fatal(err)
		}
		if !ok || m.Name != "draw" {
			t.Errorf("Find(draw) = %+v, %v, want a match named draw", m, ok)
		}
	})

	t.Run("unknown name returns ok=false, not an error", func(t *testing.T) {
		_, ok, err := Find(root, "nonexistent")
		if err != nil {
			t.Fatalf("Find() error = %v, want nil - a miss must fall through to a bare -l<name>, not fail the build", err)
		}
		if ok {
			t.Error("ok = true, want false")
		}
	})
}
