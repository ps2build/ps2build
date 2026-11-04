package compiledb

import (
	"path/filepath"
	"reflect"
	"testing"
)

const realEEVerboseOutput = `#include "..." search starts here:
#include <...> search starts here:
 D:/_PLATFORMS/PLAYSTATION/PS2DEV/toolchain/ee/bin/../lib/gcc/mips64r5900el-ps2-elf/15.3.1/../../../../mips64r5900el-ps2-elf/include/c++/15.3.1
 D:/_PLATFORMS/PLAYSTATION/PS2DEV/toolchain/ee/bin/../lib/gcc/mips64r5900el-ps2-elf/15.3.1/../../../../mips64r5900el-ps2-elf/include/c++/15.3.1/mips64r5900el-ps2-elf
 D:/_PLATFORMS/PLAYSTATION/PS2DEV/toolchain/ee/bin/../lib/gcc/mips64r5900el-ps2-elf/15.3.1/../../../../mips64r5900el-ps2-elf/include/c++/15.3.1/backward
 D:/_PLATFORMS/PLAYSTATION/PS2DEV/toolchain/ee/bin/../lib/gcc/mips64r5900el-ps2-elf/15.3.1/include
 D:/_PLATFORMS/PLAYSTATION/PS2DEV/toolchain/ee/bin/../lib/gcc/mips64r5900el-ps2-elf/15.3.1/include-fixed
 D:/_PLATFORMS/PLAYSTATION/PS2DEV/toolchain/ee/bin/../lib/gcc/mips64r5900el-ps2-elf/15.3.1/../../../../mips64r5900el-ps2-elf/include
End of search list.
# 0 "<stdin>"
# 0 "<built-in>"
# 0 "<command-line>"
# 1 "<stdin>"
COMPILER_PATH=D:/_PLATFORMS/PLAYSTATION/PS2DEV/toolchain/ee/bin/../libexec/gcc/mips64r5900el-ps2-elf/15.3.1/
`

func TestParseImplicitIncludeDirs_RealEEOutput(t *testing.T) {
	got := parseImplicitIncludeDirs(realEEVerboseOutput)
	want := []string{
		"D:/_PLATFORMS/PLAYSTATION/PS2DEV/toolchain/ee/bin/../lib/gcc/mips64r5900el-ps2-elf/15.3.1/../../../../mips64r5900el-ps2-elf/include/c++/15.3.1",
		"D:/_PLATFORMS/PLAYSTATION/PS2DEV/toolchain/ee/bin/../lib/gcc/mips64r5900el-ps2-elf/15.3.1/../../../../mips64r5900el-ps2-elf/include/c++/15.3.1/mips64r5900el-ps2-elf",
		"D:/_PLATFORMS/PLAYSTATION/PS2DEV/toolchain/ee/bin/../lib/gcc/mips64r5900el-ps2-elf/15.3.1/../../../../mips64r5900el-ps2-elf/include/c++/15.3.1/backward",
		"D:/_PLATFORMS/PLAYSTATION/PS2DEV/toolchain/ee/bin/../lib/gcc/mips64r5900el-ps2-elf/15.3.1/include",
		"D:/_PLATFORMS/PLAYSTATION/PS2DEV/toolchain/ee/bin/../lib/gcc/mips64r5900el-ps2-elf/15.3.1/include-fixed",
		"D:/_PLATFORMS/PLAYSTATION/PS2DEV/toolchain/ee/bin/../lib/gcc/mips64r5900el-ps2-elf/15.3.1/../../../../mips64r5900el-ps2-elf/include",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseImplicitIncludeDirs() =\n%v\nwant\n%v", got, want)
	}
}

func TestParseImplicitIncludeDirs_NoSearchList(t *testing.T) {
	// e.g. a compiler invocation that failed outright, or -v output from
	// something that isn't GCC at all - must not panic or return garbage.
	got := parseImplicitIncludeDirs("gcc: error: unrecognized command-line option '-x'\n")
	if got != nil {
		t.Errorf("parseImplicitIncludeDirs() = %v, want nil when there's no search list", got)
	}
}

func TestDiscoverImplicitIncludeDirs_NonexistentCompilerReturnsNil(t *testing.T) {
	// Best-effort: a missing/unbuilt toolchain must not make this function
	// (or, transitively, Entries) error out - see isystem.go's doc comment.
	got := discoverImplicitIncludeDirs(filepath.Join(t.TempDir(), "no-such-compiler"))
	if got != nil {
		t.Errorf("discoverImplicitIncludeDirs() = %v, want nil for a nonexistent compiler path", got)
	}
}
