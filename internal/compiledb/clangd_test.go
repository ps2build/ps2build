package compiledb

import (
	"strings"
	"testing"

	"github.com/ps2build/ps2build/internal/buildgraph"
)

func TestClangdConfig_AlwaysStripsPS2OnlyFlags(t *testing.T) {
	g := &buildgraph.Graph{
		Compiles: []buildgraph.CompileStep{
			{Toolchain: "gcc", FlagArgs: []string{"-D_EE", "-G0"}, Source: "/p/a.c", Object: "a.o"},
		},
	}
	got := ClangdConfig(g)
	for _, want := range []string{"-G*", "-m*", "-fno-toplevel-reorder"} {
		if !strings.Contains(got, want) {
			t.Errorf("ClangdConfig() missing %q in Remove: list:\n%s", want, got)
		}
	}
}

func TestClangdConfig_PicksUpEEDefine(t *testing.T) {
	g := &buildgraph.Graph{
		Compiles: []buildgraph.CompileStep{
			{Toolchain: "gcc", FlagArgs: []string{"-D_EE"}, Source: "/p/a.c", Object: "a.o"},
		},
	}
	got := ClangdConfig(g)
	if !strings.Contains(got, "-D_EE") {
		t.Errorf("ClangdConfig() missing -D_EE fallback:\n%s", got)
	}
}

func TestClangdConfig_PicksUpIOPDefine(t *testing.T) {
	g := &buildgraph.Graph{
		Compiles: []buildgraph.CompileStep{
			{Toolchain: "gcc", FlagArgs: []string{"-D_IOP"}, Source: "/p/a.c", Object: "a.o"},
		},
	}
	got := ClangdConfig(g)
	if !strings.Contains(got, "-D_IOP") {
		t.Errorf("ClangdConfig() missing -D_IOP fallback:\n%s", got)
	}
	if strings.Contains(got, "-D_EE") {
		t.Errorf("ClangdConfig() = %q, should not add -D_EE for an IOP-only project", got)
	}
}

func TestClangdConfig_NoCompilesStillProducesValidRemoveList(t *testing.T) {
	g := &buildgraph.Graph{}
	got := ClangdConfig(g)
	if !strings.Contains(got, "CompileFlags:") || !strings.Contains(got, "-G*") {
		t.Errorf("ClangdConfig() with no compile steps = %q, want a valid Remove: list regardless", got)
	}
	if strings.Contains(got, "Add:") {
		t.Errorf("ClangdConfig() with no compile steps = %q, want no Add: section (nothing to add)", got)
	}
}
