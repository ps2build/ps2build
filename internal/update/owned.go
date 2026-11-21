package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// An ownership record lists the files a layer installed, so a later update can tell the files a release dropped
// from files that were put there by someone else. Records live in <sdk>/.ps2build/owned/<layer>.json.
const ownedDir = ".ps2build/owned"

type ownedFile struct {
	Path   string `json:"path"` // relative to the sdk root, forward slashes
	SHA256 string `json:"sha256"`
}

type ownership struct {
	Layer string      `json:"layer"`
	Files []ownedFile `json:"files"`
}

func ownedRecordPath(sdkRoot, layer string) string {
	return filepath.Join(sdkRoot, filepath.FromSlash(ownedDir), layer+".json")
}

// collectOwned lists the regular files under staging, which mirrors the sdk root.
func collectOwned(layer, staging string) (ownership, error) {
	rec := ownership{Layer: layer, Files: []ownedFile{}}
	err := filepath.WalkDir(staging, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		sum, err := fileSHA256(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(staging, p)
		if err != nil {
			return err
		}
		rec.Files = append(rec.Files, ownedFile{Path: filepath.ToSlash(rel), SHA256: sum})
		return nil
	})
	sort.Slice(rec.Files, func(i, j int) bool { return rec.Files[i].Path < rec.Files[j].Path })
	return rec, err
}

// loadOwned reads a layer's record; a layer with no record yet returns nil.
func loadOwned(sdkRoot, layer string) (*ownership, error) {
	data, err := os.ReadFile(ownedRecordPath(sdkRoot, layer))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rec ownership
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("parsing ownership record for %s: %w", layer, err)
	}
	return &rec, nil
}

// writeOwned replaces a layer's record atomically: a temp file in the same directory, then a rename.
func writeOwned(sdkRoot string, rec ownership) error {
	dir := filepath.Dir(ownedRecordPath(sdkRoot, rec.Layer))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+rec.Layer+"-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), ownedRecordPath(sdkRoot, rec.Layer))
}

// otherOwnedPaths returns every path another layer's record lists. A record that cannot be read is an error: with
// it unknown, nothing can be shown not to belong to another layer.
func otherOwnedPaths(sdkRoot, layer string) (map[string]bool, error) {
	entries, err := os.ReadDir(filepath.Dir(ownedRecordPath(sdkRoot, layer)))
	if err != nil {
		return nil, err
	}
	paths := map[string]bool{}
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || strings.HasPrefix(e.Name(), ".") || name == layer {
			continue
		}
		rec, err := loadOwned(sdkRoot, name)
		if err != nil {
			return nil, err
		}
		for _, f := range rec.Files {
			paths[f.Path] = true
		}
	}
	return paths, nil
}

// recordLayer writes the layer's new record, then removes what the release it replaces installed and this one did not.
// A file goes only if the old record lists it, the new one does not, no other layer lists it, and it still has the
// bytes the old record saw. Directories left empty go too, except the sdk root and its top-level directories.
func recordLayer(sdkRoot string, cur ownership, previous *ownership) error {
	var others map[string]bool
	if previous != nil {
		var err error
		if others, err = otherOwnedPaths(sdkRoot, cur.Layer); err != nil {
			previous = nil // cannot rule out another owner: keep every old file
		}
	}
	if err := writeOwned(sdkRoot, cur); err != nil {
		return fmt.Errorf("recording the files of %s: %w", cur.Layer, err)
	}
	if previous == nil {
		return nil
	}

	keep := map[string]bool{}
	for _, f := range cur.Files {
		keep[f.Path] = true
	}
	for _, f := range previous.Files {
		if keep[f.Path] || others[f.Path] {
			continue
		}
		abs, ok := ownedAbsPath(sdkRoot, f.Path)
		if !ok {
			continue
		}
		if info, err := os.Lstat(abs); err != nil || !info.Mode().IsRegular() {
			continue
		}
		if sum, err := fileSHA256(abs); err != nil || sum != f.SHA256 {
			continue // changed since it was installed: it is the user's now
		}
		if err := os.Remove(abs); err != nil {
			continue
		}
		pruneEmptyDirs(sdkRoot, filepath.Dir(abs))
	}
	return nil
}

// ownedAbsPath resolves a recorded path under the sdk root, refusing anything that would leave it or touch the
// bookkeeping directories.
func ownedAbsPath(sdkRoot, rel string) (string, bool) {
	if rel == "" || strings.HasPrefix(rel, "/") || filepath.IsAbs(rel) {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	first, _, _ := strings.Cut(filepath.ToSlash(clean), "/")
	if clean == "." || first == ".." || first == ".ps2build" || first == ".update-staging" {
		return "", false
	}
	return filepath.Join(sdkRoot, clean), true
}

// pruneEmptyDirs removes dir and its parents while they are empty, stopping before the sdk root and before any
// directory directly under it.
func pruneEmptyDirs(sdkRoot, dir string) {
	for {
		rel, err := filepath.Rel(sdkRoot, dir)
		if err != nil || rel == "." || !strings.ContainsRune(filepath.ToSlash(rel), '/') {
			return
		}
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
