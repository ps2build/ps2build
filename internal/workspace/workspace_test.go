package workspace

import (
	"github.com/ps2build/ps2build/internal/buildgraph"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ps2build/ps2build/internal/test"
)

// writeFile creates path (and its parent directories) with content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTwoMemberWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "ps2workspace.yaml"), "workspace: test\nmembers: [mylib, consumer]\n")

	writeFile(t, filepath.Join(root, "mylib", "ps2.yaml"), `project: mylib
targets:
  - name: mylib
    type: ee_lib
    sources: [src/mylib.c]
    include_dirs: [include]
`)
	writeFile(t, filepath.Join(root, "mylib", "src", "mylib.c"), "int mylib_fn(void) { return 1; }\n")
	writeFile(t, filepath.Join(root, "mylib", "include", "mylib.h"), "int mylib_fn(void);\n")

	writeFile(t, filepath.Join(root, "consumer", "ps2.yaml"), `project: consumer
targets:
  - name: consumer
    type: ee
    sources: [src/main.c]
    libs: [mylib]
`)
	writeFile(t, filepath.Join(root, "consumer", "src", "main.c"), `#include "mylib.h"
int main(void) { return mylib_fn(); }
`)

	return root
}

func TestBuild_CrossMemberLibResolution(t *testing.T) {
	test.SetupCommonIncludeDir()
	root := newTwoMemberWorkspace(t)
	g, err := Build(filepath.Join(root, "ps2workspace.yaml"), filepath.Join(t.TempDir(), "out"), gccToolchain(t))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Both members' targets must be present in one merged Graph - this is
	// the entire point: one build.ninja/compile_commands.json, not two.
	if _, ok := g.TargetOutputs["mylib"]; !ok {
		t.Error(`TargetOutputs["mylib"] missing`)
	}
	if _, ok := g.TargetOutputs["consumer"]; !ok {
		t.Error(`TargetOutputs["consumer"] missing`)
	}

	foundInclude := false
	for _, c := range g.Compiles {
		if c.TargetName != "consumer" {
			continue
		}
		for _, f := range c.FlagArgs {
			if strings.Contains(f, "mylib/include") {
				foundInclude = true
			}
		}
	}
	if !foundInclude {
		t.Error("consumer's compile flags don't reference mylib/include - cross-member include resolution didn't happen")
	}

	foundArchive := false
	for _, l := range g.Links {
		if l.TargetName != "consumer" {
			continue
		}
		for _, dep := range l.LibDeps {
			if strings.Contains(dep, "mylib") {
				t.Errorf("consumer's LinkStep.LibDeps unexpectedly contains a mylib reference (%q) - a cross-member archive belongs in ExternalLibDeps, never LibDeps", dep)
			}
		}
		for _, dep := range l.ExternalLibDeps {
			if dep == "mylib/lib/libmylib.a" {
				foundArchive = true
			}
		}
	}
	if !foundArchive {
		t.Error(`consumer's LinkStep.ExternalLibDeps doesn't contain "mylib/lib/libmylib.a" - cross-member link resolution didn't happen`)
	}
}

func TestBuild_CrossMemberHeaderOnlyResolution(t *testing.T) {
	test.SetupCommonIncludeDir()
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "ps2workspace.yaml"), "workspace: test\nmembers: [mylib, consumer]\n")

	writeFile(t, filepath.Join(root, "mylib", "ps2.yaml"), `project: mylib
targets:
  - name: mylib
    type: ee_lib
    sources: [src/mylib.c]
    include_dirs: [include]
`)
	writeFile(t, filepath.Join(root, "mylib", "src", "mylib.c"), "int mylib_fn(void) { return 1; }\n")
	writeFile(t, filepath.Join(root, "mylib", "include", "mylib.h"), "int mylib_fn(void);\n")

	writeFile(t, filepath.Join(root, "consumer", "ps2.yaml"), `project: consumer
targets:
  - name: consumer
    type: ee
    sources: [src/main.c]
    headers: [mylib]
`)
	writeFile(t, filepath.Join(root, "consumer", "src", "main.c"), `#include "mylib.h"
int main(void) { return 0; }
`)

	g, err := Build(filepath.Join(root, "ps2workspace.yaml"), filepath.Join(t.TempDir(), "out"), gccToolchain(t))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	foundInclude := false
	for _, c := range g.Compiles {
		if c.TargetName != "consumer" {
			continue
		}
		for _, f := range c.FlagArgs {
			if strings.Contains(f, "mylib/include") {
				foundInclude = true
			}
		}
	}
	if !foundInclude {
		t.Error("consumer's compile flags don't reference mylib/include - headers: didn't resolve the cross-member sibling")
	}

	for _, l := range g.Links {
		if l.TargetName != "consumer" {
			continue
		}
		if len(l.ExternalLibDeps) != 0 {
			t.Errorf("consumer's LinkStep.ExternalLibDeps = %v, want empty - headers: must not link, only libs: does", l.ExternalLibDeps)
		}
	}
}

func TestBuild_DuplicateTargetNameAcrossMembers(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ps2workspace.yaml"), "workspace: test\nmembers: [a, b]\n")
	writeFile(t, filepath.Join(root, "a", "ps2.yaml"), "project: a\ntargets:\n  - name: same\n    type: ee_lib\n    sources: [x.c]\n")
	writeFile(t, filepath.Join(root, "b", "ps2.yaml"), "project: b\ntargets:\n  - name: same\n    type: ee_lib\n    sources: [x.c]\n")

	_, err := Build(filepath.Join(root, "ps2workspace.yaml"), filepath.Join(t.TempDir(), "out"), gccToolchain(t))
	if err == nil || !strings.Contains(err.Error(), "declared by both") {
		t.Errorf("Build() error = %v, want a duplicate-target-name error naming both members", err)
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

func TestBuild_MemberToolchainMustMatchWorkspace(t *testing.T) {
	test.SetupCommonIncludeDir()
	t.Setenv(buildgraph.LLVMRootEnvVar, t.TempDir())
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ps2workspace.yaml"), "workspace: test\ntoolchain: llvm\nmembers: [a]\n")
	writeFile(t, filepath.Join(root, "a", "ps2.yaml"), "project: a\ntoolchain: gcc\ntargets:\n  - name: a\n    type: ee_lib\n    sources: [x.c]\n")

	tc, err := buildgraph.NewToolchain(buildgraph.ToolchainLLVM)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Build(filepath.Join(root, "ps2workspace.yaml"), filepath.Join(t.TempDir(), "out"), tc)
	if err == nil || !strings.Contains(err.Error(), "conflicts with the workspace") {
		t.Errorf("Build() error = %v, want a toolchain conflict naming the member", err)
	}
}
