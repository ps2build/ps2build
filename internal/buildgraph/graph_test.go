package buildgraph

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ps2build/ps2build/internal/config"
	"github.com/ps2build/ps2build/internal/test"
	"github.com/ps2build/ps2build/internal/utils"
)

const testProjectRoot = "/project"

func testGraph() *Graph { return &Graph{Toolchain: gccToolchain{}} }

func TestEECompileFlags(t *testing.T) {
	test.SetupCommonIncludeDir()

	cases := []struct {
		name string
		t    config.Target
		want []string
	}{
		{
			name: "basics",
			t:    config.Target{Opt: "-O2"},
			want: []string{"-D_EE", "-G0", "-O2", "-Wall", "-I" + path.Join(utils.DetectSDKRoot(), "toolchain/ee/include"), "-I" + path.Join(utils.PackageRoot(), "core/common/include")},
		},
		{
			name: "debug adds dwarf flags",
			t:    config.Target{Opt: "-O2", Debug: true},
			want: []string{"-gdwarf-2", "-gz"},
		},
		{
			name: "defines get -D prefixed",
			t:    config.Target{Opt: "-O2", Defines: []string{"FOO", "BAR=1"}},
			want: []string{"-DFOO", "-DBAR=1"},
		},
		{
			name: "user cflags appended last",
			t:    config.Target{Opt: "-O2", CFlags: []string{"-Wextra"}},
			want: []string{"-Wextra"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := eeCompileFlags(testGraph(), tc.t, testProjectRoot, nil)
			for _, w := range tc.want {
				if !slices.Contains(got, w) {
					t.Errorf("eeCompileFlags(%+v) = %v, want it to contain %q", tc.t, got, w)
				}
			}
		})
	}

	t.Run("without debug, no dwarf flags", func(t *testing.T) {
		got := eeCompileFlags(testGraph(), config.Target{Opt: "-O2"}, testProjectRoot, nil)
		if slices.Contains(got, "-gdwarf-2") {
			t.Errorf("eeCompileFlags without Debug = %v, should not contain -gdwarf-2", got)
		}
	})

	t.Run("standard libs never appear as explicit flags", func(t *testing.T) {
		// kernel/cglue/cdvd are linked via the compiler's own default
		// specs, never as explicit -lkernel etc. flags here.
		got := eeCompileFlags(testGraph(), config.Target{Opt: "-O2"}, testProjectRoot, nil)
		for _, f := range got {
			if strings.Contains(f, "kernel") || strings.Contains(f, "cglue") || strings.Contains(f, "cdvd") {
				t.Errorf("eeCompileFlags should never mention standard libs, got %v", got)
			}
		}
	})

	t.Run("relative include_dirs resolved against projectRoot", func(t *testing.T) {
		got := eeCompileFlags(testGraph(), config.Target{Opt: "-O2", IncludeDirs: []string{"vendor/include"}}, testProjectRoot, nil)
		if !slices.Contains(got, "-I/project/vendor/include") {
			t.Errorf("eeCompileFlags() = %v, want it to contain %q", got, "-I/project/vendor/include")
		}
	})

	t.Run("absolute include_dirs entry passed through unchanged", func(t *testing.T) {
		abs := filepath.Join(t.TempDir(), "include")
		want := "-I" + filepath.ToSlash(abs)
		got := eeCompileFlags(testGraph(), config.Target{Opt: "-O2", IncludeDirs: []string{abs}}, testProjectRoot, nil)
		if !slices.Contains(got, want) {
			t.Errorf("eeCompileFlags() = %v, want it to contain %q", got, want)
		}
	})

	t.Run("pkgIncludeDirs appended as -I, already absolute", func(t *testing.T) {
		got := eeCompileFlags(testGraph(), config.Target{Opt: "-O2"}, testProjectRoot, []string{"/ps2dev/packages/world/wavekit/include"})
		if !slices.Contains(got, "-I/ps2dev/packages/world/wavekit/include") {
			t.Errorf("eeCompileFlags() = %v, want it to contain the package include dir", got)
		}
	})
}

func TestIOPCompileFlags(t *testing.T) {
	got := iopCompileFlags(testGraph(), config.Target{Opt: "-Os"}, testProjectRoot, nil)
	for _, want := range []string{
		"-D_IOP", "-fno-builtin", "-G0", "-Os", "-Wall",
		"-msoft-float", "-mno-explicit-relocs", // required for GCC >= 5.3.0, see Makefile.iopglobal
		"-I" + path.Join(utils.DetectSDKRoot(), "toolchain/iop/include"), "-I" + path.Join(utils.PackageRoot(), "core/common/include"),
	} {
		if !slices.Contains(got, want) {
			t.Errorf("iopCompileFlags() = %v, want it to contain %q", got, want)
		}
	}
}

func TestEELinkFlags(t *testing.T) {
	got := eeLinkFlags(testGraph(), config.Target{Opt: "-O2", LDFlags: []string{"-Wl,--gc-sections"}}, nil, false)
	for _, want := range []string{"-O2", "-L" + path.Join(utils.DetectSDKRoot(), "toolchain/ee/lib"), "-Wl,--gc-sections"} {
		if !slices.Contains(got, want) {
			t.Errorf("eeLinkFlags() = %v, want it to contain %q", got, want)
		}
	}
}

func TestEELinkFlags_GCSectionsUnconditional(t *testing.T) {
	got := eeLinkFlags(testGraph(), config.Target{Opt: "-O2"}, nil, false)
	if !slices.Contains(got, "-Wl,--gc-sections") {
		t.Errorf("eeLinkFlags() = %v, want it to always contain -Wl,--gc-sections, even with no LDFlags set - this is what replaces ee/kernel's old per-function -DF_ compile scheme", got)
	}
}

func TestEELinkFlags_ExtraLibDirs(t *testing.T) {
	got := eeLinkFlags(testGraph(), config.Target{Opt: "-O2"}, []string{"lib"}, false)
	if !slices.Contains(got, "-Llib") {
		t.Errorf("eeLinkFlags() = %v, want it to contain -Llib for a local ee_lib dir", got)
	}
}

func TestIOPLinkFlags(t *testing.T) {
	// -nostdlib -dc -r is required: IOP modules use a special relocatable
	// link, not a normal executable link.
	got := iopLinkFlags(testGraph(), config.Target{Opt: "-Os"}, nil)
	for _, want := range []string{"-Os", "-nostdlib", "-dc", "-r"} {
		if !slices.Contains(got, want) {
			t.Errorf("iopLinkFlags() = %v, want it to contain %q", got, want)
		}
	}
}

func TestIOPStripFlags(t *testing.T) {
	got := iopStripFlags()
	want := []string{
		"--strip-unneeded",
		"--remove-section=.pdr",
		"--remove-section=.comment",
		"--remove-section=.mdebug.abi32",
		"--remove-section=.gnu.attributes",
	}
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("iopStripFlags() = %v, want it to contain %q", got, w)
		}
	}
}

func TestIOPFixupFlags(t *testing.T) {
	t.Run("no exports_tab: --allow-zero-text included", func(t *testing.T) {
		got := gccToolchain{}.IOPFixupFlags(false)
		for _, want := range []string{"--rb", "--irx1", "--allow-zero-text"} {
			if !slices.Contains(got, want) {
				t.Errorf("gccToolchain{}.IOPFixupFlags(false) = %v, want it to contain %q", got, want)
			}
		}
	})

	t.Run("has exports_tab: --allow-zero-text omitted", func(t *testing.T) {
		got := gccToolchain{}.IOPFixupFlags(true)
		for _, want := range []string{"--rb", "--irx1"} {
			if !slices.Contains(got, want) {
				t.Errorf("gccToolchain{}.IOPFixupFlags(true) = %v, want it to contain %q", got, want)
			}
		}
		if slices.Contains(got, "--allow-zero-text") {
			t.Errorf("gccToolchain{}.IOPFixupFlags(true) = %v, should not contain --allow-zero-text (a real export table exists)", got)
		}
	})
}

func TestResolveLinkfile(t *testing.T) {
	if got := resolveLinkfile(testGraph(), config.Target{Type: config.TargetEE}, testProjectRoot); got != utils.SdkPackagePath("core", "startup/share/ee/linkfile") {
		t.Errorf("resolveLinkfile(ee) = %q, want default", got)
	}
	if got := resolveLinkfile(testGraph(), config.Target{Type: config.TargetIOP}, testProjectRoot); got != utils.SdkPackagePath("core", "startup/share/iop/linkfile") {
		t.Errorf("resolveLinkfile(iop) = %q, want default", got)
	}
	if got := resolveLinkfile(testGraph(), config.Target{Type: config.TargetEE, Linkfile: "custom/link"}, testProjectRoot); got != "/project/custom/link" {
		t.Errorf("resolveLinkfile(override) = %q, want the override resolved against projectRoot", got)
	}
	abs := filepath.Join(t.TempDir(), "link")
	want := filepath.ToSlash(abs)
	if got := resolveLinkfile(testGraph(), config.Target{Type: config.TargetEE, Linkfile: abs}, testProjectRoot); got != want {
		t.Errorf("resolveLinkfile(absolute override) = %q, want %q (passed through unchanged)", got, want)
	}
}

func TestCommonIncludeDir(t *testing.T) {
	t.Run("no common package installed: falls back to the flat tree", func(t *testing.T) {
		if got := commonIncludeDir("ee"); got != utils.SdkPackagePath("core", "common/include") {
			t.Errorf("commonIncludeDir(ee) = %q, want the flat-tree fallback", got)
		}
		if got := commonIncludeDir("iop"); got != utils.SdkPackagePath("core", "common/include") {
			t.Errorf("commonIncludeDir(iop) = %q, want the flat-tree fallback", got)
		}
	})

	t.Run("common package installed: prefers it over the flat tree", func(t *testing.T) {
		dir := test.SetupCommonIncludeDir()
		yaml := "name: common\nartifacts:\n  - kind: headers\n    target: [ee, iop]\n    include_dirs: [include]\n"
		if err := os.WriteFile(filepath.Join(dir, "package.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}

		want := filepath.ToSlash(filepath.Join(dir, "include"))
		if got := commonIncludeDir("ee"); got != want {
			t.Errorf("commonIncludeDir(ee) = %q, want %q (the installed package, not the flat tree)", got, want)
		}
		if got := commonIncludeDir("iop"); got != want {
			t.Errorf("commonIncludeDir(iop) = %q, want %q", got, want)
		}
	})
}

func TestBuild_EmbedIRXDedup(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Targets: []config.Target{
			{Name: "helper", Type: config.TargetIOP, Sources: []string{"h.c"}, Opt: "-Os"},
			{Name: "a", Type: config.TargetEE, Sources: []string{"a.c"}, EmbedIRX: []string{"helper"}, Opt: "-O2"},
			{Name: "b", Type: config.TargetEE, Sources: []string{"b.c"}, EmbedIRX: []string{"helper"}, Opt: "-O2"},
		},
	}

	g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(g.Bin2Cs) != 1 {
		t.Fatalf("len(Bin2Cs) = %d, want exactly 1 (deduplicated across both EE targets)", len(g.Bin2Cs))
	}

	var embedCompiles int
	for _, c := range g.Compiles {
		if strings.Contains(c.Object, "_embed") {
			embedCompiles++
		}
	}
	if embedCompiles != 1 {
		t.Errorf("compile steps for the embedded object = %d, want exactly 1 (shared, not duplicated per EE target)", embedCompiles)
	}

	// Both EE targets' link steps must reference the one shared object.
	for _, name := range []string{"a", "b"} {
		var link *LinkStep
		for i := range g.Links {
			if g.Links[i].TargetName == name {
				link = &g.Links[i]
			}
		}
		if link == nil {
			t.Fatalf("no link step found for target %q", name)
		}
		found := false
		for _, obj := range link.Objects {
			if strings.Contains(obj, "_embed") {
				found = true
			}
		}
		if !found {
			t.Errorf("target %q's link step objects = %v, want the shared embedded object included", name, link.Objects)
		}
	}
}

func TestBuild_EmbedIRXGeneratedFilePathsMatch(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Targets: []config.Target{
			{Name: "helper", Type: config.TargetIOP, Sources: []string{"h.c"}, Opt: "-Os"},
			{Name: "main", Type: config.TargetEE, Sources: []string{"main.c"}, EmbedIRX: []string{"helper"}, Opt: "-O2"},
		},
	}

	g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(g.Bin2Cs) != 1 {
		t.Fatalf("len(Bin2Cs) = %d, want 1", len(g.Bin2Cs))
	}
	bin2cOutput := g.Bin2Cs[0].COutPath

	var compileSource string
	for _, c := range g.Compiles {
		if c.TargetName == "helper" && strings.Contains(c.Source, "_irx") {
			compileSource = c.Source
		}
	}
	if compileSource == "" {
		t.Fatalf("no compile step found for the bin2c-generated file among %+v", g.Compiles)
	}

	if bin2cOutput.String() != compileSource {
		t.Errorf("Bin2CStep.COutPath = %q, CompileStep.Source = %q - must be identical or ninja can't see the dependency", bin2cOutput, compileSource)
	}
}

func TestRebaseGraph(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Targets: []config.Target{
			{Name: "mylib", Type: config.TargetEELib, Sources: []string{"lib.c"}},
			{Name: "helper", Type: config.TargetIOP, Sources: []string{"helper.c"}, ImportsLst: "imports.lst"},
			{Name: "main", Type: config.TargetEE, Sources: []string{"main.c"}, EmbedIRX: []string{"helper"}, OutputBin: true, Pack: true, Libs: []string{"mylib"}},
		},
	}
	config.ApplyDefaults(cfg)
	g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(g.Compiles) == 0 || len(g.Links) == 0 || len(g.Strips) == 0 || len(g.Fixups) == 0 || len(g.Archives) == 0 || len(g.Objcopies) == 0 || len(g.Packs) == 0 || len(g.IETableStubs) == 0 {
		t.Fatalf("test fixture didn't populate every Step type: Compiles=%d Links=%d Strips=%d Fixups=%d Archives=%d Objcopies=%d Packs=%d IETableStubs=%d",
			len(g.Compiles), len(g.Links), len(g.Strips), len(g.Fixups), len(g.Archives), len(g.Objcopies), len(g.Packs), len(g.IETableStubs))
	}

	// Absolute fields, captured before rebasing - must be byte-identical
	// after, since RebaseGraph must never touch them.
	wantSources := make([]string, len(g.Compiles))
	for i, c := range g.Compiles {
		wantSources[i] = c.Source
	}
	wantSourceFiles := make([]string, len(g.IETableStubs))
	wantCOutPaths := make([]GeneratedPath, len(g.IETableStubs))
	for i, ie := range g.IETableStubs {
		wantSourceFiles[i] = ie.SourceFile
		wantCOutPaths[i] = ie.COutPath
	}
	wantLinkfiles := make([]string, len(g.Links))
	for i, l := range g.Links {
		wantLinkfiles[i] = l.Linkfile
	}

	RebaseGraph(g, "core/mylib")

	for i, c := range g.Compiles {
		if c.Source != wantSources[i] {
			t.Errorf("Compiles[%d].Source = %q, want unchanged %q (absolute, must not be rebased)", i, c.Source, wantSources[i])
		}
		if !strings.HasPrefix(c.Object, "core/mylib/") {
			t.Errorf("Compiles[%d].Object = %q, want it prefixed with core/mylib/", i, c.Object)
		}
	}
	for i, ie := range g.IETableStubs {
		if ie.SourceFile != wantSourceFiles[i] {
			t.Errorf("IETableStubs[%d].SourceFile = %q, want unchanged %q (absolute)", i, ie.SourceFile, wantSourceFiles[i])
		}
		if ie.COutPath != wantCOutPaths[i] {
			t.Errorf("IETableStubs[%d].COutPath = %q, want unchanged %q (GeneratedPath is always absolute)", i, ie.COutPath, wantCOutPaths[i])
		}
	}
	for i, l := range g.Links {
		if l.Linkfile != wantLinkfiles[i] {
			t.Errorf("Links[%d].Linkfile = %q, want unchanged %q (resolveLinkfile always returns absolute)", i, l.Linkfile, wantLinkfiles[i])
		}
		if !strings.HasPrefix(l.Output, "core/mylib/") {
			t.Errorf("Links[%d].Output = %q, want it prefixed with core/mylib/", i, l.Output)
		}
		for _, obj := range l.Objects {
			if !strings.HasPrefix(obj, "core/mylib/") {
				t.Errorf("Links[%d].Objects contains %q, want it prefixed with core/mylib/", i, obj)
			}
		}
	}
	for i, s := range g.Strips {
		if !strings.HasPrefix(s.Input, "core/mylib/") || !strings.HasPrefix(s.Output, "core/mylib/") {
			t.Errorf("Strips[%d] = %+v, want both Input and Output prefixed with core/mylib/", i, s)
		}
	}
	for i, f := range g.Fixups {
		if !strings.HasPrefix(f.Input, "core/mylib/") || !strings.HasPrefix(f.Output, "core/mylib/") {
			t.Errorf("Fixups[%d] = %+v, want both Input and Output prefixed with core/mylib/", i, f)
		}
	}
	for i, a := range g.Archives {
		if !strings.HasPrefix(a.Output, "core/mylib/") {
			t.Errorf("Archives[%d].Output = %q, want it prefixed with core/mylib/", i, a.Output)
		}
	}
	for i, o := range g.Objcopies {
		if !strings.HasPrefix(o.Input, "core/mylib/") || !strings.HasPrefix(o.Output, "core/mylib/") {
			t.Errorf("Objcopies[%d] = %+v, want both Input and Output prefixed with core/mylib/", i, o)
		}
	}
	for i, p := range g.Packs {
		if !strings.HasPrefix(p.Input, "core/mylib/") || !strings.HasPrefix(p.Output, "core/mylib/") {
			t.Errorf("Packs[%d] = %+v, want both Input and Output prefixed with core/mylib/", i, p)
		}
	}
	for name, out := range g.TargetOutputs {
		if !strings.HasPrefix(out, "core/mylib/") {
			t.Errorf("TargetOutputs[%q] = %q, want it prefixed with core/mylib/", name, out)
		}
	}
	if len(g.Bin2Cs) > 0 {
		for i, b := range g.Bin2Cs {
			if !strings.HasPrefix(b.IRXPath, "core/mylib/") {
				t.Errorf("Bin2Cs[%d].IRXPath = %q, want it prefixed with core/mylib/", i, b.IRXPath)
			}
		}
	}
}

func TestNewGeneratedPath(t *testing.T) {
	t.Run("relative input resolved to absolute", func(t *testing.T) {
		got := NewGeneratedPath("build/generated/foo.c")
		if !filepath.IsAbs(got.String()) {
			t.Errorf("NewGeneratedPath(relative) = %q, want an absolute path", got)
		}
	})

	t.Run("already-absolute input stays forward-slash", func(t *testing.T) {
		abs := filepath.Join(t.TempDir(), "generated", "foo.c")
		got := NewGeneratedPath(abs)
		want := filepath.ToSlash(abs)
		if got.String() != want {
			t.Errorf("NewGeneratedPath(%q) = %q, want %q", abs, got, want)
		}
	})

	t.Run("String round-trips exactly, for the CompileStep.Source assignment", func(t *testing.T) {
		abs := filepath.Join(t.TempDir(), "generated", "foo.c")
		p := NewGeneratedPath(abs)
		if p.String() != string(p) {
			t.Errorf("String() = %q, want it identical to the underlying value %q", p.String(), string(p))
		}
	})
}

func TestResolveLibDeps(t *testing.T) {
	libsByName := map[string]libInfo{
		"mymath":  {Archive: "lib/libmymath.a", IncludeDirs: []string{"/project/mymath/include"}, Local: true},
		"physics": {Archive: "lib/libphysics.a", Local: true},
		"snd2":    {Archive: "libsnd2/lib/libsnd2.a", Local: false},
	}

	localDirs, localArchives, externalDirs, externalArchives, includeDirs, needsCXX :=
		resolveLibDeps([]string{"png", "mymath", "physics", "snd2", "m"}, libsByName)

	if want := []string{"lib/libmymath.a", "lib/libphysics.a"}; !stringsEqual(localArchives, want) {
		t.Errorf("localArchives = %v, want %v", localArchives, want)
	}
	if want := []string{"libsnd2/lib/libsnd2.a"}; !stringsEqual(externalArchives, want) {
		t.Errorf("externalArchives = %v, want %v", externalArchives, want)
	}
	// Both local libs share a directory - it must be deduplicated, not
	// appear twice as two separate -L flags.
	if want := []string{"lib"}; !stringsEqual(localDirs, want) {
		t.Errorf("localDirs = %v, want %v (deduplicated)", localDirs, want)
	}
	if want := []string{"libsnd2/lib"}; !stringsEqual(externalDirs, want) {
		t.Errorf("externalDirs = %v, want %v", externalDirs, want)
	}
	if want := []string{"/project/mymath/include"}; !stringsEqual(includeDirs, want) {
		t.Errorf("includeDirs = %v, want %v", includeDirs, want)
	}
	if needsCXX {
		t.Errorf("needsCXX = true, want false (none of the matched libs need C++)")
	}
}

func TestResolveLibDeps_NeedsCXX(t *testing.T) {
	libsByName := map[string]libInfo{
		"hashlib": {Archive: "lib/libhashlib.a", NeedsCXX: true, Local: true},
	}
	_, _, _, _, _, needsCXX := resolveLibDeps([]string{"hashlib"}, libsByName)
	if !needsCXX {
		t.Error("needsCXX = false, want true (matched lib has C++ sources)")
	}
}

func writeTestPackage(t *testing.T, tier, name string, headers []string) {
	t.Helper()
	writeTestPackageWithLinks(t, tier, name, headers, nil)
}

// writeTestPackageWithLinks is writeTestPackage plus a links: list, for
// resolveLinkOrder tests.
func writeTestPackageWithLinks(t *testing.T, tier, name string, headers, links []string) {
	t.Helper()
	dir := filepath.Join(utils.PackageRoot(), tier, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "name: " + name + "\nartifacts:\n  - kind: library\n    target: [ee]\n    include_dirs: [include]\n    lib: lib/lib" + name + ".a\n"
	if len(headers) > 0 {
		yaml += "    headers: [" + strings.Join(headers, ", ") + "]\n"
	}
	if len(links) > 0 {
		yaml += "    links: [" + strings.Join(links, ", ") + "]\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "package.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveLinkOrder(t *testing.T) {
	t.Run("no links: declared, nothing added", func(t *testing.T) {
		writeTestPackage(t, "core", "draw", nil)
		got, err := resolveLinkOrder([]string{"draw", "external"}, nil, "ee")
		if err != nil {
			t.Fatal(err)
		}
		if !stringsEqual(got, []string{"draw", "external"}) {
			t.Errorf("got %v, want unchanged input (external isn't even a real package, must still pass through)", got)
		}
	})

	t.Run("chain: consumer appears before its dependency, recursively", func(t *testing.T) {
		writeTestPackageWithLinks(t, "core", "a", nil, []string{"b"})
		writeTestPackageWithLinks(t, "core", "b", nil, []string{"c"})
		writeTestPackage(t, "core", "c", nil)

		got, err := resolveLinkOrder([]string{"a"}, nil, "ee")
		if err != nil {
			t.Fatal(err)
		}
		if !stringsEqual(got, []string{"a", "b", "c"}) {
			t.Errorf("got %v, want [a b c] (each consumer before its dependency)", got)
		}
	})

	t.Run("diamond: a shared dependency ends up after every consumer that needs it", func(t *testing.T) {
		writeTestPackageWithLinks(t, "core", "a", nil, []string{"c"})
		writeTestPackageWithLinks(t, "core", "d", nil, []string{"c"})
		writeTestPackage(t, "core", "c", nil)

		got, err := resolveLinkOrder([]string{"a", "d"}, nil, "ee")
		if err != nil {
			t.Fatal(err)
		}
		if !stringsEqual(got, []string{"a", "d", "c"}) {
			t.Errorf("got %v, want [a d c] (a/d order preserved, c placed after both)", got)
		}
	})

	t.Run("same-file target has no links: concept, left alone", func(t *testing.T) {
		libsByName := map[string]libInfo{"mylib": {Archive: "lib/libmylib.a"}}
		got, err := resolveLinkOrder([]string{"mylib"}, libsByName, "ee")
		if err != nil {
			t.Fatal(err)
		}
		if !stringsEqual(got, []string{"mylib"}) {
			t.Errorf("got %v, want [mylib] unchanged - same-file targets have no links: field to expand", got)
		}
	})
}

func TestResolvePackageLibs(t *testing.T) {
	writeTestPackage(t, "core", "wavekit", nil)

	dirs, archives, includeDirs, _, _, err := ResolvePackageLibs([]string{"wavekit"}, nil, "ee", "test-target")
	if err != nil {
		t.Fatal(err)
	}
	wantArchive := utils.SdkPackagePath("core", "wavekit/lib/libwavekit.a")
	if len(archives) != 1 || archives[0] != wantArchive {
		t.Errorf("archives = %v, want [%q]", archives, wantArchive)
	}
	wantInclude := utils.SdkPackagePath("core", "wavekit/include")
	if len(includeDirs) != 1 || includeDirs[0] != wantInclude {
		t.Errorf("includeDirs = %v, want [%q]", includeDirs, wantInclude)
	}
	wantDir := utils.SdkPackagePath("core", "wavekit/lib")
	if len(dirs) != 1 || dirs[0] != wantDir {
		t.Errorf("dirs = %v, want [%q]", dirs, wantDir)
	}
}

func TestResolvePackageLibs_DeprecatedArtifactWarns(t *testing.T) {
	dir := utils.SdkPackagePath("core", "oldlib")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "name: oldlib\nartifacts:\n  - kind: library\n    target: [ee]\n    include_dirs: [include]\n    lib: lib/liboldlib.a\n    deprecated: \"use newlib instead\"\n"
	if err := os.WriteFile(filepath.Join(dir, "package.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestPackage(t, "core", "freshlib", nil)

	_, _, _, deprecations, _, err := ResolvePackageLibs([]string{"oldlib", "freshlib"}, nil, "ee", "my-target")
	if err != nil {
		t.Fatal(err)
	}
	if len(deprecations) != 1 {
		t.Fatalf("deprecations = %v, want exactly 1 (freshlib must not warn)", deprecations)
	}
	want := `my-target: package "oldlib" (library) is deprecated: use newlib instead`
	if deprecations[0] != want {
		t.Errorf("deprecations[0] = %q, want %q", deprecations[0], want)
	}
}

func TestResolvePackageLibs_UnknownNameFallsThrough(t *testing.T) {
	dirs, archives, includeDirs, _, _, err := ResolvePackageLibs([]string{"nonexistent"}, nil, "ee", "test-target")
	if err != nil {
		t.Fatalf("error = %v, want nil - an unresolved name must fall through to a bare -l<name>, not fail the build", err)
	}
	if dirs != nil || archives != nil || includeDirs != nil {
		t.Errorf("got non-nil results for an unresolved name: dirs=%v archives=%v includeDirs=%v", dirs, archives, includeDirs)
	}
}

func TestResolveHeaderIncludeDirs_UnknownPackageErrors(t *testing.T) {
	writeTestPackage(t, "core", "wavekit", nil)

	_, _, err := resolveHeaderIncludeDirs([]string{"wavekit", "audsrv"}, nil, "ee", "my-target")
	if err == nil {
		t.Fatal("error = nil, want an error naming the unresolved headers: entry")
	}
	want := `my-target: headers: "audsrv" is not an installed package or a library target in this workspace`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestUnresolvedLibWarnings(t *testing.T) {
	writeTestPackage(t, "core", "wavekit", nil)
	multiDir := filepath.Join(utils.PackageRoot(), "world", "codec")
	if err := os.MkdirAll(multiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	multi := `name: codec
artifacts:
  - kind: library
    target: [ee]
    lib: lib/libcodec.a
  - kind: library
    target: [ee]
    lib: lib/libcodecfile.a
`
	if err := os.WriteFile(filepath.Join(multiDir, "package.yaml"), []byte(multi), 0o644); err != nil {
		t.Fatal(err)
	}
	local := map[string]libInfo{"mylib": {Archive: "build/libmylib.a", Local: true}}

	tests := []struct {
		name    string
		libs    []string
		ldflags []string
		want    int
	}{
		{"unresolved name warns", []string{"wavekit", "audsrv"}, nil, 1},
		{"toolchain and local libs do not warn", []string{"m", "stdc++", "mylib", "wavekit"}, nil, 0},
		{"an explicit -L suppresses the warning", []string{"audsrv"}, []string{"-L../ee_libs/audsrv/lib"}, 0},
		{"a second archive of a listed package does not warn", []string{"codecfile", "codec"}, nil, 0},
		{"a second archive without its package listed still warns", []string{"codecfile"}, nil, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := unresolvedLibWarnings(tc.libs, tc.ldflags, local, "my-target")
			if len(got) != tc.want {
				t.Errorf("warnings = %v, want %d", got, tc.want)
			}
		})
	}
}

func TestResolvePackageLibs_TransitiveHeadersDeduped(t *testing.T) {
	writeTestPackage(t, "core", "packet", nil)
	writeTestPackage(t, "core", "dma", []string{"packet"})
	writeTestPackage(t, "core", "draw", []string{"packet", "dma"})
	writeTestPackage(t, "core", "graph", []string{"dma", "packet"})

	_, _, includeDirs, _, _, err := ResolvePackageLibs([]string{"draw", "graph"}, nil, "ee", "test-target")
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]int)
	for _, d := range includeDirs {
		seen[d]++
	}
	wantPacket := utils.SdkPackagePath("core", "packet/include")
	wantDMA := utils.SdkPackagePath("core", "dma/include")
	if seen[wantPacket] != 1 {
		t.Errorf("packet's include dir appears %d times (via draw and graph both), want exactly 1: %v", seen[wantPacket], includeDirs)
	}
	if seen[wantDMA] != 1 {
		t.Errorf("dma's include dir appears %d times (via draw and graph both), want exactly 1: %v", seen[wantDMA], includeDirs)
	}
}

func writeTestDriverPackage(t *testing.T, tier, name, target string, headers []string) {
	t.Helper()
	dir := utils.SdkPackagePath(tier, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "name: " + name + "\nartifacts:\n  - kind: driver\n    target: [" + target + "]\n    include_dirs: [include]\n    irx: bin/" + name + ".irx\n"
	if len(headers) > 0 {
		yaml += "    headers: [" + strings.Join(headers, ", ") + "]\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "package.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolvePackageLibs_DriverOnlyPackageResolvesHeadersOnly(t *testing.T) {
	writeTestDriverPackage(t, "core", "loadcore", "iop", nil)

	dirs, archives, includeDirs, _, needsCXX, err := ResolvePackageLibs([]string{"loadcore"}, nil, "iop", "test-target")
	if err != nil {
		t.Fatal(err)
	}
	if archives != nil {
		t.Errorf("archives = %v, want nil - a driver-only package has nothing to link", archives)
	}
	if dirs != nil {
		t.Errorf("dirs = %v, want nil - a driver-only package has no -L dir", dirs)
	}
	if needsCXX {
		t.Error("needsCXX = true, want false for a driver-only package")
	}
	wantInclude := utils.SdkPackagePath("core", "loadcore/include")
	if len(includeDirs) != 1 || includeDirs[0] != wantInclude {
		t.Errorf("includeDirs = %v, want [%q]", includeDirs, wantInclude)
	}
}

func TestResolvePackageLibs_DriverOnlyPackageTransitiveHeaders(t *testing.T) {
	writeTestDriverPackage(t, "core", "loadcore", "iop", nil)
	writeTestDriverPackage(t, "core", "bdm", "iop", []string{"loadcore"})

	_, _, includeDirs, _, _, err := ResolvePackageLibs([]string{"bdm"}, nil, "iop", "test-target")
	if err != nil {
		t.Fatal(err)
	}
	wantBdm := utils.SdkPackagePath("core", "bdm/include")
	wantLoadcore := utils.SdkPackagePath("core", "loadcore/include")
	if !stringsEqual(includeDirs, []string{wantBdm, wantLoadcore}) {
		t.Errorf("includeDirs = %v, want [%q, %q]", includeDirs, wantBdm, wantLoadcore)
	}
}

func TestExpandSources_LiteralPathsPassThroughUnchanged(t *testing.T) {
	got, err := expandSources(testProjectRoot, []string{"src/main.c", "src/util.cpp"})
	if err != nil {
		t.Fatal(err)
	}
	if !stringsEqual(got, []string{"src/main.c", "src/util.cpp"}) {
		t.Errorf("expandSources() = %v, want unchanged input", got)
	}
}

func TestExpandSources_GlobExpandsSortedMatches(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"c.c", "a.c", "b.c"} {
		if err := os.WriteFile(filepath.Join(root, "src", name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A sibling file that shouldn't match the *.c pattern.
	if err := os.WriteFile(filepath.Join(root, "src", "readme.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := expandSources(root, []string{"src/*.c"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"src/a.c", "src/b.c", "src/c.c"}
	if !stringsEqual(got, want) {
		t.Errorf("expandSources() = %v, want %v (sorted, .md excluded)", got, want)
	}
}

func TestExpandSources_GlobMatchingNothingErrors(t *testing.T) {
	root := t.TempDir()
	_, err := expandSources(root, []string{"src/*.c"})
	if err == nil || !strings.Contains(err.Error(), "matched no files") {
		t.Errorf("expandSources() error = %v, want a \"matched no files\" error, not a silent empty result", err)
	}
}

func TestExpandSources_MixedLiteralAndGlob(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"gen1.c", "gen2.c"} {
		if err := os.WriteFile(filepath.Join(root, "src", name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "hand-written.c"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := expandSources(root, []string{"hand-written.c", "src/*.c"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"hand-written.c", "src/gen1.c", "src/gen2.c"}
	if !stringsEqual(got, want) {
		t.Errorf("expandSources() = %v, want %v (literal entry kept in its own position)", got, want)
	}
}

func TestBuild_SourcesGlobProducesCompileSteps(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.c", "b.c"} {
		if err := os.WriteFile(filepath.Join(root, "src", name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := &config.Config{
		Project: "p",
		Targets: []config.Target{
			{Name: "helper", Type: config.TargetIOP, Sources: []string{"src/*.c"}, ImportsLst: "src/imports.lst"},
		},
	}
	config.ApplyDefaults(cfg)
	g, err := Build(cfg, root, "/out", nil, gccToolchain{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var compiledSources []string
	for _, c := range g.Compiles {
		compiledSources = append(compiledSources, c.Source)
	}
	wantA := filepath.ToSlash(filepath.Join(root, "src/a.c"))
	wantB := filepath.ToSlash(filepath.Join(root, "src/b.c"))
	if !slices.Contains(compiledSources, wantA) || !slices.Contains(compiledSources, wantB) {
		t.Errorf("Compiles sources = %v, want both %q and %q present", compiledSources, wantA, wantB)
	}
}

func TestCompilerFor_VSM(t *testing.T) {
	t.Run("ee target: dvp-as", func(t *testing.T) {
		toolchain, extraFlags, err := compilerFor(config.TargetEE, ".vsm")
		if err != nil {
			t.Fatalf("compilerFor(ee, .vsm) error = %v", err)
		}
		if toolchain != dvpAS {
			t.Errorf("compilerFor(ee, .vsm) toolchain = %q, want %q", toolchain, dvpAS)
		}
		if extraFlags != nil {
			t.Errorf("compilerFor(ee, .vsm) extraFlags = %v, want nil", extraFlags)
		}
	})

	for _, tt := range []config.TargetType{config.TargetEELib, config.TargetIOP, config.TargetIOPLib} {
		t.Run(string(tt)+" target: unsupported", func(t *testing.T) {
			_, _, err := compilerFor(tt, ".vsm")
			if err == nil {
				t.Errorf("compilerFor(%q, .vsm) error = nil, want an unsupported-extension error", tt)
			}
		})
	}
}

func TestBuild_VSMSource(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Targets: []config.Target{
			{Name: "main", Type: config.TargetEE, Sources: []string{"main.c", "vu1.vsm"}, Opt: "-O2", Defines: []string{"FOO"}},
		},
	}
	config.ApplyDefaults(cfg)
	g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var vsmCompile *CompileStep
	for i := range g.Compiles {
		if g.Compiles[i].Toolchain == dvpAS {
			vsmCompile = &g.Compiles[i]
		}
	}
	if vsmCompile == nil {
		t.Fatalf("no CompileStep found with Toolchain %q among %+v", dvpAS, g.Compiles)
	}
	if len(vsmCompile.FlagArgs) != 0 {
		t.Errorf("vsm CompileStep.FlagArgs = %v, want empty - dvp-as doesn't understand this target's C compile flags (-D_EE, -I, -O2, ...)", vsmCompile.FlagArgs)
	}
	if vsmCompile.Object != "obj/main/vu1.vsm.o" {
		t.Errorf("vsm CompileStep.Object = %q, want %q (extension kept, unlike .c/.cpp/.s/.S)", vsmCompile.Object, "obj/main/vu1.vsm.o")
	}
	if !vsmCompile.Assembler {
		t.Errorf("vsm CompileStep.Assembler = false, want true - internal/ninja and internal/compiledb key off this to skip -c/isystem/_rsp handling meant for C compilers")
	}

	if len(g.Links) != 1 {
		t.Fatalf("len(Links) = %d, want 1", len(g.Links))
	}
	if !slices.Contains(g.Links[0].Objects, vsmCompile.Object) {
		t.Errorf("Links[0].Objects = %v, want it to include the vsm object %q", g.Links[0].Objects, vsmCompile.Object)
	}
}

func TestBuild_VSMSourceSharesBasenameWithC(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Targets: []config.Target{
			{Name: "client", Type: config.TargetEE, Sources: []string{"src/ps2_vu1_probe.c", "src/ps2_vu1_probe.vsm"}, Opt: "-O2"},
		},
	}
	config.ApplyDefaults(cfg)
	g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	objects := make(map[string]int)
	for _, c := range g.Compiles {
		objects[c.Object]++
	}
	if len(objects) != 2 {
		t.Fatalf("distinct CompileStep.Object values = %v, want 2 (one .c file and one .vsm file must never collide)", objects)
	}
	for obj, count := range objects {
		if count != 1 {
			t.Errorf("object %q produced by %d CompileSteps, want exactly 1 (ninja errors on \"multiple rules generate\")", obj, count)
		}
	}
	if objects["obj/client/src/ps2_vu1_probe.o"] != 1 {
		t.Errorf("Compiles = %+v, want the .c source to still produce obj/client/src/ps2_vu1_probe.o unchanged", g.Compiles)
	}
	if objects["obj/client/src/ps2_vu1_probe.vsm.o"] != 1 {
		t.Errorf("Compiles = %+v, want the .vsm source to produce obj/client/src/ps2_vu1_probe.vsm.o", g.Compiles)
	}

	if len(g.Links) != 1 {
		t.Fatalf("len(Links) = %d, want 1", len(g.Links))
	}
	for _, want := range []string{"obj/client/src/ps2_vu1_probe.o", "obj/client/src/ps2_vu1_probe.vsm.o"} {
		if !slices.Contains(g.Links[0].Objects, want) {
			t.Errorf("Links[0].Objects = %v, want it to include %q", g.Links[0].Objects, want)
		}
	}
}

func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuild_ImportsLstExportsTab(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Targets: []config.Target{
			{
				Name:       "mymod",
				Type:       config.TargetIOP,
				Sources:    []string{"main.c"},
				ImportsLst: "imports.lst",
				ExportsTab: "exports.tab",
				Opt:        "-Os",
			},
		},
	}

	g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(g.IETableStubs) != 2 {
		t.Fatalf("len(IETableStubs) = %d, want 2 (imports + exports)", len(g.IETableStubs))
	}

	var importsStub, exportsStub *IETableStubStep
	for i := range g.IETableStubs {
		switch g.IETableStubs[i].Header {
		case "irx_imports.h":
			importsStub = &g.IETableStubs[i]
		case "irx.h":
			exportsStub = &g.IETableStubs[i]
		}
	}
	if importsStub == nil {
		t.Fatal("no IETableStubStep with Header \"irx_imports.h\" (imports_lst) found")
	}
	if exportsStub == nil {
		t.Fatal("no IETableStubStep with Header \"irx.h\" (exports_tab) found")
	}
	if importsStub.SourceFile != "/project/imports.lst" {
		t.Errorf("importsStub.SourceFile = %q, want %q", importsStub.SourceFile, "/project/imports.lst")
	}
	if exportsStub.SourceFile != "/project/exports.tab" {
		t.Errorf("exportsStub.SourceFile = %q, want %q", exportsStub.SourceFile, "/project/exports.tab")
	}

	// Both generated .c files must compile with -fno-toplevel-reorder.
	var importsCompile, exportsCompile *CompileStep
	for i := range g.Compiles {
		switch g.Compiles[i].Source {
		case importsStub.COutPath.String():
			importsCompile = &g.Compiles[i]
		case exportsStub.COutPath.String():
			exportsCompile = &g.Compiles[i]
		}
	}
	if importsCompile == nil || !slices.Contains(importsCompile.FlagArgs, "-fno-toplevel-reorder") {
		t.Errorf("imports stub CompileStep FlagArgs = %v, want it to contain -fno-toplevel-reorder", importsCompile)
	}
	if exportsCompile == nil || !slices.Contains(exportsCompile.FlagArgs, "-fno-toplevel-reorder") {
		t.Errorf("exports stub CompileStep FlagArgs = %v, want it to contain -fno-toplevel-reorder", exportsCompile)
	}

	// exports.o must be linked first (the loader must find the real
	// export table before any other data that might resemble one).
	if len(g.Links) != 1 {
		t.Fatalf("len(Links) = %d, want 1", len(g.Links))
	}
	link := g.Links[0]
	if len(link.Objects) < 3 {
		t.Fatalf("Links[0].Objects = %v, want at least 3 (exports.o, main.o, imports.o)", link.Objects)
	}
	if link.Objects[0] != "obj/mymod/exports.o" {
		t.Errorf("Links[0].Objects[0] = %q, want %q (exports.o linked first)", link.Objects[0], "obj/mymod/exports.o")
	}
	if link.Objects[len(link.Objects)-1] != "obj/mymod/imports.o" {
		t.Errorf("Links[0].Objects[last] = %q, want %q (imports.o)", link.Objects[len(link.Objects)-1], "obj/mymod/imports.o")
	}

	// A real export table means --allow-zero-text must NOT be added.
	if len(g.Fixups) != 1 {
		t.Fatalf("len(Fixups) = %d, want 1", len(g.Fixups))
	}
	if slices.Contains(g.Fixups[0].FlagArgs, "--allow-zero-text") {
		t.Errorf("Fixups[0].FlagArgs = %v, should not contain --allow-zero-text (exports_tab is set)", g.Fixups[0].FlagArgs)
	}
}

func TestBuild_ImportsLstOnly_AllowZeroTextStillAdded(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Targets: []config.Target{
			{Name: "mymod", Type: config.TargetIOP, Sources: []string{"main.c"}, ImportsLst: "imports.lst", Opt: "-Os"},
		},
	}

	g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(g.IETableStubs) != 1 {
		t.Fatalf("len(IETableStubs) = %d, want 1 (imports only)", len(g.IETableStubs))
	}
	if !slices.Contains(g.Fixups[0].FlagArgs, "--allow-zero-text") {
		t.Errorf("Fixups[0].FlagArgs = %v, want it to contain --allow-zero-text (no exports_tab)", g.Fixups[0].FlagArgs)
	}
}

func TestBuild_ExternalLibsOverriddenBySameFileTarget(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Targets: []config.Target{
			{Name: "mylib", Type: config.TargetEELib, Sources: []string{"local.c"}},
			{Name: "main", Type: config.TargetEE, Sources: []string{"main.c"}, Libs: []string{"mylib"}},
		},
	}
	external := map[string]ExternalLib{
		"mylib": {Archive: "some/other/member/lib/libmylib.a", NeedsCXX: true},
	}

	g, err := Build(cfg, "/project", "/out", external, gccToolchain{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(g.Links) != 1 {
		t.Fatalf("len(Links) = %d, want 1", len(g.Links))
	}
	link := g.Links[0]
	for _, dep := range link.LibDeps {
		if dep == "some/other/member/lib/libmylib.a" {
			t.Errorf("LinkStep.LibDeps = %v, contains the externalLibs archive - the same-file target should have overridden it entirely", link.LibDeps)
		}
	}
	if len(link.LibDeps) != 1 || link.LibDeps[0] != "lib/libmylib.a" {
		t.Errorf("LinkStep.LibDeps = %v, want [\"lib/libmylib.a\"] (the same-file target's own archive)", link.LibDeps)
	}

	if link.Toolchain != eeGCC {
		t.Errorf("Toolchain = %q, want %q - externalLibs' NeedsCXX:true must not leak through when the same-file target overrides it", link.Toolchain, eeGCC)
	}
}

func TestImplicitStartupLibs(t *testing.T) {
	t.Run("same-file startup target: resolves without touching disk", func(t *testing.T) {
		libsByName := map[string]libInfo{"startup": {Archive: "lib/libstartup.a", Local: true}}
		got := implicitStartupLibs(libsByName, "ee")
		if want := []string{"startup"}; !stringsEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("installed startup package for the requested target: resolves", func(t *testing.T) {
		writeTestPackage(t, "core", "startup", nil)
		got := implicitStartupLibs(nil, "ee")
		if want := []string{"startup"}; !stringsEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("installed startup package for a DIFFERENT target: nil", func(t *testing.T) {
		// writeTestPackage always writes target: [ee] - asking for "iop"
		// must not match it.
		writeTestPackage(t, "core", "startup", nil)
		got := implicitStartupLibs(nil, "iop")
		if got != nil {
			t.Errorf("got %v, want nil (package only offers an ee artifact)", got)
		}
	})
}

func TestBuild_EEStrip(t *testing.T) {
	t.Run("release (default): stripped, link writes an intermediate", func(t *testing.T) {
		cfg := &config.Config{
			Project: "p",
			Targets: []config.Target{
				{Name: "main", Type: config.TargetEE, Sources: []string{"main.c"}},
			},
		}
		g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if len(g.Strips) != 1 {
			t.Fatalf("len(Strips) = %d, want 1", len(g.Strips))
		}
		s := g.Strips[0]
		if s.Toolchain != eeStrip {
			t.Errorf("Strips[0].Toolchain = %q, want %q", s.Toolchain, eeStrip)
		}
		if want := []string{"--strip-all"}; !stringsEqual(s.FlagArgs, want) {
			t.Errorf("Strips[0].FlagArgs = %v, want %v", s.FlagArgs, want)
		}
		if s.Output != "bin/main.elf" {
			t.Errorf("Strips[0].Output = %q, want %q (the final target output)", s.Output, "bin/main.elf")
		}
		if g.Links[0].Output != s.Input {
			t.Errorf("Links[0].Output = %q, want it to feed Strips[0].Input = %q", g.Links[0].Output, s.Input)
		}
		if g.Links[0].Output == "bin/main.elf" {
			t.Errorf("Links[0].Output = %q, a release build must link to an intermediate, not the final path directly", g.Links[0].Output)
		}
		if g.TargetOutputs["main"] != "bin/main.elf" {
			t.Errorf(`TargetOutputs["main"] = %q, want "bin/main.elf" (the stripped output)`, g.TargetOutputs["main"])
		}
	})

	t.Run("debug: not stripped, link writes the final .elf directly", func(t *testing.T) {
		cfg := &config.Config{
			Project: "p",
			Targets: []config.Target{
				{Name: "main", Type: config.TargetEE, Sources: []string{"main.c"}, Debug: true},
			},
		}
		g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if len(g.Strips) != 0 {
			t.Fatalf("len(Strips) = %d, want 0 (a debug build must not strip its own debug info back off)", len(g.Strips))
		}
		if g.Links[0].Output != "bin/main.elf" {
			t.Errorf("Links[0].Output = %q, want %q directly (no strip step to hand off to)", g.Links[0].Output, "bin/main.elf")
		}
		if g.TargetOutputs["main"] != "bin/main.elf" {
			t.Errorf(`TargetOutputs["main"] = %q, want "bin/main.elf"`, g.TargetOutputs["main"])
		}
	})

	t.Run("IOP targets are unaffected: always stripped via iopStrip regardless of Debug", func(t *testing.T) {
		cfg := &config.Config{
			Project: "p",
			Targets: []config.Target{
				{Name: "mod", Type: config.TargetIOP, Sources: []string{"main.c"}},
			},
		}
		g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if len(g.Strips) != 1 || g.Strips[0].Toolchain != iopStrip {
			t.Fatalf("Strips = %+v, want exactly one iopStrip step, unrelated to the new EE-only gating", g.Strips)
		}
	})
}

func TestImplicitKernelHeaders(t *testing.T) {
	t.Run("kernel not installed: unchanged", func(t *testing.T) {
		got := implicitKernelHeaders([]string{"cglue"}, nil)
		if want := []string{"cglue"}; !stringsEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("kernel installed: appended", func(t *testing.T) {
		writeTestPackage(t, "core", "kernel", nil)
		got := implicitKernelHeaders([]string{"cglue"}, nil)
		if want := []string{"cglue", "kernel"}; !stringsEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("already listed: unchanged", func(t *testing.T) {
		writeTestPackage(t, "core", "kernel", nil)
		got := implicitKernelHeaders([]string{"kernel"}, nil)
		if want := []string{"kernel"}; !stringsEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

func TestBuild_KernelHeadersNotLibrary(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Targets: []config.Target{
			{Name: "main", Type: config.TargetEE, Sources: []string{"main.c"}, Libs: []string{"png"}},
		},
	}
	writeTestPackage(t, "core", "kernel", nil)

	g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if want := []string{"startup", "png"}; !stringsEqual(g.Links[0].Libs, want) {
		t.Errorf("Links[0].Libs = %v, want %v (kernel must not be linked ahead of libc)", g.Links[0].Libs, want)
	}
	found := false
	for _, c := range g.Compiles {
		for _, a := range c.FlagArgs {
			if strings.HasSuffix(filepath.ToSlash(a), "core/kernel/include") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("no -I for the kernel package's include dir in %+v", g.Compiles)
	}
}

func TestBuild_LibTarget(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Targets: []config.Target{
			{Name: "mymath", Type: config.TargetEELib, Sources: []string{"mymath.c"}, Opt: "-O2"},
			{Name: "main", Type: config.TargetEE, Sources: []string{"main.c"}, Libs: []string{"mymath", "png"}, Opt: "-O2"},
		},
	}

	g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(g.Archives) != 1 {
		t.Fatalf("len(Archives) = %d, want 1", len(g.Archives))
	}
	if g.Archives[0].Output != "lib/libmymath.a" {
		t.Errorf("Archives[0].Output = %q, want %q", g.Archives[0].Output, "lib/libmymath.a")
	}
	if g.TargetOutputs["mymath"] != "lib/libmymath.a" {
		t.Errorf(`TargetOutputs["mymath"] = %q, want %q`, g.TargetOutputs["mymath"], "lib/libmymath.a")
	}

	if len(g.Links) != 1 {
		t.Fatalf("len(Links) = %d, want 1", len(g.Links))
	}
	link := g.Links[0]

	if want := []string{"startup", "mymath", "png"}; !stringsEqual(link.Libs, want) {
		t.Errorf("Links[0].Libs = %v, want %v", link.Libs, want)
	}
	if want := []string{"lib/libmymath.a"}; !stringsEqual(link.LibDeps, want) {
		t.Errorf("Links[0].LibDeps = %v, want %v", link.LibDeps, want)
	}
	if want := []string{"lib"}; !stringsEqual(link.LocalLibDirs, want) {
		t.Errorf("Links[0].LocalLibDirs = %v, want %v (the emitter renders these as -L flags, ahead of FlagArgs - see LinkStep's doc comment)", link.LocalLibDirs, want)
	}
	if link.Toolchain != eeGCC {
		t.Errorf("Links[0].Toolchain = %q, want %q (a plain-C target linking a plain-C lib)", link.Toolchain, eeGCC)
	}
}

func TestBuild_CXXLinkDriverSelection(t *testing.T) {
	t.Run("target's own C++ sources select the C++ link driver", func(t *testing.T) {
		cfg := &config.Config{
			Project: "p",
			Targets: []config.Target{
				{Name: "main", Type: config.TargetEE, Sources: []string{"main.cpp"}, Opt: "-O2"},
			},
		}
		g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if g.Links[0].Toolchain != eeGXX {
			t.Errorf("Links[0].Toolchain = %q, want %q", g.Links[0].Toolchain, eeGXX)
		}
	})

	t.Run("a plain-C target linking a C++ ee_lib also selects the C++ link driver", func(t *testing.T) {
		cfg := &config.Config{
			Project: "p",
			Targets: []config.Target{
				{Name: "hashlib", Type: config.TargetEELib, Sources: []string{"hash.cpp"}, Opt: "-O2"},
				{Name: "main", Type: config.TargetEE, Sources: []string{"main.c"}, Libs: []string{"hashlib"}, Opt: "-O2"},
			},
		}
		g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		var mainLink *LinkStep
		for i := range g.Links {
			if g.Links[i].TargetName == "main" {
				mainLink = &g.Links[i]
			}
		}
		if mainLink == nil {
			t.Fatal("no link step found for target \"main\"")
		}
		if mainLink.Toolchain != eeGXX {
			t.Errorf("main's Links[].Toolchain = %q, want %q (references a C++ ee_lib)", mainLink.Toolchain, eeGXX)
		}
	})

	t.Run("all-C target linking an all-C lib keeps the plain-C link driver", func(t *testing.T) {
		cfg := &config.Config{
			Project: "p",
			Targets: []config.Target{
				{Name: "utillib", Type: config.TargetEELib, Sources: []string{"util.c"}, Opt: "-O2"},
				{Name: "main", Type: config.TargetEE, Sources: []string{"main.c"}, Libs: []string{"utillib"}, Opt: "-O2"},
			},
		}
		g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		var mainLink *LinkStep
		for i := range g.Links {
			if g.Links[i].TargetName == "main" {
				mainLink = &g.Links[i]
			}
		}
		if mainLink == nil {
			t.Fatal("no link step found for target \"main\"")
		}
		if mainLink.Toolchain != eeGCC {
			t.Errorf("main's Links[].Toolchain = %q, want %q", mainLink.Toolchain, eeGCC)
		}
	})
}

func TestBuild_PathsAreForwardSlash(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Targets: []config.Target{
			{Name: "main", Type: config.TargetEE, Sources: []string{"src/main.c"}, Opt: "-O2"},
		},
	}

	// Even with an OS-native-looking (backslash) projectRoot/outDir, every
	// Step path must come out forward-slash (see Build's doc comment).
	g, err := Build(cfg, `C:/project`, `C:/out`, nil, gccToolchain{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, c := range g.Compiles {
		if strings.Contains(c.Object, `\`) || strings.Contains(c.Source, `\`) {
			t.Errorf("CompileStep has a backslash: Source=%q Object=%q", c.Source, c.Object)
		}
	}
	for _, l := range g.Links {
		if strings.Contains(l.Output, `\`) {
			t.Errorf("LinkStep.Output has a backslash: %q", l.Output)
		}
	}
}

func TestCompilerFor_VCL(t *testing.T) {
	toolchain, extraFlags, err := compilerFor(config.TargetEE, ".vcl")
	if err != nil {
		t.Fatalf("compilerFor(ee, .vcl) error = %v", err)
	}
	if toolchain != vclTool || extraFlags != nil {
		t.Errorf("compilerFor(ee, .vcl) = %q, %v; want %q, nil", toolchain, extraFlags, vclTool)
	}
	for _, tt := range []config.TargetType{config.TargetEELib, config.TargetIOP, config.TargetIOPLib} {
		if _, _, err := compilerFor(tt, ".vcl"); err == nil {
			t.Errorf("compilerFor(%q, .vcl) error = nil, want an unsupported-extension error", tt)
		}
	}
}

func TestBuild_VCLSource(t *testing.T) {
	cfg := &config.Config{
		Project: "p",
		Targets: []config.Target{
			{Name: "main", Type: config.TargetEE, Sources: []string{"main.c", "vu1.vcl"}, Opt: "-O2", Defines: []string{"FOO"}, IncludeDirs: []string{"inc"}},
		},
	}
	config.ApplyDefaults(cfg)
	g, err := Build(cfg, "/project", "/out", nil, gccToolchain{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var step *CompileStep
	for i := range g.Compiles {
		if g.Compiles[i].Toolchain == vclTool {
			step = &g.Compiles[i]
		}
	}
	if step == nil {
		t.Fatalf("no CompileStep with Toolchain %q among %+v", vclTool, g.Compiles)
	}
	if !step.VCL || step.Assembler {
		t.Errorf("VCL = %v, Assembler = %v; want true, false", step.VCL, step.Assembler)
	}
	if step.Object != "obj/main/vu1.vcl.o" {
		t.Errorf("Object = %q, want obj/main/vu1.vcl.o", step.Object)
	}
	if !slices.Contains(step.FlagArgs, "-DFOO") || !slices.Contains(step.FlagArgs, "-I"+utils.AbsPath("/project", "inc")) {
		t.Errorf("FlagArgs = %v, want -DFOO and the target include dir", step.FlagArgs)
	}
	for _, f := range step.FlagArgs {
		if f == "-O2" || f == "-Wall" || f == "-G0" {
			t.Errorf("FlagArgs = %v contains gcc-only flag %q", step.FlagArgs, f)
		}
	}
	if !slices.Contains(g.Links[0].Objects, step.Object) {
		t.Errorf("link objects %v missing %q", g.Links[0].Objects, step.Object)
	}
}
