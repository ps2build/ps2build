package buildgraph

import (
	"fmt"
	"os"
	"strings"

	"github.com/ps2build/ps2build/internal/config"
)

// ToolchainKind names a compiler toolchain ps2build can drive.
type ToolchainKind = config.ToolchainKind

const (
	ToolchainGCC  = config.ToolchainGCC
	ToolchainLLVM = config.ToolchainLLVM

	// ToolchainEnvVar overrides ps2.yaml's toolchain: field; --toolchain overrides both.
	ToolchainEnvVar = "PS2BUILD_TOOLCHAIN"
)

// ParseToolchainKind accepts "gcc" or "llvm" (case-insensitive).
func ParseToolchainKind(s string) (ToolchainKind, error) {
	switch ToolchainKind(strings.ToLower(strings.TrimSpace(s))) {
	case ToolchainGCC:
		return ToolchainGCC, nil
	case ToolchainLLVM:
		return ToolchainLLVM, nil
	}
	return "", fmt.Errorf("unknown toolchain %q (want %q or %q)", s, ToolchainGCC, ToolchainLLVM)
}

// NotInstalledError reports a selected toolchain whose files are not on disk.
type NotInstalledError struct {
	Kind      ToolchainKind
	Component string // the update component that provides it
	Root      string
}

func (e *NotInstalledError) Error() string {
	return fmt.Sprintf("the %s toolchain is not installed at %s (run \"ps2build update %s\")", e.Kind, e.Root, e.Component)
}

// SelectToolchain applies the precedence --toolchain flag, PS2BUILD_TOOLCHAIN, the project file's toolchain:, then gcc.
func SelectToolchain(flagValue string, projectValue ToolchainKind) (ToolchainKind, error) {
	for _, src := range []struct{ name, value string }{
		{"--toolchain", flagValue},
		{ToolchainEnvVar, os.Getenv(ToolchainEnvVar)},
		{"toolchain:", string(projectValue)},
	} {
		if strings.TrimSpace(src.value) == "" {
			continue
		}
		kind, err := ParseToolchainKind(src.value)
		if err != nil {
			return "", fmt.Errorf("%s: %w", src.name, err)
		}
		return kind, nil
	}
	return ToolchainGCC, nil
}

// NewToolchain returns the implementation for kind.
func NewToolchain(kind ToolchainKind) (Toolchain, error) {
	switch kind {
	case ToolchainGCC:
		return gccToolchain{}, nil
	case ToolchainLLVM:
		return newLLVMToolchain()
	}
	return nil, fmt.Errorf("unknown toolchain %q", kind)
}

// Toolchain is everything that differs between the compiler toolchains ps2build can drive.
// Symbolic tool names (eeGCC, eeGXX, ...) are shared by both: they name a role, not a binary.
type Toolchain interface {
	Kind() ToolchainKind

	// Paths maps each symbolic tool name to an absolute path.
	Paths() map[string]string

	// EECompileArgs and IOPCompileArgs lead a target's compile flags. compile_commands.json needs them
	// because clangd does not run the compiler wrapper that would otherwise supply them.
	EECompileArgs() []string
	IOPCompileArgs() []string

	// IncludeFlag is the flag for package and project header directories.
	IncludeFlag() string

	// SystemIncludeDirs lists the toolchain's own header directory for a processor, when the compiler is not already configured with it.
	SystemIncludeDirs(processor string) []string

	// IOPCodegenFlags are the IOP code generation flags that follow -Wall.
	IOPCodegenFlags() []string

	// AdaptCFlags drops or rewrites target cflags the compiler does not accept.
	AdaptCFlags(cflags []string) []string

	// CompileExtraFlags are appended for a source file with the given extension.
	CompileExtraFlags(ext string) []string

	// IETableStubFlags are appended when compiling a generated import/export table stub.
	IETableStubFlags() []string

	// EELinkTool and IOPLinkTool return the symbolic name of the program that links.
	EELinkTool(cxx bool) string
	IOPLinkTool() string

	// EELinkTrailer is appended to the EE link command after the target's libraries.
	EELinkTrailer() string

	EELinkFlags(opt string) []string
	EELinkLibDirs() []string
	EELinkExtra(ldflags []string, needsCXX bool) []string

	IOPLinkFlags(opt string) []string
	IOPLinkLibDirs() []string
	IOPLinkExtra(ldflags []string) []string

	// MapLibs rewrites library names the toolchain does not provide, for the given processor.
	MapLibs(libs []string, processor string) []string

	IOPFixupFlags(hasExportsTab bool) []string

	// Linkfile returns the linker script to use in place of the given default.
	Linkfile(defaultPath string) string
}
