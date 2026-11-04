package utils

import (
	"os"
	"path"
	"path/filepath"
	"testing"
)

func TestIsSDKRoot_PackagesOrVersionsFile(t *testing.T) {
	t.Run("neither marker present", func(t *testing.T) {
		dir := t.TempDir()
		if isSDKRoot(dir) {
			t.Error("isSDKRoot reported a root with neither packages/ nor versions.yaml as present")
		}
	})

	t.Run("packages/ present", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "packages"), 0o755); err != nil {
			t.Fatal(err)
		}
		if !isSDKRoot(dir) {
			t.Error("isSDKRoot reported a root with packages/ as absent")
		}
	})

	t.Run("versions.yaml present", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "versions.yaml"), []byte("components: {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if !isSDKRoot(dir) {
			t.Error("isSDKRoot reported a root with versions.yaml as absent")
		}
	})
}

func TestPackageRootFollowsToolchain(t *testing.T) {
	root := filepath.ToSlash(t.TempDir())
	t.Cleanup(SetSDKRootOverride(root))
	t.Cleanup(func() { UseLLVMPackages(false) })

	UseLLVMPackages(false)
	if got, want := PackageRoot(), path.Join(root, "packages"); got != want {
		t.Errorf("gcc PackageRoot() = %q, want %q", got, want)
	}

	UseLLVMPackages(true)
	if got, want := PackageRoot(), path.Join(root, "packages"); got != want {
		t.Errorf("llvm without packages-llvm: PackageRoot() = %q, want the gcc set %q", got, want)
	}
	if got, want := InstallPackageRoot(), path.Join(root, "packages-llvm"); got != want {
		t.Errorf("llvm InstallPackageRoot() = %q, want %q", got, want)
	}

	if err := os.MkdirAll(path.Join(root, "packages-llvm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, want := PackageRoot(), path.Join(root, "packages-llvm"); got != want {
		t.Errorf("llvm with packages-llvm: PackageRoot() = %q, want %q", got, want)
	}
}
