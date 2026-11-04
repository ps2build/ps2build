package buildgraph

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ps2build/ps2build/internal/config"
)

func TestSelectToolchain(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		env     string
		cfg     config.ToolchainKind
		want    ToolchainKind
		wantErr string
	}{
		{name: "defaults to gcc", want: ToolchainGCC},
		{name: "project file", cfg: ToolchainLLVM, want: ToolchainLLVM},
		{name: "env beats project file", env: "gcc", cfg: ToolchainLLVM, want: ToolchainGCC},
		{name: "flag beats env", flag: "llvm", env: "gcc", want: ToolchainLLVM},
		{name: "flag beats everything", flag: "gcc", env: "llvm", cfg: ToolchainLLVM, want: ToolchainGCC},
		{name: "case and spacing are ignored", flag: " LLVM ", want: ToolchainLLVM},
		{name: "bad flag names the flag", flag: "msvc", wantErr: "--toolchain"},
		{name: "bad env names the variable", env: "msvc", wantErr: ToolchainEnvVar},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(ToolchainEnvVar, tc.env)
			got, err := SelectToolchain(tc.flag, tc.cfg)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("SelectToolchain() error = %v, want it to mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("SelectToolchain(): %v", err)
			}
			if got != tc.want {
				t.Errorf("SelectToolchain() = %q, want %q", got, tc.want)
			}
		})
	}
}

func newTestLLVM(t *testing.T) llvmToolchain {
	t.Helper()
	root := filepath.ToSlash(t.TempDir())
	t.Setenv(LLVMRootEnvVar, root)
	tc, err := NewToolchain(ToolchainLLVM)
	if err != nil {
		t.Fatal(err)
	}
	return tc.(llvmToolchain)
}

func TestLLVMToolchainVerifyReportsMissingInstall(t *testing.T) {
	tc := newTestLLVM(t)
	err := tc.Verify()
	var notInstalled *NotInstalledError
	if !errors.As(err, &notInstalled) || notInstalled.Kind != ToolchainLLVM || !strings.Contains(err.Error(), "ps2build update llvm-toolchain") {
		t.Fatalf("Verify() = %v, want a NotInstalledError naming the install command", err)
	}

	write(t, filepath.Join(tc.eeBin(), "placeholder"))
	if err := tc.Verify(); err != nil {
		t.Errorf("Verify() on an installed tree = %v, want nil", err)
	}
}

func TestLLVMToolchainLinksThroughLLD(t *testing.T) {
	tc := newTestLLVM(t)
	if tc.EELinkTool(false) != eeLD || tc.EELinkTool(true) != eeLD || tc.IOPLinkTool() != iopLD {
		t.Errorf("link tools = %q/%q/%q, want lld for every link", tc.EELinkTool(false), tc.EELinkTool(true), tc.IOPLinkTool())
	}
	paths := tc.Paths()
	if !strings.Contains(paths[eeLD], "ld.lld") || paths[eeLD] != paths[iopLD] {
		t.Errorf("Paths()[eeLD] = %q, Paths()[iopLD] = %q, want the one shared ld.lld", paths[eeLD], paths[iopLD])
	}
	if !strings.HasSuffix(paths[eeGXX], " --driver-mode=g++") {
		t.Errorf("Paths()[eeGXX] = %q, want the clang wrapper in C++ driver mode", paths[eeGXX])
	}
	if !strings.Contains(tc.EELinkTrailer(), "-latomic") || !strings.Contains(tc.EELinkTrailer(), "-lclang_rt.builtins-mips64el") {
		t.Errorf("EELinkTrailer() = %q, want the default libraries", tc.EELinkTrailer())
	}
}

func TestLLVMToolchainFindsVersionedLLVMDir(t *testing.T) {
	tc := newTestLLVM(t)
	write(t, filepath.Join(tc.root, "llvm-abc123", "bin", "ld.lld"))
	if got := tc.llvmBin(); !strings.HasSuffix(got, "llvm-abc123/bin") {
		t.Errorf("llvmBin() = %q, want the developer-build llvm-<revision> directory", got)
	}

	write(t, filepath.Join(tc.root, "llvm", "bin", "ld.lld"))
	if got := tc.llvmBin(); !strings.HasSuffix(got, "llvm/bin") || strings.Contains(got, "abc123") {
		t.Errorf("llvmBin() = %q, want the release llvm/bin directory to win", got)
	}
}

func TestLLVMToolchainEELinkExtra(t *testing.T) {
	tc := newTestLLVM(t)

	if got := tc.EELinkExtra([]string{"-lgcc"}, false); len(got) != 1 || got[0] != "-lclang_rt.builtins-mips64el" {
		t.Errorf("EELinkExtra(-lgcc) = %v, want -lgcc replaced by compiler-rt", got)
	}
	got := tc.EELinkExtra(nil, true)
	if strings.Join(got, " ") != "-lc++ -lc++abi" {
		t.Errorf("EELinkExtra(c++) = %v, want the C++ runtime", got)
	}
	if got := tc.EELinkExtra([]string{"-lc++"}, true); len(got) != 1 {
		t.Errorf("EELinkExtra with -lc++ already listed = %v, want it not repeated", got)
	}

	write(t, filepath.Join(tc.eeSysroot(), "lib", "libunwind.a"))
	if got := tc.EELinkExtra(nil, true); strings.Join(got, " ") != "-lc++ -lc++abi -lunwind" {
		t.Errorf("EELinkExtra(c++) with libunwind installed = %v, want -lunwind added", got)
	}
}

func TestLLVMToolchainLinkfilePrefersLLVMSibling(t *testing.T) {
	tc := newTestLLVM(t)
	base := filepath.ToSlash(filepath.Join(t.TempDir(), "linkfile"))
	write(t, base)
	if got := tc.Linkfile(base); got != base {
		t.Errorf("Linkfile() = %q, want the shared linkfile when no .llvm sibling exists", got)
	}
	write(t, base+".llvm")
	if got := tc.Linkfile(base); got != base+".llvm" {
		t.Errorf("Linkfile() = %q, want the .llvm sibling", got)
	}
}

func TestToolchainMapLibsReplacesLibgcc(t *testing.T) {
	llvm := newTestLLVM(t)
	if got := llvm.MapLibs([]string{"gcc", "kernel"}, "ee"); got[0] != "clang_rt.builtins-mips64el" || got[1] != "kernel" {
		t.Errorf("llvm MapLibs(ee) = %v", got)
	}
	if got := llvm.MapLibs([]string{"gcc"}, "iop"); got[0] != "clang_rt.builtins-mipsel" {
		t.Errorf("llvm MapLibs(iop) = %v", got)
	}
	if got := (gccToolchain{}).MapLibs([]string{"gcc"}, "ee"); got[0] != "gcc" {
		t.Errorf("gcc MapLibs() = %v, want libs untouched", got)
	}
}

func TestToolchainIOPFixupFormat(t *testing.T) {
	llvm := newTestLLVM(t)
	if got := strings.Join(llvm.IOPFixupFlags(true), " "); got != "--rb --irx1" {
		t.Errorf("llvm IOPFixupFlags(true) = %q, want IRX1 only", got)
	}
	if got := strings.Join((gccToolchain{}).IOPFixupFlags(true), " "); got != "--rb --irx1" {
		t.Errorf("gcc IOPFixupFlags(true) = %q", got)
	}
}

func TestParseToolchainKindRejectsUnknown(t *testing.T) {
	if _, err := ParseToolchainKind("clang"); err == nil {
		t.Error("ParseToolchainKind(clang) = nil error, want one")
	}
	if os.Getenv(LLVMRootEnvVar) != "" {
		t.Skip("environment overrides the toolchain root")
	}
}

func TestLLVMLinkerFlagsTranslateDriverFlags(t *testing.T) {
	got := linkerFlags([]string{"-nostartfiles", "-nostdlib", "-Wl,-Map,eecore.map", "-Wl,--gc-sections", "-fdata-sections", "-ffunction-sections", "-lpadx", "-Wl,-zmax-page-size=128"})
	want := []string{"-Map", "eecore.map", "--gc-sections", "-lpadx", "-zmax-page-size=128"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("linkerFlags() = %v, want %v", got, want)
	}
}

func TestLLVMAdaptCFlagsDropsGCCMemcpyOptions(t *testing.T) {
	llvm := newTestLLVM(t)
	got := llvm.AdaptCFlags([]string{"-O2", "-mno-memcpy", "-Wall", "-mmemcpy"})
	if strings.Join(got, " ") != "-O2 -Wall" {
		t.Errorf("AdaptCFlags() = %v, want -O2 -Wall", got)
	}
	in := []string{"-mno-memcpy"}
	if got := (gccToolchain{}).AdaptCFlags(in); strings.Join(got, " ") != "-mno-memcpy" {
		t.Errorf("gcc AdaptCFlags() = %v, want the flags unchanged", got)
	}
}

func TestProjectIncludeDirsUseDashIOnEveryToolchain(t *testing.T) {
	for _, tc := range []Toolchain{gccToolchain{}, newTestLLVM(t)} {
		g := &Graph{Toolchain: tc}
		target := config.Target{IncludeDirs: []string{"include"}}
		for name, flags := range map[string][]string{
			"ee":  eeCompileFlags(g, target, "/proj", nil),
			"iop": iopCompileFlags(g, target, "/proj", nil),
		} {
			found := false
			for _, f := range flags {
				if f == "-I/proj/include" {
					found = true
				}
				if f == "-isystem/proj/include" {
					t.Errorf("%s %s: project include dir passed as -isystem, which -MMD leaves out of dependency tracking", tc.Kind(), name)
				}
			}
			if !found {
				t.Errorf("%s %s: -I/proj/include missing from %v", tc.Kind(), name, flags)
			}
		}
	}
}
