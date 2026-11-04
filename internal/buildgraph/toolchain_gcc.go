package buildgraph

import (
	"os"
	"path"

	"github.com/ps2build/ps2build/internal/utils"
)

// gccToolchain drives the GNU toolchain installed under <root>/toolchain.
type gccToolchain struct{}

func (gccToolchain) Kind() ToolchainKind { return ToolchainGCC }

func (gccToolchain) Paths() map[string]string {
	paths := make(map[string]string, 12)
	resolve := func(name, bindir string) {
		paths[name] = path.Join(bindir, utils.ExeSuffix(name))
	}

	sdkRoot := utils.DetectSDKRoot()

	eeBin := path.Join(sdkRoot, "toolchain/ee/bin")
	resolve(eeGCC, eeBin)
	resolve(eeGXX, eeBin)
	resolve(eeObjcopy, eeBin)
	resolve(eeAR, eeBin)
	resolve(eeStrip, eeBin)

	iopBin := path.Join(sdkRoot, "toolchain/iop/bin")
	resolve(iopGCC, iopBin)
	resolve(iopStrip, iopBin)
	resolve(iopAR, iopBin)

	addHostTools(paths, sdkRoot)
	return paths
}

// Verify reports a missing install.
func (gccToolchain) Verify() error {
	root := path.Join(utils.DetectSDKRoot(), "toolchain")
	if _, err := os.Stat(path.Join(root, "ee/bin")); err != nil {
		return &NotInstalledError{Kind: ToolchainGCC, Component: "toolchain", Root: root}
	}
	return nil
}

func (gccToolchain) EECompileArgs() []string  { return nil }
func (gccToolchain) IOPCompileArgs() []string { return nil }
func (gccToolchain) IncludeFlag() string      { return "-I" }

func (gccToolchain) SystemIncludeDirs(processor string) []string {
	return []string{path.Join(utils.DetectSDKRoot(), "toolchain", processor, "include")}
}

// -msoft-float/-mno-explicit-relocs: required for correct IOP codegen with GCC >= 5.3.0.
func (gccToolchain) IOPCodegenFlags() []string {
	return []string{"-msoft-float", "-mno-explicit-relocs"}
}

func (gccToolchain) AdaptCFlags(cflags []string) []string { return cflags }

func (gccToolchain) CompileExtraFlags(ext string) []string { return nil }

// -fno-toplevel-reorder: without it GCC may reorder DECLARE_IMPORT_TABLE's asm statements, corrupting the table.
func (gccToolchain) IETableStubFlags() []string { return []string{"-fno-toplevel-reorder"} }

func (gccToolchain) EELinkTool(cxx bool) string {
	if cxx {
		return eeGXX
	}
	return eeGCC
}

func (gccToolchain) IOPLinkTool() string   { return iopGCC }
func (gccToolchain) EELinkTrailer() string { return "" }

// -nostartfiles: crt0.o lives inside libstartup.a, not a toolchain startfile dir.
func (gccToolchain) EELinkFlags(opt string) []string {
	return []string{opt, "-Wl,--gc-sections", "-nostartfiles"}
}

func (gccToolchain) EELinkLibDirs() []string {
	return []string{path.Join(utils.DetectSDKRoot(), "toolchain/ee/lib")}
}

func (gccToolchain) EELinkExtra(ldflags []string, needsCXX bool) []string { return ldflags }

// -nostdlib -dc -r: IOP modules are relocatable, not linked like normal executables.
func (gccToolchain) IOPLinkFlags(opt string) []string {
	return []string{opt, "-nostdlib", "-dc", "-r"}
}

func (gccToolchain) IOPLinkLibDirs() []string                 { return nil }
func (gccToolchain) IOPLinkExtra(ldflags []string) []string   { return ldflags }
func (gccToolchain) MapLibs(libs []string, _ string) []string { return libs }
func (gccToolchain) Linkfile(defaultPath string) string       { return defaultPath }

func (gccToolchain) IOPFixupFlags(hasExportsTab bool) []string {
	flags := []string{"--rb", "--irx1"}
	if !hasExportsTab {
		flags = append(flags, "--allow-zero-text")
	}
	return flags
}
