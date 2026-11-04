package utils

import (
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

var ExeSuffixValue = computeExeSuffix()

var sdkRootOverride string

func computeExeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func ExeSuffix(exe string) string {
	return exe + ExeSuffixValue
}

func SetSDKRootOverride(root string) (reset func()) {
	sdkRootOverride = root
	return func() { sdkRootOverride = "" }
}

func DetectSDKRoot() string {
	if sdkRootOverride != "" {
		return filepath.ToSlash(sdkRootOverride)
	}

	selfExe, err := os.Executable()
	if err != nil {
		panic(err)
	}

	dir := filepath.Dir(selfExe)

	if filepath.Base(dir) == "bin" {
		parent := filepath.Dir(dir)
		if isSDKRoot(parent) && !isSDKRoot(dir) {
			dir = parent
		}
	}

	return filepath.ToSlash(dir)
}

func isSDKRoot(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "versions.yaml")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "packages")); err == nil {
		return true
	}
	return false
}

const (
	gccPackagesDir  = "packages"
	llvmPackagesDir = "packages-llvm"
)

var llvmPackages bool

func UseLLVMPackages(on bool) { llvmPackages = on }

func PackageRoot() string {
	if llvmPackages {
		if root := path.Join(DetectSDKRoot(), llvmPackagesDir); FolderExists(root) {
			return root
		}
	}
	return path.Join(DetectSDKRoot(), gccPackagesDir)
}

func InstallPackageRoot() string {
	if llvmPackages {
		return path.Join(DetectSDKRoot(), llvmPackagesDir)
	}
	return path.Join(DetectSDKRoot(), gccPackagesDir)
}

func SDKPath(path_ string) string {
	return path.Join(DetectSDKRoot(), path_)
}

func SdkToolchainPath(path_ string) string {
	return path.Join(DetectSDKRoot(), "toolchain", path_)
}

func SdkPackagePath(tier, path_ string) string {
	return path.Join(PackageRoot(), tier, path_)
}

func SdkToolPath(path_ string) string {
	return path.Join(DetectSDKRoot(), "tools", path_)
}

func FileExists(path_ string) bool {
	info, err := os.Stat(path_)
	return err == nil && !info.IsDir()
}

func FolderExists(path_ string) bool {
	info, err := os.Stat(path_)
	return err == nil && info.IsDir()
}

func Join(elem ...string) string {
	return filepath.ToSlash(filepath.Join(elem...))
}

// AbsPath resolves a ps2.yaml-relative path to an absolute forward-slash path; ninja runs from outDir, not projectRoot.
func AbsPath(root, p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	return filepath.ToSlash(p)
}

func AbsPaths(root string, ps []string) []string {
	if ps == nil {
		return nil
	}
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = AbsPath(root, p)
	}
	return out
}

func SanitizeCIdent(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
