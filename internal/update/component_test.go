package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComponentInstalled_MissingToolFileNotReportedPresent(t *testing.T) {
	ps2dev := t.TempDir()
	c := component{Name: "bin2c", AssetSuffix: ".exe", Test: "tools/bin2c.exe"}

	if componentInstalled(ps2dev, c) {
		t.Fatal("componentInstalled reported bin2c present with nothing on disk at all")
	}

	toolDir := filepath.Join(ps2dev, "tools")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "bin2c.exe"), []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !componentInstalled(ps2dev, c) {
		t.Fatal("componentInstalled reported bin2c missing after the file was created")
	}

	if err := os.Remove(filepath.Join(toolDir, "bin2c.exe")); err != nil {
		t.Fatal(err)
	}
	if componentInstalled(ps2dev, c) {
		t.Fatal("componentInstalled reported bin2c present after the file was deleted")
	}
}

func TestComponentInstalled_ZipBasedTool(t *testing.T) {
	ps2dev := t.TempDir()
	c := component{Name: "ninja", AssetSuffix: ".zip", Test: "tools/ninja.exe"}

	if componentInstalled(ps2dev, c) {
		t.Fatal("componentInstalled reported ninja present with no tools dir at all")
	}

	toolDir := filepath.Join(ps2dev, "tools")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if componentInstalled(ps2dev, c) {
		t.Fatal("componentInstalled reported ninja present with an empty tools dir")
	}

	if err := os.WriteFile(filepath.Join(toolDir, "ninja.exe"), []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !componentInstalled(ps2dev, c) {
		t.Fatal("componentInstalled reported ninja missing once the tools dir had content")
	}
}

func TestComponentInstalled_Toolchain(t *testing.T) {
	ps2dev := t.TempDir()
	c := component{Name: "toolchain", Test: "toolchain/ee"}

	if componentInstalled(ps2dev, c) {
		t.Fatal("componentInstalled reported toolchain present with nothing on disk")
	}

	for _, name := range []string{"ee", "iop", "dvp"} {
		dir := filepath.Join(ps2dev, "toolchain", name, "bin")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "placeholder"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !componentInstalled(ps2dev, c) {
		t.Fatal("componentInstalled reported toolchain missing once ee/iop/dvp all had content")
	}
}

func TestPS2BuildComponent_RunsFirstAndSelfReplaces(t *testing.T) {
	if len(allComponents) == 0 || allComponents[0].Name != "ps2build" {
		t.Fatalf("allComponents[0] = %q, want \"ps2build\" first so a full update self-updates before anything else", allComponents[0].Name)
	}

	c, ok := findComponent("ps2build")
	if !ok {
		t.Fatal("findComponent(\"ps2build\") not found - it must stay an updatable component")
	}
	if !c.SelfReplace {
		t.Error("ps2build component must have SelfReplace set - its release asset is a raw executable, not a zip")
	}
	if c.WindowsSuffix == "" || c.LinuxSuffix == "" || c.DarwinSuffix == "" {
		t.Error("ps2build component must support Windows, Linux, and Darwin asset suffixes")
	}
}

func TestRequireFreshTarget_AllowsSetupToolchainBootstrap(t *testing.T) {
	sdkRoot := t.TempDir()
	for _, name := range []string{"ee", "iop", "dvp"} {
		dir := filepath.Join(sdkRoot, "toolchain", name, "bin")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := requireFreshTarget(sdkRoot); err != nil {
		t.Fatalf("requireFreshTarget rejected a setup-toolchain-only bootstrap: %v", err)
	}
}

func TestRequireFreshTarget_RejectsExistingPackages(t *testing.T) {
	sdkRoot := t.TempDir()
	pkgDir := filepath.Join(sdkRoot, "packages", "core", "loadcore")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := requireFreshTarget(sdkRoot); err == nil {
		t.Fatal("requireFreshTarget accepted a target with real packages/ content and no versions.yaml")
	}
}

func TestComponentInstalled_SDKTier(t *testing.T) {
	ps2dev := t.TempDir()
	c := component{Name: "sdk-core", Tier: "core", Test: "packages/core"}

	if componentInstalled(ps2dev, c) {
		t.Fatal("componentInstalled reported sdk-core present with no packages dir")
	}

	pkgDir := filepath.Join(ps2dev, "packages", "core", "loadcore")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if !componentInstalled(ps2dev, c) {
		t.Fatal("componentInstalled reported sdk-core missing once packages/core had a package dir")
	}
}

func TestSDKTierAssetSelection(t *testing.T) {
	rel := func(names ...string) releaseInfo {
		r := releaseInfo{}
		for _, n := range names {
			r.Assets = append(r.Assets, releaseAsset{Name: n})
		}
		return r
	}
	cases := []struct {
		name         string
		assets       []string
		wantGCC      string
		wantLLVM     string
		wantUnsuffix string
	}{
		{"split release", []string{"sdk-core-v1-llvm.zip", "sdk-core-v1-gcc.zip"}, "sdk-core-v1-gcc.zip", "sdk-core-v1-llvm.zip", ""},
		{"legacy release", []string{"sdk-core-v1.zip"}, "", "", "sdk-core-v1.zip"},
		{"split release ignores the legacy lookup", []string{"sdk-core-v1-llvm.zip", "sdk-core-v1-gcc.zip"}, "sdk-core-v1-gcc.zip", "sdk-core-v1-llvm.zip", ""},
	}
	for _, tc := range cases {
		r := rel(tc.assets...)
		name := func(a *releaseAsset) string {
			if a == nil {
				return ""
			}
			return a.Name
		}
		if got := name(findAsset(r, "sdk-core-", "-gcc.zip")); got != tc.wantGCC {
			t.Errorf("%s: gcc asset = %q, want %q", tc.name, got, tc.wantGCC)
		}
		if got := name(findAsset(r, "sdk-core-", "-llvm.zip")); got != tc.wantLLVM {
			t.Errorf("%s: llvm asset = %q, want %q", tc.name, got, tc.wantLLVM)
		}
		if got := name(findUnsuffixedAsset(r, "sdk-core-", ".zip")); got != tc.wantUnsuffix {
			t.Errorf("%s: unsuffixed asset = %q, want %q", tc.name, got, tc.wantUnsuffix)
		}
	}
}

func TestRemoveAsideDeletesTheReplacedBinary(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "ps2build.exe")
	old := exe + ".old"
	for _, p := range []string{exe, old} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	removeAside(exe)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("%s still exists after removeAside", old)
	}
	if _, err := os.Stat(exe); err != nil {
		t.Errorf("the running executable was removed: %v", err)
	}
	removeAside(exe)
}

func TestWithoutSelfReplaceDropsOnlyPS2Build(t *testing.T) {
	var names []string
	for _, c := range withoutSelfReplace(allComponents) {
		names = append(names, c.Name)
	}
	for _, n := range names {
		if n == "ps2build" {
			t.Fatalf("ps2build still present: %v", names)
		}
	}
	if len(names) != len(allComponents)-1 {
		t.Fatalf("got %d components, want %d", len(names), len(allComponents)-1)
	}
}

func TestOggVorbis_OptionalWorldComponents(t *testing.T) {
	for _, name := range []string{"ogg", "vorbis"} {
		c, ok := findComponent(name)
		if !ok {
			t.Fatalf("findComponent(%q) not found", name)
		}
		if !c.Optional {
			t.Errorf("%s must be Optional so a bare update leaves it alone until it is installed by name", name)
		}
		if c.Tier != "world" {
			t.Errorf("%s Tier = %q, want \"world\" so its gcc and llvm zips are both applied", name, c.Tier)
		}
		if want := filepath.Join("packages", "world", name); c.Test != want {
			t.Errorf("%s Test = %q, want %q - packages/world alone is sdk-world's and would count as installed", name, c.Test, want)
		}
	}
}

func TestDefaultTargets_OggVorbisOnlyOnceInstalled(t *testing.T) {
	has := func(cs []component, name string) bool {
		for _, c := range cs {
			if c.Name == name {
				return true
			}
		}
		return false
	}

	ps2dev := t.TempDir()
	// sdk-world on its own must not make ogg or vorbis look installed.
	if err := os.MkdirAll(filepath.Join(ps2dev, "packages", "world", "gskit"), 0o755); err != nil {
		t.Fatal(err)
	}
	targets := defaultTargets(ps2dev)
	for _, name := range []string{"ogg", "vorbis"} {
		if has(targets, name) {
			t.Errorf("bare update would install %s, which was never installed by name", name)
		}
	}
	if !has(targets, "sdk-world") {
		t.Error("bare update dropped sdk-world")
	}

	if err := os.MkdirAll(filepath.Join(ps2dev, "packages", "world", "ogg", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	targets = defaultTargets(ps2dev)
	if !has(targets, "ogg") {
		t.Error("bare update skips ogg after it was installed - it would never be kept current")
	}
	if has(targets, "vorbis") {
		t.Error("installing ogg made a bare update pick up vorbis too")
	}
}
