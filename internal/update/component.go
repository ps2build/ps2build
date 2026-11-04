package update

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ps2build/ps2build/internal/utils"
)

// component describes one independently-releasing piece of a
// ps2dev-suite install.
type component struct {
	Name          string // matches the CLI's positional arg and versions.yaml's components key
	Host          string // API host; "" defaults to github.com
	Repo          string // "owner/name" on Host
	AssetPrefix   string
	AssetSuffix   string
	Tier          string // "core" | "world", kindSDKTier only
	PinnedVersion string
	WindowsSuffix string
	LinuxSuffix   string
	// DarwinSuffix covers macOS/arm64 only; there is no Intel Mac asset.
	DarwinSuffix string
	Test         string
	// Optional components are skipped by a bare "ps2build update" until they have been installed by name.
	Optional    bool
	SelfReplace bool // ps2build only - see applyPS2Build. Asset is a raw executable, not a zip.
}

var allComponents = []component{
	{Name: "ps2build", Repo: "ps2build/ps2build", AssetPrefix: "ps2build-", WindowsSuffix: "-windows-amd64.exe", LinuxSuffix: "-linux-amd64", DarwinSuffix: "-darwin-arm64", Test: utils.ExeSuffix("ps2build"), SelfReplace: true},
	{Name: "toolchain", Repo: "ps2build/toolchain", AssetPrefix: "ps2-toolchain-", AssetSuffix: ".zip", WindowsSuffix: "-windows-amd64", LinuxSuffix: "-linux-amd64", DarwinSuffix: "-darwin-arm64", Test: "toolchain"},
	{Name: "llvm-toolchain", Repo: "ps2build/llvm-toolchain", AssetPrefix: "ps2-llvm-toolchain-", AssetSuffix: ".zip", WindowsSuffix: "-windows-amd64", LinuxSuffix: "-linux-amd64", Test: "llvm-toolchain", Optional: true},
	{Name: "ninja", Repo: "ps2build/ninja", AssetPrefix: "ninja-", AssetSuffix: ".zip", WindowsSuffix: "-windows-amd64", LinuxSuffix: "-linux-amd64", DarwinSuffix: "-darwin-arm64", Test: filepath.Join("tools", utils.ExeSuffix("ninja"))},
	{Name: "tools", Repo: "ps2build/tools", AssetPrefix: "tools-", AssetSuffix: ".zip", WindowsSuffix: "-windows-amd64", LinuxSuffix: "-linux-amd64", DarwinSuffix: "-darwin-arm64", Test: filepath.Join("tools", utils.ExeSuffix("bin2c"))},
	{Name: "vcl", Repo: "ps2build/vcl", AssetPrefix: "vcl-", AssetSuffix: ".zip", WindowsSuffix: "-windows-amd64", LinuxSuffix: "-linux-amd64", DarwinSuffix: "-darwin-arm64", Test: filepath.Join("vcl", utils.ExeSuffix("vcl"))},
	{Name: "cmake", Repo: "ps2build/cmake", AssetPrefix: "cmake-", AssetSuffix: ".zip", Test: filepath.Join("cmake", "ps2dev.cmake")},
	{Name: "sdk-core", Repo: "ps2build/sdk-core", AssetPrefix: "sdk-core-", AssetSuffix: ".zip", Tier: "core", Test: filepath.Join("packages", "core")},
	{Name: "sdk-world", Repo: "ps2build/sdk-world", AssetPrefix: "sdk-world-", AssetSuffix: ".zip", Tier: "world", Test: filepath.Join("packages", "world")},
}

func findComponent(name string) (component, bool) {
	for _, c := range allComponents {
		if c.Name == name {
			return c, true
		}
	}
	return component{}, false
}

func componentNames() []string {
	names := make([]string, len(allComponents))
	for i, c := range allComponents {
		names[i] = c.Name
	}
	return names
}

func componentInstalled(ps2dev string, c component) bool {
	return fileExists(filepath.Join(ps2dev, c.Test))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func applyLayer(ps2dev, name, url string) error {
	zipPath, err := downloadToTemp(name, url)
	if err != nil {
		return err
	}
	defer os.Remove(zipPath)

	staging := filepath.Join(ps2dev, ".update-staging", name)
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("clearing old staging dir: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := extractZip(zipPath, staging); err != nil {
		return fmt.Errorf("extracting downloaded %s: %w", name, err)
	}

	entries, err := os.ReadDir(staging)
	if err != nil {
		return fmt.Errorf("reading staged component %s: %w", name, err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("downloaded %s component contains no files", name)
	}

	for _, e := range entries {
		if err := replacePath(
			filepath.Join(ps2dev, e.Name()),
			filepath.Join(staging, e.Name()),
		); err != nil {
			return fmt.Errorf("replacing %s/%s: %w", name, e.Name(), err)
		}
	}

	return nil
}

// defaultTargets lists what a bare update touches: every component except optional ones not yet installed.
func defaultTargets(ps2dev string) []component {
	var out []component
	for _, c := range allComponents {
		if c.Optional && !componentInstalled(ps2dev, c) {
			continue
		}
		out = append(out, c)
	}
	return out
}

func withoutSelfReplace(in []component) []component {
	var out []component
	for _, c := range in {
		if !c.SelfReplace {
			out = append(out, c)
		}
	}
	return out
}
