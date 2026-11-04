package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ps2build/ps2build/internal/buildgraph"
	"github.com/ps2build/ps2build/internal/test"
)

// resetInitFlags restores the package-level initCmd flag variables between subtests.
func resetInitFlags() {
	initProjectName = ""
	initOutDir = "build"
	initForce = false
	initPackage = false
	initWorkspace = false
	initToolchain = ""
}

func runInit(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	initCmd.SetOut(&buf)
	err := initCmd.RunE(initCmd, args)
	return buf.String(), err
}

func TestInit_ScaffoldsProjectWithoutEnv(t *testing.T) {
	test.SetupCommonIncludeDir()
	resetInitFlags()

	dir := t.TempDir()
	out, err := runInit(t, dir)
	if err != nil {
		t.Fatalf("init: %v (output: %s)", err, out)
	}

	for _, rel := range []string{"ps2.yaml", "src/main.c", ".gitignore", ".clangd"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}

	gitignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if got, want := string(gitignore), "/build/\n/.cache/\n"; got != want {
		t.Errorf(".gitignore = %q, want %q", got, want)
	}

	clangd, err := os.ReadFile(filepath.Join(dir, ".clangd"))
	if err != nil {
		t.Fatalf("reading .clangd: %v", err)
	}
	if !bytes.Contains(clangd, []byte("-m*")) || !bytes.Contains(clangd, []byte("-G*")) {
		t.Errorf(".clangd = %q, want it to strip -G*/-m*", clangd)
	}
}

func TestInit_GeneratesBuildFolderWhenEnvResolves(t *testing.T) {
	resetInitFlags()

	dir := t.TempDir()

	out, err := runInit(t, dir)
	if err != nil {
		t.Fatalf("init: %v (output: %s)", err, out)
	}

	if _, err := os.Stat(filepath.Join(dir, "build", "build.ninja")); err != nil {
		t.Errorf("expected build/build.ninja to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "build", "compile_commands.json")); err != nil {
		t.Errorf("expected build/compile_commands.json to exist: %v", err)
	}
}

func TestInit_RefusesToOverwriteWithoutForce(t *testing.T) {
	resetInitFlags()

	dir := t.TempDir()
	if _, err := runInit(t, dir); err != nil {
		t.Fatalf("first init: %v", err)
	}

	resetInitFlags()
	if _, err := runInit(t, dir); err == nil {
		t.Fatalf("expected second init without --force to fail")
	}

	resetInitFlags()
	initForce = true
	if _, err := runInit(t, dir); err != nil {
		t.Fatalf("init --force: %v", err)
	}
}

func TestInit_ProjectNameDefaultsToDirName(t *testing.T) {
	resetInitFlags()

	parent := t.TempDir()
	dir := filepath.Join(parent, "my-cool-game")

	if _, err := runInit(t, dir); err != nil {
		t.Fatalf("init: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "ps2.yaml"))
	if err != nil {
		t.Fatalf("reading ps2.yaml: %v", err)
	}
	if !bytes.Contains(data, []byte("project: my-cool-game")) {
		t.Errorf("ps2.yaml = %q, want project name derived from directory", data)
	}
}

func TestInit_LLVMToolchainSetsConfigAndSkipsClangd(t *testing.T) {
	resetInitFlags()
	initToolchain = "llvm"
	t.Setenv(buildgraph.LLVMRootEnvVar, t.TempDir())

	dir := t.TempDir()
	if out, err := runInit(t, dir); err != nil && !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("init --toolchain llvm: %v (output: %s)", err, out)
	}

	data, err := os.ReadFile(filepath.Join(dir, "ps2.yaml"))
	if err != nil {
		t.Fatalf("reading ps2.yaml: %v", err)
	}
	if !bytes.Contains(data, []byte("toolchain: llvm\n")) {
		t.Errorf("ps2.yaml = %q, want it to set toolchain: llvm", data)
	}
	if _, err := os.Stat(filepath.Join(dir, ".clangd")); !os.IsNotExist(err) {
		t.Errorf("expected no .clangd for an llvm project, stat error = %v", err)
	}
}

func TestInit_RejectsUnknownToolchain(t *testing.T) {
	resetInitFlags()
	initToolchain = "msvc"
	if _, err := runInit(t, t.TempDir()); err == nil || !strings.Contains(err.Error(), "--toolchain") {
		t.Errorf("init --toolchain msvc error = %v, want one naming --toolchain", err)
	}
}
