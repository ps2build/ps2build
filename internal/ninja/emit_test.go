package ninja

import (
	"strings"
	"testing"

	"github.com/ps2build/ps2build/internal/buildgraph"
)

func TestWriterEscapesSpacesInPaths(t *testing.T) {
	var buf strings.Builder
	w := NewWriter(&buf)
	w.Build(BuildEdge{
		Outputs: []string{"out/my file.o"},
		Rule:    "cc",
		Inputs:  []string{"C:/Program Files/src/main.c"},
	})
	if err := w.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `out/my$ file.o`) {
		t.Errorf("output path not escaped, got: %s", got)
	}
	if !strings.Contains(got, `C$:/Program$ Files/src/main.c`) {
		t.Errorf("input path not escaped (space and drive-letter colon), got: %s", got)
	}
}

func TestWriterEscapesColonInBuildLinePaths(t *testing.T) {
	var buf strings.Builder
	w := NewWriter(&buf)
	w.Build(BuildEdge{
		Outputs: []string{"D:/out/obj/main.o"},
		Rule:    "cc",
		Inputs:  []string{"D:/project/src/main.c"},
	})

	got := buf.String()
	if !strings.Contains(got, `build D$:/out/obj/main.o: cc D$:/project/src/main.c`) {
		t.Errorf("drive-letter colon not escaped in build line, got: %s", got)
	}
}

func TestFlagsVariableDoesNotEscapeColon(t *testing.T) {
	var buf strings.Builder
	w := NewWriter(&buf)
	w.Build(BuildEdge{
		Outputs: []string{"out.o"},
		Rule:    "cc",
		Inputs:  []string{"in.c"},
		Vars:    [][2]string{{"flags", "-IC:/ps2sdk/ee/include"}},
	})

	got := buf.String()
	if !strings.Contains(got, `flags = -IC:/ps2sdk/ee/include`) {
		t.Errorf("variable value's colon was incorrectly escaped, got: %s", got)
	}
}

func TestWriterEscapesDollarInVariable(t *testing.T) {
	var buf strings.Builder
	w := NewWriter(&buf)
	w.Build(BuildEdge{
		Outputs: []string{"out.o"},
		Rule:    "cc",
		Inputs:  []string{"in.c"},
		Vars:    [][2]string{{"flags", "-DPRICE=$5"}},
	})

	got := buf.String()
	if !strings.Contains(got, `-DPRICE=$$5`) {
		t.Errorf("$ in variable value not escaped, got: %s", got)
	}
}

func TestRuleCommandEscapesDollarButNotSpaces(t *testing.T) {
	var buf strings.Builder
	w := NewWriter(&buf)
	w.Rule("cc", "gcc $flags -c $in -o $out", "")

	got := buf.String()
	// $flags/$in/$out must survive as ninja variable references (a single
	// '$'), not become spaces-in-a-single-token or double-escaped $$.
	if !strings.Contains(got, "gcc $flags -c $in -o $out") {
		t.Errorf("rule command was mangled, got: %s", got)
	}
}

func TestLinkRulesPutObjectsBeforeFlags(t *testing.T) {
	var buf strings.Builder
	nw := NewWriter(&buf)
	writeRules(nw, &buildgraph.Graph{Toolchain: gccToolchain(t)}, rspUsage{})
	got := buf.String()

	for _, rule := range []string{"ee_link", "ee_link_cxx", "iop_link"} {
		start := strings.Index(got, "rule "+rule+"\n")
		if start == -1 {
			t.Fatalf("rule %q not found in emitted rules:\n%s", rule, got)
		}
		line := got[start:]
		inIdx := strings.Index(line, "$in")
		flagsIdx := strings.Index(line, "$flags")
		if inIdx == -1 || flagsIdx == -1 {
			t.Fatalf("rule %q missing $in or $flags: %s", rule, line)
		}
		if flagsIdx < inIdx {
			t.Errorf("rule %q: $flags appears before $in in %q - a real -l<name>/archive-group flag in ldflags would then link before the objects that need it", rule, line[:strings.Index(line, "\n")])
		}
	}
}

func TestCompileEdgeFallsBackToRspOnlyWhenNeeded(t *testing.T) {
	bigFlags := make([]string, 400)
	for i := range bigFlags {
		bigFlags[i] = "-Ivery/long/package/include/path/number/" + strings.Repeat("x", 20)
	}

	small := &buildgraph.Graph{Toolchain: gccToolchain(t), Compiles: []buildgraph.CompileStep{
		{Toolchain: "mips64r5900el-ps2-elf-gcc", FlagArgs: []string{"-D_EE", "-G0"}, Source: "src/main.c", Object: "obj/main.o"},
	}}
	var smallBuf strings.Builder
	if err := Emit(small, &smallBuf); err != nil {
		t.Fatalf("Emit (small): %v", err)
	}
	smallOut := smallBuf.String()
	if strings.Contains(smallOut, "ee_cc_rsp") {
		t.Errorf("small project should never emit or use ee_cc_rsp, got:\n%s", smallOut)
	}
	if !strings.Contains(smallOut, "build obj/main.o: ee_cc src/main.c") {
		t.Errorf("small project's compile edge should use the plain ee_cc rule, got:\n%s", smallOut)
	}

	big := &buildgraph.Graph{Toolchain: gccToolchain(t), Compiles: []buildgraph.CompileStep{
		{Toolchain: "mips64r5900el-ps2-elf-gcc", FlagArgs: bigFlags, Source: "src/main.c", Object: "obj/main.o"},
	}}
	var bigBuf strings.Builder
	if err := Emit(big, &bigBuf); err != nil {
		t.Fatalf("Emit (big): %v", err)
	}
	bigOut := bigBuf.String()
	if !strings.Contains(bigOut, "rule ee_cc_rsp") {
		t.Errorf("large flag list should emit the ee_cc_rsp rule, got:\n%s", bigOut)
	}
	if !strings.Contains(bigOut, "build obj/main.o: ee_cc_rsp src/main.c") {
		t.Errorf("large flag list's compile edge should use ee_cc_rsp, got:\n%s", bigOut)
	}
	if !strings.Contains(bigOut, "@$out.rsp") || !strings.Contains(bigOut, "rspfile_content = $flags") {
		t.Errorf("ee_cc_rsp rule missing rspfile plumbing, got:\n%s", bigOut)
	}
}

func TestCompileRulesTrackHeaderDependencies(t *testing.T) {
	bigFlags := make([]string, 400)
	for i := range bigFlags {
		bigFlags[i] = "-Ivery/long/package/include/path/number/" + strings.Repeat("x", 20)
	}

	g := &buildgraph.Graph{
		Toolchain: gccToolchain(t),
		Compiles: []buildgraph.CompileStep{
			{Toolchain: "mips64r5900el-ps2-elf-gcc", FlagArgs: []string{"-D_EE"}, Source: "src/main.c", Object: "obj/main.o"},
			{Toolchain: "mips64r5900el-ps2-elf-gcc", FlagArgs: bigFlags, Source: "src/big.c", Object: "obj/big.o"},
		},
	}
	var buf strings.Builder
	if err := Emit(g, &buf); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	got := buf.String()

	for _, rule := range []string{"ee_cc", "ee_cc_rsp"} {
		start := strings.Index(got, "rule "+rule+"\n")
		if start == -1 {
			t.Fatalf("rule %q not found in emitted rules:\n%s", rule, got)
		}
		block := got[start:]
		if end := strings.Index(block, "\nrule "); end != -1 {
			block = block[:end]
		}
		if !strings.Contains(block, "-MMD -MF $out.d") {
			t.Errorf("rule %q missing -MMD -MF $out.d in its command:\n%s", rule, block)
		}
		if !strings.Contains(block, "depfile = $out.d") {
			t.Errorf("rule %q missing depfile binding:\n%s", rule, block)
		}
		if !strings.Contains(block, "deps = gcc") {
			t.Errorf("rule %q missing deps binding:\n%s", rule, block)
		}
	}

	for _, rule := range []string{"ee_link"} {
		start := strings.Index(got, "rule "+rule+"\n")
		if start == -1 {
			t.Fatalf("rule %q not found in emitted rules:\n%s", rule, got)
		}
		block := got[start:]
		if end := strings.Index(block, "\nrule "); end != -1 {
			block = block[:end]
		}
		if strings.Contains(block, "depfile") || strings.Contains(block, "deps = gcc") {
			t.Errorf("link rule %q must never get depfile/deps - its command produces no depfile:\n%s", rule, block)
		}
	}
}

func TestDVPAssembleRule(t *testing.T) {
	g := &buildgraph.Graph{
		Toolchain: gccToolchain(t),
		Compiles: []buildgraph.CompileStep{
			{Toolchain: "dvp-as", Source: "src/vu1_probe.vsm", Object: "obj/main/vu1_probe.o", Assembler: true},
		},
	}
	var buf strings.Builder
	if err := Emit(g, &buf); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	got := buf.String()

	start := strings.Index(got, "rule dvp_as\n")
	if start == -1 {
		t.Fatalf("rule %q not found in emitted rules:\n%s", "dvp_as", got)
	}
	block := got[start:]
	if end := strings.Index(block, "\nrule "); end != -1 {
		block = block[:end]
	}
	if !strings.Contains(block, "dvp-as") {
		t.Errorf("dvp_as rule command doesn't mention dvp-as:\n%s", block)
	}
	if strings.Contains(block, "depfile") || strings.Contains(block, "deps = gcc") {
		t.Errorf("dvp_as rule must not get a depfile/deps binding, dvp-as has no such flag:\n%s", block)
	}

	if !strings.Contains(got, "build obj/main/vu1_probe.o: dvp_as src/vu1_probe.vsm") {
		t.Errorf("no dvp_as build edge for the .vsm source found in:\n%s", got)
	}
}

func TestPhonyAndDefault(t *testing.T) {
	var buf strings.Builder
	w := NewWriter(&buf)
	w.Phony("mytarget", []string{"bin/mytarget.elf"})
	w.Default([]string{"all"})

	got := buf.String()
	if !strings.Contains(got, "build mytarget: phony bin/mytarget.elf") {
		t.Errorf("phony edge missing/wrong, got: %s", got)
	}
	if !strings.Contains(got, "default all") {
		t.Errorf("default statement missing/wrong, got: %s", got)
	}
}

func gccToolchain(t *testing.T) buildgraph.Toolchain {
	t.Helper()
	tc, err := buildgraph.NewToolchain(buildgraph.ToolchainGCC)
	if err != nil {
		t.Fatal(err)
	}
	return tc
}

func TestEmitLLVMNoDefaultLibsClearsTheTrailerForThatLink(t *testing.T) {
	t.Setenv("PS2BUILD_TOOLCHAIN_ROOT", t.TempDir())
	tc, err := buildgraph.NewToolchain(buildgraph.ToolchainLLVM)
	if err != nil {
		t.Fatal(err)
	}
	g := &buildgraph.Graph{Toolchain: tc, Links: []buildgraph.LinkStep{
		{TargetName: "plain", Toolchain: "mips64r5900el-ps2-elf-gcc", Linkfile: "l.ld", Output: "plain.elf"},
		{TargetName: "bare", Toolchain: "mips64r5900el-ps2-elf-gcc", Linkfile: "l.ld", Output: "bare.elf", NoDefaultLibs: true},
	}}
	var out strings.Builder
	if err := Emit(g, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "deflibs = --start-group") {
		t.Errorf("default libraries missing from the build file:\n%s", text)
	}
	if got := strings.Count(text, "\n  deflibs = \n") + strings.Count(text, "\n  deflibs =\n"); got != 1 {
		t.Errorf("%d links clear deflibs, want only the NoDefaultLibs one:\n%s", got, text)
	}
}
