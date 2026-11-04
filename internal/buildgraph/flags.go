package buildgraph

import (
	"fmt"
	"path"

	"github.com/ps2build/ps2build/internal/config"
	"github.com/ps2build/ps2build/internal/pkgmanifest"
	"github.com/ps2build/ps2build/internal/utils"
)

var (
	eeGCC     = "mips64r5900el-ps2-elf-gcc"
	eeGXX     = "mips64r5900el-ps2-elf-g++"
	eeObjcopy = "mips64r5900el-ps2-elf-objcopy"
	eeAR      = "mips64r5900el-ps2-elf-ar"
	eeStrip   = "mips64r5900el-ps2-elf-strip"

	iopGCC   = "mipsel-none-elf-gcc"
	iopStrip = "mipsel-none-elf-strip"
	iopAR    = "mipsel-none-elf-ar"

	dvpAS   = "dvp-as"
	vclTool = "vcl"

	// Only the llvm toolchain links with these: it calls lld directly instead of going through the compiler driver.
	eeLD  = "mips64r5900el-ps2-elf-ld"
	iopLD = "mipsel-none-elf-ld"
)

func eeCompileFlags(g *Graph, t config.Target, projectRoot string, pkgIncludeDirs []string) []string {
	tc := g.Toolchain
	flags := append(append([]string{}, tc.EECompileArgs()...), "-D_EE", "-G0", t.Opt, "-Wall", "-ffunction-sections")
	if t.Debug {
		flags = append(flags, "-gdwarf-2", "-gz")
	}
	for _, d := range t.Defines {
		flags = append(flags, "-D"+d)
	}
	inc := tc.IncludeFlag()
	for _, dir := range tc.SystemIncludeDirs("ee") {
		flags = append(flags, inc+dir)
	}
	flags = append(flags, inc+commonIncludeDir("ee"))
	for _, d := range t.IncludeDirs {
		flags = append(flags, "-I"+utils.AbsPath(projectRoot, d))
	}
	for _, d := range pkgIncludeDirs {
		flags = append(flags, inc+d)
	}
	flags = append(flags, g.Toolchain.AdaptCFlags(t.CFlags)...)
	return flags
}

func iopCompileFlags(g *Graph, t config.Target, projectRoot string, pkgIncludeDirs []string) []string {
	tc := g.Toolchain
	flags := append(append([]string{}, tc.IOPCompileArgs()...), "-D_IOP", "-fno-builtin", "-G0", t.Opt, "-Wall")
	flags = append(flags, tc.IOPCodegenFlags()...)
	if t.Debug {
		flags = append(flags, "-gdwarf-2", "-gz")
	}
	for _, d := range t.Defines {
		flags = append(flags, "-D"+d)
	}
	inc := tc.IncludeFlag()
	for _, dir := range tc.SystemIncludeDirs("iop") {
		flags = append(flags, inc+dir)
	}
	flags = append(flags, inc+commonIncludeDir("iop"))
	for _, d := range t.IncludeDirs {
		flags = append(flags, "-I"+utils.AbsPath(projectRoot, d))
	}
	for _, d := range pkgIncludeDirs {
		flags = append(flags, inc+d)
	}
	flags = append(flags, g.Toolchain.AdaptCFlags(t.CFlags)...)
	return flags
}

func eeLinkFlags(g *Graph, t config.Target, extraLibDirs []string, needsCXX bool) []string {
	tc := g.Toolchain
	flags := tc.EELinkFlags(t.Opt)
	for _, dir := range extraLibDirs {
		flags = append(flags, "-L"+dir)
	}
	for _, dir := range defaultEELibDirs() {
		flags = append(flags, "-L"+dir)
	}
	for _, dir := range tc.EELinkLibDirs() {
		flags = append(flags, "-L"+dir)
	}
	return append(flags, tc.EELinkExtra(t.LDFlags, needsCXX)...)
}

func defaultEELibDirs() []string {
	packageRoot := utils.PackageRoot()
	if packageRoot == "" {
		return nil
	}
	var dirs []string
	for _, name := range []string{"cdvd", "pthreadglue", "cglue", "kernel", "profglue"} {
		m, ok, err := pkgmanifest.Find(packageRoot, name)
		if err != nil || !ok {
			continue
		}
		a, ok := m.LibraryArtifact("ee")
		if !ok || a.Lib == "" {
			continue
		}
		dirs = append(dirs, path.Dir(m.AbsLib(a)))
	}
	return dirs
}

func iopLinkFlags(g *Graph, t config.Target, extraLibDirs []string) []string {
	tc := g.Toolchain
	flags := tc.IOPLinkFlags(t.Opt)
	for _, dir := range extraLibDirs {
		flags = append(flags, "-L"+dir)
	}
	for _, dir := range tc.IOPLinkLibDirs() {
		flags = append(flags, "-L"+dir)
	}
	return append(flags, tc.IOPLinkExtra(t.LDFlags)...)
}

func iopStripFlags() []string {
	return []string{
		"--strip-unneeded",
		"--remove-section=.pdr",
		"--remove-section=.comment",
		"--remove-section=.mdebug.abi32",
		"--remove-section=.gnu.attributes",
	}
}

func eeStripFlags() []string {
	return []string{"--strip-all"}
}

func resolveLinkfile(g *Graph, t config.Target, projectRoot string) string {
	return g.Toolchain.Linkfile(resolveDefaultLinkfile(t, projectRoot))
}

func resolveDefaultLinkfile(t config.Target, projectRoot string) string {
	if t.Linkfile != "" {
		return utils.AbsPath(projectRoot, t.Linkfile)
	}
	var processor string
	switch t.Type {
	case config.TargetEE:
		processor = "ee"
	case config.TargetIOP:
		processor = "iop"
	default:
		panic(fmt.Sprintf("resolveLinkfile: unknown target type %q", t.Type))
	}

	if lf := startupPackageLinkfile(processor); lf != "" {
		return lf
	}
	return utils.SdkPackagePath("core", "startup/share/"+processor+"/linkfile")
}

func startupPackageLinkfile(processor string) string {
	packageRoot := utils.PackageRoot()
	if packageRoot == "" {
		return ""
	}
	m, ok, err := pkgmanifest.Find(packageRoot, "startup")
	if err != nil || !ok {
		return ""
	}
	a, ok := m.LibraryArtifact(processor)
	if !ok || a.Linkfile == "" {
		return ""
	}
	return m.AbsLinkfile(a)
}

func commonIncludeDir(processor string) string {
	packageRoot := utils.PackageRoot()
	if packageRoot != "" {
		if m, ok, err := pkgmanifest.Find(packageRoot, "common"); err == nil && ok {
			if a, ok := m.HeadersArtifact(processor); ok && len(a.IncludeDirs) > 0 {
				return m.AbsIncludeDirs(a)[0]
			}
		}
	}
	panic("Common include directory not found. Try reinstalling ps2build")
}
