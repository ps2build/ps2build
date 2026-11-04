package buildgraph

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ps2build/ps2build/internal/utils"
)

const (
	// LLVMRootEnvVar points at an llvm-toolchain tree other than the installed component, for toolchain developers.
	LLVMRootEnvVar = "PS2BUILD_TOOLCHAIN_ROOT"

	llvmComponentDir = "llvm-toolchain"

	eeSysrootName  = "mips64r5900el-ps2-elf"
	iopSysrootName = "mipsel-none-elf"

	eeBuiltinsLib  = "clang_rt.builtins-mips64el"
	iopBuiltinsLib = "clang_rt.builtins-mipsel"
)

// llvmToolchain drives clang and lld from the llvm-toolchain component.
type llvmToolchain struct {
	root string
}

func newLLVMToolchain() (Toolchain, error) {
	root := os.Getenv(LLVMRootEnvVar)
	if root == "" {
		root = path.Join(utils.DetectSDKRoot(), llvmComponentDir)
	}
	return llvmToolchain{root: filepath.ToSlash(root)}, nil
}

func (llvmToolchain) Kind() ToolchainKind { return ToolchainLLVM }

// Verify reports a missing install.
func (t llvmToolchain) Verify() error {
	if _, err := os.Stat(t.eeBin()); err != nil {
		return &NotInstalledError{Kind: ToolchainLLVM, Component: "llvm-toolchain", Root: t.root}
	}
	return nil
}

func (t llvmToolchain) eeBin() string  { return path.Join(t.root, "ee/bin") }
func (t llvmToolchain) iopBin() string { return path.Join(t.root, "iop/bin") }

func (t llvmToolchain) eeSysroot() string  { return path.Join(t.root, "ee", eeSysrootName) }
func (t llvmToolchain) iopSysroot() string { return path.Join(t.root, "iop", iopSysrootName) }

// llvmBin finds the shared LLVM binaries: llvm/bin in a release, llvm-<revision>/bin in a developer build.
func (t llvmToolchain) llvmBin() string {
	if _, err := os.Stat(path.Join(t.root, "llvm/bin")); err == nil {
		return path.Join(t.root, "llvm/bin")
	}
	if matches, _ := filepath.Glob(filepath.Join(t.root, "llvm-*", "bin")); len(matches) > 0 {
		return filepath.ToSlash(matches[0])
	}
	return path.Join(t.root, "llvm/bin")
}

// wrapper is the per-target command next to the compiler configs: a .bat on Windows, or an .exe where the install ships one.
func wrapper(bindir, name string) string {
	p := path.Join(bindir, name)
	if utils.ExeSuffixValue == "" {
		return p
	}
	if utils.FileExists(p + ".exe") {
		return p + ".exe"
	}
	return p + ".bat"
}

func (t llvmToolchain) Paths() map[string]string {
	paths := make(map[string]string, 14)

	eePrefix := eeSysrootName + "-"
	paths[eeGCC] = wrapper(t.eeBin(), eePrefix+"clang")
	paths[eeGXX] = paths[eeGCC] + " --driver-mode=g++"
	paths[eeObjcopy] = wrapper(t.eeBin(), eePrefix+"objcopy")
	paths[eeAR] = wrapper(t.eeBin(), eePrefix+"ar")
	paths[eeStrip] = wrapper(t.eeBin(), eePrefix+"strip")

	iopPrefix := iopSysrootName + "-"
	paths[iopGCC] = wrapper(t.iopBin(), iopPrefix+"clang")
	paths[iopAR] = wrapper(t.iopBin(), iopPrefix+"ar")
	paths[iopStrip] = wrapper(t.iopBin(), iopPrefix+"strip")

	lld := path.Join(t.llvmBin(), utils.ExeSuffix("ld.lld"))
	paths[eeLD] = lld
	paths[iopLD] = lld

	addHostTools(paths, utils.DetectSDKRoot())
	return paths
}

func (t llvmToolchain) EECompileArgs() []string {
	return []string{
		"--target=mips64el-unknown-elf", "-mcpu=r5900", "-mabi=n32", "-fno-pic",
		"-D__ps2sdk__", "-D_GNU_SOURCE",
		"-isystem", path.Join(t.eeSysroot(), "include/c++/v1"),
		"-isystem", path.Join(t.eeSysroot(), "include"),
	}
}

func (t llvmToolchain) IOPCompileArgs() []string {
	return []string{"--target=mipsel-unknown-elf", "-mcpu=r3000a", "-mabi=32", "-mno-abicalls"}
}

// clang searches -I directories before any -isystem one whatever the command-line order, so package headers
// go in as -isystem to avoid shadowing a libc++ wrapper header that shares a basename.
func (llvmToolchain) IncludeFlag() string { return "-isystem" }

func (llvmToolchain) SystemIncludeDirs(string) []string { return nil }

// -ffreestanding stops clang turning loops into calls to memset/memcpy, which IOP modules have no libc to provide.
func (llvmToolchain) IOPCodegenFlags() []string { return []string{"-msoft-float", "-ffreestanding"} }

// -mmemcpy and -mno-memcpy are GCC MIPS options; clang rejects them.
func (llvmToolchain) AdaptCFlags(cflags []string) []string {
	var out []string
	for _, f := range cflags {
		if f != "-mmemcpy" && f != "-mno-memcpy" {
			out = append(out, f)
		}
	}
	return out
}

// gcc 15 defaults to gnu23 for C; clang still defaults to gnu17.
func (llvmToolchain) CompileExtraFlags(ext string) []string {
	if ext == ".c" {
		return []string{"-std=gnu23"}
	}
	return nil
}

func (llvmToolchain) IETableStubFlags() []string { return nil }

func (llvmToolchain) EELinkTool(bool) string { return eeLD }
func (llvmToolchain) IOPLinkTool() string    { return iopLD }

// The EE link calls lld directly, so the default libraries gcc's specs would add are listed here in one group.
func (llvmToolchain) EELinkTrailer() string {
	return " --start-group -lc -lm -lcdvd -lpthread -lpthreadglue -lcglue -lkernel -latomic -l" + eeBuiltinsLib + " --end-group"
}

func (llvmToolchain) EELinkFlags(string) []string { return []string{"--gc-sections"} }

func (t llvmToolchain) EELinkLibDirs() []string {
	return []string{path.Join(t.eeSysroot(), "lib")}
}

// Linking bypasses the compiler driver, so the C++ runtime is added here when a target links C++ code.
func (t llvmToolchain) EELinkExtra(ldflags []string, needsCXX bool) []string {
	ldflags = linkerFlags(ldflags)
	var flags []string
	if needsCXX && !hasLDFlag(ldflags, "-lc++") {
		flags = append(flags, "-lc++", "-lc++abi")
		if utils.FileExists(path.Join(t.eeSysroot(), "lib/libunwind.a")) {
			flags = append(flags, "-lunwind")
		}
	}
	return append(flags, replaceLibgcc(ldflags, eeBuiltinsLib)...)
}

// -dc has no lld equivalent and only changes when common symbols are allocated.
func (llvmToolchain) IOPLinkFlags(string) []string { return []string{"-r"} }

func (t llvmToolchain) IOPLinkLibDirs() []string {
	return []string{path.Join(t.iopSysroot(), "lib")}
}

// libiopmini supplies the memset/memcpy/memmove/memcmp calls the compiler may still emit for a freestanding module.
func (llvmToolchain) IOPLinkExtra(ldflags []string) []string {
	return append(replaceLibgcc(linkerFlags(ldflags), iopBuiltinsLib), "-liopmini")
}

func (llvmToolchain) MapLibs(libs []string, processor string) []string {
	replacement := eeBuiltinsLib
	if processor == "iop" {
		replacement = iopBuiltinsLib
	}
	out := make([]string, len(libs))
	for i, l := range libs {
		if l == "gcc" {
			l = replacement
		}
		out[i] = l
	}
	return out
}

// --irx1 makes a module that the console cannot load fail at build time instead of at load time with error -201.
func (llvmToolchain) IOPFixupFlags(hasExportsTab bool) []string {
	flags := []string{"--rb", "--irx1"}
	if !hasExportsTab {
		flags = append(flags, "--allow-zero-text")
	}
	return flags
}

// A "<linkfile>.llvm" sibling takes precedence: lld and GNU ld accept different OUTPUT_FORMAT names for MIPS.
func (llvmToolchain) Linkfile(defaultPath string) string {
	if alt := defaultPath + ".llvm"; utils.FileExists(alt) {
		return alt
	}
	return defaultPath
}

func hasLDFlag(ldflags []string, flag string) bool {
	for _, f := range ldflags {
		if f == flag {
			return true
		}
	}
	return false
}

// replaceLibgcc swaps -lgcc for the compiler-rt builtins library: no libgcc exists in this toolchain.
func replaceLibgcc(ldflags []string, replacement string) []string {
	out := make([]string, len(ldflags))
	for i, f := range ldflags {
		if f == "-lgcc" {
			f = "-l" + replacement
		}
		out[i] = f
	}
	return out
}

// ldflags written for a compiler driver (-Wl,... and driver-only options) reach lld directly, which rejects them.
func linkerFlags(ldflags []string) []string {
	var out []string
	for _, f := range ldflags {
		switch {
		case strings.HasPrefix(f, "-Wl,"):
			out = append(out, strings.Split(strings.TrimPrefix(f, "-Wl,"), ",")...)
		case f == "-nostartfiles", f == "-nostdlib", f == "-nodefaultlibs", strings.HasPrefix(f, "-f"):
		default:
			out = append(out, f)
		}
	}
	return out
}
