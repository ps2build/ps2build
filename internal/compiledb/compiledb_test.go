package compiledb

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ps2build/ps2build/internal/buildgraph"
)

func TestEntries(t *testing.T) {
	g := &buildgraph.Graph{
		OutDir: "/out",
		Compiles: []buildgraph.CompileStep{
			{
				Toolchain: "mips64r5900el-ps2-elf-gcc",
				FlagArgs:  []string{"-D_EE", "-G0"},
				Source:    "/project/src/main.c",
				Object:    "obj/main.o",
			},
		},
		// Link/strip/fixup/bin2c/objcopy steps must never produce compiledb
		// entries - they aren't translation units.
		Links: []buildgraph.LinkStep{
			{TargetName: "main", Output: "bin/main.elf"},
		},
	}

	entries := Entries(g)
	if len(entries) != 1 {
		t.Fatalf("len(Entries) = %d, want 1 (link steps must not produce entries)", len(entries))
	}

	e := entries[0]
	if e.Directory != "/out" {
		t.Errorf("Directory = %q, want /out", e.Directory)
	}
	if e.File != "/project/src/main.c" {
		t.Errorf("File = %q, want /project/src/main.c", e.File)
	}
	if e.Output != "obj/main.o" {
		t.Errorf("Output = %q, want obj/main.o", e.Output)
	}

	want := []string{"mips64r5900el-ps2-elf-gcc", "-D_EE", "-G0", "-c", "/project/src/main.c", "-o", "/out/obj/main.o"}
	if len(e.Arguments) != len(want) {
		t.Fatalf("Arguments = %v, want %v", e.Arguments, want)
	}
	for i, w := range want {
		if e.Arguments[i] != w {
			t.Errorf("Arguments[%d] = %q, want %q", i, e.Arguments[i], w)
		}
	}
}

func TestEntriesReusesCompileStepFlagArgsVerbatim(t *testing.T) {
	flags := []string{"-D_EE", "-G0", "-O2", "-Wall", "-I/ps2sdk/ee/include"}
	g := &buildgraph.Graph{
		OutDir: "/out",
		Compiles: []buildgraph.CompileStep{
			{Toolchain: "gcc", FlagArgs: flags, Source: "/p/a.c", Object: "a.o"},
		},
	}

	entries := Entries(g)
	args := entries[0].Arguments[1 : 1+len(flags)] // skip the toolchain name
	for i, f := range flags {
		if args[i] != f {
			t.Errorf("Arguments flag[%d] = %q, want %q (from CompileStep.FlagArgs)", i, args[i], f)
		}
	}
}

func TestEntriesOmitsDashCForAssemblerSteps(t *testing.T) {
	g := &buildgraph.Graph{
		OutDir: "/out",
		Compiles: []buildgraph.CompileStep{
			{Toolchain: "dvp-as", Source: "/project/src/vu1.vsm", Object: "obj/vu1.vsm.o", Assembler: true},
		},
	}

	entries := Entries(g)
	if len(entries) != 1 {
		t.Fatalf("len(Entries) = %d, want 1", len(entries))
	}

	want := []string{"dvp-as", "/project/src/vu1.vsm", "-o", "/out/obj/vu1.vsm.o"}
	if len(entries[0].Arguments) != len(want) {
		t.Fatalf("Arguments = %v, want %v", entries[0].Arguments, want)
	}
	for i, w := range want {
		if entries[0].Arguments[i] != w {
			t.Errorf("Arguments[%d] = %q, want %q", i, entries[0].Arguments[i], w)
		}
	}
}

func TestWrite(t *testing.T) {
	entries := []Entry{
		{Directory: "/out", File: "/p/a.c", Output: "a.o", Arguments: []string{"gcc", "-c", "/p/a.c", "-o", "a.o"}},
	}

	var buf bytes.Buffer
	if err := Write(entries, &buf); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var got []Entry
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("Write produced invalid JSON: %v\n%s", err, buf.String())
	}
	if len(got) != 1 || got[0].File != "/p/a.c" {
		t.Errorf("round-tripped entries = %+v, want the original entry back", got)
	}
}

func TestDedupeIncludeArgsCollapsesRepeatsAndBackslashes(t *testing.T) {
	bs := string(rune(92))
	in := []string{"cc", "-isystemC:/a/inc", "-IC:/p", "-isystemC:/a/inc", "-isystem", "C:" + bs + "a" + bs + "bin/../inc", "-isystem", "C:/b", "-c"}
	want := []string{"cc", "-isystemC:/a/inc", "-IC:/p", "-isystem", "C:/b", "-c"}
	got := dedupeIncludeArgs(in)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
