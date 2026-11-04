package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindLicenseFile(t *testing.T) {
	tests := []struct {
		name     string
		create   string // filename to create in the temp dir, "" for none
		wantName string // expected basename found, "" if wantOK is false
		wantOK   bool
	}{
		{name: "real LICENSE", create: "LICENSE", wantName: "LICENSE", wantOK: true},
		{name: "real COPYING", create: "COPYING", wantName: "COPYING", wantOK: true},
		{name: "real LICENSE.TXT", create: "LICENSE.TXT", wantName: "LICENSE.TXT", wantOK: true},
		// Case-insensitive match, real filename casing preserved in the result.
		{name: "lowercase license", create: "license", wantName: "license", wantOK: true},
		{name: "no license file at all", create: "", wantOK: false},
		{name: "unrelated file present", create: "README.md", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.create != "" {
				if err := os.WriteFile(filepath.Join(dir, tt.create), []byte("license text"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			got, ok := findLicenseFile(dir)
			if ok != tt.wantOK {
				t.Fatalf("findLicenseFile() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && filepath.Base(got) != tt.wantName {
				t.Errorf("findLicenseFile() = %q, want basename %q", got, tt.wantName)
			}
		})
	}
}

func TestFindLicenseFile_PrefersFirstMatchInPriorityOrder(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"LICENSE", "COPYING"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, ok := findLicenseFile(dir)
	if !ok {
		t.Fatal("expected a match")
	}
	if filepath.Base(got) != "LICENSE" {
		t.Errorf("got %q, want LICENSE (first in licenseFileNames priority order)", got)
	}
}

func TestFindLicenseFile_IgnoresDirectories(t *testing.T) {
	// A real member could have a "LICENSE" subdirectory (unlikely but
	// not impossible) - must not be mistaken for the file itself.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "LICENSE"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := findLicenseFile(dir); ok {
		t.Error("a directory named LICENSE should not count as a license file")
	}
}
