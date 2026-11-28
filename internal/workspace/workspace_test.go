package workspace

import (
	"github.com/ps2build/ps2build/internal/buildgraph"
	"io"
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

func TestLoadSchemaVersion(t *testing.T) {
	cases := []struct {
		name, body, wantErr string
	}{
		{"missing means 1", "workspace: w\nmembers: [a]\n", ""},
		{"1 loads", "schema_version: 1\nworkspace: w\nmembers: [a]\n", ""},
		{"2 needs a newer ps2build", "schema_version: 2\nworkspace: w\nmembers: [a]\n", "schema_version 2 needs a newer ps2build (this is dev, which reads up to 1); run \"ps2build update\""},
		{"0 is an error", "schema_version: 0\nworkspace: w\nmembers: [a]\n", "schema_version must be 1 or higher, got 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ps2workspace.yaml")
			writeFile(t, path, tc.body)
			_, err := Load(path)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Load error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadUnknownKeysWarnAndAreIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ps2workspace.yaml")
	writeFile(t, path, "workspace: w\nfoo: 1\nmembers: [a, b]\n")
	c, warnings, err := LoadWithWarnings(path)
	if err != nil {
		t.Fatalf("Load: %v - an unknown key must not be an error", err)
	}
	if want := path + ": unknown key \"foo\" ignored"; len(warnings) != 1 || warnings[0] != want {
		t.Errorf("warnings = %q, want [%q]", warnings, want)
	}
	if len(c.Members) != 2 {
		t.Errorf("members = %v, want both kept", c.Members)
	}
}

func TestLoadEmptyFileStillSaysMembersMustNotBeEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ps2workspace.yaml")
	writeFile(t, path, "")
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "members must not be empty") {
		t.Errorf("Load error = %v, want the members message", err)
	}
}

func TestBuild_MemberUnknownKeyWarnsAndBuilds(t *testing.T) {
	test.SetupCommonIncludeDir()
	root := newTwoMemberWorkspace(t)
	member := filepath.Join(root, "consumer", "ps2.yaml")
	body, err := os.ReadFile(member)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, member, string(body)+"    foo: 1\n")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	_, buildErr := Build(filepath.Join(root, "ps2workspace.yaml"), filepath.Join(t.TempDir(), "out"), gccToolchain(t))
	os.Stderr = orig
	w.Close()
	if buildErr != nil {
		t.Fatalf("Build: %v - an unknown key in a member must not fail the build", buildErr)
	}
	got, _ := io.ReadAll(r)
	if want := "warning: " + member + ": unknown key \"targets[0].foo\" ignored\n"; !strings.Contains(string(got), want) {
		t.Errorf("stderr = %q, want it to contain %q", got, want)
	}
}
